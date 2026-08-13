package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/appstate"
	workspacearchive "github.com/m31-labs/rostrum/internal/archive"
	internalaudit "github.com/m31-labs/rostrum/internal/audit"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/identity"
	"github.com/m31-labs/rostrum/internal/store"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func TestRostrumFileRouterDeclaresEnglishLanguageAndPreservesContract(t *testing.T) {
	router := route.NewRouter()
	router.SetLayout(rostrumRouteDocument)
	router.Add(route.Route{Pattern: "/", Handler: func(ctx *route.RouteContext) gosx.Node {
		ctx.SetMetadata(server.Metadata{Title: server.Title{Absolute: "Rostrum test"}})
		ctx.AddHead(gosx.El("meta", gosx.Attrs(gosx.Attr("name", "description"), gosx.Attr("content", "Test page"))))
		return gosx.El("main", gosx.Text("Ready"))
	}})
	handler, err := router.BuildChecked()
	if err != nil {
		t.Fatalf("BuildChecked: %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	html := response.Body.String()
	for _, want := range []string{
		`<!DOCTYPE html>`,
		`<html lang="en" data-gosx-document="true">`,
		`<meta charset="utf-8">`,
		`<meta name="viewport" content="width=device-width, initial-scale=1">`,
		`<title>Rostrum test</title>`,
		`<body data-gosx-document-body="true" data-gosx-enhancement-layer="html">`,
		`<main>Ready</main>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("document missing %q: %s", want, html)
		}
	}
}

func TestSessionOptionsEncryptCookiePayloads(t *testing.T) {
	localOptions := sessionOptions(true)
	if !localOptions.Encrypt || localOptions.Secure || !localOptions.AllowInsecure {
		t.Fatalf("local session options = %+v", localOptions)
	}
	publicOptions := sessionOptions(false)
	if !publicOptions.Encrypt || !publicOptions.Secure || publicOptions.AllowInsecure {
		t.Fatalf("public session options = %+v", publicOptions)
	}

	manager, err := session.New("encrypted-session-test-secret-at-least-32-bytes", localOptions)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Current(r).Set("email", "organizer@example.com")
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !strings.HasPrefix(cookies[0].Value, "v2.") {
		t.Fatalf("expected one encrypted v2 session cookie, got %+v", cookies)
	}
}

// TestMain seeds the package-wide appstate singleton once with an in-memory
// workspace, the same way main() does, so the handler tests below (which
// call appstate.MustGet() the same way the real handlers do) have state to
// read and mutate.
func TestMain(m *testing.M) {
	workspace, err := store.Open(":memory:", fixture.Seed(time.Now().UTC()))
	if err != nil {
		panic(err)
	}
	appstate.Set(workspace)
	os.Exit(m.Run())
}

func TestLoadInitialWorkspaceDefaultsAndRejectsUnknownSelection(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	fresh, err := loadInitialWorkspace(t.TempDir(), "", "", "", "", now)
	if err != nil {
		t.Fatalf("load default workspace: %v", err)
	}
	if fresh.State.Event.Name != domain.FreshState(now).Event.Name || fresh.TemplatePath != "" {
		t.Fatalf("default workspace = %+v, want built-in fresh state", fresh)
	}
	empty, err := loadInitialWorkspace(t.TempDir(), "empty", "", "", "", now)
	if err != nil {
		t.Fatalf("load empty workspace: %v", err)
	}
	if len(empty.State.Forms) != 0 {
		t.Fatalf("empty workspace has %d forms, want none", len(empty.State.Forms))
	}
	if _, err := loadInitialWorkspace(t.TempDir(), "demo", "", "", "", now); err == nil || !strings.Contains(err.Error(), "fresh or empty") {
		t.Fatalf("unknown INITIAL_WORKSPACE error = %v", err)
	}
}

func TestLoadInitialWorkspaceStrictTemplateAndChecksumSources(t *testing.T) {
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "fixtures")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(domain.FreshState(now))
	if err != nil {
		t.Fatalf("marshal workspace: %v", err)
	}
	workspacePath := filepath.Join(fixtureDir, "workspace.json")
	if err := os.WriteFile(workspacePath, raw, 0o600); err != nil {
		t.Fatalf("write workspace: %v", err)
	}
	digest := sha256.Sum256(raw)
	wantChecksum := hex.EncodeToString(digest[:])

	inline, err := loadInitialWorkspace(root, "fresh", "fixtures/workspace.json", wantChecksum, "", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("load inline-pinned workspace: %v", err)
	}
	if inline.TemplatePath != workspacePath || inline.ExpectedSHA256 != wantChecksum || inline.ChecksumFile != "" {
		t.Fatalf("inline source = %+v", inline)
	}

	checksumPath := filepath.Join(fixtureDir, "workspace.sha256")
	if err := os.WriteFile(checksumPath, []byte(wantChecksum+"\n"), 0o600); err != nil {
		t.Fatalf("write checksum: %v", err)
	}
	fromFile, err := loadInitialWorkspace(root, "empty", "fixtures/workspace.json", "", "fixtures/workspace.sha256", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("load file-pinned workspace: %v", err)
	}
	if fromFile.ChecksumFile != checksumPath || fromFile.ExpectedSHA256 != wantChecksum {
		t.Fatalf("checksum file source = %+v", fromFile)
	}

	if _, err := loadInitialWorkspace(root, "fresh", workspacePath, wantChecksum, checksumPath, now); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("dual checksum source error = %v", err)
	}
	if _, err := loadInitialWorkspace(root, "fresh", workspacePath, strings.Repeat("0", 64), "", now); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched checksum error = %v", err)
	}
}

func TestLoadInitialWorkspaceRejectsInvalidPathJSONAndState(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	if _, err := loadInitialWorkspace(root, "fresh", "missing.json", "", "", now); err == nil || !strings.Contains(err.Error(), "read INITIAL_WORKSPACE_PATH") {
		t.Fatalf("missing path error = %v", err)
	}

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "invalid json", raw: []byte(`{"event":`), want: "decode INITIAL_WORKSPACE_PATH"},
		{name: "unknown field", raw: append(mustMarshalState(t, domain.FreshState(now))[:len(mustMarshalState(t, domain.FreshState(now)))-1], []byte(`,"unexpected":true}`)...), want: "unknown field"},
		{name: "invalid state", raw: mustMarshalState(t, func() domain.State { state := domain.FreshState(now); state.Event.Name = ""; return state }()), want: "event id, name, and slug"},
		{name: "trailing json", raw: append(mustMarshalState(t, domain.FreshState(now)), []byte(` {}`)...), want: "trailing content"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, strings.ReplaceAll(test.name, " ", "-")+".json")
			if err := os.WriteFile(path, test.raw, 0o600); err != nil {
				t.Fatalf("write invalid fixture: %v", err)
			}
			if _, err := loadInitialWorkspace(root, "fresh", path, "", "", now); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("load error = %v, want %q", err, test.want)
			}
		})
	}

	if _, err := loadInitialWorkspace(root, "fresh", "", strings.Repeat("a", 64), "", now); err == nil || !strings.Contains(err.Error(), "requires INITIAL_WORKSPACE_PATH") {
		t.Fatalf("orphan checksum error = %v", err)
	}
}

func mustMarshalState(t *testing.T, state domain.State) []byte {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	return raw
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

func TestValidateRuntimePosture(t *testing.T) {
	strongSecret := "unique-test-session-secret-at-least-32-characters"
	tests := []struct {
		name         string
		publicURL    string
		appEnv       string
		dataPath     string
		secret       string
		staticExport string
		wantError    bool
	}{
		{name: "local development", publicURL: "http://127.0.0.1:8080", appEnv: "development", dataPath: ":memory:", secret: developmentSessionSecret},
		{name: "production https durable", publicURL: "https://program.example.com", appEnv: "production", dataPath: "/srv/rostrum.json", secret: strongSecret},
		{name: "public development still strict", publicURL: "https://program.example.com", appEnv: "development", dataPath: "/srv/rostrum.json", secret: strongSecret},
		{name: "public weak secret", publicURL: "https://program.example.com", appEnv: "development", dataPath: "/srv/rostrum.json", secret: "short", wantError: true},
		{name: "public plain http", publicURL: "http://program.example.com", appEnv: "development", dataPath: "/srv/rostrum.json", secret: strongSecret, wantError: true},
		{name: "public memory state", publicURL: "https://program.example.com", appEnv: "development", dataPath: ":memory:", secret: strongSecret, wantError: true},
		{name: "public static export bypass", publicURL: "https://program.example.com", appEnv: "development", dataPath: "/srv/rostrum.json", secret: strongSecret, staticExport: "1", wantError: true},
		{name: "production loopback remains strict", publicURL: "http://localhost:8080", appEnv: "production", dataPath: "/srv/rostrum.json", secret: strongSecret, wantError: true},
		{name: "unknown environment", publicURL: "http://localhost:8080", appEnv: "staging", dataPath: ":memory:", secret: developmentSessionSecret, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateRuntimePosture(test.publicURL, test.appEnv, test.dataPath, test.secret, test.staticExport)
			if (err != nil) != test.wantError {
				t.Fatalf("validateRuntimePosture() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestResetWorkspaceRequiresMutatingRoleAndFormSecret(t *testing.T) {
	initial := fixture.Seed(time.Now().UTC())
	initial.Event.Name = "Reset target"
	workspace, err := store.Open(":memory:", initial)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	t.Cleanup(func() { _ = workspace.Close() })

	uploads := filepath.Join(t.TempDir(), "uploads")
	if err := os.MkdirAll(uploads, 0o700); err != nil {
		t.Fatalf("create uploads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "proof.txt"), []byte("proof"), 0o600); err != nil {
		t.Fatalf("write proof upload: %v", err)
	}

	t.Setenv("APP_ENV", "development")
	t.Setenv("RESET_SECRET", "reset-only-from-form")
	manager := testSessionManager(t)
	authManager := identity.New(manager)
	handler := http.HandlerFunc(resetWorkspace(uploads))

	request := func(target, body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	for _, test := range []struct {
		name string
		wrap func(http.Handler) http.Handler
	}{
		{name: "anonymous", wrap: func(next http.Handler) http.Handler { return next }},
		{name: "observer", wrap: signInAs(authManager, identity.RoleObserver)},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			wrapped := manager.Middleware(test.wrap(authManager.Middleware(handler)))
			wrapped.ServeHTTP(response, request("/workspace/reset", "secret=reset-only-from-form"))
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", response.Code, response.Body.String())
			}
		})
	}

	organizer := manager.Middleware(signInAs(authManager, identity.RoleOrganizer)(authManager.Middleware(handler)))
	queryOnly := httptest.NewRecorder()
	organizer.ServeHTTP(queryOnly, request("/workspace/reset?secret=reset-only-from-form", ""))
	if queryOnly.Code != http.StatusNotFound {
		t.Fatalf("query-only secret status = %d, want 404", queryOnly.Code)
	}
	if _, err := os.Stat(filepath.Join(uploads, "proof.txt")); err != nil {
		t.Fatalf("rejected reset removed upload: %v", err)
	}

	accepted := httptest.NewRecorder()
	organizer.ServeHTTP(accepted, request("/workspace/reset", "secret=reset-only-from-form"))
	if accepted.Code != http.StatusSeeOther {
		t.Fatalf("organizer reset status = %d, want 303; body=%s", accepted.Code, accepted.Body.String())
	}
	if _, err := os.Stat(filepath.Join(uploads, "proof.txt")); !os.IsNotExist(err) {
		t.Fatalf("accepted reset retained upload, stat err=%v", err)
	}
}

func TestResetWorkspaceProductionWithoutSecretIsUnavailable(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("RESET_SECRET", "")
	manager := testSessionManager(t)
	authManager := identity.New(manager)
	handler := manager.Middleware(signInAs(authManager, identity.RoleOrganizer)(authManager.Middleware(resetWorkspace(t.TempDir()))))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/workspace/reset", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("production reset without secret status = %d, want 404", response.Code)
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
// explicit data-gosx-form contract and the hosted CFP's client-only preview
// form, which has no server action by design. This protects the launch UX
// guarantee that ordinary actions do not trigger a document refresh after
// JavaScript has loaded.
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
			if !strings.Contains(tag, "data-gosx-form") && !strings.Contains(tag, `data-preview-only="true"`) {
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
	uploadDir := filepath.Join(root, "data", "uploads")
	path := "/portal-upload/spk_owner/task_headshot"
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	manager := testSessionManager(t)
	authManager := identity.New(manager)
	handler := http.HandlerFunc(portalUpload(uploadDir))

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
	if _, err := os.Stat(uploadDir); !os.IsNotExist(err) {
		t.Fatalf("unauthorized upload prepared storage, stat err = %v", err)
	}
}

func TestPortalUploadChecksAssignmentImageBytesAndLimit(t *testing.T) {
	_, workspace := testPortalWorkspace(t)
	root := t.TempDir()
	uploadDir := filepath.Join(root, "data", "uploads")
	manager := testSessionManager(t)
	handler := manager.Middleware(bindPortalSpeaker("spk_owner")(http.HandlerFunc(portalUpload(uploadDir))))

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
	entries, err := os.ReadDir(uploadDir)
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
	uploadDir := filepath.Join(root, "data", "uploads")
	manager := testSessionManager(t)
	handler := manager.Middleware(bindPortalSpeaker("spk_owner")(http.HandlerFunc(portalUpload(uploadDir))))
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
	uploadDir := filepath.Join(root, "data", "uploads")
	manager := testSessionManager(t)
	handler := manager.Middleware(bindPortalSpeaker("spk_owner")(http.HandlerFunc(portalUpload(uploadDir))))
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

	removeSupersededUpload(uploadDir, filepath.ToSlash(oldPath), filepath.ToSlash(replacementPath))
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("shared upload removed despite a current reference: %v", err)
	}
	removeSupersededUpload(uploadDir, filepath.ToSlash(outsidePath), filepath.ToSlash(replacementPath))
	if _, err := os.Stat(outsidePath); err != nil {
		t.Fatalf("outside path changed during cleanup: %v", err)
	}

	if err := workspace.Update(func(state *domain.State) error {
		state.TaskCompletions[0].StoredPath = ""
		return nil
	}); err != nil {
		t.Fatalf("clear shared completion: %v", err)
	}
	removeSupersededUpload(uploadDir, filepath.ToSlash(oldPath), filepath.ToSlash(replacementPath))
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
	handler := testSecurityHeaders("https://rostrum.example", hash)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Nonce", server.RequestNonce(r))
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	policy := recorder.Header().Get("Content-Security-Policy")
	for _, required := range []string{
		"script-src 'self' 'wasm-unsafe-eval' 'nonce-",
		hash,
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
	nonce := recorder.Header().Get("X-Test-Nonce")
	if nonce == "" || !strings.Contains(policy, "'nonce-"+nonce+"'") {
		t.Fatalf("CSP nonce was not threaded through the request: nonce=%q policy=%q", nonce, policy)
	}
	if strings.Contains(policy, server.NoncePlaceholder) {
		t.Fatalf("CSP leaked nonce placeholder: %s", policy)
	}
}

func TestCSRFProtectionAllowsOnlyBoundedClientDiagnosticsWithoutAToken(t *testing.T) {
	t.Setenv("APP_MODE", "live")
	manager := testSessionManager(t)
	reached := make(map[string]int)
	handler := manager.Middleware(csrfProtection(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached[r.URL.Path]++
		w.WriteHeader(http.StatusNoContent)
	})))

	diagnostics := httptest.NewRecorder()
	handler.ServeHTTP(diagnostics, httptest.NewRequest(http.MethodPost, server.ClientEventsRoute, strings.NewReader(`{"events":[]}`)))
	if diagnostics.Code != http.StatusNoContent || reached[server.ClientEventsRoute] != 1 {
		t.Fatalf("client diagnostics status=%d reached=%d, want 204 and one handler call", diagnostics.Code, reached[server.ClientEventsRoute])
	}

	mutation := httptest.NewRecorder()
	handler.ServeHTTP(mutation, httptest.NewRequest(http.MethodPost, "/organizer/agenda/__actions/moveSession", strings.NewReader("session_id=ses_memory")))
	if mutation.Code != http.StatusForbidden {
		t.Fatalf("workspace mutation without CSRF status=%d, want 403", mutation.Code)
	}
	if reached["/organizer/agenda/__actions/moveSession"] != 0 {
		t.Fatal("workspace mutation without CSRF reached the action handler")
	}
}

func TestReadOnlyPreviewGateBlocksMutationsAndSensitiveSurfaces(t *testing.T) {
	t.Setenv("APP_MODE", "preview")
	handler := readOnlyPreviewGate()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		{http.MethodGet, "/workspace/reset", http.StatusForbidden},
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

func TestOrganizerGateAllowsAnonymousReadOnlyPreviewInspection(t *testing.T) {
	t.Setenv("APP_MODE", "preview")
	t.Setenv("GOSX_STATIC_EXPORT", "")
	handler := organizerGate()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/organizer/agenda", nil))
	if read.Code != http.StatusNoContent {
		t.Fatalf("anonymous preview organizer GET status = %d, want 204", read.Code)
	}
	write := httptest.NewRecorder()
	handler.ServeHTTP(write, httptest.NewRequest(http.MethodPost, "/organizer/agenda", nil))
	if write.Code != http.StatusForbidden {
		t.Fatalf("anonymous preview organizer POST status = %d, want 403", write.Code)
	}
}

func TestSecurityHeadersMarkReadOnlyPreviewResponsesNoindex(t *testing.T) {
	t.Setenv("APP_MODE", "preview")
	handler := testSecurityHeaders("https://preview.rostrum.example", navigationScriptCSPHash())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := testSecurityHeaders("https://rostrum.example", hash)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func testSecurityHeaders(publicBase string, scriptHashes ...string) server.Middleware {
	return func(next http.Handler) http.Handler {
		app := server.New()
		app.EnableSecurityPolicy(rostrumSecurityPolicy(publicBase, scriptHashes...))
		app.Use(routeSecurityHeaders())
		app.Mount("/", next)
		return app.Build()
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
			FormID:      state.Forms[0].ID,
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
	state := fixture.Seed(time.Now().UTC())
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
	source := fixture.Seed(time.Now().UTC())
	source.Event.Name = "Imported program"
	exportData, err := workspacearchive.Marshal(source)
	if err != nil {
		t.Fatalf("marshal source export: %v", err)
	}

	current := fixture.Seed(time.Now().UTC())
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
	handler := manager.Middleware(signInAs(authManager, identity.RoleOrganizer)(authManager.Middleware(workspaceImport(filepath.Join(t.TempDir(), "uploads"), backups))))

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
	state := fixture.Seed(time.Now().UTC())
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
	handler := workspaceArchive(uploads, auditPath)
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

	clearUploads(uploadDir)

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

func TestResolveUploadDirectoryUsesAppRootAndRejectsBroadTargets(t *testing.T) {
	root := t.TempDir()
	defaultDir, err := resolveUploadDirectory(root, "")
	if err != nil {
		t.Fatalf("resolve default UPLOAD_DIR: %v", err)
	}
	if want := filepath.Join(root, "data", "uploads"); defaultDir != want {
		t.Fatalf("default UPLOAD_DIR = %q, want %q", defaultDir, want)
	}
	relativeDir, err := resolveUploadDirectory(root, "var/uploads")
	if err != nil {
		t.Fatalf("resolve relative UPLOAD_DIR: %v", err)
	}
	if want := filepath.Join(root, "var", "uploads"); relativeDir != want {
		t.Fatalf("relative UPLOAD_DIR = %q, want %q", relativeDir, want)
	}
	for _, unsafe := range []string{root, string(filepath.Separator)} {
		if _, err := resolveUploadDirectory(root, unsafe); err == nil {
			t.Fatalf("unsafe UPLOAD_DIR %q accepted", unsafe)
		}
	}
}

func TestPublicHeadshotRequiresApprovalAndServesSafeImage(t *testing.T) {
	_, workspace := testPortalWorkspace(t)
	uploadDir := filepath.Join(t.TempDir(), "uploads")
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		t.Fatalf("create uploads: %v", err)
	}
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	storedPath := filepath.Join(uploadDir, "portrait.jpg")
	if err := os.WriteFile(storedPath, payload, 0o600); err != nil {
		t.Fatalf("write portrait: %v", err)
	}
	now := time.Now().UTC()
	if err := workspace.Update(func(state *domain.State) error {
		state.TaskCompletions = append(state.TaskCompletions, domain.TaskCompletion{
			ID: "done_headshot", TaskID: "task_headshot", SpeakerID: "spk_owner", Status: domain.TaskSubmitted,
			FileName: "portrait.jpg", ContentType: "image/jpeg", StoredPath: filepath.ToSlash(storedPath), CompletedAt: now, UpdatedAt: now,
		})
		return nil
	}); err != nil {
		t.Fatalf("seed submitted completion: %v", err)
	}
	handler := publicHeadshot(uploadDir)

	pending := httptest.NewRecorder()
	handler.ServeHTTP(pending, httptest.NewRequest(http.MethodGet, "/public-headshot/spk_owner", nil))
	if pending.Code != http.StatusNotFound {
		t.Fatalf("submitted headshot status = %d, want 404", pending.Code)
	}

	if err := workspace.Update(func(state *domain.State) error {
		completion, _ := state.Completion("task_headshot", "spk_owner")
		completion.Status = domain.TaskApproved
		return nil
	}); err != nil {
		t.Fatalf("approve completion: %v", err)
	}
	approved := httptest.NewRecorder()
	handler.ServeHTTP(approved, httptest.NewRequest(http.MethodGet, "/public-headshot/spk_owner", nil))
	if approved.Code != http.StatusOK || !bytes.Equal(approved.Body.Bytes(), payload) {
		t.Fatalf("approved headshot status=%d body=%x", approved.Code, approved.Body.Bytes())
	}
	if got := approved.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("approved headshot Content-Type = %q", got)
	}
	if got := approved.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("approved headshot nosniff = %q", got)
	}
	if got := approved.Header().Get("Cache-Control"); got != "public, no-cache, must-revalidate" {
		t.Fatalf("approved headshot cache = %q", got)
	}
}

func TestPublicHeadshotRejectsOutsideMissingAndNonImageFiles(t *testing.T) {
	_, workspace := testPortalWorkspace(t)
	root := t.TempDir()
	uploadDir := filepath.Join(root, "uploads")
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.jpg")
	if err := os.WriteFile(outside, []byte{0xff, 0xd8, 0xff, 0xe0}, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := workspace.Update(func(state *domain.State) error {
		state.TaskCompletions = append(state.TaskCompletions, domain.TaskCompletion{
			ID: "done_headshot", TaskID: "task_headshot", SpeakerID: "spk_owner", Status: domain.TaskApproved,
			FileName: "portrait.jpg", ContentType: "image/jpeg", StoredPath: filepath.ToSlash(outside), CompletedAt: now, UpdatedAt: now,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := publicHeadshot(uploadDir)
	assertNotFound := func(label string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/public-headshot/spk_owner", nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", label, recorder.Code)
		}
	}
	assertNotFound("outside")

	missing := filepath.Join(uploadDir, "missing.jpg")
	if err := workspace.Update(func(state *domain.State) error {
		completion, _ := state.Completion("task_headshot", "spk_owner")
		completion.StoredPath = filepath.ToSlash(missing)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertNotFound("missing")

	nonImage := filepath.Join(uploadDir, "portrait.jpg")
	if err := os.WriteFile(nonImage, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Update(func(state *domain.State) error {
		completion, _ := state.Completion("task_headshot", "spk_owner")
		completion.StoredPath = filepath.ToSlash(nonImage)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertNotFound("non-image")
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
	submitSource, err := os.ReadFile(filepath.Join("app", "submit", "[slug]", "page.gsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(submitSource), "//gosx:island\nfunc PreviewSubmissionForm(props any) Node") {
		t.Error("submission CFP is missing the client-only preview form island")
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
