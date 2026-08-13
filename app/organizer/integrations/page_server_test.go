package integrations

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/airtable"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

func TestAirtableDryRunRecordsAuditedCredentialFreeProjection(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	t.Setenv("AIRTABLE_PAT", "")
	t.Setenv("AIRTABLE_BASE_ID", "")

	ctx := &action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/organizer/integrations/__actions/airtableDryRun", nil),
		FormData: map[string]string{},
	}
	if err := airtableDryRun(ctx); err != nil {
		t.Fatalf("airtableDryRun: %v", err)
	}
	snapshot := workspace.Snapshot()
	if len(snapshot.SyncRuns) != 2 {
		t.Fatalf("sync runs = %d, want seeded Accelevents run plus Airtable dry run", len(snapshot.SyncRuns))
	}
	run := snapshot.SyncRuns[len(snapshot.SyncRuns)-1]
	if run.Integration != airtable.IntegrationID || run.Mode != "dry-run" || run.Status != "complete" || run.Speakers == 0 || run.Sessions == 0 {
		t.Fatalf("Airtable dry-run record = %#v", run)
	}
	if len(snapshot.SyncOutbox) != 0 {
		t.Fatalf("dry run enqueued %d records, want no durable delivery work", len(snapshot.SyncOutbox))
	}
	if len(snapshot.AuditEvents) != 1 || snapshot.AuditEvents[0].Action != "integration.airtable_dry_run" {
		t.Fatalf("audit events = %#v, want Airtable dry-run provenance", snapshot.AuditEvents)
	}
}

func TestAirtableSyncQueuesBeforeDeliveryAndRecordsOutcome(t *testing.T) {
	requests := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-pat" {
			t.Errorf("authorization = %q, want PAT bearer", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	defer remote.Close()

	state := fixture.Seed(time.Now().UTC())
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	t.Setenv("AIRTABLE_PAT", "test-pat")
	t.Setenv("AIRTABLE_BASE_ID", "app_test")
	t.Setenv("AIRTABLE_API_BASE_URL", remote.URL)
	t.Setenv("AIRTABLE_SPEAKERS_TABLE", "Speakers")
	t.Setenv("AIRTABLE_SESSIONS_TABLE", "Sessions")

	ctx := &action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/organizer/integrations/__actions/airtableSync", nil),
		FormData: map[string]string{},
	}
	if err := airtableSync(ctx); err != nil {
		t.Fatalf("airtableSync: %v", err)
	}
	snapshot := workspace.Snapshot()
	if requests != 2 {
		t.Fatalf("remote requests = %d, want speaker and session upsert batches", requests)
	}
	if len(snapshot.SyncOutbox) != 16 {
		t.Fatalf("outbox = %d, want 16 projected records", len(snapshot.SyncOutbox))
	}
	for _, item := range snapshot.SyncOutbox {
		if item.DeliveredAt.IsZero() || item.IdempotencyKey == "" || item.Attempts != 1 {
			t.Fatalf("outbox item after sync = %#v, want delivered idempotent record", item)
		}
	}
	if len(snapshot.SyncRuns) != 2 || snapshot.SyncRuns[len(snapshot.SyncRuns)-1].Integration != airtable.IntegrationID || snapshot.SyncRuns[len(snapshot.SyncRuns)-1].Status != "complete" {
		t.Fatalf("sync runs = %#v, want complete Airtable run", snapshot.SyncRuns)
	}
	if len(snapshot.AuditEvents) != 2 || snapshot.AuditEvents[0].Action != "integration.airtable_queued" || snapshot.AuditEvents[1].Action != "integration.airtable_complete" {
		t.Fatalf("audit events = %#v, want queue and completion provenance", snapshot.AuditEvents)
	}
}
