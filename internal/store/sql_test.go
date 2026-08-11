package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestSQLiteStorePersistsStateAndAuditAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rostrum.sqlite")
	seed := domain.Seed(time.Now().UTC())
	workspace, err := OpenSQLite(path, seed)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	if err := workspace.UpdateAudit(domain.AuditMeta{
		Actor:      "organizer:test",
		Action:     "event.updated",
		EntityType: "event",
		EntityID:   seed.Event.ID,
		Summary:    "Changed the test event title.",
		Origin:     "test",
	}, func(state *domain.State) error {
		state.Event.Name = "SQLite Rostrum"
		return nil
	}); err != nil {
		t.Fatalf("UpdateAudit: %v", err)
	}
	if err := workspace.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := OpenSQLite(path, seed)
	if err != nil {
		t.Fatalf("reopen SQLite store: %v", err)
	}
	defer reopened.Close()
	snapshot := reopened.Snapshot()
	if snapshot.Event.Name != "SQLite Rostrum" {
		t.Fatalf("reopened event name = %q, want SQLite Rostrum", snapshot.Event.Name)
	}
	if len(snapshot.AuditEvents) != 1 || snapshot.AuditEvents[0].Action != "event.updated" {
		t.Fatalf("reopened audit trail = %+v, want one retained event", snapshot.AuditEvents)
	}
	if err := snapshot.VerifyAuditTrail(); err != nil {
		t.Fatalf("VerifyAuditTrail: %v", err)
	}
	var journalMode string
	if err := reopened.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal mode = %q, want WAL", journalMode)
	}
	var migrations int
	if err := reopened.db.QueryRow("SELECT COUNT(*) FROM "+migrationsTable+" WHERE version = ?", workspaceSchemaLevel).Scan(&migrations); err != nil {
		t.Fatalf("read store migrations: %v", err)
	}
	if migrations != 1 {
		t.Fatalf("migration records = %d, want one level %d record", migrations, workspaceSchemaLevel)
	}
}

func TestSQLiteStoreReplaceRecordsRestoreProvenance(t *testing.T) {
	workspace, err := OpenSQLite(":memory:", domain.Seed(time.Now().UTC()))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer workspace.Close()

	replacement := domain.Seed(time.Now().UTC().Add(time.Hour))
	replacement.Event.Name = "Restored workspace"
	if err := workspace.Replace(replacement, domain.AuditMeta{
		Actor:      "organizer:test",
		Action:     "archive.imported",
		EntityType: "workspace",
		Summary:    "Restored a signed test archive.",
		Origin:     "test",
	}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	snapshot := workspace.Snapshot()
	if snapshot.Event.Name != "Restored workspace" {
		t.Fatalf("replacement event name = %q", snapshot.Event.Name)
	}
	if len(snapshot.AuditEvents) != 1 || snapshot.AuditEvents[0].Action != "archive.imported" {
		t.Fatalf("replacement audit trail = %+v", snapshot.AuditEvents)
	}
}
