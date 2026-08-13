package accelevents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
)

func TestBuildPayloadsUsesOnlyPublishedProgram(t *testing.T) {
	payloads := BuildPayloads(fixture.Seed(time.Now().UTC()))
	if len(payloads.Sessions) != 6 || len(payloads.Speakers) != 6 {
		t.Fatalf("unexpected payload counts: %d sessions, %d speakers", len(payloads.Sessions), len(payloads.Speakers))
	}
	for _, session := range payloads.Sessions {
		if session.Title == "" || session.StartTime == "" {
			t.Fatalf("incomplete session payload: %#v", session)
		}
		if session.ExternalID == "ses_eval" {
			t.Fatalf("draft session leaked into payload: %#v", session)
		}
	}
}

func TestSyncPublishesSpeakersBeforeSessions(t *testing.T) {
	paths := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q", got)
		}
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	payloads := Payloads{
		Speakers: []SpeakerPayload{{ExternalID: "spk_1", FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com"}},
		Sessions: []SessionPayload{{ExternalID: "ses_1", Title: "A governed program", StartTime: time.Now().UTC().Format(time.RFC3339), EndTime: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}},
	}
	client := Client{BaseURL: server.URL, Token: "test-token", HTTPClient: server.Client()}
	if err := client.Sync(context.Background(), "m31-forum", payloads); err != nil {
		t.Fatal(err)
	}
	want := []string{"/rest/host/event/m31-forum/speaker", "/rest/host/event/m31-forum/session"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestSyncFailsClosedWithoutToken(t *testing.T) {
	client := Client{BaseURL: "https://example.invalid"}
	err := client.Sync(context.Background(), "m31-forum", Payloads{})
	if err == nil {
		t.Fatal("expected missing-token error")
	}
}
