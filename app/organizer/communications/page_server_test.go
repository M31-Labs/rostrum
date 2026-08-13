package communications

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

// testWorkspaceState builds the minimum valid domain.State a queueMessage
// test needs: one speaker, one merge-field-bearing template, and no other
// entities, so domain.State.Validate accepts it without a full seed.
func testWorkspaceState() domain.State {
	now := time.Now().UTC()
	return domain.State{
		Event: domain.Event{
			ID: "evt_test", Name: "Test Forum", Slug: "test-forum",
			StartsAt: now, EndsAt: now.Add(48 * time.Hour),
		},
		Speakers: []domain.Speaker{
			{ID: "spk_ada", FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com"},
		},
		EmailTemplates: []domain.EmailTemplate{
			{
				ID: "tpl_test", Name: "Test template", Audience: "speaker",
				Subject: "Hello {{speaker.first_name}}", Body: "Hi {{speaker.first_name}}, welcome.",
				ReplyTo: "program@example.com",
			},
		},
	}
}

// TestQueueMessageSendsAndRecordsOutcome proves the local outbox path
// actually records -- through messageSender (mail.FromEnv), the network-free
// OutboxSender here since the test process sets no SMTP_HOST -- and
// records the real outcome on the appended Communication row: a "sent"
// status and a stamped SentAt, not the old queue-only bookkeeping.
func TestQueueMessageSendsAndRecordsOutcome(t *testing.T) {
	workspace, err := store.Open(":memory:", testWorkspaceState())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	appstate.Set(workspace)

	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/communications/__actions/queueMessage", nil),
		FormData: map[string]string{
			"template_id": "tpl_test",
			"speaker_id":  "spk_ada",
			"provider":    "outbox",
		},
	}

	if err := queueMessage(ctx); err != nil {
		t.Fatalf("queueMessage returned an error: %v", err)
	}

	snapshot := appstate.MustGet().Snapshot()
	if len(snapshot.Communications) != 1 {
		t.Fatalf("communications = %d, want 1", len(snapshot.Communications))
	}
	comm := snapshot.Communications[0]
	if comm.Status != "sent" {
		t.Fatalf("comm.Status = %q, want sent", comm.Status)
	}
	if comm.SentAt.IsZero() {
		t.Fatal("comm.SentAt is zero, want a stamped send time")
	}
	if comm.Subject != "Hello Ada" {
		t.Fatalf("comm.Subject = %q, want the merged subject %q", comm.Subject, "Hello Ada")
	}
	if comm.Error != "" {
		t.Fatalf("comm.Error = %q, want empty on a successful send", comm.Error)
	}

	sender := messageSender()
	outbox, ok := sender.(*mail.OutboxSender)
	if !ok {
		t.Fatalf("messageSender() = %T, want *mail.OutboxSender (the test sets no SMTP_HOST)", sender)
	}
	if comm.Provider != outbox.Name() {
		t.Fatalf("comm.Provider = %q, want the sender's own name %q", comm.Provider, outbox.Name())
	}
	sent := outbox.Sent()
	if len(sent) == 0 {
		t.Fatal("the outbox recorded no messages; queueMessage never called Send")
	}
	last := sent[len(sent)-1]
	if last.To != "ada@example.com" {
		t.Fatalf("last recorded message To = %q, want ada@example.com", last.To)
	}
	if last.Subject != "Hello Ada" {
		t.Fatalf("last recorded message Subject = %q, want the merged subject", last.Subject)
	}
}

