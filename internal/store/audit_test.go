package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/audit"
	"github.com/m31-labs/rostrum/internal/domain"
)

func TestWithAuditWritesAnIndependentLedger(t *testing.T) {
	base, err := Open(":memory:", fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ledger, err := audit.Open(filepath.Join(t.TempDir(), "audit.log"))
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	workspace := WithAudit(base, ledger)
	if err := workspace.UpdateAudit(domain.AuditMeta{
		Actor: "organizer:test", Action: "event.updated", EntityType: "event", EntityID: "evt_m31_forum_2026",
		Summary: "event title updated", Origin: "test", Rule: "AllowDecisionWithQuorum", Trace: "AllowDecisionWithQuorum → AllowDecision",
	}, func(state *domain.State) error {
		state.Event.Name = "Audited Rostrum"
		return nil
	}); err != nil {
		t.Fatalf("UpdateAudit: %v", err)
	}
	records, err := ledger.Records()
	if err != nil {
		t.Fatalf("ledger.Records: %v", err)
	}
	if len(records) != 1 || records[0].Kind != "event.updated" || records[0].Rule != "AllowDecisionWithQuorum" || len(records[0].Trace) != 1 {
		t.Fatalf("ledger records = %#v", records)
	}
	if err := workspace.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
