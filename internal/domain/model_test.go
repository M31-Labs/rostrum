package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
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

func TestSpeakerTasksHideRetiredAndIneligibleAcceptedOnlyAssignments(t *testing.T) {
	now := time.Now().UTC()
	state := State{
		Submissions: []Submission{{ID: "sub_pending", SpeakerIDs: []string{"spk_pending"}, Status: SubmissionPending}, {ID: "sub_accepted", SpeakerIDs: []string{"spk_accepted"}, Status: SubmissionAccepted}},
		Tasks: []Task{
			{ID: "task_general", Title: "General", Type: "form", DueAt: now, AssignedSpeakerIDs: []string{"spk_pending"}},
			{ID: "task_gated", Title: "Gated", Type: "form", DueAt: now, AcceptedOnly: true, AssignedSpeakerIDs: []string{"spk_pending", "spk_accepted"}},
			{ID: "task_retired", Title: "Retired", Type: "file", DueAt: now, AssignedSpeakerIDs: []string{"spk_accepted"}, RetiredAt: now},
		},
	}
	pending := state.SpeakerTasks("spk_pending")
	if len(pending) != 1 || pending[0].ID != "task_general" {
		t.Fatalf("pending speaker tasks = %#v, want only active non-gated task", pending)
	}
	accepted := state.SpeakerTasks("spk_accepted")
	if len(accepted) != 1 || accepted[0].ID != "task_gated" {
		t.Fatalf("accepted speaker tasks = %#v, want only active accepted-only task", accepted)
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

func TestQueuePublishedInviteCommunicationsIsIdempotent(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	state := Seed(now)
	state.Communications = nil
	scheduled, found := state.Session("ses_maintainers")
	if !found {
		t.Fatal("seed session ses_maintainers not found")
	}
	scheduled.Status = "published"

	queued := state.QueuePublishedInviteCommunications([]string{scheduled.ID}, now)
	if queued != len(scheduled.SpeakerIDs) || queued != 2 {
		t.Fatalf("first queue: queued = %d, want 2", queued)
	}
	if _, found := state.Communication(PublishedInviteTemplateID, "spk_lina", scheduled.ID); !found {
		t.Fatal("Lina's published invite row was not persisted")
	}
	if _, found := state.Communication(PublishedInviteTemplateID, "spk_priya", scheduled.ID); !found {
		t.Fatal("Priya's published invite row was not persisted")
	}

	if queued = state.QueuePublishedInviteCommunications([]string{scheduled.ID}, now); queued != 0 {
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

func TestStateValidateRejectsChainedConditionalQuestions(t *testing.T) {
	state := Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	form := &state.Forms[0]
	// workshop_needs is already a conditional target. It must never become a
	// source too, or a browser could have to evaluate a chained rule whose
	// visibility order is ambiguous.
	form.QuestionRules = append(form.QuestionRules, QuestionRule{
		ID: "rule_chained", SourceFieldID: "workshop_needs", Operator: "equals",
		Value: "yes", TargetFieldID: "topics", Effect: "show",
	})
	if err := state.Validate(); err == nil {
		t.Fatal("State.Validate accepted a chained conditional question")
	}
}

func TestStateValidateRequiresWithdrawalTimestamp(t *testing.T) {
	state := Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	state.Submissions[0].Status = SubmissionWithdrawn
	state.Submissions[0].WithdrawnAt = time.Time{}
	if err := state.Validate(); err == nil {
		t.Fatal("State.Validate accepted a withdrawn submission without a timestamp")
	}
}

func TestStateValidateRejectsInvalidEvaluations(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*Evaluation)
	}{
		{name: "unknown plan", mutate: func(evaluation *Evaluation) { evaluation.PlanID = "plan_missing" }},
		{name: "unknown submission", mutate: func(evaluation *Evaluation) { evaluation.SubmissionID = "sub_missing" }},
		{name: "unknown reviewer", mutate: func(evaluation *Evaluation) { evaluation.ReviewerID = "rev_missing" }},
		{name: "missing criterion", mutate: func(evaluation *Evaluation) { delete(evaluation.Scores, "relevance") }},
		{name: "unknown criterion", mutate: func(evaluation *Evaluation) {
			delete(evaluation.Scores, "relevance")
			evaluation.Scores["invented"] = 5
		}},
		{name: "out of range score", mutate: func(evaluation *Evaluation) { evaluation.Scores["relevance"] = 6 }},
		{name: "not a number score", mutate: func(evaluation *Evaluation) { evaluation.Scores["relevance"] = math.NaN() }},
		{name: "unknown source", mutate: func(evaluation *Evaluation) { evaluation.Source = "manual" }},
		{name: "unknown recommendation", mutate: func(evaluation *Evaluation) { evaluation.Recommendation = "approve" }},
		{name: "missing timestamp", mutate: func(evaluation *Evaluation) { evaluation.CreatedAt = time.Time{} }},
		{name: "reversed timestamps", mutate: func(evaluation *Evaluation) { evaluation.UpdatedAt = evaluation.CreatedAt.Add(-time.Second) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := Seed(now)
			test.mutate(&state.Evaluations[0])
			if err := state.Validate(); err == nil {
				t.Fatal("State.Validate accepted an invalid evaluation")
			}
		})
	}
}
