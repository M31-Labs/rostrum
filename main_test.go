package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	workspacearchive "github.com/m31-labs/rostrum/internal/archive"
	internalaudit "github.com/m31-labs/rostrum/internal/audit"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/identity"
	"github.com/m31-labs/rostrum/internal/store"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/session"
)

// TestMain seeds the package-wide appstate singleton once with an in-memory
// workspace, the same way main() does, so the handler tests below (which
// call appstate.MustGet() the same way the real handlers do) have state to
// read and mutate.
func TestMain(m *testing.M) {
	workspace, err := store.Open(":memory:", domain.Seed(time.Now().UTC()))
	if err != nil {
		panic(err)
	}
	appstate.Set(workspace)
	os.Exit(m.Run())
}

func testSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	manager, err := session.New("test-session-secret-at-least-32-bytes-long", session.Options{AllowInsecure: true})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	return manager
}

// signInAs returns a middleware that signs a user carrying role into the
// request's session before calling next. It must run after
// session.Manager.Middleware (which attaches the session store the sign-in
// writes to) and before auth.Manager.Middleware (which reads it back), the
// same ordering main.go uses in production.
func signInAs(authManager *auth.Manager, role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authManager.SignIn(r, auth.User{ID: "test-" + role, Email: "test-" + role + "@example.com", Roles: []string{role}})
			next.ServeHTTP(w, r)
		})
	}
}

func bindPortalSpeaker(speakerID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session.Current(r).Set(portalSpeakerSessionKey, speakerID)
			next.ServeHTTP(w, r)
		})
	}
}

