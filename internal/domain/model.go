package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const CurrentSchemaVersion = 1

type State struct {
	SchemaVersion     int                     `json:"schemaVersion"`
	Event             Event                   `json:"event"`
	Forms             []SubmissionForm        `json:"forms"`
	Speakers          []Speaker               `json:"speakers"`
	Submissions       []Submission            `json:"submissions"`
	Reviewers         []Reviewer              `json:"reviewers"`
	ReviewPlans       []ReviewPlan            `json:"reviewPlans"`
	Evaluations       []Evaluation            `json:"evaluations"`
	Sessions          []Session               `json:"sessions"`
	Tasks             []Task                  `json:"tasks"`
	TaskCompletions   []TaskCompletion        `json:"taskCompletions"`
	Resources         []Resource              `json:"resources"`
	EmailTemplates    []EmailTemplate         `json:"emailTemplates"`
	TemplateRevisions []EmailTemplateRevision `json:"templateRevisions"`
	NotificationRules []NotificationRule      `json:"notificationRules"`
	Communications    []Communication         `json:"communications"`
	Integrations      []Integration           `json:"integrations"`
	SyncRuns          []SyncRun               `json:"syncRuns"`
	Principals        []Principal             `json:"principals"`
	AuthMagicLinks    []AuthMagicLink         `json:"authMagicLinks"`
	AuthPasskeys      []AuthPasskey           `json:"authPasskeys"`
	AuditEvents       []AuditEvent            `json:"auditEvents"`
	SyncOutbox        []SyncOutboxItem        `json:"syncOutbox"`
	UpdatedAt         time.Time               `json:"updatedAt"`
}

type Event struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Type        string     `json:"type"`
	WebsiteURL  string     `json:"websiteUrl"`
	Location    string     `json:"location"`
	TimeZone    string     `json:"timeZone"`
	StartsAt    time.Time  `json:"startsAt"`
	EndsAt      time.Time  `json:"endsAt"`
	Theme       string     `json:"theme"`
	Description string     `json:"description"`
	LogoURL     string     `json:"logoUrl"`
	Tracks      []Track    `json:"tracks"`
	Rooms       []Room     `json:"rooms"`
	Categories  []Category `json:"categories"`
	Formats     []string   `json:"formats"`
	Levels      []string   `json:"levels"`
}

type Track struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type Room struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
}

type Category struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OwnerName  string `json:"ownerName"`
	OwnerEmail string `json:"ownerEmail"`
	TrackID    string `json:"trackId"`
}

type SubmissionForm struct {
	ID                    string         `json:"id"`
	EventID               string         `json:"eventId"`
	Name                  string         `json:"name"`
	ExternalTitle         string         `json:"externalTitle"`
	Slug                  string         `json:"slug"`
	Kind                  string         `json:"kind"`
	Status                string         `json:"status"`
	WelcomeHeading        string         `json:"welcomeHeading"`
	WelcomeBody           string         `json:"welcomeBody"`
	SuccessHeading        string         `json:"successHeading"`
	SuccessBody           string         `json:"successBody"`
	CloseAt               time.Time      `json:"closeAt"`
	MaxDraftsPerSubmitter int            `json:"maxDraftsPerSubmitter"`
	RedirectToPortal      bool           `json:"redirectToPortal"`
	SendConfirmation      bool           `json:"sendConfirmation"`
	ConfirmationTemplate  string         `json:"confirmationTemplate"`
	RuleFile              string         `json:"ruleFile"`
	Fields                []FormField    `json:"fields"`
	QuestionRules         []QuestionRule `json:"questionRules"`
}

