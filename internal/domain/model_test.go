package domain

import "testing"

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