func TestManagedMagicLinkRequestAcceptsFormDataWithoutDocumentRedirect(t *testing.T) {
	sessions := testSessionManager(t)
	manager := identity.New(sessions)
	sent := false
	links := manager.MagicLinks(auth.MagicLinkOptions{
		BaseURL: "http://localhost:8080",
		Sender: auth.MagicLinkSenderFunc(func(_ context.Context, delivery auth.MagicLinkDelivery) error {
			sent = delivery.Email == "organizer@example.com"
			return nil
		}),
	})
	handler := sessions.Middleware(manager.Middleware(managedMagicLinkRequest(links)))
	request := httptest.NewRequest(http.MethodPost, "/auth/magic-link", strings.NewReader("email=organizer%40example.com&next=%2Forganizer"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("managed magic-link status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if !sent {
		t.Fatal("managed magic-link request did not send a delivery")
	}
	if !strings.Contains(recorder.Body.String(), `"redirect":"/login"`) || !strings.Contains(recorder.Body.String(), "Check your email") {
		t.Fatalf("managed magic-link response = %s, want soft redirect and user-visible message", recorder.Body.String())
	}
}

// Interactive forms must use GoSX's managed-form protocol. The only raw
// lowercase forms left in source are agenda island forms carrying the same
// explicit data-gosx-form contract; all other form elements are <Form> or
// <ActionForm>. This protects the launch UX guarantee that ordinary actions
// do not trigger a document refresh after JavaScript has loaded.
func TestInteractiveFormsUseManagedProtocol(t *testing.T) {
	err := filepath.WalkDir("app", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".gsx" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		remaining := string(source)
		for {
			start := strings.Index(remaining, "<form")
			if start < 0 {
				return nil
			}
			remaining = remaining[start:]
			end := strings.Index(remaining, ">")
			if end < 0 {
				t.Fatalf("%s contains an unterminated raw form tag", path)
			}
			tag := remaining[:end+1]
			if !strings.Contains(tag, "data-gosx-form") {
				t.Fatalf("%s has an unmanaged raw form tag: %s", path, tag)
			}
			remaining = remaining[end+1:]
		}
	})
	if err != nil {
		t.Fatalf("scan interactive forms: %v", err)
	}
}

func testPortalWorkspace(t *testing.T) (domain.State, *store.JSONStore) {
	t.Helper()
	state := domain.FreshState(time.Now().UTC())
	state.Speakers = append(state.Speakers,
		domain.Speaker{ID: "spk_owner", FirstName: "Owner", LastName: "Speaker", Email: "owner@example.com"},
		domain.Speaker{ID: "spk_other", FirstName: "Other", LastName: "Speaker", Email: "other@example.com"},
	)
	state.Tasks = append(state.Tasks,
		domain.Task{ID: "task_headshot", Title: "Headshot", Type: "headshot", DueAt: time.Now().UTC().Add(24 * time.Hour), AssignedSpeakerIDs: []string{"spk_owner"}},
		domain.Task{ID: "task_slides", Title: "Slides", Type: "file", DueAt: time.Now().UTC().Add(24 * time.Hour), AssignedSpeakerIDs: []string{"spk_owner"}},
		domain.Task{ID: "task_other", Title: "Other task", Type: "file", DueAt: time.Now().UTC().Add(24 * time.Hour), AssignedSpeakerIDs: []string{"spk_other"}},
	)
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	appstate.Set(workspace)
	return state, workspace
}

func portalUploadRequest(t *testing.T, path, name string, contents []byte) *http.Request {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(contents); err != nil {
		t.Fatalf("write upload fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func workspaceImportRequest(t *testing.T, contents []byte) *http.Request {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("workspace", "rostrum-workspace.json")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(contents); err != nil {
		t.Fatalf("write workspace fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close workspace multipart body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/organizer/import/workspace", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	return request
}

func TestPortalUploadRequiresBoundOwnerOrMutatingOrganizer(t *testing.T) {
	testPortalWorkspace(t)
	root := t.TempDir()
	path := "/portal-upload/spk_owner/task_headshot"
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	manager := testSessionManager(t)
	authManager := identity.New(manager)
	handler := http.HandlerFunc(portalUpload(root))

	for _, test := range []struct {
		name string
		wrap func(http.Handler) http.Handler
	}{
		{name: "anonymous", wrap: func(next http.Handler) http.Handler { return next }},
		{name: "foreign speaker", wrap: bindPortalSpeaker("spk_other")},
		{name: "observer", wrap: signInAs(authManager, identity.RoleObserver)},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			wrapped := manager.Middleware(test.wrap(authManager.Middleware(handler)))
			wrapped.ServeHTTP(recorder, portalUploadRequest(t, path, "portrait.jpg", payload))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want non-enumerating 404; body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "data", "uploads")); !os.IsNotExist(err) {
		t.Fatalf("unauthorized upload prepared storage, stat err = %v", err)
	}
}

func TestPortalUploadChecksAssignmentImageBytesAndLimit(t *testing.T) {
	_, workspace := testPortalWorkspace(t)
	root := t.TempDir()
	manager := testSessionManager(t)
	handler := manager.Middleware(bindPortalSpeaker("spk_owner")(http.HandlerFunc(portalUpload(root))))

	// An assigned speaker cannot submit an unassigned task even with a valid
	// portal session. The response is intentionally indistinguishable from a
	// missing task and no disk write happens first.
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, portalUploadRequest(t, "/portal-upload/spk_owner/task_other", "slides.pdf", []byte("%PDF-1.7")))
	if unauthorized.Code != http.StatusNotFound {
		t.Fatalf("unassigned task status = %d, want 404", unauthorized.Code)
	}

	// A file named .jpg but containing text cannot enter the public-headshot
	// review pipeline.
	notImage := httptest.NewRecorder()
	handler.ServeHTTP(notImage, portalUploadRequest(t, "/portal-upload/spk_owner/task_headshot", "portrait.jpg", []byte("not an image")))
	if notImage.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("fake headshot status = %d, want 415", notImage.Code)
	}

	// The 10 MiB byte cap removes the partial destination before responding.
	overLimit := httptest.NewRecorder()
	handler.ServeHTTP(overLimit, portalUploadRequest(t, "/portal-upload/spk_owner/task_slides", "deck.pdf", bytes.Repeat([]byte("a"), maxUploadBytes+1)))
	if overLimit.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over-limit status = %d, want 413; body=%s", overLimit.Code, overLimit.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(root, "data", "uploads"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read upload dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected uploads left %d files on disk", len(entries))
	}
	if len(workspace.Snapshot().TaskCompletions) != 0 {
		t.Fatalf("rejected uploads created task completions: %+v", workspace.Snapshot().TaskCompletions)
	}
}

