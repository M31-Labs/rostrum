package communications

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	delivery "github.com/m31-labs/rostrum/internal/communications"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/mailtemplate"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Communications(appstate.MustGet().Snapshot(), ctx.Query("template"), ctx.Query("recipient")), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Communications — Rostrum"}, Description: "Template, schedule, and preview speaker communications and calendar invites."}, nil
		},
		Actions: route.FileActions{
			"queueMessage":           queueMessage,
			"createTemplate":         createTemplate,
			"saveTemplate":           saveTemplate,
			"deleteTemplate":         deleteTemplate,
			"saveNotificationRule":   saveNotificationRule,
			"removeNotificationRule": removeNotificationRule,
			"cancelCommunication":    cancelCommunication,
			"runDue":                 runDue,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

// messageSender is the process-wide mail transport this package sends
// through, resolved once on first use. It mirrors
// app/submit/page.server.go's confirmationSender: a deferred value, not a
// plain package var, because mail.FromEnv reads SMTP_* from the
// environment, which main() loads from .env only after package-level
// initialization has already run.
var messageSender = sync.OnceValue(mail.FromEnv)

// queueMessage first persists a durable outbox row, then asks the shared
// lease-based runner to process every due automatic delivery. Gmail/Outlook
// remain explicit hand-offs and are never sent by an automated transport.
func queueMessage(ctx *action.Context) error {
	templateID := strings.TrimSpace(ctx.FormData["template_id"])
	speakerID := strings.TrimSpace(ctx.FormData["speaker_id"])
	provider := strings.TrimSpace(ctx.FormData["provider"])
	if provider != "configured" && provider != "outbox" && provider != "gmail" && provider != "outlook" {
		return action.Validation("Choose a delivery provider.", map[string]string{"provider": "Unknown provider."}, ctx.FormData)
	}

	snapshot := appstate.MustGet().Snapshot()
	speaker, found := snapshot.Speaker(speakerID)
	if !found {
		return fmt.Errorf("speaker %s not found", speakerID)
	}
	template, found := emailTemplate(snapshot, templateID)
	if !found {
		return fmt.Errorf("template %s not found", templateID)
	}
	if template.Audience == "administrator" {
		return action.Validation("Administrator templates are delivered only by configured event triggers.", map[string]string{"template_id": "Choose a speaker-facing template."}, ctx.FormData)
	}
	sessionItem, hasSession := present.RecipientSession(snapshot, speaker.ID)
	now := time.Now().UTC()
	itemID := domain.NewID("comm")
	deliveryMode := domain.DeliveryAutomatic
	if provider == "gmail" || provider == "outlook" {
		deliveryMode = domain.DeliveryHandoff
	}
	item := domain.Communication{
		ID:             itemID,
		TemplateID:     template.ID,
		SpeakerID:      speaker.ID,
		RecipientEmail: speaker.Email,
		RecipientName:  speaker.Name(),
		Subject:        template.Subject,
		Status:         domain.CommunicationQueued,
		Provider:       provider,
		DeliveryMode:   deliveryMode,
		IdempotencyKey: itemID,
		ScheduledFor:   now,
		NextAttemptAt:  now,
		CreatedAt:      now,
		MaxAttempts:    5,
	}
	if hasSession {
		item.SessionID = sessionItem.ID
	}

	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      communicationsActor(ctx),
		Action:     "communication.queued",
		EntityType: "communication",
		EntityID:   itemID,
		Summary:    "Queued an organizer-authored speaker communication in the durable outbox.",
		Origin:     "organizer-communications",
	}, func(state *domain.State) error {
		state.Communications = append(state.Communications, item)
		return nil
	}); err != nil {
		return err
	}
	if deliveryMode == domain.DeliveryAutomatic {
		if _, err := (delivery.Runner{Store: appstate.MustGet(), Sender: messageSender()}).RunDue(); err != nil {
			return err
		}
	}
	result := communicationByID(appstate.MustGet().Snapshot(), itemID)
	notice := "Queued the selected template for " + speaker.Name() + " via " + provider + "."
	switch result.Status {
	case domain.CommunicationSent:
		notice = "Sent the selected template to " + speaker.Name() + " via " + result.Provider + "."
	case domain.CommunicationFailed, domain.CommunicationRetrying:
		notice = "Could not send the selected template to " + speaker.Name() + ". Check the mail transport configuration."
	}
	session.AddFlash(ctx.Request, "notice", notice)
	live.Broadcast("communication:queued", map[string]string{"speaker": speakerID, "provider": provider})
	actionflow.Redirect(ctx, "/organizer/communications?template="+templateID)
	return nil
}

