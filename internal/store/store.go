// Package store owns Rostrum's transactional state boundary. Every backend
// presents the same small snapshot/mutation contract so application code is
// independent of JSON, SQLite, or Postgres persistence details.
package store

import "github.com/m31-labs/rostrum/internal/domain"

// StateStore is the canonical workspace persistence boundary. Snapshot values
// are immutable shared views; Update and UpdateAudit execute against a
// private copy and commit only after domain validation succeeds. Replace is
// reserved for a validated archive restore.
type StateStore interface {
	Snapshot() domain.State
	Update(func(*domain.State) error) error
	UpdateAudit(domain.AuditMeta, func(*domain.State) error) error
	Reset() error
	Replace(domain.State, domain.AuditMeta) error
	Path() string
	Close() error
}

// GenericAudit describes legacy callers that use Update rather than the
// more specific UpdateAudit. It keeps the audit trail complete during a
// staged migration of application actions without recording request data.
var GenericAudit = domain.AuditMeta{
	Actor:      "system",
	Action:     "state.updated",
	EntityType: "workspace",
	Summary:    "Workspace state updated.",
	Origin:     "rostrum",
}