func TestPortalUploadStoresAuthorizedHeadshotAndBindsProfileURL(t *testing.T) {
	_, workspace := testPortalWorkspace(t)
	root := t.TempDir()
	manager := testSessionManager(t)
	handler := manager.Middleware(bindPortalSpeaker("spk_owner")(http.HandlerFunc(portalUpload(root))))
	recorder := httptest.NewRecorder()
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	handler.ServeHTTP(recorder, portalUploadRequest(t, "/portal-upload/spk_owner/task_headshot", "portrait.jpg", payload))
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("authorized upload status = %d, want 303; body=%s", recorder.Code, recorder.Body.String())
	}
	snapshot := workspace.Snapshot()
	completion, found := snapshot.Completion("task_headshot", "spk_owner")
	if !found || completion.Status != domain.TaskSubmitted || completion.StoredPath == "" {
		t.Fatalf("completion = %+v, found=%v; want submitted stored headshot", completion, found)
	}
	speaker, found := snapshot.Speaker("spk_owner")
	if !found || speaker.HeadshotURL != "/portal-file/"+completion.ID {
		t.Fatalf("speaker headshot URL = %q, want completion portal URL", speaker.HeadshotURL)
	}
	stored, err := os.ReadFile(completion.StoredPath)
	if err != nil {
		t.Fatalf("read stored headshot: %v", err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("stored headshot bytes = %x, want %x", stored, payload)
	}
}

func TestPortalUploadReplacesAndCleansSupersededPrivateFile(t *testing.T) {
	_, workspace := testPortalWorkspace(t)
	root := t.TempDir()
	manager := testSessionManager(t)
	handler := manager.Middleware(bindPortalSpeaker("spk_owner")(http.HandlerFunc(portalUpload(root))))
	firstPayload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', '1'}
	secondPayload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', '2'}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, portalUploadRequest(t, "/portal-upload/spk_owner/task_headshot", "portrait.jpg", firstPayload))
	if first.Code != http.StatusSeeOther {
		t.Fatalf("first upload status = %d, want 303; body=%s", first.Code, first.Body.String())
	}
	snapshot := workspace.Snapshot()
	oldCompletion, found := snapshot.Completion("task_headshot", "spk_owner")
	if !found || oldCompletion.StoredPath == "" {
		t.Fatalf("first completion = %+v, found=%v; want stored path", oldCompletion, found)
	}
	oldPath := oldCompletion.StoredPath
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("first upload missing at %s: %v", oldPath, err)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, portalUploadRequest(t, "/portal-upload/spk_owner/task_headshot", "portrait-revised.jpg", secondPayload))
	if second.Code != http.StatusSeeOther {
		t.Fatalf("replacement upload status = %d, want 303; body=%s", second.Code, second.Body.String())
	}
	snapshot = workspace.Snapshot()
	newCompletion, found := snapshot.Completion("task_headshot", "spk_owner")
	if !found || newCompletion.StoredPath == "" || newCompletion.StoredPath == oldPath {
		t.Fatalf("replacement completion = %+v, found=%v; want a new stored path", newCompletion, found)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("superseded private upload still exists, stat err = %v", err)
	}
	if _, err := os.Stat(newCompletion.StoredPath); err != nil {
		t.Fatalf("replacement upload missing at %s: %v", newCompletion.StoredPath, err)
	}
}

func TestRemoveSupersededUploadProtectsSharedAndOutsidePaths(t *testing.T) {
	_, workspace := testPortalWorkspace(t)
	root := t.TempDir()
	uploadDir := filepath.Join(root, "data", "uploads")
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		t.Fatalf("create upload dir: %v", err)
	}
	oldPath := filepath.Join(uploadDir, "old.pdf")
	replacementPath := filepath.Join(uploadDir, "replacement.pdf")
	outsidePath := filepath.Join(root, "outside.pdf")
	for _, path := range []string{oldPath, replacementPath, outsidePath} {
		if err := os.WriteFile(path, []byte("upload"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	now := time.Now().UTC()
	if err := workspace.Update(func(state *domain.State) error {
		state.TaskCompletions = append(state.TaskCompletions, domain.TaskCompletion{
			ID: "done_shared", TaskID: "task_slides", SpeakerID: "spk_owner", Status: domain.TaskSubmitted,
			FileName: "old.pdf", ContentType: "application/pdf", StoredPath: filepath.ToSlash(oldPath), CompletedAt: now, UpdatedAt: now,
		})
		return nil
	}); err != nil {
		t.Fatalf("seed shared completion: %v", err)
	}

	removeSupersededUpload(root, filepath.ToSlash(oldPath), filepath.ToSlash(replacementPath))
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("shared upload removed despite a current reference: %v", err)
	}
	removeSupersededUpload(root, filepath.ToSlash(outsidePath), filepath.ToSlash(replacementPath))
	if _, err := os.Stat(outsidePath); err != nil {
		t.Fatalf("outside path changed during cleanup: %v", err)
	}

	if err := workspace.Update(func(state *domain.State) error {
		state.TaskCompletions[0].StoredPath = ""
		return nil
	}); err != nil {
		t.Fatalf("clear shared completion: %v", err)
	}
	removeSupersededUpload(root, filepath.ToSlash(oldPath), filepath.ToSlash(replacementPath))
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("unreferenced upload was not removed, stat err = %v", err)
	}
}

