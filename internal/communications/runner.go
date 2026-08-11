// Package communications owns Rostrum's durable outbound-email workflow.
//
// A Communication row is persisted before it is sent, claimed behind a short
// lease, and completed in a second transaction. That shape deliberately makes
// delivery at-least-once rather than pretending SMTP/HTTP mail and the
// canonical store can share one distributed transaction: a worker crash after
// delivery can retry, while the provider receives the same idempotency key.
package communications

import (
	"fmt"
	"strings"
	"time"

	programcalendar "github.com/m31-labs/rostrum/internal/calendar"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/store"
)

const (
	defaultMaxAttempts = 5
	leaseDuration      = 5 * time.Minute
	fiveDays           = 5 * 24 * time.Hour
	oneDay             = 24 * time.Hour
)

// Trigger identifies a domain event that can notify an operational recipient.
// It stores only stable canonical IDs; delivery renders merge data from the
// live state at claim time rather than duplicating arbitrary request bodies in
// an outbox row.
type Trigger struct {
	Name         string
	SubmissionID string
	TaskID       string
	SpeakerID    string
}

func (trigger Trigger) entityID() string {
	if strings.TrimSpace(trigger.SubmissionID) != "" {
		return strings.TrimSpace(trigger.SubmissionID)
	}
	if strings.TrimSpace(trigger.TaskID) != "" {
		return strings.TrimSpace(trigger.TaskID)
	}
	return strings.TrimSpace(trigger.SpeakerID)
}

// EnqueueNotificationRules persists one delivery decision per configured
// administrative recipient. Calling it twice for the same event is safe: the
// stable rule/entity/recipient key prevents duplicate work. Suppressed events
// are still retained as Communication rows so operators can explain why no
// mail went out.
func EnqueueNotificationRules(state *domain.State, trigger Trigger, now time.Time) int {
	if state == nil || strings.TrimSpace(trigger.Name) == "" || trigger.entityID() == "" {
		return 0
	}
	created := 0
	for _, rule := range state.NotificationRules {
		if !rule.Enabled || rule.Trigger != trigger.Name {
			continue
		}
		template, found := findTemplate(*state, rule.TemplateID)
		if !found {
			continue
		}
		seen := map[string]bool{}
		for _, rawRecipient := range rule.RecipientEmails {
			recipient := strings.ToLower(strings.TrimSpace(rawRecipient))
			if recipient == "" || seen[recipient] {
				continue
			}
			seen[recipient] = true
			key := notificationKey(rule.ID, trigger.entityID(), recipient)
			if communicationWithKey(*state, key) {
				continue
			}
			item := domain.Communication{
				ID:                 domain.NewID("comm"),
				TemplateID:         rule.TemplateID,
				SubmissionID:       trigger.SubmissionID,
				TaskID:             trigger.TaskID,
				SpeakerID:          trigger.SpeakerID,
				NotificationRuleID: rule.ID,
				RecipientEmail:     recipient,
				Subject:            template.Subject,
				Status:             domain.CommunicationQueued,
				Provider:           "scheduler",
				DeliveryMode:       domain.DeliveryAutomatic,
				Trigger:            trigger.Name,
				IdempotencyKey:     key,
				ScheduledFor:       now,
				NextAttemptAt:      now,
				CreatedAt:          now,
				MaxAttempts:        retryLimit(rule.RetryLimit),
			}
			if rule.SuppressMinutes > 0 && recentlyNotified(*state, rule.ID, recipient, now.Add(-time.Duration(rule.SuppressMinutes)*time.Minute)) {
				item.Status = domain.CommunicationSuppressed
				item.SuppressedAt = now
				item.Error = "suppressed by notification policy"
			}
			state.Communications = append(state.Communications, item)
			created++
		}
	}
	return created
}

func notificationKey(ruleID, entityID, recipient string) string {
	return "notification:" + strings.TrimSpace(ruleID) + ":" + strings.TrimSpace(entityID) + ":" + strings.ToLower(strings.TrimSpace(recipient))
}

func recentlyNotified(state domain.State, ruleID, recipient string, since time.Time) bool {
	for _, item := range state.Communications {
		if item.NotificationRuleID != ruleID || !strings.EqualFold(item.RecipientEmail, recipient) {
			continue
		}
		when := item.SentAt
		if when.IsZero() {
			when = item.CreatedAt
		}
		if !when.IsZero() && !when.Before(since) {
			return true
		}
	}
	return false
}

func communicationWithKey(state domain.State, key string) bool {
	for _, item := range state.Communications {
		if item.IdempotencyKey == key {
			return true
		}
	}
	return false
}