type FormField struct {
	ID          string   `json:"id"`
	Section     string   `json:"section"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Locked      bool     `json:"locked"`
	Placeholder string   `json:"placeholder"`
	Help        string   `json:"help"`
	Options     []string `json:"options"`
	MaxLength   int      `json:"maxLength"`
}

type QuestionRule struct {
	ID            string `json:"id"`
	SourceFieldID string `json:"sourceFieldId"`
	Operator      string `json:"operator"`
	Value         string `json:"value"`
	TargetFieldID string `json:"targetFieldId"`
	Effect        string `json:"effect"`
	Description   string `json:"description"`
}

// DefaultSuccessHeading and DefaultSuccessBody are the success-page copy a
// form falls back to when its builder has not set SuccessHeading or
// SuccessBody (FB-5). A freshly seeded form leaves both blank, so a new
// workspace shows this copy until an organizer customizes it.
const (
	DefaultSuccessHeading = "Thanks — we have your proposal"
	DefaultSuccessBody    = "Your proposal is safely in our review queue and a confirmation is on its way. You can finish your speaker profile in the portal while review begins."
)

// SuccessPageHeading returns the form's customized success-page heading, or
// DefaultSuccessHeading when the builder has not set SuccessHeading.
func (form SubmissionForm) SuccessPageHeading() string {
	if strings.TrimSpace(form.SuccessHeading) != "" {
		return form.SuccessHeading
	}
	return DefaultSuccessHeading
}

// SuccessPageBody returns the form's customized success-page body, or
// DefaultSuccessBody when the builder has not set SuccessBody.
func (form SubmissionForm) SuccessPageBody() string {
	if strings.TrimSpace(form.SuccessBody) != "" {
		return form.SuccessBody
	}
	return DefaultSuccessBody
}

type Speaker struct {
	ID          string `json:"id"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Email       string `json:"email"`
	Pronouns    string `json:"pronouns"`
	Role        string `json:"role"`
	Company     string `json:"company"`
	Biography   string `json:"biography"`
	HeadshotURL string `json:"headshotUrl"`
	LinkedInURL string `json:"linkedInUrl"`
	WebsiteURL  string `json:"websiteUrl"`
	City        string `json:"city"`
	// EmailOptOut prevents non-essential reminder and administrator-triggered
	// mail from being delivered to this speaker. Transactional confirmation
	// and account-recovery paths remain explicit product flows outside the
	// scheduler; this preference only governs the durable communications
	// outbox.
	EmailOptOut   bool      `json:"emailOptOut"`
	EmailOptOutAt time.Time `json:"emailOptOutAt"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (speaker Speaker) Name() string {
	return strings.TrimSpace(speaker.FirstName + " " + speaker.LastName)
}

func (speaker Speaker) Initials() string {
	first := firstRune(speaker.FirstName)
	last := firstRune(speaker.LastName)
	return strings.ToUpper(first + last)
}

type Submission struct {
	ID               string            `json:"id"`
	EventID          string            `json:"eventId"`
	FormID           string            `json:"formId"`
	Title            string            `json:"title"`
	Abstract         string            `json:"abstract"`
	Format           string            `json:"format"`
	CategoryID       string            `json:"categoryId"`
	TrackID          string            `json:"trackId"`
	Level            string            `json:"level"`
	Tags             []string          `json:"tags"`
	SpeakerIDs       []string          `json:"speakerIds"`
	Status           string            `json:"status"`
	DecisionActor    string            `json:"decisionActor"`
	DecisionReason   string            `json:"decisionReason"`
	DecisionRule     string            `json:"decisionRule"`
	DecisionTrace    []string          `json:"decisionTrace"`
	DecisionAt       time.Time         `json:"decisionAt"`
	RoutedQueue      string            `json:"routedQueue"`
	RoutedOwner      string            `json:"routedOwner"`
	RuleTrace        []string          `json:"ruleTrace"`
	Answers          map[string]string `json:"answers"`
	WithdrawalReason string            `json:"withdrawalReason"`
	WithdrawnAt      time.Time         `json:"withdrawnAt"`
	SubmittedAt      time.Time         `json:"submittedAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

const (
	SubmissionDraft         = "draft"
	SubmissionPending       = "pending"
	SubmissionAcceptedQueue = "accepted_queue"
	SubmissionAccepted      = "accepted"
	SubmissionDeclineQueue  = "decline_queue"
	SubmissionDeclined      = "declined"
	SubmissionWithdrawn     = "withdrawn"
)

var SubmissionStatuses = []string{
	SubmissionDraft,
	SubmissionPending,
	SubmissionAcceptedQueue,
	SubmissionAccepted,
	SubmissionDeclineQueue,
	SubmissionDeclined,
	SubmissionWithdrawn,
}

type ReviewPlan struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Round              int               `json:"round"`
	Status             string            `json:"status"`
	Instructions       string            `json:"instructions"`
	DueAt              time.Time         `json:"dueAt"`
	Anonymous          bool              `json:"anonymous"`
	WeeklyReminders    bool              `json:"weeklyReminders"`
	IncludeFiles       bool              `json:"includeFiles"`
	ReviewerIDs        []string          `json:"reviewerIds"`
	SubmissionIDs      []string          `json:"submissionIds"`
	Criteria           []RubricCriterion `json:"criteria"`
	EvaluationsPerItem int               `json:"evaluationsPerItem"`
}

type Reviewer struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Company   string   `json:"company"`
	Expertise []string `json:"expertise"`
	Kind      string   `json:"kind"`
}

type RubricCriterion struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
	MaxScore    float64 `json:"maxScore"`
}

type Evaluation struct {
	ID             string             `json:"id"`
	PlanID         string             `json:"planId"`
	SubmissionID   string             `json:"submissionId"`
	ReviewerID     string             `json:"reviewerId"`
	Scores         map[string]float64 `json:"scores"`
	Comments       string             `json:"comments"`
	Recommendation string             `json:"recommendation"`
	Source         string             `json:"source"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

type Session struct {
	ID              string    `json:"id"`
	EventID         string    `json:"eventId"`
	SubmissionID    string    `json:"submissionId"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Format          string    `json:"format"`
	TrackID         string    `json:"trackId"`
	RoomID          string    `json:"roomId"`
	SpeakerIDs      []string  `json:"speakerIds"`
	StartsAt        time.Time `json:"startsAt"`
	EndsAt          time.Time `json:"endsAt"`
	DurationMinutes int       `json:"durationMinutes"`
	Status          string    `json:"status"`
	ExternalID      string    `json:"externalId"`
	LastPublishedAt time.Time `json:"lastPublishedAt"`
}

func (session Session) Scheduled() bool {
	return !session.StartsAt.IsZero() && !session.EndsAt.IsZero()
}

