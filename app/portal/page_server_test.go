package portal

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/session"
)

func TestPortalKeyCannotAuthorizeAnotherSpeaker(t *testing.T) {
	state := domain.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	state.Speakers = append(state.Speakers, domain.Speaker{
		ID: "spk_isolated", FirstName: "Isolated", LastName: "Submitter", Email: "maya@example.com",
	})
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)

	sessions := session.MustNew("portal-test-session-secret-at-least-32-bytes", session.Options{AllowInsecure: true})
	request := httptest.NewRequest(http.MethodGet, "/portal/spk_maya?key="+token.New().Sign("spk_isolated"), nil)
	response := httptest.NewRecorder()
	var data map[string]any
	var bound string
	sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loaded, loadErr := loadPortal(&route.RouteContext{
			Request: r,
			Params:  map[string]string{"speaker": "spk_maya"},
		})
		if loadErr != nil {
			t.Fatalf("loadPortal: %v", loadErr)
		}
		data = loaded.(map[string]any)
		bound = session.Current(r).String(portalSessionKey)
	})).ServeHTTP(response, request)

	if available, _ := data["available"].(bool); available {
		t.Fatal("key for isolated submitter authorized the existing speaker portal")
	}
	if bound != "spk_isolated" {
		t.Fatalf("session bound to %q, want token subject spk_isolated", bound)
	}
}

func TestValidateTaskSubmissionRequiresConfirmationAndDeclaredOptions(t *testing.T) {
	if errors := validateTaskSubmission(domain.Task{Type: "form"}, map[string]string{}); errors["task"] == "" {
		t.Fatalf("empty confirmation errors = %#v, want task confirmation error", errors)
	}
	task := domain.Task{Type: "form", FormFields: []domain.FormField{
		{ID: "choice", Type: "select", Required: true, Options: []string{"A", "B"}},
		{ID: "agree", Type: "checkbox", Required: true},
	}}
	errors := validateTaskSubmission(task, map[string]string{"choice": "forged", "agree": "yes"})
	if errors["choice"] == "" {
		t.Fatalf("invalid select errors = %#v, want choice failure", errors)
	}
	if errors := validateTaskSubmission(task, map[string]string{"choice": "A", "agree": "on"}); len(errors) != 0 {
		t.Fatalf("valid task values errors = %#v, want none", errors)
	}
}