// Runner turns due, persisted Communications into provider calls. It is safe
// to create a Runner in each process: leases and idempotency keys coordinate
// concurrent SQLite/Postgres workers, while a JSON deployment remains a
// single-writer process by its documented storage contract.
type Runner struct {
	Store  store.StateStore
	Sender mail.Sender
	Now    func() time.Time
}

type Report struct {
	Enqueued   int
	Claimed    int
	Sent       int
	Retried    int
	Failed     int
	Suppressed int
	Cancelled  int
}

type claim struct {
	ID      string
	Message mail.Message
}

// RunDue first derives the explicitly configured task/session reminders,
// then drains all deliverable rows that are due. It performs no work for
// Gmail/Outlook hand-off rows; those remain operator-controlled records.
func (runner Runner) RunDue() (Report, error) {
	if runner.Store == nil {
		return Report{}, fmt.Errorf("communications: store is required")
	}
	if runner.Sender == nil {
		return Report{}, fmt.Errorf("communications: sender is required")
	}
	now := runner.now()
	report, err := runner.enqueueDueReminders(now)
	if err != nil {
		return report, err
	}
	for {
		item, outcome, found, err := runner.claimOne(now)
		addReport(&report, outcome)
		if err != nil {
			return report, err
		}
		if !found {
			return report, nil
		}
		report.Claimed++
		provider := "mail"
		if named, ok := runner.Sender.(mail.Named); ok {
			provider = named.Name()
		}
		sendErr := runner.Sender.Send(item.Message)
		outcome, err = runner.finish(item.ID, provider, sendErr, now)
		addReport(&report, outcome)
		if err != nil {
			return report, err
		}
	}
}

func (runner Runner) now() time.Time {
	if runner.Now != nil {
		return runner.Now().UTC()
	}
	return time.Now().UTC()
}

func (runner Runner) enqueueDueReminders(now time.Time) (Report, error) {
	snapshot := runner.Store.Snapshot()
	preview := snapshot
	if enqueueReminders(&preview, now) == 0 {
		return Report{}, nil
	}
	report := Report{}
	err := runner.Store.UpdateAudit(domain.AuditMeta{
		Actor:      "system:communications",
		Action:     "communication.reminders_enqueued",
		EntityType: "communication_outbox",
		EntityID:   "due-reminders",
		Summary:    "Derived due task and session reminders into the durable outbox.",
		Origin:     "communications-scheduler",
	}, func(state *domain.State) error {
		before := len(state.Communications)
		report.Enqueued = enqueueReminders(state, now)
		for _, item := range state.Communications[before:] {
			if item.Status == domain.CommunicationSuppressed {
				report.Suppressed++
			}
		}
		return nil
	})
	return report, err
}

func enqueueReminders(state *domain.State, now time.Time) int {
	if state == nil {
		return 0
	}
	created := 0
	if template, found := findTemplate(*state, "tpl_five_day"); found {
		for _, task := range state.Tasks {
			if !task.Active() || task.DueAt.IsZero() || task.DueAt.Before(now) || task.DueAt.After(now.Add(fiveDays)) {
				continue
			}
			for _, speakerID := range task.AssignedSpeakerIDs {
				speaker, speakerFound := state.Speaker(speakerID)
				if !speakerFound || !state.TaskAssignedToSpeaker(task, speakerID) || !taskOutstanding(*state, task.ID, speakerID) {
					continue
				}
				key := "task-reminder:" + task.ID + ":" + speakerID + ":" + task.DueAt.UTC().Format(time.RFC3339)
				if communicationWithKey(*state, key) {
					continue
				}
				item := newReminder(template, speaker, task.ID, "task.due_soon", key, now)
				if speaker.EmailOptOut {
					item.Status = domain.CommunicationSuppressed
					item.SuppressedAt = now
					item.Error = "recipient opted out of reminders"
				}
				state.Communications = append(state.Communications, item)
				created++
			}
		}
	}
	if template, found := findTemplate(*state, "tpl_one_day"); found {
		for _, sessionItem := range state.Sessions {
			if !sessionItem.Scheduled() || sessionItem.Status == "cancelled" || sessionItem.StartsAt.Before(now) || sessionItem.StartsAt.After(now.Add(oneDay)) {
				continue
			}
			for _, speakerID := range sessionItem.SpeakerIDs {
				speaker, speakerFound := state.Speaker(speakerID)
				if !speakerFound {
					continue
				}
				key := "session-reminder:" + sessionItem.ID + ":" + speakerID + ":" + sessionItem.StartsAt.UTC().Format(time.RFC3339)
				if communicationWithKey(*state, key) {
					continue
				}
				item := newReminder(template, speaker, "", "session.starts_soon", key, now)
				item.SessionID = sessionItem.ID
				if speaker.EmailOptOut {
					item.Status = domain.CommunicationSuppressed
					item.SuppressedAt = now
					item.Error = "recipient opted out of reminders"
				}
				state.Communications = append(state.Communications, item)
				created++
			}
		}
	}
	return created
}