// Duration returns the persisted placement duration when one exists, then a
// manual-session duration, and finally the format-aware program default. A
// zero DurationMinutes remains valid for workspaces created before manual
// session creation was introduced.
func (session Session) Duration() time.Duration {
	if session.Scheduled() && session.EndsAt.After(session.StartsAt) {
		return session.EndsAt.Sub(session.StartsAt)
	}
	if session.DurationMinutes > 0 {
		return time.Duration(session.DurationMinutes) * time.Minute
	}
	return DefaultSessionDuration(session.Format)
}

// DefaultSessionDuration gives unscheduled proposal-derived sessions a
// predictable placement length. Organizers can override it when adding a
// manual session; the chosen length is then preserved in DurationMinutes.
func DefaultSessionDuration(format string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "workshop":
		return 90 * time.Minute
	case "lightning talk", "lightning":
		return 15 * time.Minute
	default:
		return 45 * time.Minute
	}
}

// AddSessionForSubmission appends an unscheduled Session for the submission
// with the given ID, linked by SubmissionID. The transition is idempotent: if
// a session already exists for that submission, AddSessionForSubmission does
// nothing and reports created as false. It also reports created as false if
// no submission with that ID exists.
func (state *State) AddSessionForSubmission(submissionID string) (created bool) {
	for _, existing := range state.Sessions {
		if existing.SubmissionID == submissionID {
			return false
		}
	}
	submission, found := state.Submission(submissionID)
	if !found {
		return false
	}
	state.Sessions = append(state.Sessions, Session{
		ID:              NewID("ses"),
		EventID:         submission.EventID,
		SubmissionID:    submission.ID,
		Title:           submission.Title,
		Description:     submission.Abstract,
		Format:          submission.Format,
		TrackID:         submission.TrackID,
		SpeakerIDs:      append([]string(nil), submission.SpeakerIDs...),
		DurationMinutes: int(DefaultSessionDuration(submission.Format) / time.Minute),
		Status:          "unscheduled",
	})
	return true
}

// AssignAcceptedOnlyTasks appends each speaker in speakerIDs to every Task
// in state.Tasks marked AcceptedOnly, skipping a speaker already listed on
// that task. This is the accept-time counterpart to the task_profile
// assignment submitProposal already does at submit time
// (app/submit/page.server.go). It is idempotent: calling it twice with the
// same speakerIDs assigns nothing the second time. It reports how many
// (task, speaker) assignments it added.
func (state *State) AssignAcceptedOnlyTasks(speakerIDs []string) (assigned int) {
	for index := range state.Tasks {
		task := &state.Tasks[index]
		if !task.AcceptedOnly {
			continue
		}
		for _, speakerID := range speakerIDs {
			if speakerID == "" || contains(task.AssignedSpeakerIDs, speakerID) {
				continue
			}
			task.AssignedSpeakerIDs = append(task.AssignedSpeakerIDs, speakerID)
			assigned++
		}
	}
	return assigned
}

// HasAcceptanceCommunication reports whether a Communication already exists
// for the (AcceptanceTemplateID, speakerID, sessionID) triple --
// QueueAcceptanceCommunication's own idempotency check, exposed so a caller
// that also sends the message (not just queues the row, for example the
// accept-time transition in app/organizer/submissions/page.server.go) can
// tell, before queuing, which speakers are new and so actually need a send.
func (state *State) HasAcceptanceCommunication(sessionID, speakerID string) bool {
	for _, existing := range state.Communications {
		if existing.TemplateID == AcceptanceTemplateID && existing.SpeakerID == speakerID && existing.SessionID == sessionID {
			return true
		}
	}
	return false
}

// Communication finds the durable communication record for one template,
// speaker, and session triple. The copy is suitable for delivery metadata
// such as a provider idempotency key; callers must still use a store Update
// to change its delivery result.
func (state State) Communication(templateID, speakerID, sessionID string) (Communication, bool) {
	for _, item := range state.Communications {
		if item.TemplateID == templateID && item.SpeakerID == speakerID && item.SessionID == sessionID {
			return item, true
		}
	}
	return Communication{}, false
}

// QueueAcceptanceCommunication appends one queued Communication per speaker
// in speakerIDs, addressed to sessionID and using the AcceptanceTemplateID
// template. It is idempotent per (speakerID, sessionID) pair (see
// HasAcceptanceCommunication): if a Communication already exists on that
// template for the pair, it is skipped, so accepting the same submission
// twice never double-queues the acceptance message. It reports how many
// rows it appended.
func (state *State) QueueAcceptanceCommunication(sessionID string, speakerIDs []string) (queued int) {
	for _, speakerID := range speakerIDs {
		if speakerID == "" || state.HasAcceptanceCommunication(sessionID, speakerID) {
			continue
		}
		state.Communications = append(state.Communications, Communication{
			ID:         NewID("comm"),
			TemplateID: AcceptanceTemplateID,
			SpeakerID:  speakerID,
			SessionID:  sessionID,
			Subject:    "You're joining " + state.Event.Name,
			Status:     "queued",
			Provider:   "demo-outbox",
		})
		queued++
	}
	return queued
}