// TestQueueMessageQueuesWithoutSendingForHandOffProviders proves the
// gmail/outlook providers keep the pre-existing queue-only behavior: no
// Send call, and a "queued" row an organizer finishes delivering through
// this page's deep-link compose buttons.
func TestQueueMessageQueuesWithoutSendingForHandOffProviders(t *testing.T) {
	workspace, err := store.Open(":memory:", testWorkspaceState())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	appstate.Set(workspace)

	before := 0
	if sender, ok := messageSender().(*mail.OutboxSender); ok {
		before = len(sender.Sent())
	}

	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/communications/__actions/queueMessage", nil),
		FormData: map[string]string{
			"template_id": "tpl_test",
			"speaker_id":  "spk_ada",
			"provider":    "gmail",
		},
	}
	if err := queueMessage(ctx); err != nil {
		t.Fatalf("queueMessage returned an error: %v", err)
	}

	snapshot := appstate.MustGet().Snapshot()
	if len(snapshot.Communications) != 1 {
		t.Fatalf("communications = %d, want 1", len(snapshot.Communications))
	}
	comm := snapshot.Communications[0]
	if comm.Status != "queued" {
		t.Fatalf("comm.Status = %q, want queued for a hand-off provider", comm.Status)
	}
	if !comm.SentAt.IsZero() {
		t.Fatalf("comm.SentAt = %v, want zero for a queued (not sent) row", comm.SentAt)
	}

	if sender, ok := messageSender().(*mail.OutboxSender); ok {
		if after := len(sender.Sent()); after != before {
			t.Fatalf("outbox recorded %d new messages for provider=gmail, want 0 (no automated send)", after-before)
		}
	}
}

func TestCreateTemplateStoresInitialRevisionAndAudit(t *testing.T) {
	workspace, err := store.Open(":memory:", testWorkspaceState())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	appstate.Set(workspace)
	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/communications/__actions/createTemplate", nil),
		FormData: map[string]string{
			"name":     "Slides reminder",
			"audience": "speaker",
			"subject":  "Slides due {{task.due_date}}",
			"body":     "Hi {{speaker.first_name}}, please complete {{task.title}}.",
			"reply_to": "program@example.com",
		},
	}
	if err := createTemplate(ctx); err != nil {
		t.Fatalf("createTemplate: %v", err)
	}
	snapshot := workspace.Snapshot()
	template, found := emailTemplate(snapshot, "tpl_slides-reminder")
	if !found || template.System || template.Subject != "Slides due {{task.due_date}}" {
		t.Fatalf("created template = %#v", template)
	}
	if len(snapshot.TemplateRevisions) != 1 || snapshot.TemplateRevisions[0].TemplateID != template.ID || snapshot.TemplateRevisions[0].Revision != 1 {
		t.Fatalf("template revisions = %#v", snapshot.TemplateRevisions)
	}
	audit := snapshot.AuditEvents[len(snapshot.AuditEvents)-1]
	if audit.Action != "communication.template_created" || audit.EntityID != template.ID {
		t.Fatalf("template audit = %#v", audit)
	}
}

func TestSaveNotificationRuleRequiresAdministratorTemplate(t *testing.T) {
	state := testWorkspaceState()
	state.EmailTemplates = append(state.EmailTemplates, domain.EmailTemplate{
		ID: "tpl_admin", Name: "Admin", Audience: "administrator", Subject: "New {{submission.title}}", Body: "{{speaker.name}} sent a proposal.", ReplyTo: "program@example.com",
	})
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	appstate.Set(workspace)
	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/communications/__actions/saveNotificationRule", nil),
		FormData: map[string]string{
			"name":             "Submission alert",
			"trigger":          "submission.created",
			"template_id":      "tpl_admin",
			"recipients":       "program@example.com, ops@example.com",
			"retry_limit":      "3",
			"suppress_minutes": "15",
			"enabled":          "on",
		},
	}
	if err := saveNotificationRule(ctx); err != nil {
		t.Fatalf("saveNotificationRule: %v", err)
	}
	snapshot := workspace.Snapshot()
	if len(snapshot.NotificationRules) != 1 {
		t.Fatalf("notification rules = %#v", snapshot.NotificationRules)
	}
	rule := snapshot.NotificationRules[0]
	if !rule.Enabled || rule.TemplateID != "tpl_admin" || len(rule.RecipientEmails) != 2 || rule.RetryLimit != 3 || rule.SuppressMinutes != 15 {
		t.Fatalf("notification rule = %#v", rule)
	}
	if snapshot.AuditEvents[len(snapshot.AuditEvents)-1].Action != "communication.notification_rule_saved" {
		t.Fatalf("rule audit = %#v", snapshot.AuditEvents[len(snapshot.AuditEvents)-1])
	}
}
