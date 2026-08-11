package airtable

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestBuildProjectionsUsesScheduledProgramAndStableIDs(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	projections := BuildProjections(state)
	if len(projections) != 16 {
		t.Fatalf("projections = %d, want 8 scheduled speakers + 8 sessions", len(projections))
	}
	for _, projection := range projections {
		if projection.EntityID == "" || projection.Fields[RostrumIDField] != projection.EntityID {
			t.Fatalf("projection = %#v, want stable Rostrum ID", projection)
		}
		if projection.Fields[SchemaField] != domain.CurrentSchemaVersion {
			t.Fatalf("projection schema = %#v, want %d", projection.Fields[SchemaField], domain.CurrentSchemaVersion)
		}
	}
}

func TestEnqueueSkipsDeliveredPayloadAndRequeuesChanges(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	projections := BuildProjections(state)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	added, err := Enqueue(&state, projections, now)
	if err != nil || added != len(projections) {
		t.Fatalf("initial Enqueue = (%d, %v), want (%d, nil)", added, err, len(projections))
	}
	deliveredID := ""
	deliveredSessionID := ""
	for _, item := range state.SyncOutbox {
		if item.Kind == SessionKind {
			deliveredID = item.ID
			deliveredSessionID = item.EntityID
			break
		}
	}
	if deliveredID == "" {
		t.Fatal("outbox has no session projection")
	}
	result := SyncResult{Delivered: []string{deliveredID}}
	ApplyResult(&state, result, now)
	added, err = Enqueue(&state, projections, now.Add(time.Minute))
	if err != nil || added != 0 {
		t.Fatalf("unchanged Enqueue = (%d, %v), want (0, nil)", added, err)
	}

	for index := range state.Sessions {
		if state.Sessions[index].ID == deliveredSessionID {
			state.Sessions[index].Title = "Updated projected session"
			break
		}
	}
	changed := BuildProjections(state)
	added, err = Enqueue(&state, changed, now.Add(2*time.Minute))
	if err != nil || added != 1 {
		t.Fatalf("changed Enqueue = (%d, %v), want (1, nil)", added, err)
	}
	if len(Pending(state, now.Add(2*time.Minute))) != len(projections) {
		t.Fatalf("pending records = %d, want all records after changed first delivery is requeued", len(Pending(state, now.Add(2*time.Minute))))
	}
}

func TestSyncBatchesPerformUpsertAndPreservesOutboxResult(t *testing.T) {
	requests := make([]map[string]any, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-pat" {
			t.Errorf("authorization = %q, want PAT bearer", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		requests = append(requests, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	defer server.Close()

	items := make([]domain.SyncOutboxItem, 0, 11)
	for index := 0; index < 11; index++ {
		encoded, err := json.Marshal(payload{Fields: map[string]any{RostrumIDField: "spk_" + string(rune('a'+index)), "Name": "Speaker"}})
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, domain.SyncOutboxItem{ID: "outbox_" + string(rune('a'+index)), Integration: IntegrationID, Kind: SpeakerKind, EntityID: "spk", Payload: string(encoded)})
	}
	result, err := Sync(context.Background(), Config{
		Token: "test-pat", BaseID: "app_test", SpeakersTable: "Speakers", SessionsTable: "Sessions", APIBaseURL: server.URL, HTTPClient: server.Client(),
	}, items)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Requests != 2 || len(result.Delivered) != 11 || result.Speakers != 11 || result.Sessions != 0 {
		t.Fatalf("result = %#v, want 2 requests and 11 delivered speakers", result)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want two ten-record batches", len(requests))
	}
	for _, request := range requests {
		upsert, ok := request["performUpsert"].(map[string]any)
		if !ok || !strings.Contains(strings.Join(anyStrings(upsert["fieldsToMergeOn"]), ","), RostrumIDField) {
			t.Fatalf("performUpsert = %#v, want Rostrum ID merge field", request["performUpsert"])
		}
		if typecast, ok := request["typecast"].(bool); !ok || !typecast {
			t.Fatalf("typecast = %#v, want true", request["typecast"])
		}
	}
}

func anyStrings(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func TestApplyResultBacksOffFailures(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	state := domain.State{SyncOutbox: []domain.SyncOutboxItem{{ID: "outbox_1", Integration: IntegrationID, Attempts: 0}}}
	ApplyResult(&state, SyncResult{Failed: map[string]string{"outbox_1": "rate limited"}}, now)
	item := state.SyncOutbox[0]
	if item.Attempts != 1 || item.LastError != "rate limited" || !item.AvailableAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("failed outbox state = %#v, want first 30-second backoff", item)
	}
}
