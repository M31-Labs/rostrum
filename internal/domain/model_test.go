package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAddSessionForSubmissionIsIdempotent(t *testing.T) {
	state := &State{
		Submissions: []Submission{
			{
				ID:         "sub_1",
				EventID:    "event_1",
				Title:      "Scaling Go Services",
				Abstract:   "A talk about scaling.",
				Format:     "talk",
				TrackID:    "track_1",
				SpeakerIDs: []string{"speaker_1"},
			},
		},
	}

	created := state.AddSessionForSubmission("sub_1")
	if !created {
		t.Fatalf("first accept: created = false, want true")
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(state.Sessions))
	}

	session := state.Sessions[0]
	if session.SubmissionID != "sub_1" {
		t.Fatalf("session.SubmissionID = %q, want sub_1", session.SubmissionID)
	}
	if session.Status != "unscheduled" {
		t.Fatalf("session.Status = %q, want unscheduled", session.Status)
	}
	if session.Scheduled() {
		t.Fatalf("session.Scheduled() = true, want false")
	}
	if session.Title != "Scaling Go Services" {
		t.Fatalf("session.Title = %q, want submission title", session.Title)
	}
	if session.EventID != "event_1" {
		t.Fatalf("session.EventID = %q, want event_1", session.EventID)
	}
	if session.DurationMinutes != 45 || session.Duration() != 45*time.Minute {
		t.Fatalf("session duration = %d / %s, want 45 minutes", session.DurationMinutes, session.Duration())
	}

	created = state.AddSessionForSubmission("sub_1")
	if created {
		t.Fatalf("second accept: created = true, want false")
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("sessions after second accept = %d, want 1", len(state.Sessions))
	}
}

func TestAddSessionForSubmissionUnknownSubmission(t *testing.T) {
	state := &State{}
	if created := state.AddSessionForSubmission("missing"); created {
		t.Fatalf("created = true for unknown submission, want false")
	}
	if len(state.Sessions) != 0 {
		t.Fatalf("sessions = %d, want 0", len(state.Sessions))
	}
}

func TestAssignAcceptedOnlyTasksIsIdempotent(t *testing.T) {
	state := &State{
		Tasks: []Task{
			{ID: "task_profile", AcceptedOnly: true, AssignedSpeakerIDs: []string{"speaker_existing"}},
			{ID: "task_optional", AcceptedOnly: false},
		},
	}

	assigned := state.AssignAcceptedOnlyTasks([]string{"speaker_1", "speaker_existing"})
	if assigned != 1 {
		t.Fatalf("first assign: assigned = %d, want 1", assigned)
	}
	if !contains(state.Tasks[0].AssignedSpeakerIDs, "speaker_1") {
		t.Fatalf("task_profile.AssignedSpeakerIDs = %v, want to include speaker_1", state.Tasks[0].AssignedSpeakerIDs)
	}
	if len(state.Tasks[1].AssignedSpeakerIDs) != 0 {
		t.Fatalf("task_optional.AssignedSpeakerIDs = %v, want empty (AcceptedOnly = false)", state.Tasks[1].AssignedSpeakerIDs)
	}

	assigned = state.AssignAcceptedOnlyTasks([]string{"speaker_1", "speaker_existing"})
	if assigned != 0 {
		t.Fatalf("second assign: assigned = %d, want 0", assigned)
	}
	if len(state.Tasks[0].AssignedSpeakerIDs) != 2 {
		t.Fatalf("task_profile.AssignedSpeakerIDs = %v, want exactly 2 entries after repeat assign", state.Tasks[0].AssignedSpeakerIDs)
	}
}

func TestAssignPendingToActiveReviewPlanAndDetectCompanyRecusal(t *testing.T) {
	state := &State{
		ReviewPlans: []ReviewPlan{{ID: "plan_1", Status: "open", SubmissionIDs: []string{"sub_existing"}}},
		Submissions: []Submission{
			{ID: "sub_existing", Status: SubmissionPending},
			{ID: "sub_pending", Status: SubmissionPending, SpeakerIDs: []string{"speaker_1"}},
			{ID: "sub_accepted", Status: SubmissionAccepted},
		},
		Speakers: []Speaker{{ID: "speaker_1", Company: "Northstar Research"}},
	}

	planID, assigned, err := state.AssignPendingToActiveReviewPlan()
	if err != nil {
		t.Fatal(err)
	}
	if planID != "plan_1" || assigned != 1 || !containsReviewID(state.ReviewPlans[0].SubmissionIDs, "sub_pending") {
		t.Fatalf("assignment = (%q, %d, %#v), want pending submission in active plan", planID, assigned, state.ReviewPlans[0].SubmissionIDs)
	}
	if _, assigned, err = state.AssignPendingToActiveReviewPlan(); err != nil || assigned != 0 {
		t.Fatalf("second assignment = (%d, %v), want idempotent zero", assigned, err)
	}
	if !state.ReviewerCompanyConflict(Reviewer{Company: " northstar   research "}, state.Submissions[1]) {
		t.Fatal("ReviewerCompanyConflict = false, want normalized company match")
	}
}

