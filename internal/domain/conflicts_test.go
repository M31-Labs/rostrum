package domain

import (
	"testing"
	"time"
)

func TestSeedIsValidAndContainsDemoConflicts(t *testing.T) {
	state := Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	if err := state.Validate(); err != nil {
		t.Fatalf("seed validation failed: %v", err)
	}
	conflicts := DetectConflicts(state.Sessions)
	if len(conflicts) < 3 {
		t.Fatalf("seed conflicts = %d, want at least 3", len(conflicts))
	}
	kinds := map[string]bool{}
	for _, conflict := range conflicts {
		kinds[conflict.Kind] = true
	}
	for _, kind := range []string{ConflictSpeaker, ConflictRoom, ConflictTrack} {
		if !kinds[kind] {
			t.Fatalf("seed has no %s conflict", kind)
		}
	}
}

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
