package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/odvcencio/programma/internal/domain"
)

// JSONStore is a small durable store for the hackathon and self-hosted path.
// Mutations use copy-on-write validation and an atomic rename, so a failed
// action never leaves partially updated state in memory or on disk.
type JSONStore struct {
	mu    sync.RWMutex
	path  string
	seed  domain.State
	state domain.State
}

func Open(path string, seed domain.State) (*JSONStore, error) {
	if err := seed.Validate(); err != nil {
		return nil, fmt.Errorf("validate seed state: %w", err)
	}
	store := &JSONStore{path: filepath.Clean(path), seed: clone(seed), state: clone(seed)}
	if path == "" || path == ":memory:" {
		store.path = ""
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
	return store, nil
}

func (store *JSONStore) Snapshot() domain.State {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return clone(store.state)
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
	return nil
}

func (store *JSONStore) Reset() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.state = clone(store.seed)
	store.state.UpdatedAt = time.Now().UTC()
	return store.persistLocked()
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

	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".programma-*.tmp")
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
