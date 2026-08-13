package domain

import (
	"testing"
	"time"
)

func TestApprovedHeadshotSelectsNewestActiveAssignedCompletion(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	state := EmptyState(now)
	state.Speakers = []Speaker{{ID: "speaker", FirstName: "Example", LastName: "Person", Email: "person@example.com"}}
	state.Tasks = []Task{
		{ID: "portrait_old", Title: "Portrait", Type: "headshot", AssignedSpeakerIDs: []string{"speaker"}},
		{ID: "portrait_new", Title: "Updated portrait", Type: "headshot", AssignedSpeakerIDs: []string{"speaker"}},
		{ID: "portrait_retired", Title: "Retired portrait", Type: "headshot", AssignedSpeakerIDs: []string{"speaker"}, RetiredAt: now.Add(-time.Hour)},
		{ID: "slides", Title: "Slides", Type: "file", AssignedSpeakerIDs: []string{"speaker"}},
	}
	state.TaskCompletions = []TaskCompletion{
		{ID: "old", TaskID: "portrait_old", SpeakerID: "speaker", Status: TaskApproved, FileName: "old.jpg", StoredPath: "/uploads/old.jpg", UpdatedAt: now.Add(-4 * time.Hour)},
		{ID: "pending", TaskID: "portrait_new", SpeakerID: "speaker", Status: TaskSubmitted, FileName: "pending.jpg", StoredPath: "/uploads/pending.jpg", UpdatedAt: now.Add(-3 * time.Hour)},
		{ID: "new", TaskID: "portrait_new", SpeakerID: "speaker", Status: TaskApproved, FileName: "new.jpg", StoredPath: "/uploads/new.jpg", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "retired", TaskID: "portrait_retired", SpeakerID: "speaker", Status: TaskApproved, FileName: "retired.jpg", StoredPath: "/uploads/retired.jpg", UpdatedAt: now.Add(-time.Hour)},
		{ID: "wrong_type", TaskID: "slides", SpeakerID: "speaker", Status: TaskApproved, FileName: "deck.pdf", StoredPath: "/uploads/deck.pdf", UpdatedAt: now},
	}

	completion, found := state.ApprovedHeadshot("speaker")
	if !found || completion.ID != "new" {
		t.Fatalf("ApprovedHeadshot() = %+v, %v; want newest active portrait", completion, found)
	}

	state.Tasks[1].AssignedSpeakerIDs = nil
	completion, found = state.ApprovedHeadshot("speaker")
	if !found || completion.ID != "old" {
		t.Fatalf("unassigned newest portrait = %+v, %v; want old", completion, found)
	}
}