func TestSecurityHeadersAuthorizeOnlyTheGoSXInlineRuntime(t *testing.T) {
	t.Setenv("APP_MODE", "live")
	hash := navigationScriptCSPHash()
	if !strings.HasPrefix(hash, "'sha256-") {
		t.Fatalf("navigation CSP hash = %q", hash)
	}
	handler := securityHeaders("https://rostrum.example", hash)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	policy := recorder.Header().Get("Content-Security-Policy")
	for _, required := range []string{
		"script-src 'self' 'wasm-unsafe-eval' " + hash,
		"frame-src 'self'",
		"form-action 'self'",
		"upgrade-insecure-requests",
	} {
		if !strings.Contains(policy, required) {
			t.Fatalf("CSP missing %q: %s", required, policy)
		}
	}
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("CSP should not authorize arbitrary inline scripts: %s", policy)
	}
	if strings.Contains(policy, "'unsafe-eval'") {
		t.Fatalf("CSP should authorize only WebAssembly compilation, not generic eval: %s", policy)
	}
}

func TestReadOnlyDemoGateBlocksMutationsAndSensitiveSurfaces(t *testing.T) {
	t.Setenv("APP_MODE", "demo")
	handler := readOnlyDemoGate()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	cases := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodPost, "/submit/systems-forum-cfp", http.StatusForbidden},
		{http.MethodPut, "/organizer/settings", http.StatusForbidden},
		{http.MethodPatch, "/review/token", http.StatusForbidden},
		{http.MethodDelete, "/portal-upload/speaker/task", http.StatusForbidden},
		{http.MethodGet, "/organizer/export/workspace.json", http.StatusForbidden},
		{http.MethodGet, "/organizer/export/approved-uploads.zip", http.StatusForbidden},
		{http.MethodGet, "/organizer/import/workspace", http.StatusForbidden},
		{http.MethodGet, "/portal-file/done_demo", http.StatusForbidden},
		{http.MethodGet, "/auth/magic-link", http.StatusForbidden},
		{http.MethodGet, "/setup", http.StatusForbidden},
		{http.MethodGet, "/demo/reset", http.StatusForbidden},
		{http.MethodGet, "/organizer", http.StatusNoContent},
		{http.MethodGet, "/review/token", http.StatusNoContent},
		{http.MethodGet, "/login", http.StatusNoContent},
		{http.MethodGet, "/public/m31-systems-forum-2026/agenda", http.StatusNoContent},
	}
	for _, test := range cases {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.want, recorder.Body.String())
			}
			if test.want == http.StatusForbidden && recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("blocked response Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestOrganizerGateAllowsAnonymousReadOnlyDemoInspection(t *testing.T) {
	t.Setenv("APP_MODE", "demo")
	t.Setenv("GOSX_STATIC_EXPORT", "")
	handler := organizerGate()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/organizer/agenda", nil))
	if read.Code != http.StatusNoContent {
		t.Fatalf("anonymous demo organizer GET status = %d, want 204", read.Code)
	}
	write := httptest.NewRecorder()
	handler.ServeHTTP(write, httptest.NewRequest(http.MethodPost, "/organizer/agenda", nil))
	if write.Code != http.StatusForbidden {
		t.Fatalf("anonymous demo organizer POST status = %d, want 403", write.Code)
	}
}

func TestSecurityHeadersMarkReadOnlyDemoResponsesNoindex(t *testing.T) {
	t.Setenv("APP_MODE", "demo")
	handler := securityHeaders("https://demo.rostrum.example", navigationScriptCSPHash())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/", "/organizer", "/public/m31-systems-forum-2026/agenda", "/login"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if got := recorder.Header().Get("X-Robots-Tag"); got != "noindex, nofollow, noarchive" {
			t.Errorf("path %s X-Robots-Tag = %q, want noindex policy", path, got)
		}
	}
}