// MarkCommunicationSent finds the queued Communication row addressed to
// speakerID on templateID for sessionID — the row QueueAcceptanceCommunication
// (or an equivalent queuing step) already appended — and records the
// outcome of actually sending it: Status "sent" and Provider on a nil
// sendErr, or Status "failed" with a sanitized Error category otherwise
// (never the raw error text — M8; a caller logs the detail separately).
// SentAt is stamped either way, marking when delivery was attempted. It
// reports whether it found and updated a matching row; a caller that sent
// a message with no matching queued row (for example a race with a second
// accept) should append a fresh Communication row instead of losing the
// outcome.
//
// Call this from a second, small Update — after the Send call has
// returned, outside any store lock — the same two-step shape submitProposal
// and queueMessage use to record a real send's outcome.
func (state *State) MarkCommunicationSent(templateID, speakerID, sessionID, provider string, sendErr error) bool {
	for index := range state.Communications {
		item := &state.Communications[index]
		if item.TemplateID != templateID || item.SpeakerID != speakerID || item.SessionID != sessionID || item.Status != "queued" {
			continue
		}
		item.Provider = provider
		item.SentAt = time.Now().UTC()
		if sendErr != nil {
			item.Status = "failed"
			item.Error = "delivery failed"
		} else {
			item.Status = "sent"
			item.Error = ""
		}
		return true
	}
	return false
}

type Task struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Type               string      `json:"type"`
	Required           bool        `json:"required"`
	DueAt              time.Time   `json:"dueAt"`
	AssignedSpeakerIDs []string    `json:"assignedSpeakerIds"`
	FormFields         []FormField `json:"formFields"`
	AcceptedOnly       bool        `json:"acceptedOnly"`
}

type TaskCompletion struct {
	ID          string            `json:"id"`
	TaskID      string            `json:"taskId"`
	SpeakerID   string            `json:"speakerId"`
	Status      string            `json:"status"`
	Values      map[string]string `json:"values"`
	FileName    string            `json:"fileName"`
	ContentType string            `json:"contentType"`
	StoredPath  string            `json:"storedPath"`
	CompletedAt time.Time         `json:"completedAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

const (
	TaskOutstanding = "outstanding"
	TaskSubmitted   = "submitted"
	TaskApproved    = "approved"
	TaskDeclined    = "declined"
)

type Resource struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	EmbedURL  string `json:"embedUrl"`
	SortOrder int    `json:"sortOrder"`
}

type EmailTemplate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Audience       string `json:"audience"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	ReplyTo        string `json:"replyTo"`
	AttachCalendar bool   `json:"attachCalendar"`
	System         bool   `json:"system"`
}

