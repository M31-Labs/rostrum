package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/session"
)

func TestProfileSaveWorksInCustomWorkspaceWithoutProfileTask(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	state := domain.EmptyState(now)
	state.Speakers = []domain.Speaker{{
		ID: "spk_custom", FirstName: "Custom", LastName: "Speaker", Email: "speaker@example.com",
		CreatedAt: now, UpdatedAt: now,
	}}
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open custom workspace: %v", err)
	}
	appstate.Set(workspace)

	sessions := session.MustNew("profile-without-task-test-secret-at-least-32-bytes", session.Options{AllowInsecure: true})
	if sessions == nil {
		t.Fatal("create test session manager")
	}
	request := httptest.NewRequest(http.MethodPost, "/portal/spk_custom/__actions/updateProfile", nil)
	response := httptest.NewRecorder()
	var actionErr error
	sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Current(r).Set(portalSessionKey, "spk_custom")
		actionErr = updateProfile(&action.Context{
			Request: r,
			FormData: map[string]string{
				"pronouns":      "they/them",
				"role":          "Staff engineer",
				"company":       "Community Systems",
				"biography":     "Custom Speaker builds dependable community infrastructure and teaches teams how to operate it well.",
				"city":          "Portland",
				"linkedin":      "https://www.linkedin.com/in/custom-speaker",
				"website":       "https://speaker.example.com",
				"email_opt_out": "on",
			},
		})
	})).ServeHTTP(response, request)
	if actionErr != nil {
		t.Fatalf("updateProfile without task: %v", actionErr)
	}

	snapshot := workspace.Snapshot()
	speaker, found := snapshot.Speaker("spk_custom")
	if !found {
		t.Fatal("updated speaker is missing")
	}
	if speaker.Pronouns != "they/them" || speaker.Role != "Staff engineer" || speaker.Company != "Community Systems" || speaker.City != "Portland" {
		t.Fatalf("updated profile = %#v", speaker)
	}
	if !speaker.EmailOptOut || speaker.EmailOptOutAt.IsZero() {
		t.Fatalf("email preference = optOut:%t at:%s", speaker.EmailOptOut, speaker.EmailOptOutAt)
	}
	if len(snapshot.Tasks) != 0 || len(snapshot.TaskCompletions) != 0 {
		t.Fatalf("profile save invented task state: tasks=%#v completions=%#v", snapshot.Tasks, snapshot.TaskCompletions)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("custom workspace after profile save: %v", err)
	}
	audit := snapshot.AuditEvents[len(snapshot.AuditEvents)-1]
	if audit.Action != "speaker.profile_updated" || audit.EntityID != speaker.ID {
		t.Fatalf("profile audit = %#v", audit)
	}
}
