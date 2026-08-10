package submissions

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

// testWorkspaceState builds the minimum valid domain.State an
// updateStatus/accept test needs: one speaker, one submission with a
// scheduled session already attached to it, and the seeded-shape
// acceptance template (AttachCalendar set), so domain.State.Validate
// accepts it without a full seed.
func testWorkspaceState() domain.State {
	now := time.Now().UTC()
	starts := now.Add(72 * time.Hour)
	return domain.State{
		Event: domain.Event{
			ID: "evt_test", Name: "Test Forum", Slug: "test-forum",
			StartsAt: now, EndsAt: now.Add(96 * time.Hour),
		},
		Speakers: []domain.Speaker{
			{ID: "spk_ada", FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com"},
		},
		Submissions: []domain.Submission{
			{
				ID: "sub_ada", EventID: "evt_test", Title: "Engines and Analysis",
				SpeakerIDs: []string{"spk_ada"}, Status: domain.SubmissionPending,
			},
		},
		Sessions: []domain.Session{
			{
				ID: "ses_ada", EventID: "evt_test", SubmissionID: "sub_ada", Title: "Engines and Analysis",
				SpeakerIDs: []string{"spk_ada"}, StartsAt: starts, EndsAt: starts.Add(45 * time.Minute), Status: "unscheduled",
			},
		},
		EmailTemplates: []domain.EmailTemplate{
			{
				ID: domain.AcceptanceTemplateID, Name: "Acceptance", Audience: "speaker",
				Subject: "You're joining {{event.name}}", Body: "Hi {{speaker.first_name}}, welcome to {{event.name}}.",
				ReplyTo: "program@example.com", AttachCalendar: true, System: true,
			},
		},
	}
}

// TestAcceptingASubmissionSendsAnAcceptanceInviteWithCalendar proves
// accepting a submission does more than queue a Communication row: it
// actually sends the acceptance message -- through the demo OutboxSender
// here, since the test process sets no SMTP_HOST -- with the session's
// calendar invite attached as the message's Calendar field, and records
// the real outcome ("sent", a stamped SentAt) back onto that row.
func TestAcceptingASubmissionSendsAnAcceptanceInviteWithCalendar(t *testing.T) {
	workspace, err := store.Open(":memory:", testWorkspaceState())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	appstate.Set(workspace)

	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/submissions/__actions/updateStatus", nil),
		FormData: map[string]string{
			"submission_id": "sub_ada",
			"status":        domain.SubmissionAccepted,
		},
	}
	if err := updateStatus(ctx); err != nil {
		t.Fatalf("updateStatus returned an error: %v", err)
	}

	snapshot := appstate.MustGet().Snapshot()
	if len(snapshot.Communications) != 1 {
		t.Fatalf("communications = %d, want 1", len(snapshot.Communications))
	}
	comm := snapshot.Communications[0]
	if comm.TemplateID != domain.AcceptanceTemplateID {
		t.Fatalf("comm.TemplateID = %q, want %q", comm.TemplateID, domain.AcceptanceTemplateID)
	}
	if comm.Status != "sent" {
		t.Fatalf("comm.Status = %q, want sent", comm.Status)
	}
	if comm.SentAt.IsZero() {
		t.Fatal("comm.SentAt is zero, want a stamped send time")
	}

	sender := acceptanceSender()
	outbox, ok := sender.(*mail.OutboxSender)
	if !ok {
		t.Fatalf("acceptanceSender() = %T, want *mail.OutboxSender (the test sets no SMTP_HOST)", sender)
	}
	sent := outbox.Sent()
	if len(sent) == 0 {
		t.Fatal("the outbox recorded no messages; the acceptance flow never called Send")
	}
	last := sent[len(sent)-1]
	if last.To != "ada@example.com" {
		t.Fatalf("last recorded message To = %q, want ada@example.com", last.To)
	}
	if len(last.Calendar) == 0 {
		t.Fatal("last recorded message carries no Calendar payload, want the session's RFC 5545 invite")
	}
	if !strings.Contains(string(last.Calendar), "BEGIN:VCALENDAR") {
		t.Fatalf("Calendar payload does not look like an RFC 5545 invite:\n%s", last.Calendar)
	}
	if !strings.Contains(string(last.Calendar), "METHOD:REQUEST") {
		t.Fatalf("Calendar payload is not a METHOD:REQUEST invite:\n%s", last.Calendar)
	}
}

// TestReacceptingASubmissionDoesNotResendTheAcceptanceInvite proves
// updateStatus's real-send path stays idempotent per (speaker, session):
// re-running the accept transition on an already-accepted submission (a
// no-op status change) queues no second Communication row and calls Send
// no further times, matching QueueAcceptanceCommunication's own
// idempotency guarantee.
func TestReacceptingASubmissionDoesNotResendTheAcceptanceInvite(t *testing.T) {
	workspace, err := store.Open(":memory:", testWorkspaceState())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	appstate.Set(workspace)

	// acceptanceSender is a package-level singleton shared across every
	// test in this binary (sync.OnceValue), so a prior test's sends are
	// already sitting in its outbox; measure the delta this test's own
	// accepts add, not the outbox's absolute length.
	sender := acceptanceSender()
	outbox, ok := sender.(*mail.OutboxSender)
	if !ok {
		t.Fatalf("acceptanceSender() = %T, want *mail.OutboxSender (the test sets no SMTP_HOST)", sender)
	}
	before := len(outbox.Sent())

	accept := func() {
		ctx := &action.Context{
			Request: httptest.NewRequest(http.MethodPost, "/organizer/submissions/__actions/updateStatus", nil),
			FormData: map[string]string{
				"submission_id": "sub_ada",
				"status":        domain.SubmissionAccepted,
			},
		}
		if err := updateStatus(ctx); err != nil {
			t.Fatalf("updateStatus returned an error: %v", err)
		}
	}

	accept()
	accept()

	snapshot := appstate.MustGet().Snapshot()
	if len(snapshot.Communications) != 1 {
		t.Fatalf("communications after two accepts = %d, want 1", len(snapshot.Communications))
	}

	if after := len(outbox.Sent()); after-before != 1 {
		t.Fatalf("outbox recorded %d new messages across two accepts, want 1 (the second accept is a no-op)", after-before)
	}
}
