package store

import (
	"fmt"

	"github.com/m31-labs/rostrum/internal/audit"
	"github.com/m31-labs/rostrum/internal/domain"
)

// WithAudit decorates a canonical store with an independent append-only
// ledger. The aggregate's hash-chain remains useful for point-in-time state;
// the ledger persists separately and survives workspace import or restore.
func WithAudit(base StateStore, ledger *audit.Log) StateStore {
	if base == nil || ledger == nil {
		return base
	}
	return &auditedStore{StateStore: base, ledger: ledger}
}

type auditedStore struct {
	StateStore
	ledger *audit.Log
}

func (store *auditedStore) Update(change func(*domain.State) error) error {
	return store.UpdateAudit(GenericAudit, change)
}

func (store *auditedStore) UpdateAudit(meta domain.AuditMeta, change func(*domain.State) error) error {
	if err := store.StateStore.UpdateAudit(meta, change); err != nil {
		return err
	}
	if _, err := store.ledger.Append(audit.FromMeta(meta)); err != nil {
		// The workspace commit already succeeded. Returning the failure makes
		// the degraded audit state visible to the operator instead of silently
		// claiming a fully recorded mutation; the durable in-state event gives
		// a subsequent operation enough context to investigate and reconcile.
		return fmt.Errorf("workspace committed but independent audit append failed: %w", err)
	}
	return nil
}

func (store *auditedStore) Reset() error {
	if err := store.StateStore.Reset(); err != nil {
		return err
	}
	_, err := store.ledger.Append(audit.FromMeta(domain.AuditMeta{
		Actor: "system", Action: "workspace.reset", EntityType: "workspace",
		Summary: "Workspace reset to its configured seed.", Origin: "rostrum",
	}))
	if err != nil {
		return fmt.Errorf("workspace reset but independent audit append failed: %w", err)
	}
	return nil
}

func (store *auditedStore) Replace(next domain.State, meta domain.AuditMeta) error {
	if err := store.StateStore.Replace(next, meta); err != nil {
		return err
	}
	if _, err := store.ledger.Append(audit.FromMeta(meta)); err != nil {
		return fmt.Errorf("workspace restore committed but independent audit append failed: %w", err)
	}
	return nil
}

func (store *auditedStore) Close() error {
	ledgerErr := store.ledger.Close()
	storeErr := store.StateStore.Close()
	if ledgerErr != nil {
		return ledgerErr
	}
	return storeErr
}