func newReminder(template domain.EmailTemplate, speaker *domain.Speaker, taskID, trigger, key string, now time.Time) domain.Communication {
	speakerID, recipient, name := "", "", ""
	if speaker != nil {
		speakerID = speaker.ID
		recipient = speaker.Email
		name = speaker.Name()
	}
	return domain.Communication{
		ID:             domain.NewID("comm"),
		TemplateID:     template.ID,
		SpeakerID:      speakerID,
		TaskID:         taskID,
		RecipientEmail: recipient,
		RecipientName:  name,
		Subject:        template.Subject,
		Status:         domain.CommunicationScheduled,
		Provider:       "scheduler",
		DeliveryMode:   domain.DeliveryAutomatic,
		Trigger:        trigger,
		IdempotencyKey: key,
		ScheduledFor:   now,
		NextAttemptAt:  now,
		CreatedAt:      now,
		MaxAttempts:    defaultMaxAttempts,
	}
}

func taskOutstanding(state domain.State, taskID, speakerID string) bool {
	completion, found := state.Completion(taskID, speakerID)
	return !found || completion.Status == domain.TaskOutstanding || completion.Status == domain.TaskDeclined
}

func (runner Runner) claimOne(now time.Time) (claim, Report, bool, error) {
	var claimed claim
	var report Report
	found := false
	if !hasDueCandidate(runner.Store.Snapshot(), now) {
		return claimed, report, false, nil
	}
	err := runner.Store.UpdateAudit(domain.AuditMeta{
		Actor:      "system:communications",
		Action:     "communication.delivery_claimed",
		EntityType: "communication_outbox",
		EntityID:   "due",
		Summary:    "Claimed one due outbound communication behind a delivery lease.",
		Origin:     "communications-scheduler",
	}, func(state *domain.State) error {
		for index := range state.Communications {
			item := &state.Communications[index]
			if !dueForClaim(*item, now) {
				continue
			}
			if item.DeliveryMode == domain.DeliveryHandoff {
				continue
			}
			message, outcome, canSend := prepareMessage(state, item, now)
			addReport(&report, outcome)
			if !canSend {
				// The item transitioned terminally (suppressed/cancelled) above;
				// keep scanning for a real send in this run.
				continue
			}
			item.Status = domain.CommunicationSending
			item.AttemptCount++
			item.LastAttemptAt = now
			item.LeaseUntil = now.Add(leaseDuration)
			item.Subject = message.Subject
			claimed = claim{ID: item.ID, Message: message}
			found = true
			return nil
		}
		return nil
	})
	return claimed, report, found, err
}

func dueForClaim(item domain.Communication, now time.Time) bool {
	switch item.Status {
	case domain.CommunicationQueued, domain.CommunicationScheduled, domain.CommunicationRetrying:
		// Eligible below.
	case domain.CommunicationSending:
		return !item.LeaseUntil.IsZero() && !item.LeaseUntil.After(now)
	default:
		return false
	}
	if !item.CancelledAt.IsZero() {
		return false
	}
	if !item.SuppressedAt.IsZero() {
		return false
	}
	due := item.NextAttemptAt
	if due.IsZero() {
		due = item.ScheduledFor
	}
	return due.IsZero() || !due.After(now)
}

