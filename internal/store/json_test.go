package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestJSONStorePersistsValidatedMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rostrum.json")
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

// TestJSONStoreSnapshotIsStableAcrossReads proves the cache invariant: two
// reads with no intervening write return the same cached value, and neither
// read triggers a clone of state that has not changed.
func TestJSONStoreSnapshotIsStableAcrossReads(t *testing.T) {
	seed := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	value, err := Open(":memory:", seed)
	if err != nil {
		t.Fatal(err)
	}

	first := value.Snapshot()
	second := value.Snapshot()

	if first.Event.Name != second.Event.Name {
		t.Fatalf("event name changed between reads with no write: %q vs %q", first.Event.Name, second.Event.Name)
	}
	if len(first.Speakers) != len(second.Speakers) {
		t.Fatalf("speaker count changed between reads with no write: %d vs %d", len(first.Speakers), len(second.Speakers))
	}
	// Same backing array: proves Snapshot() returned the cached clone rather
	// than cloning again on every call.
	if len(first.Speakers) > 0 && &first.Speakers[0] != &second.Speakers[0] {
		t.Fatal("consecutive Snapshot() calls returned independently cloned slices; want the cached snapshot")
	}
}

// TestJSONStoreSnapshotSeesSubsequentUpdate proves a successful Update
// rebuilds the cache: a Snapshot() taken after the write observes the
// mutation, and the value taken before it is left untouched (copy-on-write).
func TestJSONStoreSnapshotSeesSubsequentUpdate(t *testing.T) {
	seed := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	value, err := Open(":memory:", seed)
	if err != nil {
		t.Fatal(err)
	}

	before := value.Snapshot()
	if before.Event.Name == "Renamed forum" {
		t.Fatal("seed already uses the target name; fixture is not distinguishing")
	}

	if err := value.Update(func(state *domain.State) error {
		state.Event.Name = "Renamed forum"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	after := value.Snapshot()
	if after.Event.Name != "Renamed forum" {
		t.Fatalf("event name = %q after update, want %q", after.Event.Name, "Renamed forum")
	}
	if before.Event.Name == after.Event.Name {
		t.Fatal("snapshot taken before the update was not supposed to equal the post-update name")
	}
}

// TestJSONStoreSnapshotTopLevelAppendIsIsolated proves the safe half of the
// read-only contract: appending to a slice field of a returned snapshot
// changes only the caller's local copy of the slice header and never grows
// the cached snapshot that other readers see.
func TestJSONStoreSnapshotTopLevelAppendIsIsolated(t *testing.T) {
	seed := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	value, err := Open(":memory:", seed)
	if err != nil {
		t.Fatal(err)
	}

	before := value.Snapshot()
	originalCount := len(before.Speakers)

	local := before
	local.Speakers = append(local.Speakers, domain.Speaker{ID: "spk_extra", FirstName: "Extra"})
	if len(local.Speakers) != originalCount+1 {
		t.Fatalf("local append did not extend the local copy: got %d, want %d", len(local.Speakers), originalCount+1)
	}

	after := value.Snapshot()
	if len(after.Speakers) != originalCount {
		t.Fatalf("cached snapshot speaker count = %d after a caller-side append, want unchanged %d", len(after.Speakers), originalCount)
	}
}

// TestJSONStoreSnapshotElementMutationIsForbidden documents, rather than
// endorses, the unsafe half of the read-only contract. Snapshot() shares
// backing arrays and maps with the cached copy for performance: writing
// through an *element* of a returned slice (as opposed to a top-level
// append, see TestJSONStoreSnapshotTopLevelAppendIsIsolated) mutates shared
// memory and corrupts every other reader's view, including the store's own
// cache. Callers must never do this; see the JSONStore doc comment.
func TestJSONStoreSnapshotElementMutationIsForbidden(t *testing.T) {
	seed := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	value, err := Open(":memory:", seed)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := value.Snapshot()
	if len(snapshot.Speakers) == 0 {
		t.Fatal("fixture requires at least one seeded speaker")
	}
	originalName := snapshot.Speakers[0].FirstName

	// Forbidden: element mutation writes through the shared backing array.
	snapshot.Speakers[0].FirstName = "Corrupted"

	leaked := value.Snapshot()
	if leaked.Speakers[0].FirstName != "Corrupted" {
		t.Fatalf("expected element mutation to leak into the cache (demonstrating why it is forbidden); got %q, want %q", leaked.Speakers[0].FirstName, "Corrupted")
	}
	if originalName == "Corrupted" {
		t.Fatal("fixture speaker was already named Corrupted; cannot demonstrate the leak")
	}
}

// TestJSONStoreSnapshotConcurrentReadsAndWriteRaceClean exercises concurrent
// Snapshot() reads against a concurrent Update() writer. Run with -race:
// the RWMutex must serialize every access to store.snapshot so the race
// detector reports nothing, and every observed snapshot must be a value the
// writer actually published (never a torn or partial struct).
func TestJSONStoreSnapshotConcurrentReadsAndWriteRaceClean(t *testing.T) {
	seed := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	value, err := Open(":memory:", seed)
	if err != nil {
		t.Fatal(err)
	}

	const readers = 16
	const writes = 50
	valid := map[string]bool{seed.Event.Name: true}
	for i := 0; i < writes; i++ {
		valid[nameFor(i)] = true
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				got := value.Snapshot().Event.Name
				if !valid[got] {
					t.Errorf("snapshot observed unexpected event name %q", got)
					return
				}
			}
		}()
	}

	for i := 0; i < writes; i++ {
		name := nameFor(i)
		if err := value.Update(func(state *domain.State) error {
			state.Event.Name = name
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()

	if got := value.Snapshot().Event.Name; got != nameFor(writes-1) {
		t.Fatalf("final snapshot event name = %q, want %q", got, nameFor(writes-1))
	}
}

func nameFor(i int) string {
	return "Renamed forum " + string(rune('A'+i%26))
}