func createTemplate(ctx *action.Context) error {
	name := strings.TrimSpace(ctx.FormData["name"])
	audience := strings.TrimSpace(ctx.FormData["audience"])
	if audience == "" {
		audience = "speaker"
	}
	subject := strings.TrimSpace(ctx.FormData["subject"])
	body := strings.TrimSpace(ctx.FormData["body"])
	replyTo := strings.TrimSpace(ctx.FormData["reply_to"])
	if err := mailtemplate.Validate(name, audience, subject, body, replyTo); err != nil {
		return action.Validation("Correct the template details.", map[string]string{"template": err.Error()}, ctx.FormData)
	}
	if !validAudience(audience) {
		return action.Validation("Choose a supported audience.", map[string]string{"audience": "Use speaker, submitter, or administrator."}, ctx.FormData)
	}
	base := "tpl_" + domain.Slugify(name)
	if base == "tpl_" {
		return action.Validation("Use a name with letters or numbers.", map[string]string{"name": "Use a clearer template name."}, ctx.FormData)
	}
	templateID := uniqueTemplateID(appstate.MustGet().Snapshot().EmailTemplates, base)
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor: communicationsActor(ctx), Action: "communication.template_created", EntityType: "email_template", EntityID: templateID,
		Summary: "Created an editable communications template.", Origin: "organizer-communications",
	}, func(state *domain.State) error {
		if templateIDTaken(state.EmailTemplates, templateID) {
			return action.Validation("Choose a different template name.", map[string]string{"name": "A template with that name already exists."}, ctx.FormData)
		}
		template := domain.EmailTemplate{ID: templateID, Name: name, Audience: audience, Subject: subject, Body: body, ReplyTo: replyTo, AttachCalendar: ctx.FormData["attach_calendar"] == "on"}
		state.EmailTemplates = append(state.EmailTemplates, template)
		appendTemplateRevision(state, template, communicationsActor(ctx), time.Now().UTC())
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Created a new editable template.")
	actionflow.Redirect(ctx, templateURL(templateID))
	return nil
}