func TestFrameAncestorsScopedToPublicRoutes(t *testing.T) {
	t.Setenv("APP_MODE", "live")
	hash := navigationScriptCSPHash()
	handler := securityHeaders("https://rostrum.example", hash)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		path string
		want string
	}{
		{"/public/summit/agenda", "frame-ancestors *"},
		{"/organizer", "frame-ancestors 'none'"},
		{"/organizer/settings", "frame-ancestors 'none'"},
		{"/portal/spk_maya", "frame-ancestors 'none'"},
		{"/", "frame-ancestors 'none'"},
	}
	for _, test := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		policy := recorder.Header().Get("Content-Security-Policy")
		if !strings.Contains(policy, test.want) {
			t.Errorf("path %s: CSP = %q, want to contain %q", test.path, policy, test.want)
		}
	}
}

func TestSubmissionsCSVRequiresOrganizerSessionAndEscapesFormulas(t *testing.T) {
	manager := testSessionManager(t)
	authManager := identity.New(manager)

	var submissionID string
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		now := time.Now().UTC()
		submissionID = domain.NewID("sub")
		state.Submissions = append(state.Submissions, domain.Submission{
			ID:          submissionID,
			EventID:     state.Event.ID,
			Title:       `=HYPERLINK("https://evil.example")`,
			Status:      domain.SubmissionPending,
			SubmittedAt: now,
			UpdatedAt:   now,
		})
		return nil
	}); err != nil {
		t.Fatalf("seed submission: %v", err)
	}

	// SE-5b: a cookie-less request must be refused, not served a row of data.
	unauth := httptest.NewRequest(http.MethodGet, "/organizer/export/submissions.csv", nil)
	unauthRecorder := httptest.NewRecorder()
	manager.Middleware(authManager.Middleware(http.HandlerFunc(submissionsCSV))).ServeHTTP(unauthRecorder, unauth)
	if unauthRecorder.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated export status = %d, want 403", unauthRecorder.Code)
	}
	if strings.Contains(unauthRecorder.Body.String(), submissionID) {
		t.Fatal("unauthenticated export leaked submission data")
	}

	// An observer session reaches /organizer but never this export: it
	// carries speaker PII the read-only role must not receive.
	observed := httptest.NewRequest(http.MethodGet, "/organizer/export/submissions.csv", nil)
	observedRecorder := httptest.NewRecorder()
	manager.Middleware(signInAs(authManager, identity.RoleObserver)(authManager.Middleware(http.HandlerFunc(submissionsCSV)))).ServeHTTP(observedRecorder, observed)
	if observedRecorder.Code != http.StatusForbidden {
		t.Fatalf("observer export status = %d, want 403", observedRecorder.Code)
	}

	// An organizer session may fetch the export, and a formula-looking title
	// exports neutralized (SE-5).
	authed := httptest.NewRequest(http.MethodGet, "/organizer/export/submissions.csv", nil)
	authedRecorder := httptest.NewRecorder()
	manager.Middleware(signInAs(authManager, identity.RoleOrganizer)(authManager.Middleware(http.HandlerFunc(submissionsCSV)))).ServeHTTP(authedRecorder, authed)
	if authedRecorder.Code != http.StatusOK {
		t.Fatalf("authorized export status = %d, want 200", authedRecorder.Code)
	}
	body := authedRecorder.Body.String()
	if !strings.Contains(body, `'=HYPERLINK`) {
		t.Fatalf("export did not neutralize a formula-looking title: %s", body)
	}
}

func TestWorkspaceExportRequiresMutatingRoleAndRecordsAccess(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	state.AuthMagicLinks = []domain.AuthMagicLink{{Token: "transient-token", Email: "owner@example.com", ExpiresAt: time.Now().Add(time.Hour)}}
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)

	manager := testSessionManager(t)
	authManager := identity.New(manager)
	handler := http.HandlerFunc(workspaceExport)

	for _, test := range []struct {
		name string
		wrap func(http.Handler) http.Handler
	}{
		{name: "anonymous", wrap: func(next http.Handler) http.Handler { return next }},
		{name: "observer", wrap: signInAs(authManager, identity.RoleObserver)},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			wrapped := manager.Middleware(test.wrap(authManager.Middleware(handler)))
			wrapped.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/organizer/export/workspace.json", nil))
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}

	recorder := httptest.NewRecorder()
	wrapped := manager.Middleware(signInAs(authManager, identity.RoleOrganizer)(authManager.Middleware(handler)))
	wrapped.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/organizer/export/workspace.json", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private no-store", got)
	}
	exported, err := workspacearchive.Decode(recorder.Body.Bytes())
	if err != nil {
		t.Fatalf("decode authorized export: %v", err)
	}
	if exported.Event.ID != state.Event.ID || len(exported.AuthMagicLinks) != 0 {
		t.Fatalf("exported state = %#v, want original event with transient links stripped", exported)
	}

	snapshot := workspace.Snapshot()
	if count := len(snapshot.AuditEvents); count != 1 || snapshot.AuditEvents[0].Action != "export.workspace" {
		t.Fatalf("audit events = %#v, want one workspace export", snapshot.AuditEvents)
	}
}