func TestVerifyAuditTrailAcceptsLegacyHashWithoutGovernanceFields(t *testing.T) {
	event := AuditEvent{
		ID: "audit_legacy", At: time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC),
		Actor: "organizer", Action: "event.updated", EntityType: "event", EntityID: "evt_1",
		Summary: "event updated", Origin: "organizer-settings",
	}
	legacyPayload := strings.Join([]string{
		event.ID, event.At.UTC().Format(time.RFC3339Nano), event.Actor, event.Action,
		event.EntityType, event.EntityID, event.Summary, event.Origin, event.PreviousHash,
	}, "\x1f")
	sum := sha256.Sum256([]byte(legacyPayload))
	event.Hash = hex.EncodeToString(sum[:])

	if err := (State{AuditEvents: []AuditEvent{event}}).VerifyAuditTrail(); err != nil {
		t.Fatalf("VerifyAuditTrail legacy event: %v", err)
	}
}

func TestQueueAcceptanceCommunicationIsIdempotent(t *testing.T) {
	state := &State{Event: Event{Name: "M31 Systems Forum 2026"}}

	queued := state.QueueAcceptanceCommunication("ses_1", []string{"speaker_1", "speaker_2"})
	if queued != 2 {
		t.Fatalf("first queue: queued = %d, want 2", queued)
	}
	if len(state.Communications) != 2 {
		t.Fatalf("communications = %d, want 2", len(state.Communications))
	}
	for _, comm := range state.Communications {
		if comm.TemplateID != AcceptanceTemplateID {
			t.Fatalf("comm.TemplateID = %q, want %q", comm.TemplateID, AcceptanceTemplateID)
		}
		if comm.Status != "queued" {
			t.Fatalf("comm.Status = %q, want queued", comm.Status)
		}
		if comm.SessionID != "ses_1" {
			t.Fatalf("comm.SessionID = %q, want ses_1", comm.SessionID)
		}
	}

	queued = state.QueueAcceptanceCommunication("ses_1", []string{"speaker_1", "speaker_2"})
	if queued != 0 {
		t.Fatalf("second queue: queued = %d, want 0", queued)
	}
	if len(state.Communications) != 2 {
		t.Fatalf("communications after repeat queue = %d, want 2", len(state.Communications))
	}
}

func TestMarkCommunicationSentRecordsSuccess(t *testing.T) {
	state := &State{Event: Event{Name: "M31 Systems Forum 2026"}}
	state.QueueAcceptanceCommunication("ses_1", []string{"speaker_1"})

	found := state.MarkCommunicationSent(AcceptanceTemplateID, "speaker_1", "ses_1", "demo-outbox", nil)
	if !found {
		t.Fatal("MarkCommunicationSent found = false, want true for a queued row")
	}
	comm := state.Communications[0]
	if comm.Status != "sent" {
		t.Fatalf("comm.Status = %q, want sent", comm.Status)
	}
	if comm.Provider != "demo-outbox" {
		t.Fatalf("comm.Provider = %q, want demo-outbox", comm.Provider)
	}
	if comm.SentAt.IsZero() {
		t.Fatal("comm.SentAt is zero, want a stamped time")
	}
	if comm.Error != "" {
		t.Fatalf("comm.Error = %q, want empty on success", comm.Error)
	}
}

func TestMarkCommunicationSentRecordsFailureWithoutRawError(t *testing.T) {
	state := &State{Event: Event{Name: "M31 Systems Forum 2026"}}
	state.QueueAcceptanceCommunication("ses_1", []string{"speaker_1"})

	sendErr := errors.New("dial tcp 10.0.0.1:587: connection refused")
	found := state.MarkCommunicationSent(AcceptanceTemplateID, "speaker_1", "ses_1", "smtp", sendErr)
	if !found {
		t.Fatal("MarkCommunicationSent found = false, want true for a queued row")
	}
	comm := state.Communications[0]
	if comm.Status != "failed" {
		t.Fatalf("comm.Status = %q, want failed", comm.Status)
	}
	if comm.Error == "" {
		t.Fatal("comm.Error is empty, want a sanitized category")
	}
	if strings.Contains(comm.Error, "10.0.0.1") || strings.Contains(comm.Error, "connection refused") {
		t.Fatalf("comm.Error = %q, leaked the raw send error (M8)", comm.Error)
	}
}

func TestMarkCommunicationSentReportsNotFoundForNoMatchingRow(t *testing.T) {
	state := &State{}
	if found := state.MarkCommunicationSent(AcceptanceTemplateID, "speaker_1", "ses_1", "demo-outbox", nil); found {
		t.Fatal("MarkCommunicationSent found = true with no queued row, want false")
	}
}