func prepareMessage(state *domain.State, item *domain.Communication, now time.Time) (mail.Message, Report, bool) {
	if item == nil || state == nil {
		return mail.Message{}, Report{}, false
	}
	template, templateFound := findTemplate(*state, item.TemplateID)
	if !templateFound {
		item.Status = domain.CommunicationCancelled
		item.CancelledAt = now
		item.Error = "template is no longer available"
		return mail.Message{}, Report{Cancelled: 1}, false
	}
	var speaker domain.Speaker
	if item.SpeakerID != "" {
		found, speakerFound := state.Speaker(item.SpeakerID)
		if !speakerFound {
			item.Status = domain.CommunicationCancelled
			item.CancelledAt = now
			item.Error = "recipient is no longer available"
			return mail.Message{}, Report{Cancelled: 1}, false
		}
		speaker = *found
		if speaker.EmailOptOut && item.NotificationRuleID == "" && item.Trigger != "" {
			item.Status = domain.CommunicationSuppressed
			item.SuppressedAt = now
			item.Error = "recipient opted out of automated mail"
			return mail.Message{}, Report{Suppressed: 1}, false
		}
	}
	recipient := strings.TrimSpace(item.RecipientEmail)
	if recipient == "" {
		recipient = strings.TrimSpace(speaker.Email)
	}
	if recipient == "" {
		item.Status = domain.CommunicationCancelled
		item.CancelledAt = now
		item.Error = "recipient has no email address"
		return mail.Message{}, Report{Cancelled: 1}, false
	}
	sessionItem := sessionByID(*state, item.SessionID)
	submission := submissionByID(*state, item.SubmissionID)
	task := taskByID(*state, item.TaskID)
	subject, body := present.RenderCommunicationContext(*state, template, speaker, sessionItem, submission, task)
	message := mail.Message{
		To:             recipient,
		ToName:         firstNonEmpty(item.RecipientName, speaker.Name()),
		Subject:        subject,
		TextBody:       body,
		IdempotencyKey: firstNonEmpty(item.IdempotencyKey, item.ID),
	}
	if template.AttachCalendar && speaker.ID != "" && sessionItem.Scheduled() {
		if calendar, err := programcalendar.Invite(*state, sessionItem, speaker, organizerEmail(template)); err == nil {
			message.Calendar = calendar
		}
	}
	return message, Report{}, true
}

func hasDueCandidate(state domain.State, now time.Time) bool {
	for _, item := range state.Communications {
		if item.DeliveryMode == domain.DeliveryHandoff {
			continue
		}
		if dueForClaim(item, now) {
			return true
		}
	}
	return false
}

func (runner Runner) finish(id, provider string, sendErr error, now time.Time) (Report, error) {
	report := Report{}
	err := runner.Store.UpdateAudit(domain.AuditMeta{
		Actor:      "system:communications",
		Action:     "communication.delivery_recorded",
		EntityType: "communication",
		EntityID:   id,
		Summary:    "Recorded a durable outbound communication delivery result.",
		Origin:     "communications-scheduler",
	}, func(state *domain.State) error {
		for index := range state.Communications {
			item := &state.Communications[index]
			if item.ID != id || item.Status != domain.CommunicationSending {
				continue
			}
			item.Provider = provider
			item.LeaseUntil = time.Time{}
			if sendErr == nil {
				item.Status = domain.CommunicationSent
				item.SentAt = now
				item.Error = ""
				report.Sent++
				return nil
			}
			item.Error = "delivery failed"
			if item.AttemptCount >= retryLimit(item.MaxAttempts) {
				item.Status = domain.CommunicationFailed
				report.Failed++
				return nil
			}
			item.Status = domain.CommunicationRetrying
			item.NextAttemptAt = now.Add(backoff(item.AttemptCount))
			report.Retried++
			return nil
		}
		return fmt.Errorf("communications: claimed item %s disappeared before result was recorded", id)
	})
	return report, err
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 6 {
		shift = 6
	}
	return time.Minute * time.Duration(1<<shift)
}

func retryLimit(value int) int {
	if value <= 0 {
		return defaultMaxAttempts
	}
	return value
}

func findTemplate(state domain.State, id string) (domain.EmailTemplate, bool) {
	for _, template := range state.EmailTemplates {
		if template.ID == id {
			return template, true
		}
	}
	return domain.EmailTemplate{}, false
}

func sessionByID(state domain.State, id string) domain.Session {
	for _, item := range state.Sessions {
		if item.ID == id {
			return item
		}
	}
	return domain.Session{}
}

func submissionByID(state domain.State, id string) domain.Submission {
	for _, item := range state.Submissions {
		if item.ID == id {
			return item
		}
	}
	return domain.Submission{}
}

func taskByID(state domain.State, id string) domain.Task {
	for _, item := range state.Tasks {
		if item.ID == id {
			return item
		}
	}
	return domain.Task{}
}

func organizerEmail(template domain.EmailTemplate) string {
	if value := strings.TrimSpace(template.ReplyTo); value != "" {
		return value
	}
	return "program@example.com"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func addReport(target *Report, addition Report) {
	if target == nil {
		return
	}
	target.Enqueued += addition.Enqueued
	target.Claimed += addition.Claimed
	target.Sent += addition.Sent
	target.Retried += addition.Retried
	target.Failed += addition.Failed
	target.Suppressed += addition.Suppressed
	target.Cancelled += addition.Cancelled
}
