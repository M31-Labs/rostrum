package store

import (
	"errors"

	"github.com/m31-labs/rostrum/internal/domain"
)

// ErrReadOnly is returned for every operation that could change a workspace
// decorated with the mutation-proof read-only boundary.
var ErrReadOnly = errors.New("workspace is read-only")

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
	return ErrReadOnly
}

func (store *readOnlyStore) UpdateAudit(domain.AuditMeta, func(*domain.State) error) error {
	return ErrReadOnly
}

func (store *readOnlyStore) Reset() error {
	return ErrReadOnly
}

func (store *readOnlyStore) Replace(domain.State, domain.AuditMeta) error {
	return ErrReadOnly
}