func TestWorkspaceImportValidatesBeforeBackupAndRetainsLocalIdentity(t *testing.T) {
	source := domain.Seed(time.Now().UTC())
	source.Event.Name = "Imported program"
	exportData, err := workspacearchive.Marshal(source)
	if err != nil {
		t.Fatalf("marshal source export: %v", err)
	}

	current := domain.Seed(time.Now().UTC())
	current.Event.Name = "Current program"
	current.Principals = []domain.Principal{{ID: "principal_current", Email: "current@example.com"}}
	current.AuthPasskeys = []domain.AuthPasskey{{ID: "passkey_current"}}
	workspace, err := store.Open(":memory:", current)
	if err != nil {
		t.Fatalf("open current workspace: %v", err)
	}
	appstate.Set(workspace)

	manager := testSessionManager(t)
	authManager := identity.New(manager)
	backups := filepath.Join(t.TempDir(), "backups")
	handler := manager.Middleware(signInAs(authManager, identity.RoleOrganizer)(authManager.Middleware(workspaceImport(t.TempDir(), backups))))

	// A stale checksum is rejected before the backup directory is created or
	// the in-memory workspace is touched.
	tampered := bytes.Replace(exportData, []byte("Imported program"), []byte("Tampered program"), 1)
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, workspaceImportRequest(t, tampered))
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("tampered import status = %d, want 400; body=%s", rejected.Code, rejected.Body.String())
	}
	if _, err := os.Stat(backups); !os.IsNotExist(err) {
		t.Fatalf("rejected import created backup path, stat err = %v", err)
	}
	if got := workspace.Snapshot().Event.Name; got != "Current program" {
		t.Fatalf("rejected import changed event name to %q", got)
	}

	accepted := httptest.NewRecorder()
	handler.ServeHTTP(accepted, workspaceImportRequest(t, exportData))
	if accepted.Code != http.StatusOK {
		t.Fatalf("valid import status = %d, want 200; body=%s", accepted.Code, accepted.Body.String())
	}
	snapshot := workspace.Snapshot()
	if snapshot.Event.Name != "Imported program" {
		t.Fatalf("restored event name = %q, want imported program", snapshot.Event.Name)
	}
	if len(snapshot.Principals) != 1 || snapshot.Principals[0].ID != "principal_current" || len(snapshot.AuthPasskeys) != 1 || snapshot.AuthPasskeys[0].ID != "passkey_current" {
		t.Fatalf("restored identity = principals=%#v passkeys=%#v, want local identity", snapshot.Principals, snapshot.AuthPasskeys)
	}
	if count := len(snapshot.AuditEvents); count == 0 || snapshot.AuditEvents[count-1].Action != "import.workspace" {
		t.Fatalf("restored audit trail = %#v, want final import.workspace record", snapshot.AuditEvents)
	}

	paths, err := filepath.Glob(filepath.Join(backups, "rostrum-*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("backups = %v, err = %v; want one pre-import backup", paths, err)
	}
	backupState, err := workspacearchive.Decode(mustReadFile(t, paths[0]))
	if err != nil {
		t.Fatalf("decode pre-import backup: %v", err)
	}
	if backupState.Event.Name != "Current program" || len(backupState.Principals) != 1 || backupState.Principals[0].ID != "principal_current" {
		t.Fatalf("pre-import backup = %#v, want exact current workspace", backupState)
	}
}

