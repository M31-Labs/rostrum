package appstate

import (
	"errors"
	"sync"

	"github.com/m31-labs/rostrum/internal/store"
)

var (
	mu      sync.RWMutex
	current store.StateStore
)

// Set installs the process-wide workspace store. Application packages only
// see store.StateStore, so changing DATA_STORE from JSON to SQLite or
// Postgres does not require a second set of handlers or identity adapters.
func Set(value store.StateStore) {
	mu.Lock()
	defer mu.Unlock()
	current = value
}

func Get() (store.StateStore, error) {
	mu.RLock()
	defer mu.RUnlock()
	if current == nil {
		return nil, errors.New("application store is not initialized")
	}
	return current, nil
}

func MustGet() store.StateStore {
	value, err := Get()
	if err != nil {
		panic(err)
	}
	return value
}
