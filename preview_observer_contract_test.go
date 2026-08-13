package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/identity"
	"github.com/m31-labs/rostrum/internal/previewmode"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/auth"
)

func TestAnonymousPreviewOrganizerViewsNeverEstablishAnOrganizerIdentity(t *testing.T) {
	t.Setenv("APP_MODE", "preview")
	t.Setenv("GOSX_STATIC_EXPORT", "")

	sessions := testSessionManager(t)
	authManager := identity.New(sessions)
	visited := make(map[string]bool)
	observerView := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited[r.URL.Path] = true
		if user, ok := auth.Current(r); ok {
			t.Errorf("anonymous preview GET %s unexpectedly established identity %#v", r.URL.Path, user)
		}
		if isOrganizerSession(r) {
			t.Errorf("anonymous preview GET %s unexpectedly became an organizer-facing session", r.URL.Path)
		}
		if canMutateWorkspace(r) {
			t.Errorf("anonymous preview GET %s unexpectedly gained workspace mutation authority", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := sessions.Middleware(authManager.Middleware(readOnlyPreviewGate()(organizerGate()(observerView))))

	paths := []string{
		"/organizer",
		"/organizer/forms",
		"/organizer/submissions",
		"/organizer/review",
		"/organizer/speakers",
		"/organizer/agenda",
		"/organizer/communications",
		"/organizer/portal",
		"/organizer/embeds",
		"/organizer/integrations",
		"/organizer/settings",
	}
	for _, path := range paths {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNoContent {
			t.Errorf("anonymous observer GET %s status = %d, want 204", path, recorder.Code)
		}
		if !visited[path] {
			t.Errorf("anonymous observer GET %s did not reach the read-only view", path)
		}
	}
}

func TestHostedPreviewActionProbesCannotReachHandlersOrGrowState(t *testing.T) {
	t.Setenv("APP_MODE", "preview")

	template := domain.FreshState(time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC))
	base, err := store.Open(":memory:", template)
	if err != nil {
		t.Fatalf("open preview workspace: %v", err)
	}
	defer base.Close()
	workspace := store.ReadOnly(base)
	before, err := previewmode.StateFingerprint(workspace.Snapshot())
	if err != nil {
		t.Fatalf("fingerprint initial preview workspace: %v", err)
	}

	reached := 0
	mutationHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached++
		_ = workspace.Update(func(state *domain.State) error {
			state.Sessions = append(state.Sessions, domain.Session{ID: "ses_forbidden_growth"})
			return nil
		})
		w.WriteHeader(http.StatusNoContent)
	})
	handler := readOnlyPreviewGate()(organizerGate()(mutationHandler))

	actionPaths := []string{
		"/organizer/agenda/__actions/createSession",
		"/organizer/agenda/__actions/moveSession",
		"/organizer/agenda/__actions/unscheduleSession",
		"/organizer/agenda/__actions/publishAgenda",
		"/organizer/forms/__actions/createForm",
		"/organizer/review/__actions/createReviewPlan",
		"/organizer/communications/__actions/queueMessage",
		"/organizer/portal/__actions/createTask",
		"/organizer/integrations/__actions/liveSync",
		"/organizer/settings/__actions/saveEvent",
		"/portal-upload/spk_sample/task_headshot",
		"/submit/sample-cfp/__actions/submitProposal",
	}
	for _, path := range actionPaths {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader("probe=1"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("preview action POST %s status = %d, want 403", path, recorder.Code)
		}
		if recorder.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("preview action POST %s Cache-Control = %q, want no-store", path, recorder.Header().Get("Cache-Control"))
		}
		if !strings.Contains(recorder.Body.String(), "read-only preview") {
			t.Errorf("preview action POST %s did not explain the read-only boundary", path)
		}
	}
	if reached != 0 {
		t.Fatalf("%d blocked preview action probe(s) reached a mutation handler", reached)
	}

	after, err := previewmode.StateFingerprint(workspace.Snapshot())
	if err != nil {
		t.Fatalf("fingerprint final preview workspace: %v", err)
	}
	if after != before {
		t.Fatalf("preview workspace grew after blocked action probes: before=%s after=%s", before, after)
	}
}