func TestWorkspaceArchiveRequiresMutatingRoleAndIncludesAssets(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	root := t.TempDir()
	uploads := filepath.Join(root, "data", "uploads")
	if err := os.MkdirAll(uploads, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "upload_example.pdf"), []byte("slides"), 0o600); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(root, "data", "audit.log")
	ledger, err := internalaudit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Append(internalaudit.Event{Kind: "test.archive", Subject: "workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	manager := testSessionManager(t)
	authManager := identity.New(manager)
	handler := workspaceArchive(root, auditPath)
	unauthorized := httptest.NewRecorder()
	manager.Middleware(authManager.Middleware(handler)).ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/organizer/export/archive.tar.gz", nil))
	if unauthorized.Code != http.StatusForbidden {
		t.Fatalf("unauthorized archive status = %d, want 403", unauthorized.Code)
	}

	archiveResponse := httptest.NewRecorder()
	wrapped := manager.Middleware(signInAs(authManager, identity.RoleOrganizer)(authManager.Middleware(handler)))
	wrapped.ServeHTTP(archiveResponse, httptest.NewRequest(http.MethodGet, "/organizer/export/archive.tar.gz", nil))
	if archiveResponse.Code != http.StatusOK {
		t.Fatalf("authorized archive status = %d, want 200; body=%s", archiveResponse.Code, archiveResponse.Body.String())
	}
	reader, err := gzip.NewReader(bytes.NewReader(archiveResponse.Body.Bytes()))
	if err != nil {
		t.Fatalf("open archive gzip: %v", err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	files := map[string]string{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		contents, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatalf("read %s: %v", header.Name, err)
		}
		files[header.Name] = string(contents)
	}
	if files["workspace.json"] == "" || files["uploads/upload_example.pdf"] != "slides" || files["audit/audit.log"] == "" {
		t.Fatalf("archive files = %#v, want workspace, uploads, and audit ledger", files)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestCalendarDownloadRequiresKeyOrSession(t *testing.T) {
	manager := testSessionManager(t)

	snapshot := appstate.MustGet().Snapshot()
	if len(snapshot.Speakers) == 0 {
		t.Fatal("seed has no speakers to exercise the calendar gate")
	}
	speakerID := snapshot.Speakers[0].ID

	// SE-6: an unkeyed, unauthenticated fetch gets the friendly not-found,
	// never a calendar file.
	unauth := httptest.NewRequest(http.MethodGet, "/calendar/"+speakerID+".ics", nil)
	unauthRecorder := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(calendarDownload)).ServeHTTP(unauthRecorder, unauth)
	if unauthRecorder.Code != http.StatusNotFound {
		t.Fatalf("unkeyed calendar fetch status = %d, want 404", unauthRecorder.Code)
	}

	// A valid signed portal token for this speaker (the emailed
	// subscribe-link flow) unlocks the same route.
	signed := token.New().Sign(speakerID)
	keyed := httptest.NewRequest(http.MethodGet, "/calendar/"+speakerID+".ics?key="+signed, nil)
	keyedRecorder := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(calendarDownload)).ServeHTTP(keyedRecorder, keyed)
	if keyedRecorder.Code != http.StatusOK {
		t.Fatalf("keyed calendar fetch status = %d, want 200", keyedRecorder.Code)
	}

	// A token signed for a different speaker must not unlock this one.
	otherKeyed := httptest.NewRequest(http.MethodGet, "/calendar/"+speakerID+".ics?key="+token.New().Sign("spk_someone_else"), nil)
	otherRecorder := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(calendarDownload)).ServeHTTP(otherRecorder, otherKeyed)
	if otherRecorder.Code != http.StatusNotFound {
		t.Fatalf("mismatched-speaker key status = %d, want 404", otherRecorder.Code)
	}
}

func TestClearUploadsRemovesFilesButKeepsDirectory(t *testing.T) {
	root := t.TempDir()
	uploadDir := filepath.Join(root, "data", "uploads")
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		t.Fatalf("prepare upload dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "upload_abc.pdf"), []byte("content"), 0o600); err != nil {
		t.Fatalf("seed upload file: %v", err)
	}

	clearUploads(root)

	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatalf("read upload dir after clear: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("uploads directory still has %d entries after clearUploads", len(entries))
	}

	// A missing uploads directory (a fresh workspace) must not be an error.
	clearUploads(filepath.Join(root, "never-created"))
}

func TestBrowserBehaviorHasNoBespokeJavaScript(t *testing.T) {
	if _, err := os.Stat(filepath.Join("public", "app.js")); !os.IsNotExist(err) {
		t.Fatalf("public/app.js must not exist, stat error = %v", err)
	}

	walkFiles(t, "public", func(path string) {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".js" || ext == ".mjs" {
			t.Errorf("authored browser JavaScript is forbidden: %s", path)
		}
	})
	walkFiles(t, "app", func(path string) {
		if !strings.EqualFold(filepath.Ext(path), ".gsx") {
			return
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := strings.ToLower(string(source))
		for _, forbidden := range []string{"<script", "javascript:", "data-auto-submit"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains forbidden browser source %q", path, forbidden)
			}
		}
	})

	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mainSource), "app.js") {
		t.Fatal("main.go must not inject or serve app.js")
	}
}

