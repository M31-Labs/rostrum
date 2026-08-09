package appstate

import (
	"errors"
	"sync"

	"github.com/odvcencio/programma/internal/store"
)

var (
	mu      sync.RWMutex
	current *store.JSONStore
)

func Set(value *store.JSONStore) {
	mu.Lock()
	defer mu.Unlock()
	current = value
}

func Get() (*store.JSONStore, error) {
	mu.RLock()
	defer mu.RUnlock()
	if current == nil {
		return nil, errors.New("application store is not initialized")
	}
	return current, nil
}

func MustGet() *store.JSONStore {
	value, err := Get()
	if err != nil {
		panic(err)
	}
	return value
}
