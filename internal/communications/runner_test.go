package communications

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/store"
)

type testSender struct {
	sent     []mail.Message
	failures int
}

func (sender *testSender) Name() string { return "test-sender" }

func (sender *testSender) Send(message mail.Message) error {
	sender.sent = append(sender.sent, message)
	if sender.failures > 0 {
		sender.failures--
		return errors.New("transient delivery failure")
	}
	return nil
}

func schedulerWorkspace(t *testing.T, now time.Time) *store.JSONStore {
	t.Helper()
	state := fixture.Seed(now)
	state.Communications = nil
	state.Tasks = []domain.Task{{
		ID: "task_due", Title: "Submit final slides", Type: "file", DueAt: now.Add(24 * time.Hour),
		AssignedSpeakerIDs: []string{"spk_maya"},
	}}
	state.TaskCompletions = nil
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	return workspace
}

func TestRunnerPersistsThenDeliversDerivedTaskReminder(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	workspace := schedulerWorkspace(t, now)
	sender := &testSender{}
	report, err := (Runner{Store: workspace, Sender: sender, Now: func() time.Time { return now }}).RunDue()
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if report.Enqueued != 1 || report.Sent != 1 || len(sender.sent) != 1 {
		t.Fatalf("report/sends = %#v / %#v", report, sender.sent)
	}
	if sender.sent[0].IdempotencyKey == "" || sender.sent[0].To != "maya@example.com" {
		t.Fatalf("sent reminder = %#v", sender.sent[0])
	}
	if sender.sent[0].Subject != "A speaker task is due in five days" || sender.sent[0].TextBody == "" {
		t.Fatalf("rendered reminder = %#v", sender.sent[0])
	}
	snapshot := workspace.Snapshot()
	if len(snapshot.Communications) != 1 {
		t.Fatalf("communications = %#v", snapshot.Communications)
	}
	item := snapshot.Communications[0]
	if item.Status != domain.CommunicationSent || item.AttemptCount != 1 || item.Provider != "test-sender" || item.SentAt.IsZero() {
		t.Fatalf("delivery row = %#v", item)
	}
}

func TestRunnerDeliversPublishedAgendaInvites(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	state := fixture.Seed(now)
	state.Communications = nil
	scheduled, found := state.Session("ses_maintainers")
	if !found {
		t.Fatal("seed session ses_maintainers not found")
	}
	scheduled.Status = "published"
	if queued := state.QueuePublishedInviteCommunications([]string{scheduled.ID}, now); queued != 2 {
		t.Fatalf("queued invites = %d, want 2", queued)
	}
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	sender := &testSender{}
	report, err := (Runner{Store: workspace, Sender: sender, Now: func() time.Time { return now }}).RunDue()
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if report.Sent != 2 || len(sender.sent) != 2 {
		t.Fatalf("report/sends = %#v / %#v", report, sender.sent)
	}
	for _, message := range sender.sent {
		if message.Calendar == nil || !strings.Contains(string(message.Calendar), "METHOD:REQUEST") {
			t.Fatalf("published invite message missing calendar request: %#v", message)
		}
		if message.Subject != domain.PublishedInviteSubject {
			t.Fatalf("published invite subject = %q, want %q", message.Subject, domain.PublishedInviteSubject)
		}
	}
	if second, err := (Runner{Store: workspace, Sender: sender, Now: func() time.Time { return now }}).RunDue(); err != nil || second.Sent != 0 || len(sender.sent) != 2 {
		t.Fatalf("repeat RunDue = %#v, %v; sends = %d, want no duplicate", second, err, len(sender.sent))
	}
}

func TestRunnerRetriesWithTheSamePersistedMessageIdentity(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	workspace := schedulerWorkspace(t, now)
	sender := &testSender{failures: 1}
	runner := Runner{Store: workspace, Sender: sender, Now: func() time.Time { return now }}
	first, err := runner.RunDue()
	if err != nil {
		t.Fatalf("first RunDue: %v", err)
	}
	if first.Retried != 1 || first.Sent != 0 {
		t.Fatalf("first report = %#v", first)
	}
	intermediate := workspace.Snapshot().Communications[0]
	if intermediate.Status != domain.CommunicationRetrying || intermediate.NextAttemptAt != now.Add(time.Minute) {
		t.Fatalf("retry row = %#v", intermediate)
	}
	runner.Now = func() time.Time { return now.Add(time.Minute) }
	second, err := runner.RunDue()
	if err != nil {
		t.Fatalf("second RunDue: %v", err)
	}
	if second.Sent != 1 || len(sender.sent) != 2 {
		t.Fatalf("second report/sends = %#v / %#v", second, sender.sent)
	}
	if sender.sent[0].IdempotencyKey != sender.sent[1].IdempotencyKey {
		t.Fatalf("retry changed idempotency key: %q / %q", sender.sent[0].IdempotencyKey, sender.sent[1].IdempotencyKey)
	}
	final := workspace.Snapshot().Communications[0]
	if final.Status != domain.CommunicationSent || final.AttemptCount != 2 {
		t.Fatalf("final retry row = %#v", final)
	}
}

func TestRunnerRecordsOptOutAsSuppressedWithoutCallingSender(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	workspace := schedulerWorkspace(t, now)
	if err := workspace.Update(func(state *domain.State) error {
		speaker, _ := state.Speaker("spk_maya")
		speaker.EmailOptOut = true
		speaker.EmailOptOutAt = now
		return nil
	}); err != nil {
		t.Fatalf("set opt-out: %v", err)
	}
	sender := &testSender{}
	report, err := (Runner{Store: workspace, Sender: sender, Now: func() time.Time { return now }}).RunDue()
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if report.Suppressed != 1 || len(sender.sent) != 0 {
		t.Fatalf("opt-out report/sends = %#v / %#v", report, sender.sent)
	}
	item := workspace.Snapshot().Communications[0]
	if item.Status != domain.CommunicationSuppressed || item.SuppressedAt.IsZero() {
		t.Fatalf("suppressed row = %#v", item)
	}
}

func TestNotificationRulesAreIdempotentAndAuditableAsOutboxRows(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	state := fixture.Seed(now)
	state.Communications = nil
	created := EnqueueNotificationRules(&state, Trigger{
		Name: "submission.created", SubmissionID: "sub_memory", SpeakerID: "spk_maya",
	}, now)
	if created != 1 || len(state.Communications) != 1 {
		t.Fatalf("notification enqueue = %d / %#v", created, state.Communications)
	}
	if again := EnqueueNotificationRules(&state, Trigger{Name: "submission.created", SubmissionID: "sub_memory", SpeakerID: "spk_maya"}, now); again != 0 {
		t.Fatalf("duplicate notification enqueue = %d, want 0", again)
	}
	item := state.Communications[0]
	if item.NotificationRuleID == "" || item.RecipientEmail != "program@example.com" || item.IdempotencyKey == "" {
		t.Fatalf("notification row = %#v", item)
	}
}