func saveTemplate(ctx *action.Context) error {
	templateID := strings.TrimSpace(ctx.FormData["template_id"])
	snapshot := appstate.MustGet().Snapshot()
	current, found := emailTemplate(snapshot, templateID)
	if !found {
		return action.Error(404, "Template not found.")
	}
	name := strings.TrimSpace(ctx.FormData["name"])
	if current.System {
		name = current.Name
	}
	audience := current.Audience
	if !current.System && strings.TrimSpace(ctx.FormData["audience"]) != "" {
		audience = strings.TrimSpace(ctx.FormData["audience"])
	}
	subject, body, replyTo := strings.TrimSpace(ctx.FormData["subject"]), strings.TrimSpace(ctx.FormData["body"]), strings.TrimSpace(ctx.FormData["reply_to"])
	if err := mailtemplate.Validate(name, audience, subject, body, replyTo); err != nil {
		return action.Validation("Correct the template details.", map[string]string{"template": err.Error()}, ctx.FormData)
	}
	if !validAudience(audience) {
		return action.Validation("Choose a supported audience.", map[string]string{"audience": "Use speaker, submitter, or administrator."}, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor: communicationsActor(ctx), Action: "communication.template_revised", EntityType: "email_template", EntityID: templateID,
		Summary: "Revised a communications template and retained its previous version.", Origin: "organizer-communications",
	}, func(state *domain.State) error {
		template, exists := stateTemplate(state, templateID)
		if !exists {
			return action.Error(404, "Template not found.")
		}
		template.Name, template.Audience, template.Subject, template.Body, template.ReplyTo = name, audience, subject, body, replyTo
		template.AttachCalendar = ctx.FormData["attach_calendar"] == "on"
		appendTemplateRevision(state, *template, communicationsActor(ctx), time.Now().UTC())
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Saved the template revision.")
	actionflow.Redirect(ctx, templateURL(templateID))
	return nil
}

func deleteTemplate(ctx *action.Context) error {
	templateID := strings.TrimSpace(ctx.FormData["template_id"])
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor: communicationsActor(ctx), Action: "communication.template_deleted", EntityType: "email_template", EntityID: templateID,
		Summary: "Deleted an unused custom communications template.", Origin: "organizer-communications",
	}, func(state *domain.State) error {
		template, found := stateTemplate(state, templateID)
		if !found {
			return action.Error(404, "Template not found.")
		}
		if template.System {
			return action.Validation("System templates stay available for core lifecycle email.", map[string]string{"template": "System templates cannot be deleted."}, ctx.FormData)
		}
		for _, item := range state.Communications {
			if item.TemplateID == templateID {
				return action.Validation("This template is retained because the delivery ledger references it.", map[string]string{"template": "Cancel or retain existing delivery history; do not delete it."}, ctx.FormData)
			}
		}
		for _, rule := range state.NotificationRules {
			if rule.TemplateID == templateID {
				return action.Validation("Remove notification rules that use this template first.", map[string]string{"template": "A notification rule still references it."}, ctx.FormData)
			}
		}
		state.EmailTemplates = withoutTemplate(state.EmailTemplates, templateID)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Deleted the unused custom template.")
	actionflow.Redirect(ctx, "/organizer/communications")
	return nil
}

func saveNotificationRule(ctx *action.Context) error {
	ruleID := strings.TrimSpace(ctx.FormData["rule_id"])
	name := strings.TrimSpace(ctx.FormData["name"])
	trigger := strings.TrimSpace(ctx.FormData["trigger"])
	templateID := strings.TrimSpace(ctx.FormData["template_id"])
	recipients := splitRecipients(ctx.FormData["recipients"])
	retryLimit, retryErr := parseRange(ctx.FormData["retry_limit"], 1, 10, 5)
	suppressMinutes, suppressErr := parseRange(ctx.FormData["suppress_minutes"], 0, 60*24*365, 10)
	if name == "" || !validTrigger(trigger) || len(recipients) == 0 || retryErr != nil || suppressErr != nil {
		return action.Validation("Correct the notification rule.", map[string]string{"rule": "Provide a name, supported trigger, recipients, retry limit, and suppression window."}, ctx.FormData)
	}
	if ruleID == "" {
		ruleID = uniqueRuleID(appstate.MustGet().Snapshot().NotificationRules, "notify_"+domain.Slugify(name))
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor: communicationsActor(ctx), Action: "communication.notification_rule_saved", EntityType: "notification_rule", EntityID: ruleID,
		Summary: "Created or updated an administrator notification rule.", Origin: "organizer-communications",
	}, func(state *domain.State) error {
		template, found := emailTemplate(*state, templateID)
		if !found || template.Audience != "administrator" {
			return action.Validation("Choose an administrator-facing template.", map[string]string{"template_id": "Administrator rules require an administrator template."}, ctx.FormData)
		}
		rule := domain.NotificationRule{ID: ruleID, Name: name, Trigger: trigger, TemplateID: templateID, RecipientEmails: recipients, Enabled: ctx.FormData["enabled"] == "on", RetryLimit: retryLimit, SuppressMinutes: suppressMinutes}
		for index := range state.NotificationRules {
			if state.NotificationRules[index].ID == ruleID {
				state.NotificationRules[index] = rule
				return nil
			}
		}
		if ruleIDTaken(state.NotificationRules, ruleID) {
			return action.Validation("Choose a different notification rule name.", map[string]string{"name": "That rule ID is already used."}, ctx.FormData)
		}
		state.NotificationRules = append(state.NotificationRules, rule)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Saved the administrator notification rule.")
	actionflow.Redirect(ctx, templateURL(strings.TrimSpace(ctx.FormData["selected_template"])))
	return nil
}

func removeNotificationRule(ctx *action.Context) error {
	ruleID := strings.TrimSpace(ctx.FormData["rule_id"])
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor: communicationsActor(ctx), Action: "communication.notification_rule_removed", EntityType: "notification_rule", EntityID: ruleID,
		Summary: "Removed an administrator notification rule.", Origin: "organizer-communications",
	}, func(state *domain.State) error {
		if !ruleIDTaken(state.NotificationRules, ruleID) {
			return action.Error(404, "Notification rule not found.")
		}
		state.NotificationRules = withoutRule(state.NotificationRules, ruleID)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Removed the notification rule. Existing queued rows remain visible in the ledger.")
	actionflow.Redirect(ctx, templateURL(strings.TrimSpace(ctx.FormData["selected_template"])))
	return nil
}

func cancelCommunication(ctx *action.Context) error {
	id := strings.TrimSpace(ctx.FormData["communication_id"])
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor: communicationsActor(ctx), Action: "communication.cancelled", EntityType: "communication", EntityID: id,
		Summary: "Cancelled a queued outbound communication before delivery.", Origin: "organizer-communications",
	}, func(state *domain.State) error {
		for index := range state.Communications {
			item := &state.Communications[index]
			if item.ID != id {
				continue
			}
			switch item.Status {
			case domain.CommunicationQueued, domain.CommunicationScheduled, domain.CommunicationRetrying:
				item.Status = domain.CommunicationCancelled
				item.CancelledAt = time.Now().UTC()
				item.Error = "cancelled by organizer"
				return nil
			default:
				return action.Validation("Only queued or retrying messages can be cancelled.", map[string]string{"communication": "This delivery is already terminal or in flight."}, ctx.FormData)
			}
		}
		return action.Error(404, "Communication not found.")
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Cancelled the queued communication.")
	actionflow.Redirect(ctx, templateURL(strings.TrimSpace(ctx.FormData["selected_template"])))
	return nil
}

func runDue(ctx *action.Context) error {
	report, err := (delivery.Runner{Store: appstate.MustGet(), Sender: messageSender()}).RunDue()
	if err != nil {
		return err
	}
	notice := fmt.Sprintf("Outbox run complete: %d sent, %d retried, %d failed, %d suppressed.", report.Sent, report.Retried, report.Failed, report.Suppressed)
	session.AddFlash(ctx.Request, "notice", notice)
	live.Broadcast("communication:outbox_run", map[string]int{"sent": report.Sent, "retried": report.Retried, "failed": report.Failed})
	actionflow.Redirect(ctx, templateURL(strings.TrimSpace(ctx.FormData["selected_template"])))
	return nil
}

// emailTemplate finds the EmailTemplate named id in state, if any.
func emailTemplate(state domain.State, id string) (domain.EmailTemplate, bool) {
	for _, template := range state.EmailTemplates {
		if template.ID == id {
			return template, true
		}
	}
	return domain.EmailTemplate{}, false
}

func communicationByID(state domain.State, id string) domain.Communication {
	for _, item := range state.Communications {
		if item.ID == id {
			return item
		}
	}
	return domain.Communication{}
}

func stateTemplate(state *domain.State, id string) (*domain.EmailTemplate, bool) {
	if state == nil {
		return nil, false
	}
	for index := range state.EmailTemplates {
		if state.EmailTemplates[index].ID == id {
			return &state.EmailTemplates[index], true
		}
	}
	return nil, false
}

func appendTemplateRevision(state *domain.State, template domain.EmailTemplate, actor string, now time.Time) {
	if state == nil {
		return
	}
	revision := 0
	for _, existing := range state.TemplateRevisions {
		if existing.TemplateID == template.ID && existing.Revision > revision {
			revision = existing.Revision
		}
	}
	state.TemplateRevisions = append(state.TemplateRevisions, domain.EmailTemplateRevision{
		ID:             domain.NewID("tmplrev"),
		TemplateID:     template.ID,
		Revision:       revision + 1,
		Name:           template.Name,
		Subject:        template.Subject,
		Body:           template.Body,
		ReplyTo:        template.ReplyTo,
		AttachCalendar: template.AttachCalendar,
		Actor:          actor,
		CreatedAt:      now,
	})
}

func validAudience(value string) bool {
	switch value {
	case "speaker", "submitter", "administrator":
		return true
	default:
		return false
	}
}

func validTrigger(value string) bool {
	switch value {
	case "submission.created", "submission.withdrawn", "task.submitted", "task.approved":
		return true
	default:
		return false
	}
}

func splitRecipients(value string) []string {
	parts := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == ';' || character == '\n' || character == '\r'
	})
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		email := strings.ToLower(strings.TrimSpace(part))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		result = append(result, email)
	}
	return result
}

