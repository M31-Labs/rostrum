package domain

import (
	"testing"
	"time"
)

func TestTouchingSessionsDoNotConflict(t *testing.T) {
	start := time.Date(2026, time.October, 15, 9, 0, 0, 0, time.UTC)
	sessions := []Session{
		{ID: "a", RoomID: "room", StartsAt: start, EndsAt: start.Add(45 * time.Minute), SpeakerIDs: []string{"speaker"}},
		{ID: "b", RoomID: "room", StartsAt: start.Add(45 * time.Minute), EndsAt: start.Add(90 * time.Minute), SpeakerIDs: []string{"speaker"}},
	}
	if conflicts := DetectConflicts(sessions); len(conflicts) != 0 {
		t.Fatalf("touching sessions produced conflicts: %#v", conflicts)
	}
}

func TestOverlapProducesSeparatePolicyFacts(t *testing.T) {
	start := time.Date(2026, time.October, 15, 9, 0, 0, 0, time.UTC)
	sessions := []Session{
		{ID: "a", RoomID: "room", TrackID: "track", StartsAt: start, EndsAt: start.Add(time.Hour), SpeakerIDs: []string{"speaker"}},
		{ID: "b", RoomID: "room", TrackID: "track", StartsAt: start.Add(30 * time.Minute), EndsAt: start.Add(90 * time.Minute), SpeakerIDs: []string{"speaker"}},
	}
	conflicts := DetectConflicts(sessions)
	if len(conflicts) != 3 {
		t.Fatalf("conflicts = %d, want speaker, room, and track", len(conflicts))
	}
	for _, conflict := range conflicts {
		if conflict.OverlapMinutes != 30 {
			t.Fatalf("%s overlap = %d, want 30", conflict.Kind, conflict.OverlapMinutes)
		}
	}
}
