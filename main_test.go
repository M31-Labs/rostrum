package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
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

func TestSecurityHeadersAuthorizeOnlyTheGoSXInlineRuntime(t *testing.T) {
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

func TestFrameAncestorsScopedToPublicRoutes(t *testing.T) {
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
