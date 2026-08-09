package publicapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/programma/internal/domain"
)

func TestPublicEndpointsDoNotExposePrivateWorkspaceFields(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	state.TaskCompletions[0].StoredPath = "/private/uploads/agreement.pdf"

	for name, payload := range map[string]any{
		"index": Index(state), "speakers": Speakers(state), "schedule": Schedule(state),
	} {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, secret := range []string{"maya@example.com", "agreement.pdf", "ownerEmail", "reviewPlans", "evaluations"} {
			if strings.Contains(text, secret) {
				t.Errorf("%s API exposed %q", name, secret)
			}
		}
	}
}

func TestScheduleIncludesOnlyPublishedSessions(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	payload := Schedule(state)
	sessions := payload["sessions"].([]map[string]any)
	if len(sessions) != 6 {
		t.Fatalf("got %d published sessions, want 6", len(sessions))
	}
	for _, session := range sessions {
		if session["status"] != "published" {
			t.Fatalf("unpublished session in API: %#v", session)
		}
	}
}
