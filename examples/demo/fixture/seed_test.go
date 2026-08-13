package fixture_test

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/domain"
)

func TestExampleFixtureIsValidAndContainsVisibleScheduleConflicts(t *testing.T) {
	state := fixture.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	if err := state.Validate(); err != nil {
		t.Fatalf("example fixture validation failed: %v", err)
	}
	conflicts := domain.DetectConflicts(state.Sessions)
	if len(conflicts) < 3 {
		t.Fatalf("example conflicts = %d, want at least 3", len(conflicts))
	}
	kinds := map[string]bool{}
	for _, conflict := range conflicts {
		kinds[conflict.Kind] = true
	}
	for _, kind := range []string{domain.ConflictSpeaker, domain.ConflictRoom, domain.ConflictTrack} {
		if !kinds[kind] {
			t.Fatalf("example fixture has no %s conflict", kind)
		}
	}
}
