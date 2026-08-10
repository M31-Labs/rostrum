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