func parseRange(raw string, minimum, maximum, fallback int) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("value must be between %d and %d", minimum, maximum)
	}
	return parsed, nil
}

func uniqueTemplateID(templates []domain.EmailTemplate, base string) string {
	if !templateIDTaken(templates, base) {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := base + "-" + strconv.Itoa(suffix)
		if !templateIDTaken(templates, candidate) {
			return candidate
		}
	}
}

func templateIDTaken(templates []domain.EmailTemplate, id string) bool {
	for _, template := range templates {
		if template.ID == id {
			return true
		}
	}
	return false
}

func uniqueRuleID(rules []domain.NotificationRule, base string) string {
	if base == "notify_" {
		base = "notify_rule"
	}
	if !ruleIDTaken(rules, base) {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := base + "-" + strconv.Itoa(suffix)
		if !ruleIDTaken(rules, candidate) {
			return candidate
		}
	}
}

func ruleIDTaken(rules []domain.NotificationRule, id string) bool {
	for _, rule := range rules {
		if rule.ID == id {
			return true
		}
	}
	return false
}

func withoutTemplate(items []domain.EmailTemplate, id string) []domain.EmailTemplate {
	result := items[:0]
	for _, item := range items {
		if item.ID != id {
			result = append(result, item)
		}
	}
	return result
}

func withoutRule(items []domain.NotificationRule, id string) []domain.NotificationRule {
	result := items[:0]
	for _, item := range items {
		if item.ID != id {
			result = append(result, item)
		}
	}
	return result
}

func templateURL(templateID string) string {
	if strings.TrimSpace(templateID) == "" {
		return "/organizer/communications"
	}
	return "/organizer/communications?template=" + templateID
}

func communicationsActor(ctx *action.Context) string {
	if ctx != nil && ctx.Request != nil {
		if user, found := auth.Current(ctx.Request); found && strings.TrimSpace(user.ID) != "" {
			return "organizer:" + user.ID
		}
	}
	return "organizer"
}