func TestFocusedGoSXIslandInventory(t *testing.T) {
	expected := map[string]string{
		filepath.Join("app", "organizer", "layout.gsx"):                "func WorkspaceChrome() Node",
		filepath.Join("app", "organizer", "agenda", "page.gsx"):        "func AgendaBoard(props any) Node",
		filepath.Join("app", "organizer", "embeds", "page.gsx"):        "func EmbedClipboard(props any) Node",
		filepath.Join("app", "public", "[slug]", "agenda", "page.gsx"): "func PublicItinerary(props any) Node",
		filepath.Join("app", "submit", "[slug]", "page.gsx"):           "func ConditionalFormatFields(props any) Node",
	}
	for path, declaration := range expected {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(source)
		if !strings.Contains(text, "//gosx:island\n"+declaration) {
			t.Errorf("%s does not declare the expected local island %q", path, declaration)
		}
	}
	reviewSource, err := os.ReadFile(filepath.Join("app", "organizer", "review", "page.gsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{"data-gosx-disclosure-target", "data-gosx-disclosure-close", "data-gosx-disclosure-backdrop"} {
		if !strings.Contains(string(reviewSource), contract) {
			t.Errorf("review method dialog is missing declarative %s", contract)
		}
	}
}

func TestManagedActionFormsExposeLocalResultTargets(t *testing.T) {
	actionForm := regexp.MustCompile(`(?s)<ActionForm\b.*?</ActionForm>`)
	count := 0
	walkFiles(t, "app", func(path string) {
		if !strings.EqualFold(filepath.Ext(path), ".gsx") {
			return
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, form := range actionForm.FindAllString(string(source), -1) {
			count++
			if !strings.Contains(form, `class="form-status`) && !strings.Contains(form, `class="action-message`) {
				t.Errorf("managed ActionForm in %s has no form-local result target", path)
			}
		}
	})
	if count < 14 {
		t.Fatalf("managed ActionForm inventory = %d, want at least 14", count)
	}

	agendaSource, err := os.ReadFile(filepath.Join("app", "organizer", "agenda", "page.gsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{"agenda-drag-form", "agenda-drag-status", "agenda-drag-error"} {
		if !strings.Contains(string(agendaSource), contract) {
			t.Errorf("managed agenda drag form is missing %s", contract)
		}
	}
}

func TestBuiltOutputHasNoLegacyAppScript(t *testing.T) {
	if _, err := os.Stat("dist"); os.IsNotExist(err) {
		t.Skip("dist is created by the release build")
	} else if err != nil {
		t.Fatal(err)
	}
	walkFiles(t, "dist", func(path string) {
		if strings.EqualFold(filepath.Base(path), "app.js") {
			t.Errorf("legacy app.js survived the build: %s", path)
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".js" || extension == ".mjs" {
			normalized := filepath.ToSlash(path)
			inPublic := strings.HasPrefix(normalized, "dist/public/")
			inStatic := strings.HasPrefix(normalized, "dist/static/")
			isGoSXRuntime := strings.HasPrefix(normalized, "dist/static/gosx/assets/runtime/")
			if inPublic || (inStatic && !isGoSXRuntime) {
				t.Errorf("bespoke browser JavaScript survived the build: %s", path)
			}
		}
		if !strings.EqualFold(filepath.Ext(path), ".html") {
			return
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(strings.ToLower(string(source)), "app.js") {
			t.Errorf("built page still references app.js: %s", path)
		}
	})
	walkFiles(t, filepath.Join("dist", "assets", "islands"), func(path string) {
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".js" || extension == ".mjs" || extension == ".json" {
			t.Errorf("island output must be binary GoSX IR, found %s", path)
		}
	})
}

func walkFiles(t *testing.T, root string, visit func(string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			visit(path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
