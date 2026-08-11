package store

import (
	"errors"

	"github.com/m31-labs/rostrum/internal/domain"
)

// ErrReadOnlyDemo is returned for every operation that could change the
// workspace while the process is serving the hosted, fictional demo.
var ErrReadOnlyDemo = errors.New("workspace is read-only in demo mode")

// ReadOnly decorates a StateStore with a mutation-proof boundary. Reads and
// lifecycle operations remain delegated to the underlying store so the
// normal presenter and shutdown paths keep working, but no application path
// can persist a mutation, reset, or archive restore by accident.
func ReadOnly(base StateStore) StateStore {
	if base == nil {
		return nil
	}
	return &readOnlyStore{StateStore: base}
}

type readOnlyStore struct {
	StateStore
}

func (store *readOnlyStore) Update(func(*domain.State) error) error {
	return ErrReadOnlyDemo
}

func (store *readOnlyStore) UpdateAudit(domain.AuditMeta, func(*domain.State) error) error {
	return ErrReadOnlyDemo
}

func (store *readOnlyStore) Reset() error {
	return ErrReadOnlyDemo
}

func (store *readOnlyStore) Replace(domain.State, domain.AuditMeta) error {
	return ErrReadOnlyDemo
}