// EmailTemplateRevision is an immutable copy of a template's content at a
// successful organizer edit. Keeping revisions in canonical state lets a
// historical Communication retain its stable TemplateID while operators can
// still inspect the exact prior wording that was approved for use.
type EmailTemplateRevision struct {
	ID             string    `json:"id"`
	TemplateID     string    `json:"templateId"`
	Revision       int       `json:"revision"`
	Name           string    `json:"name"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	ReplyTo        string    `json:"replyTo"`
	AttachCalendar bool      `json:"attachCalendar"`
	Actor          string    `json:"actor"`
	CreatedAt      time.Time `json:"createdAt"`
}

// NotificationRule describes an administrator-facing event trigger. Its
// recipients are direct operational addresses rather than Speaker records,
// so administrative notification delivery can remain wholly independent of
// the public speaker workflow.
type NotificationRule struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Trigger         string   `json:"trigger"`
	TemplateID      string   `json:"templateId"`
	RecipientEmails []string `json:"recipientEmails"`
	Enabled         bool     `json:"enabled"`
	RetryLimit      int      `json:"retryLimit"`
	SuppressMinutes int      `json:"suppressMinutes"`
}

type Communication struct {
	ID                 string    `json:"id"`
	TemplateID         string    `json:"templateId"`
	SpeakerID          string    `json:"speakerId"`
	SessionID          string    `json:"sessionId"`
	SubmissionID       string    `json:"submissionId"`
	TaskID             string    `json:"taskId"`
	NotificationRuleID string    `json:"notificationRuleId"`
	RecipientEmail     string    `json:"recipientEmail"`
	RecipientName      string    `json:"recipientName"`
	Subject            string    `json:"subject"`
	Status             string    `json:"status"`
	Provider           string    `json:"provider"`
	DeliveryMode       string    `json:"deliveryMode"`
	Trigger            string    `json:"trigger"`
	IdempotencyKey     string    `json:"idempotencyKey"`
	ScheduledFor       time.Time `json:"scheduledFor"`
	NextAttemptAt      time.Time `json:"nextAttemptAt"`
	LeaseUntil         time.Time `json:"leaseUntil"`
	LastAttemptAt      time.Time `json:"lastAttemptAt"`
	SentAt             time.Time `json:"sentAt"`
	CancelledAt        time.Time `json:"cancelledAt"`
	SuppressedAt       time.Time `json:"suppressedAt"`
	CreatedAt          time.Time `json:"createdAt"`
	AttemptCount       int       `json:"attemptCount"`
	MaxAttempts        int       `json:"maxAttempts"`
	Error              string    `json:"error"`
}

const (
	CommunicationQueued     = "queued"
	CommunicationScheduled  = "scheduled"
	CommunicationSending    = "sending"
	CommunicationRetrying   = "retrying"
	CommunicationSent       = "sent"
	CommunicationFailed     = "failed"
	CommunicationCancelled  = "cancelled"
	CommunicationSuppressed = "suppressed"

	DeliveryAutomatic = "automatic"
	DeliveryHandoff   = "handoff"
)

// AuditMeta is the small, secret-free description a mutating operation
// supplies to the durable store. The store appends an AuditEvent in the same
// transaction as the state mutation, so an audit row can never claim a
// change that failed to persist (or omit a change that succeeded).
//
// Values intentionally describe an action rather than retain request bodies,
// tokens, message contents, or uploaded bytes. Archives may be kept for a
// long time and must never become a second source of credentials or PII.
type AuditMeta struct {
	Actor      string
	Action     string
	EntityType string
	EntityID   string
	Summary    string
	Origin     string
	Rule       string
	Trace      string
	At         time.Time
}

// AuditEvent is an append-only, hash-chained mutation record. PreviousHash
// and Hash make accidental or manual edits to a restored archive detectable;
// they are an integrity aid, not a substitute for a signed external ledger.
type AuditEvent struct {
	ID           string    `json:"id"`
	At           time.Time `json:"at"`
	Actor        string    `json:"actor"`
	Action       string    `json:"action"`
	EntityType   string    `json:"entityType"`
	EntityID     string    `json:"entityId"`
	Summary      string    `json:"summary"`
	Origin       string    `json:"origin"`
	Rule         string    `json:"rule"`
	Trace        string    `json:"trace"`
	PreviousHash string    `json:"previousHash"`
	Hash         string    `json:"hash"`
}

// SyncOutboxItem is a durable, idempotent external-projection request. The
// payload stays in the canonical state and is delivered only by an explicit
// connector run; an integration can therefore fail or be disabled without
// making Airtable (or any future projection) the source of truth.
type SyncOutboxItem struct {
	ID             string    `json:"id"`
	Integration    string    `json:"integration"`
	Kind           string    `json:"kind"`
	EntityID       string    `json:"entityId"`
	Payload        string    `json:"payload"`
	IdempotencyKey string    `json:"idempotencyKey"`
	Attempts       int       `json:"attempts"`
	AvailableAt    time.Time `json:"availableAt"`
	DeliveredAt    time.Time `json:"deliveredAt"`
	LastError      string    `json:"lastError"`
	CreatedAt      time.Time `json:"createdAt"`
}

// AppendAudit appends one hash-chained record. It is called only by a
// StateStore after a mutation closure succeeds, while the same state copy is
// still private to that transaction.
func (state *State) AppendAudit(meta AuditMeta) {
	at := meta.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if strings.TrimSpace(meta.Action) == "" {
		meta.Action = "state.updated"
	}
	previous := ""
	if count := len(state.AuditEvents); count > 0 {
		previous = state.AuditEvents[count-1].Hash
	}
	event := AuditEvent{
		ID:           NewID("audit"),
		At:           at,
		Actor:        auditText(meta.Actor, 160),
		Action:       auditText(meta.Action, 120),
		EntityType:   auditText(meta.EntityType, 120),
		EntityID:     auditText(meta.EntityID, 160),
		Summary:      auditText(meta.Summary, 500),
		Origin:       auditText(meta.Origin, 120),
		Rule:         auditText(meta.Rule, 160),
		Trace:        auditText(meta.Trace, 1_000),
		PreviousHash: previous,
	}
	event.Hash = event.chainHash()
	state.AuditEvents = append(state.AuditEvents, event)
}

// VerifyAuditTrail validates the hash chain stored in the snapshot. An empty
// trail is valid for pre-audit workspaces and is upgraded on their first
// mutation; this keeps the schema migration backward-compatible.
func (state State) VerifyAuditTrail() error {
	previous := ""
	for index, event := range state.AuditEvents {
		if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Action) == "" || event.At.IsZero() {
			return fmt.Errorf("audit event %d is incomplete", index)
		}
		if event.PreviousHash != previous {
			return fmt.Errorf("audit event %s has an invalid previous hash", event.ID)
		}
		if event.Hash != event.chainHash() {
			return fmt.Errorf("audit event %s has an invalid hash", event.ID)
		}
		previous = event.Hash
	}
	return nil
}

func (event AuditEvent) chainHash() string {
	parts := []string{
		event.ID,
		event.At.UTC().Format(time.RFC3339Nano),
		event.Actor,
		event.Action,
		event.EntityType,
		event.EntityID,
		event.Summary,
		event.Origin,
	}
	// Version-one audit hashes ended after Origin and PreviousHash. Retain
	// that byte-for-byte shape while Rule and Trace are both empty, so a
	// workspace written before governed decision metadata was introduced still
	// verifies on upgrade. New governed records bind both fields into the hash.
	if event.Rule != "" || event.Trace != "" {
		parts = append(parts, event.Rule, event.Trace)
	}
	parts = append(parts, event.PreviousHash)
	payload := strings.Join(parts, "\x1f")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func auditText(value string, limit int) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	if len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// AcceptanceTemplateID names the seeded EmailTemplate ("tpl_acceptance",
// internal/domain/seed.go) that QueueAcceptanceCommunication and the
// accept-time transition in app/organizer/submissions/page.server.go send
// an accepted speaker.
const AcceptanceTemplateID = "tpl_acceptance"

type Integration struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	EventURL      string    `json:"eventUrl"`
	CredentialsOK bool      `json:"credentialsOk"`
	LastSyncAt    time.Time `json:"lastSyncAt"`
	LastStatus    string    `json:"lastStatus"`
}

type SyncRun struct {
	ID          string    `json:"id"`
	Integration string    `json:"integration"`
	Mode        string    `json:"mode"`
	Status      string    `json:"status"`
	Speakers    int       `json:"speakers"`
	Sessions    int       `json:"sessions"`
	StartedAt   time.Time `json:"startedAt"`
	FinishedAt  time.Time `json:"finishedAt"`
	Summary     string    `json:"summary"`
}

func (state *State) Form(id string) (*SubmissionForm, bool) {
	for index := range state.Forms {
		if state.Forms[index].ID == id || state.Forms[index].Slug == id {
			return &state.Forms[index], true
		}
	}
	return nil, false
}

func (state *State) Speaker(id string) (*Speaker, bool) {
	for index := range state.Speakers {
		if state.Speakers[index].ID == id {
			return &state.Speakers[index], true
		}
	}
	return nil, false
}

func (state *State) Submission(id string) (*Submission, bool) {
	for index := range state.Submissions {
		if state.Submissions[index].ID == id {
			return &state.Submissions[index], true
		}
	}
	return nil, false
}

func (state *State) Session(id string) (*Session, bool) {
	for index := range state.Sessions {
		if state.Sessions[index].ID == id {
			return &state.Sessions[index], true
		}
	}
	return nil, false
}

// SessionBySubmission finds the Session linked to submissionID, the same
// link AddSessionForSubmission establishes at accept time. It returns false
// when the submission has no session yet.
func (state *State) SessionBySubmission(submissionID string) (*Session, bool) {
	for index := range state.Sessions {
		if state.Sessions[index].SubmissionID == submissionID {
			return &state.Sessions[index], true
		}
	}
	return nil, false
}

func (state *State) ReviewPlan(id string) (*ReviewPlan, bool) {
	for index := range state.ReviewPlans {
		if state.ReviewPlans[index].ID == id {
			return &state.ReviewPlans[index], true
		}
	}
	return nil, false
}

func (state *State) Task(id string) (*Task, bool) {
	for index := range state.Tasks {
		if state.Tasks[index].ID == id {
			return &state.Tasks[index], true
		}
	}
	return nil, false
}

func (state *State) Completion(taskID, speakerID string) (*TaskCompletion, bool) {
	for index := range state.TaskCompletions {
		completion := &state.TaskCompletions[index]
		if completion.TaskID == taskID && completion.SpeakerID == speakerID {
			return completion, true
		}
	}
	return nil, false
}

func (state State) Track(id string) (Track, bool) {
	for _, track := range state.Event.Tracks {
		if track.ID == id {
			return track, true
		}
	}
	return Track{}, false
}

func (state State) Room(id string) (Room, bool) {
	for _, room := range state.Event.Rooms {
		if room.ID == id {
			return room, true
		}
	}
	return Room{}, false
}

func (state State) Category(id string) (Category, bool) {
	for _, category := range state.Event.Categories {
		if category.ID == id {
			return category, true
		}
	}
	return Category{}, false
}

func (state State) SpeakerTasks(speakerID string) []Task {
	tasks := make([]Task, 0)
	for _, task := range state.Tasks {
		if contains(task.AssignedSpeakerIDs, speakerID) {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].DueAt.Before(tasks[j].DueAt) })
	return tasks
}

func (state State) OutstandingTaskCount(speakerID string) int {
	count := 0
	for _, task := range state.SpeakerTasks(speakerID) {
		completion, found := state.completionValue(task.ID, speakerID)
		if !found || completion.Status == TaskOutstanding || completion.Status == TaskDeclined {
			count++
		}
	}
	return count
}

func (state State) completionValue(taskID, speakerID string) (TaskCompletion, bool) {
	for _, completion := range state.TaskCompletions {
		if completion.TaskID == taskID && completion.SpeakerID == speakerID {
			return completion, true
		}
	}
	return TaskCompletion{}, false
}

func (state State) Validate() error {
	if state.Event.ID == "" || state.Event.Name == "" || state.Event.Slug == "" {
		return errors.New("event id, name, and slug are required")
	}
	if !state.Event.EndsAt.After(state.Event.StartsAt) {
		return errors.New("event end must be after start")
	}
	if err := validateUniqueIDs(state); err != nil {
		return err
	}
	if err := state.VerifyAuditTrail(); err != nil {
		return err
	}
	if err := validateSubmissionForms(state); err != nil {
		return err
	}
	if err := validateCommunications(state); err != nil {
		return err
	}
	for _, submission := range state.Submissions {
		if !submissionStatusKnown(submission.Status) {
			return fmt.Errorf("submission %s has an unknown status %q", submission.ID, submission.Status)
		}
		if submission.Status == SubmissionWithdrawn && submission.WithdrawnAt.IsZero() {
			return fmt.Errorf("withdrawn submission %s has no withdrawal timestamp", submission.ID)
		}
	}
	for _, session := range state.Sessions {
		if session.Scheduled() && !session.EndsAt.After(session.StartsAt) {
			return fmt.Errorf("session %s ends before it starts", session.ID)
		}
		if session.DurationMinutes < 0 || session.DurationMinutes > 12*60 {
			return fmt.Errorf("session %s has an invalid duration", session.ID)
		}
	}
	for _, plan := range state.ReviewPlans {
		if len(plan.Criteria) == 0 {
			return fmt.Errorf("review plan %s has no rubric criteria", plan.ID)
		}
		weight := 0.0
		for _, criterion := range plan.Criteria {
			weight += criterion.Weight
		}
		if weight < 99.99 || weight > 100.01 {
			return fmt.Errorf("review plan %s rubric weight is %.2f, want 100", plan.ID, weight)
		}
	}
	return nil
}

func validateSubmissionForms(state State) error {
	slugs := map[string]string{}
	for _, form := range state.Forms {
		if strings.TrimSpace(form.EventID) == "" || strings.TrimSpace(form.Name) == "" || strings.TrimSpace(form.Slug) == "" {
			return fmt.Errorf("submission form %s is incomplete", form.ID)
		}
		if previous, exists := slugs[form.Slug]; exists {
			return fmt.Errorf("submission forms %s and %s share slug %s", previous, form.ID, form.Slug)
		}
		slugs[form.Slug] = form.ID
		if form.MaxDraftsPerSubmitter < 0 || form.MaxDraftsPerSubmitter > 10 {
			return fmt.Errorf("submission form %s has an invalid draft limit", form.ID)
		}
		fields := map[string]FormField{}
		for _, field := range form.Fields {
			if strings.TrimSpace(field.ID) == "" || strings.TrimSpace(field.Section) == "" || strings.TrimSpace(field.Type) == "" {
				return fmt.Errorf("submission form %s has an incomplete field", form.ID)
			}
			if _, exists := fields[field.ID]; exists {
				return fmt.Errorf("submission form %s has duplicate field %s", form.ID, field.ID)
			}
			fields[field.ID] = field
		}
		targets := map[string]bool{}
		sources := map[string]bool{}
		for _, rule := range form.QuestionRules {
			source, sourceOK := fields[rule.SourceFieldID]
			target, targetOK := fields[rule.TargetFieldID]
			if strings.TrimSpace(rule.ID) == "" || !sourceOK || !targetOK {
				return fmt.Errorf("submission form %s has a rule with an unknown field", form.ID)
			}
			if rule.SourceFieldID == rule.TargetFieldID || rule.Operator != "equals" || rule.Effect != "show" || strings.TrimSpace(rule.Value) == "" {
				return fmt.Errorf("submission form %s has an unsupported question rule %s", form.ID, rule.ID)
			}
			if source.Section != target.Section || target.Locked {
				return fmt.Errorf("submission form %s has an invalid conditional target %s", form.ID, target.ID)
			}
			if targets[target.ID] || sources[target.ID] || targets[source.ID] {
				return fmt.Errorf("submission form %s has a chained or duplicate conditional target %s", form.ID, target.ID)
			}
			targets[target.ID] = true
			sources[source.ID] = true
		}
	}
	return nil
}

func validateCommunications(state State) error {
	templates := map[string]bool{}
	for _, template := range state.EmailTemplates {
		if strings.TrimSpace(template.ID) == "" || strings.TrimSpace(template.Name) == "" || strings.TrimSpace(template.Audience) == "" {
			return fmt.Errorf("email template %s is incomplete", template.ID)
		}
		templates[template.ID] = true
	}
	keys := map[string]string{}
	for _, item := range state.Communications {
		if strings.TrimSpace(item.TemplateID) == "" || !templates[item.TemplateID] {
			return fmt.Errorf("communication %s has an unknown template %s", item.ID, item.TemplateID)
		}
		if strings.TrimSpace(item.SpeakerID) == "" && strings.TrimSpace(item.RecipientEmail) == "" {
			return fmt.Errorf("communication %s has no recipient", item.ID)
		}
		if !communicationStatusKnown(item.Status) {
			return fmt.Errorf("communication %s has an unknown status %q", item.ID, item.Status)
		}
		if item.DeliveryMode != "" && item.DeliveryMode != DeliveryAutomatic && item.DeliveryMode != DeliveryHandoff {
			return fmt.Errorf("communication %s has an unknown delivery mode %q", item.ID, item.DeliveryMode)
		}
		if key := strings.TrimSpace(item.IdempotencyKey); key != "" {
			if previous, exists := keys[key]; exists {
				return fmt.Errorf("communications %s and %s share idempotency key %s", previous, item.ID, key)
			}
			keys[key] = item.ID
		}
	}
	for _, rule := range state.NotificationRules {
		if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Name) == "" || strings.TrimSpace(rule.Trigger) == "" || !templates[rule.TemplateID] {
			return fmt.Errorf("notification rule %s is incomplete or references an unknown template", rule.ID)
		}
		if rule.RetryLimit < 0 || rule.RetryLimit > 10 || rule.SuppressMinutes < 0 || rule.SuppressMinutes > 60*24*365 {
			return fmt.Errorf("notification rule %s has invalid delivery limits", rule.ID)
		}
		if rule.Enabled && len(rule.RecipientEmails) == 0 {
			return fmt.Errorf("enabled notification rule %s has no recipients", rule.ID)
		}
		for _, recipient := range rule.RecipientEmails {
			if !strings.Contains(strings.TrimSpace(recipient), "@") {
				return fmt.Errorf("notification rule %s has an invalid recipient", rule.ID)
			}
		}
	}
	return nil
}

func communicationStatusKnown(status string) bool {
	switch status {
	case CommunicationQueued, CommunicationScheduled, CommunicationSending, CommunicationRetrying,
		CommunicationSent, CommunicationFailed, CommunicationCancelled, CommunicationSuppressed:
		return true
	default:
		return false
	}
}

func submissionStatusKnown(status string) bool {
	for _, candidate := range SubmissionStatuses {
		if candidate == status {
			return true
		}
	}
	return false
}

func validateUniqueIDs(state State) error {
	seen := map[string]string{}
	groups := []struct {
		name string
		ids  []string
	}{
		{"form", collectIDs(len(state.Forms), func(i int) string { return state.Forms[i].ID })},
		{"speaker", collectIDs(len(state.Speakers), func(i int) string { return state.Speakers[i].ID })},
		{"submission", collectIDs(len(state.Submissions), func(i int) string { return state.Submissions[i].ID })},
		{"reviewer", collectIDs(len(state.Reviewers), func(i int) string { return state.Reviewers[i].ID })},
		{"review plan", collectIDs(len(state.ReviewPlans), func(i int) string { return state.ReviewPlans[i].ID })},
		{"evaluation", collectIDs(len(state.Evaluations), func(i int) string { return state.Evaluations[i].ID })},
		{"session", collectIDs(len(state.Sessions), func(i int) string { return state.Sessions[i].ID })},
		{"task", collectIDs(len(state.Tasks), func(i int) string { return state.Tasks[i].ID })},
		{"task completion", collectIDs(len(state.TaskCompletions), func(i int) string { return state.TaskCompletions[i].ID })},
		{"resource", collectIDs(len(state.Resources), func(i int) string { return state.Resources[i].ID })},
		{"email template", collectIDs(len(state.EmailTemplates), func(i int) string { return state.EmailTemplates[i].ID })},
		{"template revision", collectIDs(len(state.TemplateRevisions), func(i int) string { return state.TemplateRevisions[i].ID })},
		{"notification rule", collectIDs(len(state.NotificationRules), func(i int) string { return state.NotificationRules[i].ID })},
		{"communication", collectIDs(len(state.Communications), func(i int) string { return state.Communications[i].ID })},
		{"audit event", collectIDs(len(state.AuditEvents), func(i int) string { return state.AuditEvents[i].ID })},
		{"sync outbox item", collectIDs(len(state.SyncOutbox), func(i int) string { return state.SyncOutbox[i].ID })},
	}
	for _, group := range groups {
		for _, id := range group.ids {
			if id == "" {
				return fmt.Errorf("%s has an empty id", group.name)
			}
			if previous, exists := seen[id]; exists {
				return fmt.Errorf("id %s is shared by %s and %s", id, previous, group.name)
			}
			seen[id] = group.name
		}
	}
	return nil
}

func collectIDs(count int, value func(int) string) []string {
	ids := make([]string, count)
	for index := 0; index < count; index++ {
		ids[index] = value(index)
	}
	return ids
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstRune(value string) string {
	for _, character := range strings.TrimSpace(value) {
		return string(character)
	}
	return ""
}

// Principal is one identity the organizer identity plane recognizes: an
// email address, the roles it carries, and when it first and last signed in.
// internal/identity grants RoleOrganizer to a Principal either through the
// ORGANIZER_EMAILS allowlist or through the break-glass setup flow; a
// magic-link or OAuth sign-in then upserts this record so the granted role
// survives a restart.
type Principal struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	Roles      []string  `json:"roles"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

// AuthMagicLink is one issued, not-yet-consumed magic-link sign-in token.
// Token holds the SHA-256 hex digest of the token, never the token itself,
// so a leaked backup or log line never hands out a working sign-in link.
// UserJSON carries the auth.User the token signs in on consumption, encoded
// as JSON so this package does not import the auth package.
type AuthMagicLink struct {
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	UserJSON  string    `json:"userJson"`
	Next      string    `json:"next,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// AuthPasskey is one registered WebAuthn credential. PublicKey and the
// signature counter are exactly what the WebAuthn ceremony needs to verify
// the next assertion; UserJSON carries the auth.User the credential signs
// in, encoded as JSON for the same reason AuthMagicLink.UserJSON is.
type AuthPasskey struct {
	ID         string    `json:"id"`
	UserJSON   string    `json:"userJson"`
	PublicKey  []byte    `json:"publicKey"`
	Algorithm  int       `json:"algorithm"`
	SignCount  uint32    `json:"signCount"`
	Transports []string  `json:"transports,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}
