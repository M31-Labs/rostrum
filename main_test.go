package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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
