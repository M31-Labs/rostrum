package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/odvcencio/programma/internal/domain"
)

func TestJSONStorePersistsValidatedMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "programma.json")
	seed := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	first, err := Open(path, seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Update(func(state *domain.State) error {
		state.Event.Name = "Changed forum"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path, seed)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Snapshot().Event.Name; got != "Changed forum" {
		t.Fatalf("event name = %q, want persisted value", got)
	}
}

func TestJSONStoreRollsBackFailedMutation(t *testing.T) {
	seed := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	value, err := Open(":memory:", seed)
	if err != nil {
		t.Fatal(err)
	}
	want := value.Snapshot().Event.Name
	marker := errors.New("stop")
	err = value.Update(func(state *domain.State) error {
		state.Event.Name = "should not stick"
		return marker
	})
	if !errors.Is(err, marker) {
		t.Fatalf("update error = %v, want marker", err)
	}
	if got := value.Snapshot().Event.Name; got != want {
		t.Fatalf("event name = %q, want rollback to %q", got, want)
	}
}
