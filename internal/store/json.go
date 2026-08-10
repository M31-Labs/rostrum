package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

// JSONStore is a small durable store for the hackathon and self-hosted path.
// Mutations use copy-on-write validation and an atomic rename, so a failed
// action never leaves partially updated state in memory or on disk.
//
// Snapshot cache contract: JSONStore clones store.state once per successful
// write (Open, Update, Reset) and caches the clone in store.snapshot.
// Snapshot() returns that cached value directly, under RLock, without
// cloning. This makes reads O(1) instead of O(state size) per request.
//
// Read-only contract: the domain.State returned by Snapshot() is shared
// across every concurrent caller and MUST be treated as immutable.
//   - Never assign into a field, slice element, or map entry reached through
//     the returned value (for example `snapshot.Speakers[0].Name = x` or
//     `snapshot.Integrations["x"] = y`): slices and maps are reference types,
//     so such a write mutates the shared cache and every other reader,
//     including the store's own state, without holding the store's lock.
//   - A top-level `append` to a slice field of the returned value (for
//     example `snapshot.Speakers = append(snapshot.Speakers, s)`) is safe: it
//     only ever changes the local copy of the slice header. It may share
//     backing array capacity with the cached snapshot, but never grows the
//     cached snapshot's observable length, and any element it does share is
//     only visible through the local variable, not the cache.
//   - Callers that need a private, independently mutable copy must clone the
//     snapshot themselves (for example via the unexported clone helper's
//     approach: JSON marshal/unmarshal, or a targeted deep copy) rather than
//     mutate the value Snapshot() returns.
type JSONStore struct {
	mu       sync.RWMutex
	path     string
	seed     domain.State
	state    domain.State
	snapshot domain.State
}

func Open(path string, seed domain.State) (*JSONStore, error) {
	if err := seed.Validate(); err != nil {
		return nil, fmt.Errorf("validate seed state: %w", err)
	}
	store := &JSONStore{path: filepath.Clean(path), seed: clone(seed), state: clone(seed)}
	if path == "" || path == ":memory:" {
		store.path = ""
		store.snapshot = clone(store.state)
		return store, nil
	}

	data, err := os.ReadFile(store.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &store.state); err != nil {
			return nil, fmt.Errorf("decode %s: %w", store.path, err)
		}
		if store.state.SchemaVersion != domain.CurrentSchemaVersion {
			return nil, fmt.Errorf("data schema %d is not supported; want %d", store.state.SchemaVersion, domain.CurrentSchemaVersion)
		}
		if err := store.state.Validate(); err != nil {
			return nil, fmt.Errorf("validate %s: %w", store.path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read %s: %w", store.path, err)
	}
	store.snapshot = clone(store.state)
	return store, nil
}

// Snapshot returns the cached, immutable copy of the store's state. It does
// not clone on every call: the clone happens once per successful write (see
// the JSONStore doc comment for the read-only contract callers must honor).
func (store *JSONStore) Snapshot() domain.State {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.snapshot
}

func (store *JSONStore) Update(change func(*domain.State) error) error {
	if change == nil {
		return errors.New("store update requires a mutation")
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	next := clone(store.state)
	if err := change(&next); err != nil {
		return err
	}
	next.SchemaVersion = domain.CurrentSchemaVersion
	next.UpdatedAt = time.Now().UTC()
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate update: %w", err)
	}
	previous := store.state
	store.state = next
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return err
	}
	// One clone per write, taken only after persistLocked succeeds, so
	// readers never observe a snapshot for state that failed to persist.
	store.snapshot = clone(store.state)
	return nil
}

func (store *JSONStore) Reset() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.state = clone(store.seed)
	store.state.UpdatedAt = time.Now().UTC()
	// Reset has no rollback path (matching its pre-existing behavior): the
	// in-memory state always becomes the seed, whether or not persisting it
	// succeeds. Keep the cached snapshot in lockstep with store.state so
	// Snapshot() reports exactly what it always has here.
	err := store.persistLocked()
	store.snapshot = clone(store.state)
	return err
}

func (store *JSONStore) Path() string {
	return store.path
}

func (store *JSONStore) persistLocked() error {
	if store.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	data, err := json.MarshalIndent(store.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode data: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".rostrum-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary data file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("protect temporary data file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary data file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary data file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temporary data file: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		cleanup()
		return fmt.Errorf("replace data file: %w", err)
	}
	return nil
}

func clone(state domain.State) domain.State {
	data, err := json.Marshal(state)
	if err != nil {
		panic(fmt.Sprintf("clone state: %v", err))
	}
	var result domain.State
	if err := json.Unmarshal(data, &result); err != nil {
		panic(fmt.Sprintf("clone state: %v", err))
	}
	return result
}
