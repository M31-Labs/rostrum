package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	workspacearchive "github.com/m31-labs/rostrum/internal/archive"
	"github.com/m31-labs/rostrum/internal/audit"
	programcalendar "github.com/m31-labs/rostrum/internal/calendar"
	delivery "github.com/m31-labs/rostrum/internal/communications"
	"github.com/m31-labs/rostrum/internal/demomode"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/identity"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/publicapi"
	"github.com/m31-labs/rostrum/internal/ratelimit"
	"github.com/m31-labs/rostrum/internal/store"
	"github.com/m31-labs/rostrum/internal/token"
	_ "github.com/m31-labs/rostrum/modules"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/controller"
	"m31labs.dev/gosx/env"
	"m31labs.dev/gosx/hydrate"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

const developmentSessionSecret = "rostrum-development-secret-change-me"

// portalSpeakerSessionKey names the GoSX session value that binds a browser
// session to one speaker ID, mirroring app/portal/page.server.go's private
// portalSessionKey constant; keep the literal in sync if either side
// changes it. There is no organizer counterpart to this key any more: an
// organizer session is an auth.User with a role, resolved through
// auth.Current(r), not a session flag any visitor to /organizer could earn
// for free.
const portalSpeakerSessionKey = "portal_speaker"

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	root := server.ResolveAppRoot(thisFile)
	if err := env.LoadDir(root, ""); err != nil {
		log.Fatal(err)
	}

	now := time.Now().UTC()
	storeDriver := strings.ToLower(strings.TrimSpace(getenv("STORE_DRIVER", "json")))
	defaultDataPath := filepath.Join(root, "data", "rostrum.json")
	if storeDriver == "sqlite" {
		defaultDataPath = filepath.Join(root, "data", "rostrum.sqlite")
	}
	dataPath := getenv("DATA_PATH", defaultDataPath)
	if strings.EqualFold(getenv("DEMO_MODE", "true"), "memory") {
		dataPath = ":memory:"
	}
	port := getenv("PORT", "8080")
	publicBase := getenv("PUBLIC_URL", "http://localhost:"+port)
	// Release identity is deployment-owned rather than tied to the framework
	// version. A container or process manager should set ROSTRUM_VERSION to an
	// immutable tag or commit SHA, allowing /api/health to prove the exact app
	// build that is serving traffic.
	rostrumVersion := getenv("ROSTRUM_VERSION", "dev")
	appEnv := strings.ToLower(getenv("APP_ENV", "development"))
	readOnlyDemo := demomode.Enabled()
	if err := demomode.Validate(demomode.Config{
		Mode:           getenv("APP_MODE", demomode.ModeLive),
		Seed:           getenv("SEED", "demo"),
		LegacyDemoMode: getenv("DEMO_MODE", "true"),
		StoreDriver:    storeDriver,
		DataPath:       dataPath,
		RostrumVersion: rostrumVersion,
	}); err != nil {
		log.Fatal(err)
	}
	seed := selectSeed(getenv("SEED", "demo"), now)
	workspace, err := store.OpenConfigured(storeDriver, dataPath, getenv("DATABASE_URL", ""), seed)
	if err != nil {
		log.Fatal(err)
	}
	if readOnlyDemo {
		if err := demomode.ValidateState(workspace.Snapshot(), seed); err != nil {
			_ = workspace.Close()
			log.Fatal(err)
		}
	}
	auditPath := getenv("AUDIT_LOG_PATH", filepath.Join(root, "data", "audit.log"))
	backupDirectory := getenv("BACKUP_DIR", filepath.Join(root, "data", "backups"))
	ledger, err := audit.Open(auditPath)
	if err != nil {
		_ = workspace.Close()
		log.Fatal(err)
	}
	workspace = store.WithAudit(workspace, ledger)
	if readOnlyDemo {
		workspace = store.ReadOnly(workspace)
	}
	defer func() {
		if err := workspace.Close(); err != nil {
			log.Printf("close workspace store: %v", err)
		}
	}()
	appstate.Set(workspace)

	sessionSecret := getenv("SESSION_SECRET", developmentSessionSecret)
	// Organizer roles now live in the signed, encrypted session cookie, so the
	// session secret is the sole trust anchor for organizer access. Refuse the
	// default or a weak secret for any non-local PUBLIC_URL, independent of
	// APP_ENV: a public instance started without APP_ENV=production must never
	// boot with a forgeable cookie key that an attacker could use to mint an
	// organizer session.
	if !isLocalPublicURL(publicBase) && (sessionSecret == developmentSessionSecret || len(sessionSecret) < 32) {
		log.Fatal("a non-local PUBLIC_URL requires a unique SESSION_SECRET of at least 32 characters")
	}
	if appEnv == "production" {
		if sessionSecret == developmentSessionSecret || len(sessionSecret) < 32 {
			log.Fatal("production requires a unique SESSION_SECRET of at least 32 characters")
		}
		if !strings.HasPrefix(publicBase, "https://") {
			log.Fatal("production requires an https PUBLIC_URL")
		}
		if dataPath == ":memory:" {
			log.Fatal("production cannot use DEMO_MODE=memory")
		}
	}
	overHTTP := strings.HasPrefix(publicBase, "http://")
	sessions, err := session.New(sessionSecret, session.Options{
		Secure:        !overHTTP,
		AllowInsecure: overHTTP,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Identity plane (see specs/identity-plane.md): one auth.Manager over the
	// session manager above, with magic-link, OAuth, and WebAuthn sign-in
	// wired on top. mailConfigured tracks whether a complete real transport
	// (Resend/API or SMTP, not the demo outbox) is available, because the
	// outbox has nowhere a
	// self-hoster can read a link from — setup.go signs the browser in
	// directly instead when it is false.
	authManager := identity.New(sessions)
	mailSender := mail.FromEnv()
	if !readOnlyDemo {
		startOutboxRunner(workspace, mailSender)
	}
	mailConfigured := mail.TransportConfigured()
	magicLinks := authManager.MagicLinks(auth.MagicLinkOptions{
		BaseURL:     publicBase,
		SuccessPath: "/organizer",
		FailurePath: "/login",
		FlashKey:    identity.MagicLinkFlashKey,
		Resolver:    auth.MagicLinkResolverFunc(identity.ResolveEmail),
		Sender:      identity.MailSender{Sender: mailSender},
		Store:       identity.DurableMagicLinkStore{},
	})
	oauthProviders := identity.Providers(publicBase)
	oauthManager := authManager.OAuth(auth.OAuthOptions{
		Providers:   oauthProviders,
		SuccessPath: "/organizer",
		FailurePath: "/login",
	})
	webAuthnManager := authManager.WebAuthn(auth.WebAuthnOptions{
		RPName: "Rostrum",
		Origin: publicBase,
		Store:  identity.DurableWebAuthnStore{},
	})
	if readOnlyDemo {
		// The break-glass setup path is an identity mutation and must not be
		// reachable from a hosted demo, even when its workspace is empty.
		identity.SetSetup(nil)
	} else {
		identity.SetSetup(identity.NewSetup(authManager, magicLinks, mailConfigured, publicBase))
	}

	router := route.NewRouter()
	router.SetLayout(func(ctx *route.RouteContext, body gosx.Node) gosx.Node {
		// MetadataBase resolves every relative canonical, icon, and Open
		// Graph URL below to an absolute one, and — because no page sets
		// its own Alternates.Canonical — also makes resolveCanonicalURL
		// fall back to the current request path, so every route (not just
		// the public agenda and speakers pages) gets a correct canonical
		// link for free (N8).
		ctx.SetMetadata(server.Metadata{
			MetadataBase: publicBase,
			Links: []server.LinkTag{
				{Rel: "preconnect", Href: "https://fonts.googleapis.com"},
				{Rel: "preconnect", Href: "https://fonts.gstatic.com", CrossOrigin: "anonymous"},
				{Rel: "stylesheet", Href: "/styles.css"},
				{Rel: "stylesheet", Href: "https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,500;9..144,600&family=Instrument+Sans:wght@400;500;600&family=Spline+Sans+Mono:wght@400;500&display=swap"},
				{Rel: "icon", Href: "/favicon.svg", Type: "image/svg+xml"},
			},
			Icons: &server.Icons{
				Apple: []server.IconAsset{{URL: "/apple-touch-icon.png", Sizes: "180x180"}},
			},
			// A page-level Metadata func only ever sets Title/Description
			// (see app/*/page.server.go), so this site default always wins
			// the OpenGraph merge and every page gets a real preview image
			// when shared, instead of the empty card a bare Title/Description
			// pair renders as (N8).
			OpenGraph: &server.OpenGraph{
				Type:     "website",
				SiteName: "Rostrum",
				Images: []server.MediaAsset{
					{URL: "/og-image.png", Width: 1200, Height: 630, Alt: "Rostrum — governed program operations"},
				},
			},
		})
		ctx.AddHead(gosx.El("meta", gosx.Attrs(
			gosx.Attr("name", "theme-color"),
			gosx.Attr("content", "#1e2a24"),
		)))
		ctx.AddHead(gosx.El("meta", gosx.Attrs(
			gosx.Attr("name", "csrf-token"),
			gosx.Attr("content", sessions.Token(ctx.Request)),
		)))
		// File-router documents are mounted beneath server.App, so opt them into
		// the GoSX navigation runtime explicitly at the document boundary.
		ctx.AddHead(server.NavigationScript())
		configureRouteRuntime(ctx)
		return server.HTMLDocument(ctx.Title("Rostrum"), ctx.Head(), body)
	})
	if err := router.AddDir(filepath.Join(root, "app"), route.FileRoutesOptions{}); err != nil {
		log.Fatal(err)
	}

	app := server.New()
	// app.EnableGzip() is intentionally omitted. The GoSX gzip middleware
	// (server/gzip.go, v0.38.0) double-encodes precompressed assets: its
	// gzipWriter.WriteHeader skips when Content-Encoding is already set, but
	// gzipWriter.Write still routes bytes through the gzip.Writer. A brotli
	// runtime sidecar is re-gzipped while the header still reads "br", so the
	// browser cannot decode any island script or WASM and nothing hydrates.
	// Runtime assets self-negotiate br/gzip in server.serveRuntimeFile, and
	// dynamic HTML is compressed at the CDN edge. Restore this call once the
	// framework Write path honors the skip.
	app.Use(securityHeaders(publicBase, navigationScriptCSPHash(), webAuthnScriptCSPHash()))
	app.Use(clearStaleBrowserCache(publicBase))
	app.Use(noCacheStaticCSS())
	app.Use(sessions.Middleware)
	app.Use(authManager.Middleware)
	app.Use(readOnlyDemoGate())
	app.Use(organizerGate())
	app.Use(bodyLimit())
	app.Use(sessions.Protect)
	app.SetPublicDir(filepath.Join(root, "public"))
	app.API("GET /api/health", func(ctx *server.Context) (any, error) {
		ctx.CachePublic(30 * time.Second)
		return map[string]any{
			"ok":      true,
			"app":     "Rostrum",
			"version": rostrumVersion,
			"time":    time.Now().UTC().Format(time.RFC3339),
		}, nil
	})
	app.API("GET /api/v1/workspace", func(ctx *server.Context) (any, error) {
		ctx.CachePublic(60 * time.Second)
		return publicapi.Index(appstate.MustGet().Snapshot()), nil
	})
	app.API("GET /api/v1/schedule", func(ctx *server.Context) (any, error) {
		ctx.CachePublic(60 * time.Second)
		return publicapi.Schedule(appstate.MustGet().Snapshot()), nil
	})
	app.API("GET /api/v1/speakers", func(ctx *server.Context) (any, error) {
		ctx.CachePublic(60 * time.Second)
		return publicapi.Speakers(appstate.MustGet().Snapshot()), nil
	})
	// /live streams workspace activity events (new submissions, task uploads),
	// so gate it to organizer-facing roles in live mode. The isolated demo
	// explicitly uses the same read-only stream as an inspection surface.
	app.Mount("/live", liveDashboardHandler())
	app.Mount("/calendar/", http.HandlerFunc(calendarDownload))
	app.Mount("/portal-upload/", http.HandlerFunc(portalUpload(root)))
	app.Mount("/portal-file/", http.HandlerFunc(portalFile(root)))
	app.Mount("/organizer/export/submissions.csv", http.HandlerFunc(submissionsCSV))
	app.Mount("/organizer/export/workspace.json", http.HandlerFunc(workspaceExport))
	app.Mount("/organizer/export/archive.tar.gz", http.HandlerFunc(workspaceArchive(root, auditPath)))
	app.Mount("/organizer/export/approved-uploads.zip", http.HandlerFunc(approvedUploadBundle(root)))
	app.Mount("/organizer/import/workspace", http.HandlerFunc(workspaceImport(root, backupDirectory)))
	app.Mount("/favicon.ico", http.RedirectHandler("/favicon.svg", http.StatusTemporaryRedirect))
	app.Mount("/demo/reset", resetDemo(root))

	// Identity plane routes. GET /auth/magic-link is the callback a clicked
	// email link opens; POST /auth/magic-link is the sign-in form
	// submission, rate-limited so the endpoint cannot be used to spam
	// arbitrary addresses. Both pass session.Protect's CSRF check unbothered
	// on the GET (Protect only guards POST/PUT/PATCH/DELETE) and via the
	// hidden csrf_token field on the POST.
	app.Mount("GET /auth/magic-link", magicLinks.CallbackHandler())
	app.Mount("POST /auth/magic-link", magicLinkRequestGate(managedMagicLinkRequest(magicLinks)))
	for _, provider := range oauthProviders {
		name := provider.Name
		app.Mount("GET /auth/oauth/"+name, oauthManager.BeginHandler(name))
		app.Mount("GET /auth/oauth/"+name+"/callback", oauthManager.CallbackHandler(name))
	}
	// Registration is a second factor of convenience: only an already
	// signed-in user may register a passkey (authManager.Require blocks an
	// anonymous POST with the framework's usual 401-JSON-or-redirect
	// response). Login stays open to anyone -- it is how a not-yet-signed-in
	// visitor authenticates in the first place.
	app.Mount("POST /auth/webauthn/register-options", authManager.Require(webAuthnManager.RegisterOptionsHandler()))
	app.Mount("POST /auth/webauthn/register", authManager.Require(webAuthnManager.RegisterHandler()))
	app.Mount("POST /auth/webauthn/login-options", webAuthnManager.LoginOptionsHandler())
	app.Mount("POST /auth/webauthn/login", webAuthnManager.LoginHandler())

	rootHandler, err := router.BuildChecked()
	if err != nil {
		log.Fatal(err)
	}
	app.Mount("/", rootHandler)

	log.Printf("Rostrum listening on %s (data: %s)", publicBase, workspace.Path())
	log.Fatal(app.ListenAndServe(":" + port))
}

// startOutboxRunner invokes the persisted outbox at startup and at a modest
// cadence. The ticker is merely a wake-up source: due time, retries, leases,
// cancellation, and idempotency all live in canonical state, so a restart or
// another worker can safely resume work. Operators can also invoke the same
// runner interactively from Communications; delivery never depends solely on
// this in-process loop.
func startOutboxRunner(workspace store.StateStore, sender mail.Sender) {
	run := func() {
		report, err := (delivery.Runner{Store: workspace, Sender: sender}).RunDue()
		if err != nil {
			log.Printf("communications outbox run: %v", err)
			return
		}
		if report.Enqueued+report.Sent+report.Retried+report.Failed+report.Suppressed+report.Cancelled > 0 {
			log.Printf("communications outbox: %d enqueued, %d sent, %d retried, %d failed, %d suppressed", report.Enqueued, report.Sent, report.Retried, report.Failed, report.Suppressed)
		}
	}
	run()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}

func configureRouteRuntime(ctx *route.RouteContext) {
	if ctx == nil || ctx.Request == nil {
		return
	}
	path := strings.TrimSuffix(ctx.Request.URL.Path, "/")
	if path == "" {
		path = "/"
	}

	if path == "/organizer" || strings.HasPrefix(path, "/organizer/") {
		ctx.Runtime().Controller(controller.Config{
			Name: "rostrum-workspace-preferences",
			Root: "#workspace-sidebar",
			Storage: &controller.Storage{
				Area:      "local",
				Namespace: "rostrum-workspace",
				Load: []controller.StorageSlot{{
					Key:    "rail-collapsed",
					Signal: "$rostrumWorkspaceRail",
				}},
				Save: []controller.StorageSlot{{
					Key:    "rail-collapsed",
					Signal: "$rostrumWorkspaceRail",
				}},
			},
		})
	}

	if strings.HasPrefix(path, "/public/") && strings.HasSuffix(path, "/agenda") {
		slug := strings.TrimSpace(ctx.Param("slug"))
		if slug == "" {
			slug = "event"
		}
		ctx.Runtime().Controller(controller.Config{
			Name: "rostrum-public-itinerary",
			Root: ".public-itinerary",
			Storage: &controller.Storage{
				Area:      "local",
				Namespace: "rostrum-itinerary-" + slug,
				Load: []controller.StorageSlot{{
					Key:    "sessions",
					Signal: "$rostrumItinerary",
				}},
				Save: []controller.StorageSlot{{
					Key:    "sessions",
					Signal: "$rostrumItinerary",
				}},
			},
		})
	}

	switch path {
	case "/organizer":
		ctx.Runtime().BindHub("rostrum-overview", "/live", refreshBindings(
			"agenda:moved",
			"agenda:published",
			"communication:queued",
			"event:category-created",
			"event:room-created",
			"event:track-created",
			"event:updated",
			"form:updated",
			"integration:dry-run",
			"integration:live-sync",
			"review:updated",
			"session:created",
			"speaker:updated",
			"submission:created",
			"submission:updated",
			"task:approved",
			"task:uploaded",
			"task:submitted",
			"workspace:reset",
		))
	case "/organizer/portal":
		ctx.Runtime().BindHub("rostrum-portal-matrix", "/live", refreshBindings(
			"speaker:updated",
			"task:approved",
			"task:uploaded",
			"task:submitted",
			"workspace:reset",
		))
	}
}

func refreshBindings(events ...string) []hydrate.HubBinding {
	bindings := make([]hydrate.HubBinding, 0, len(events))
	for _, event := range events {
		bindings = append(bindings, hydrate.HubBinding{
			Event:             event,
			Refresh:           true,
			RefreshDebounceMS: 180,
		})
	}
	return bindings
}

func securityHeaders(publicBase string, scriptHashes ...string) server.Middleware {
	// GoSX islands execute the framework's compiled WebAssembly VM. Authorize
	// WebAssembly compilation without opening generic eval or inline scripts.
	// scriptHashes carries the navigation runtime and the WebAuthn runtime
	// (see webAuthnScriptCSPHash): both are framework-owned inline scripts,
	// authorized by exact content hash, never by a blanket 'unsafe-inline'.
	scriptPolicy := "script-src 'self' 'wasm-unsafe-eval'"
	for _, hash := range scriptHashes {
		if hash != "" {
			scriptPolicy += " " + hash
		}
	}
	base := "default-src 'self'; base-uri 'self'; object-src 'none'; " + scriptPolicy + "; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self' ws: wss:; frame-src 'self' https://www.youtube-nocookie.com https://player.vimeo.com; form-action 'self'"
	secure := strings.HasPrefix(publicBase, "https://")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			policy := base + "; " + frameAncestorsDirective(r.URL.Path)
			if secure {
				policy += "; upgrade-insecure-requests"
			}
			w.Header().Set("Content-Security-Policy", policy)
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			if readOnlyDemoMode() {
				// The demo is a review surface, not a canonical public event
				// site. Keep search engines and archive crawlers from indexing
				// its fictional back-of-house data.
				w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
			}
			if secure {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// frameAncestorsDirective returns the CSP frame-ancestors directive for
// path (SE-8/M7). Only the /public/ surface -- the embeddable agenda widget
// present.EmbedAdmin hands organizers a snippet for -- may be framed by
// another site. Every other route, including /organizer and /portal, must
// never be embeddable, so a malicious page cannot dress either surface up
// for clickjacking.
func frameAncestorsDirective(path string) string {
	if path == "/public" || strings.HasPrefix(path, "/public/") {
		return "frame-ancestors *"
	}
	return "frame-ancestors 'none'"
}

// cacheClearGuardCookie names the cookie clearStaleBrowserCache sets after it
// asks a browser to clear its HTTP cache once, so it never repeats the
// instruction to that browser again.
const cacheClearGuardCookie = "rostrum_cache_v2"

// clearStaleBrowserCache answers B1: content-hash-named runtime assets under
// /gosx/assets/runtime/ are served immutable, max-age=1y, which is normally
// safe because a new build gets a new hash. It is not safe against the
// specific incident this guards: an earlier deploy briefly served those
// files with the wrong Content-Encoding header, and a browser that cached
// the corrupt bytes during that window keeps them forever -- a later
// deploy under a new hash never evicts an entry filed under the old one.
//
// On the first HTML document GET from a browser that has not seen this
// guard, the middleware sends Clear-Site-Data: "cache" (dropping the whole
// HTTP cache, corrupt entries included) and sets cacheClearGuardCookie for
// a year so the instruction never repeats on that browser. It only ever
// fires on a document navigation -- never on an asset, API, download, or
// calendar request -- so it cannot interrupt a fetch already in flight for
// one of those.
func clearStaleBrowserCache(publicBase string) server.Middleware {
	secure := strings.HasPrefix(publicBase, "https://")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldClearBrowserCache(r) {
				http.SetCookie(w, &http.Cookie{
					Name:     cacheClearGuardCookie,
					Value:    "1",
					Path:     "/",
					MaxAge:   int((365 * 24 * time.Hour).Seconds()),
					HttpOnly: true,
					Secure:   secure,
					SameSite: http.SameSiteLaxMode,
				})
				w.Header().Set("Clear-Site-Data", `"cache"`)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// shouldClearBrowserCache reports whether r is an HTML document navigation
// from a browser that has not already received the cache-clear guard.
func shouldClearBrowserCache(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return false
	}
	path := r.URL.Path
	if strings.HasPrefix(path, "/gosx/") || strings.HasPrefix(path, "/api/") ||
		strings.HasPrefix(path, "/portal-file/") || strings.HasPrefix(path, "/calendar/") {
		return false
	}
	if _, err := r.Cookie(cacheClearGuardCookie); err == nil {
		return false
	}
	return true
}

// noCacheStaticCSS answers the CSS staleness fix: the public directory's
// styles.css carries no content-hash fingerprint, so a browser or CDN that
// still honors the framework's public-dir default (SetPublicDir's
// long-lived, revalidate-only policy) can keep serving an old stylesheet
// for hours after a redeploy. Forcing Cache-Control: no-cache on every .css
// response makes the browser revalidate on every load -- a redeploy takes
// effect immediately -- while a normal, unchanged load still gets a 304 and
// no re-download.
//
// This wraps the ResponseWriter rather than setting the header up front,
// because server.App.servePublic (the framework handler that actually
// serves styles.css) sets its own Cache-Control value after every
// app.Use middleware ahead of it has already run, so this middleware would
// otherwise be overwritten before the response is written.
func noCacheStaticCSS() server.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.HasSuffix(r.URL.Path, ".css") {
				w = noCacheResponseWriter{ResponseWriter: w}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// noCacheResponseWriter forces Cache-Control: no-cache immediately before
// any response byte is written, overriding whatever the wrapped handler set.
type noCacheResponseWriter struct {
	http.ResponseWriter
}

func (w noCacheResponseWriter) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-cache")
	w.ResponseWriter.WriteHeader(status)
}

func (w noCacheResponseWriter) Write(b []byte) (int, error) {
	// http.ServeFile/http.ServeContent (server.App.servePublic's transport)
	// send status 200 implicitly on the first Write call rather than calling
	// WriteHeader explicitly, so the header must be forced here too.
	w.Header().Set("Cache-Control", "no-cache")
	return w.ResponseWriter.Write(b)
}

// Upload body sizing. uploadBodyEnvelope bounds the whole multipart request
// (the 10 MiB file cap plus boundary and header overhead); maxUploadBytes
// bounds the stored file payload itself. defaultBodyLimit bounds every other
// route so no form endpoint spools an unbounded body to memory or disk.
const (
	uploadBodyEnvelope      = 12 << 20
	maxUploadBytes          = 10 << 20
	workspaceImportEnvelope = 34 << 20
	maxWorkspaceImportBytes = 32 << 20
	defaultBodyLimit        = 1 << 20
)

// bodyLimit caps the request body via http.MaxBytesReader before any
// handler reads it. It runs ahead of sessions.Protect, whose CSRF check
// calls r.FormValue and so triggers an unbounded ParseMultipartForm read on
// the raw body if nothing has capped it first. Upload routes get the
// upload envelope; every other route gets the default cap.
func bodyLimit() server.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := int64(defaultBodyLimit)
			if r.Method == http.MethodPost {
				switch {
				case strings.HasPrefix(r.URL.Path, "/portal-upload/"):
					limit = uploadBodyEnvelope
				case r.URL.Path == "/organizer/import/workspace":
					limit = workspaceImportEnvelope
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// readOnlyDemoMode is deliberately an explicit environment check. Startup
// validation fails closed for an unknown APP_MODE, while request-level tests
// and middleware keep the deployment boundary easy to audit.
func readOnlyDemoMode() bool {
	return demomode.Enabled()
}

// readOnlyDemoGate is the last defense before any route handler. In the
// hosted demo every unsafe HTTP method is refused, and sensitive surfaces
// that could otherwise issue a GET-side effect or reveal an identity/setup
// flow are refused too. The store wrapper remains the authoritative backstop
// for writes made by code paths that are added later.
func readOnlyDemoGate() server.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !readOnlyDemoMode() {
				next.ServeHTTP(w, r)
				return
			}
			if ip := ratelimit.ClientIP(r); ip != "" && !readOnlyDemoIPLimiter.Allow(ip) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "demo rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			if isMutationMethod(r.Method) || readOnlyDemoForbiddenPath(r.URL.Path) {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "read-only demo", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func readOnlyDemoForbiddenPath(path string) bool {
	trimmed := strings.TrimSuffix(path, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	if trimmed == "/auth" || strings.HasPrefix(trimmed, "/auth/") {
		return true
	}
	if trimmed == "/setup" || strings.HasPrefix(trimmed, "/setup/") {
		return true
	}
	if trimmed == "/demo/reset" {
		return true
	}
	if trimmed == "/organizer/import" || strings.HasPrefix(trimmed, "/organizer/import/") {
		return true
	}
	if trimmed == "/portal-file" || strings.HasPrefix(trimmed, "/portal-file/") {
		return true
	}
	return trimmed == "/organizer/export" || strings.HasPrefix(trimmed, "/organizer/export/")
}

// The hosted preview is intentionally small, but it is still an internet
// facing process. Keep accidental crawlers or a single noisy client from
// turning the anonymous read surface into an unbounded origin workload.
var readOnlyDemoIPLimiter = ratelimit.NewTokenBucket(300, time.Minute)

func liveDashboardHandler() http.Handler {
	if readOnlyDemoMode() {
		return live.Dashboard
	}
	return identity.RequireAnyRole(identity.RoleOrganizer, identity.RoleChair, identity.RoleObserver)(live.Dashboard)
}

// organizerGate requires an organizer, chair, or observer session on every
// `/organizer` path in live mode (identity-plane spec AU-5). Anonymous or
// under-privileged visitors get RequireAnyRole's usual response: a JSON
// request gets 401, everything else redirects to /login with the original
// path preserved. The isolated APP_MODE=demo deployment is the deliberate
// exception: readOnlyDemoGate has already removed every unsafe path, so it
// can expose the same workspace UI anonymously for inspection. This replaces
// the deleted organizerContext, which trusted whoever a reverse proxy already
// let reach `/organizer` -- the leak this gate closes.
//
// The three /organizer/export paths are deliberately excluded: each enforces
// its own tighter, non-redirecting 403 (organizer and chair only, never
// observer, because the exports carry speaker PII), and an API-shaped export
// must never answer a cookie-less request with a browser redirect.
//
// GOSX_STATIC_EXPORT=1 disables the gate entirely. `gosx build --prod`
// (cmd/gosx/prerender.go) launches this exact binary as a short-lived,
// loopback-only subprocess to render every discovered route once and
// measure its client bundle capabilities for cmd/sizecheck; a real
// deployment never sets this variable. Gating /organizer during that pass
// would export the /login shell's capabilities instead of the organizer
// dashboard's, and cmd/sizecheck would flag every organizer route as
// missing its islands and controllers.
func organizerGate() server.Middleware {
	// The static-export bypass is build-time only. Honor it only outside a
	// production runtime, so a leaked GOSX_STATIC_EXPORT in a deployed
	// environment can never open /organizer to the world (review m2).
	if strings.TrimSpace(os.Getenv("GOSX_STATIC_EXPORT")) == "1" &&
		!strings.EqualFold(getenv("APP_ENV", "development"), "production") {
		return func(next http.Handler) http.Handler { return next }
	}
	gate := identity.RequireAnyRole(identity.RoleOrganizer, identity.RoleChair, identity.RoleObserver)
	return func(next http.Handler) http.Handler {
		guarded := gate(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isOrganizerGatedPath(r.URL.Path) {
				if readOnlyDemoMode() {
					// The hosted demo is intentionally anonymous and inspect-only.
					// readOnlyDemoGate catches the real app chain first; keep this
					// guard here too so the organizer boundary is safe in isolation.
					if isMutationMethod(r.Method) {
						http.Error(w, "read-only demo", http.StatusForbidden)
						return
					}
					next.ServeHTTP(w, r)
					return
				}
				// Observers are intentionally read-only. File-route actions and
				// custom organizer endpoints are POSTs, so enforce the narrower
				// role here instead of trusting every individual action to repeat
				// the same guard.
				if isMutationMethod(r.Method) && !canMutateWorkspace(r) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				guarded.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isMutationMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isOrganizerGatedPath(path string) bool {
	trimmed := strings.TrimSuffix(path, "/")
	switch trimmed {
	case "/organizer/export/submissions.csv", "/organizer/export/workspace.json", "/organizer/export/archive.tar.gz":
		return false
	}
	return trimmed == "/organizer" || strings.HasPrefix(trimmed, "/organizer/")
}

// isOrganizerSession reports whether the request carries a session with any
// organizer-facing role (organizer, chair, or observer) -- the replacement
// for the deleted portalAdminSessionKey flag portalFile and
// calendarDownload used to check.
func isOrganizerSession(r *http.Request) bool {
	user, ok := auth.Current(r)
	if !ok {
		return false
	}
	for _, role := range user.Roles {
		switch role {
		case identity.RoleOrganizer, identity.RoleChair, identity.RoleObserver:
			return true
		}
	}
	return false
}

// canMutateWorkspace is intentionally narrower than isOrganizerSession:
// observers may inspect the organizer surface but may never change data,
// upload on behalf of a speaker, export archives, or restore a backup.
func canMutateWorkspace(r *http.Request) bool {
	user, ok := auth.Current(r)
	if !ok {
		return false
	}
	for _, role := range user.Roles {
		if role == identity.RoleOrganizer || role == identity.RoleChair {
			return true
		}
	}
	return false
}

// isLocalPublicURL reports whether base points at a loopback host. It gates
// safeguards that must hold for every internet-facing deployment but that
// would block local development, where the default PUBLIC_URL and the default
// session secret are expected.
func isLocalPublicURL(base string) bool {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return false
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1", "":
		return true
	default:
		return false
	}
}

// magicLinkIPLimiter and magicLinkSessionLimiter throttle POST
// /auth/magic-link (SE-3b style): a rolling cap per client IP address so
// one address cannot keep requesting links, and a lifetime cap per session
// so one browser cannot either. Both use internal/ratelimit, the same
// package the public submission and review flows already rely on.
var (
	magicLinkIPLimiter      = ratelimit.NewTokenBucket(10, time.Hour)
	magicLinkSessionLimiter = ratelimit.NewCounter(20)
)

// magicLinkRequestGate wraps auth.MagicLinks.RequestHandler() with the rate
// limiters above. It runs before the handler ever calls the resolver, so a
// caller past their cap costs one map lookup, never a store write or an
// outbound email.
func magicLinkRequestGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := ratelimit.ClientIP(r); ip != "" && !magicLinkIPLimiter.Allow(ip) {
			http.Error(w, "Too many sign-in requests. Try again in a little while.", http.StatusTooManyRequests)
			return
		}
		if key := ratelimit.RequestIdentity(r); key != "" && !magicLinkSessionLimiter.Allow(key) {
			http.Error(w, "Too many sign-in requests. Try again in a little while.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// managedMagicLinkRequest keeps the standard GoSX magic-link primitives but
// accepts either JSON or a browser FormData post. Generic <Form> submits use
// FormData plus Accept: application/json; the framework's stock
// MagicLinks.RequestHandler treats that combination as JSON-only. Handling
// both encodings here lets the sign-in page remain a managed, no-refresh form
// while retaining the same durable token store, resolver, sender, and rate
// gate as the native fallback.
func managedMagicLinkRequest(magicLinks *auth.MagicLinks) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email, next, err := readManagedMagicLinkRequest(r)
		if err != nil {
			writeManagedMagicLinkError(w, r, http.StatusBadRequest)
			return
		}
		if _, err := magicLinks.Send(r, email, next); err != nil {
			// Do not expose resolver, provider, or origin details to a public
			// sign-in form. Operators receive the concrete error in logs.
			log.Printf("magic link request: %v", err)
			writeManagedMagicLinkError(w, r, http.StatusBadRequest)
			return
		}
		session.AddFlash(r, identity.MagicLinkFlashKey, map[string]any{
			"status": "sent",
			"email":  strings.TrimSpace(strings.ToLower(email)),
		})
		if requestWantsJSON(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":       true,
				"message":  "Check your email for a sign-in link.",
				"redirect": "/login",
			})
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
}

func readManagedMagicLinkRequest(r *http.Request) (email, next string, err error) {
	if r == nil {
		return "", "", errors.New("missing request")
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
		var payload struct {
			Email string `json:"email"`
			Next  string `json:"next"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&payload); decodeErr != nil {
			return "", "", decodeErr
		}
		return payload.Email, payload.Next, nil
	}
	if parseErr := r.ParseForm(); parseErr != nil {
		return "", "", parseErr
	}
	return r.Form.Get("email"), r.Form.Get("next"), nil
}

func writeManagedMagicLinkError(w http.ResponseWriter, r *http.Request, status int) {
	message := "We could not request a sign-in link. Check your email address and try again."
	if requestWantsJSON(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": message})
		return
	}
	session.AddFlash(r, identity.MagicLinkFlashKey, map[string]any{"status": "error"})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func navigationScriptCSPHash() string {
	rendered := gosx.RenderHTML(server.NavigationScript())
	start := strings.Index(rendered, ">")
	end := strings.LastIndex(rendered, "</script>")
	if start < 0 || end <= start {
		return ""
	}
	sum := sha256.Sum256([]byte(rendered[start+1 : end]))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// webAuthnScriptCSPHash hashes auth.WebAuthnScript() the same way
// navigationScriptCSPHash hashes the navigation runtime: both are inline
// <script> tags GoSX itself owns, so the CSP authorizes them by exact
// content hash instead of a blanket 'unsafe-inline'.
func webAuthnScriptCSPHash() string {
	rendered := gosx.RenderHTML(auth.WebAuthnScript())
	start := strings.Index(rendered, ">")
	end := strings.LastIndex(rendered, "</script>")
	if start < 0 || end <= start {
		return ""
	}
	sum := sha256.Sum256([]byte(rendered[start+1 : end]))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// selectSeed picks the initial workspace state from the SEED environment
// variable: "demo" (the default) seeds the full polished demo dataset,
// "fresh" seeds one placeholder event and one open call for proposals with
// nothing else, and "empty" seeds only the event skeleton with no call for
// proposals at all. An unrecognized value falls back to "demo".
func selectSeed(mode string, now time.Time) domain.State {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "fresh":
		return domain.FreshState(now)
	case "empty":
		return domain.EmptyState(now)
	default:
		return domain.Seed(now)
	}
}

// canExportSubmissions reports whether the request carries an organizer or
// chair session. Observers reach the rest of /organizer but never this
// export: it carries speaker PII the read-only role must not receive.
func canExportSubmissions(r *http.Request) bool {
	return canMutateWorkspace(r)
}

// submissionsCSV serves GET /organizer/export/submissions.csv.
//
// Authorization (SE-5b): this handler enforces its own role check rather
// than relying on organizerGate, which excludes this exact path (see
// isOrganizerGatedPath) so an API-shaped export answers a cookie-less
// request with 403, never a browser redirect. An unauthenticated or
// under-privileged request gets 403, never a row of data.
func submissionsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !canExportSubmissions(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	state := appstate.MustGet().Snapshot()
	if err := recordWorkspaceExport(r, "export.submissions", "rostrum-submissions.csv"); err != nil {
		log.Printf("record submissions export: %v", err)
		http.Error(w, "could not record export", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="rostrum-submissions.csv"`)
	w.Header().Set("Cache-Control", "private, no-store")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"id", "title", "status", "category", "format", "level", "speakers", "routed_owner", "submitted_at"})
	for _, submission := range state.Submissions {
		// SE-5: present.CSVSafe prefixes any cell a spreadsheet application
		// would read as a formula (=, +, -, @, tab, or CR) with a single
		// quote, so a submission title of =HYPERLINK(...) exports as inert
		// text instead of a formula the export's opener evaluates.
		_ = writer.Write([]string{
			present.CSVSafe(submission.ID),
			present.CSVSafe(submission.Title),
			present.CSVSafe(submission.Status),
			present.CSVSafe(present.CategoryName(state, submission.CategoryID)),
			present.CSVSafe(submission.Format),
			present.CSVSafe(submission.Level),
			present.CSVSafe(present.SpeakerNames(state, submission.SpeakerIDs)),
			present.CSVSafe(submission.RoutedOwner),
			present.CSVSafe(submission.SubmittedAt.Format(time.RFC3339)),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Printf("write submissions export: %v", err)
	}
}

// workspaceExport serves the validated, portable workspace envelope. It is
// intentionally a standalone handler rather than a file-route action so an
// operator can download a privacy-sensitive file with ordinary HTTP and a
// cookie-less request always receives 403 instead of a login redirect.
func workspaceExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !canExportSubmissions(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	state := appstate.MustGet().Snapshot()
	data, err := workspacearchive.Marshal(state)
	if err != nil {
		log.Printf("encode workspace export: %v", err)
		http.Error(w, "could not export workspace", http.StatusInternalServerError)
		return
	}
	if err := recordWorkspaceExport(r, "export.workspace", "rostrum-workspace.json"); err != nil {
		log.Printf("record workspace export: %v", err)
		http.Error(w, "could not record export", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="rostrum-workspace.json"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(data)
}

// workspaceArchive serves a streaming cold archive containing the portable
// workspace envelope, every local upload, and the independent audit ledger.
// Its archive is deliberately restoreable without a running Rostrum process;
// see docs/deployment.md for the stopped-process recovery procedure.
func workspaceArchive(root, auditPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !canExportSubmissions(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		state := appstate.MustGet().Snapshot()
		// Confirm encoding before we commit the audit access event or start a
		// response stream. WriteTarGZ encodes once more when it writes the tar
		// entry, but this first pass makes a malformed state fail cleanly.
		if _, err := workspacearchive.Marshal(state); err != nil {
			log.Printf("encode workspace archive: %v", err)
			http.Error(w, "could not export workspace archive", http.StatusInternalServerError)
			return
		}
		if err := recordWorkspaceExport(r, "export.archive", "rostrum-archive.tar.gz"); err != nil {
			log.Printf("record workspace archive export: %v", err)
			http.Error(w, "could not record export", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="rostrum-archive.tar.gz"`)
		w.Header().Set("Cache-Control", "private, no-store")
		if err := workspacearchive.WriteTarGZ(w, state, filepath.Join(root, "data", "uploads"), auditPath); err != nil {
			// The response may already have started, so preserve its valid stream
			// prefix and log the operational detail instead of attempting a second
			// incompatible HTTP response.
			log.Printf("write workspace archive: %v", err)
		}
	}
}

// approvedUploadBundle serves a deterministic ZIP of the subset of task
// uploads that an organizer has explicitly approved. Like the other
// privacy-sensitive exports, authorization is checked here rather than
// delegated to organizerGate so a cookie-less request receives 403 and never
// an authentication redirect that might expose route behavior.
func approvedUploadBundle(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !canExportSubmissions(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		state := appstate.MustGet().Snapshot()
		bundle, err := workspacearchive.BuildApprovedUploadBundle(state, filepath.Join(root, "data", "uploads"))
		if err != nil {
			log.Printf("build approved upload bundle: %v", err)
			http.Error(w, "could not build approved upload bundle", http.StatusConflict)
			return
		}
		if err := recordWorkspaceExport(r, "export.approved_upload_bundle", "rostrum-approved-uploads.zip"); err != nil {
			log.Printf("record approved upload bundle export: %v", err)
			http.Error(w, "could not record export", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="rostrum-approved-uploads.zip"`)
		w.Header().Set("Cache-Control", "private, no-store")
		if err := bundle.Write(w); err != nil {
			// Writing may already have started, so preserve the valid prefix and
			// leave the detailed integrity diagnosis in the server log.
			log.Printf("write approved upload bundle: %v", err)
		}
	}
}

// workspaceImport accepts only a checksummed workspace envelope. Full
// archive recovery intentionally remains a stopped-process operation: the
// archive is a forensic/asset bundle and must never replace active files
// halfway through an HTTP request. Before this JSON-only restore changes any
// state, it validates the whole envelope and writes the current state to a
// durable, retention-managed backup.
func workspaceImport(root, backupDirectory string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !canMutateWorkspace(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseMultipartForm(maxWorkspaceImportBytes); err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				writeMutationError(w, r, http.StatusRequestEntityTooLarge, "Workspace import must be smaller than 32 MB.")
				return
			}
			writeMutationError(w, r, http.StatusBadRequest, "Choose a valid workspace export file.")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, _, err := r.FormFile("workspace")
		if err != nil {
			writeMutationError(w, r, http.StatusBadRequest, "Choose a workspace export file.")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxWorkspaceImportBytes+1))
		if err != nil {
			writeMutationError(w, r, http.StatusBadRequest, "Could not read the workspace export file.")
			return
		}
		if len(data) > maxWorkspaceImportBytes {
			writeMutationError(w, r, http.StatusRequestEntityTooLarge, "Workspace import must be smaller than 32 MB.")
			return
		}
		imported, err := workspacearchive.Decode(data)
		if err != nil {
			writeMutationError(w, r, http.StatusBadRequest, "Workspace import was rejected: "+err.Error())
			return
		}

		current := appstate.MustGet().Snapshot()
		next := workspacearchive.PreserveCurrentIdentity(current, imported)
		next, err = workspacearchive.RebaseUploadPaths(next, filepath.Join(root, "data", "uploads"))
		if err != nil {
			writeMutationError(w, r, http.StatusBadRequest, "Workspace import was rejected: "+err.Error())
			return
		}
		if err := next.Validate(); err != nil {
			writeMutationError(w, r, http.StatusBadRequest, "Workspace import was rejected: "+err.Error())
			return
		}
		backupPath, err := workspacearchive.WriteBackup(backupDirectory, current)
		if err != nil {
			log.Printf("create workspace import backup: %v", err)
			writeMutationError(w, r, http.StatusInternalServerError, "Could not create the required pre-import backup.")
			return
		}
		if err := appstate.MustGet().Replace(next, domain.AuditMeta{
			Actor:      workspaceActor(r),
			Action:     "import.workspace",
			EntityType: "workspace",
			EntityID:   "workspace",
			Summary:    "Validated workspace import applied; local sign-in principals retained.",
			Origin:     "organizer-import",
		}); err != nil {
			log.Printf("replace imported workspace after backup %s: %v", backupPath, err)
			writeMutationError(w, r, http.StatusInternalServerError, "Could not apply the workspace import; the pre-import backup was retained.")
			return
		}
		session.AddFlash(r, "notice", "Workspace restored. This host's sign-in principals were retained; a pre-import backup was created.")
		live.Broadcast("workspace:restored", map[string]any{"at": time.Now().UTC()})
		writeMutationSuccess(w, r, "Workspace restored. This host's sign-in principals were retained.", "/organizer/settings")
	}
}

func recordWorkspaceExport(r *http.Request, actionName, exportName string) error {
	return appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      workspaceActor(r),
		Action:     actionName,
		EntityType: "workspace",
		EntityID:   "workspace",
		Summary:    "Operator exported " + exportName + ".",
		Origin:     "organizer-export",
	}, func(*domain.State) error {
		return nil
	})
}

func workspaceActor(r *http.Request) string {
	if user, ok := auth.Current(r); ok && strings.TrimSpace(user.ID) != "" {
		return "organizer:" + user.ID
	}
	return "organizer"
}

// calendarDownload serves GET /calendar/{speaker}.ics.
//
// Authorization (SE-6): a visitor must be one of three things -- the
// speaker's own bound portal session (portalSpeakerSessionKey, set by
// app/portal/page.server.go's loadPortal), an organizer-facing session
// (isOrganizerSession), or the holder of a signed portal token for this
// speaker, presented as ?key=<token> the way an emailed calendar-subscribe
// link does (internal/token, reused from PT-2). Any other request -- an
// unkeyed fetch from a stranger -- gets the same friendly not-found
// response as an unknown speaker ID, never a file.
func calendarDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	speakerID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/calendar/"), ".ics")

	visitor := session.Current(r)
	isOrganizer := isOrganizerSession(r)
	isBoundSpeaker := speakerID != "" && visitor.String(portalSpeakerSessionKey) == speakerID
	isKeyed := false
	if key := strings.TrimSpace(r.URL.Query().Get("key")); key != "" {
		if id, ok := token.New().Verify(key); ok && id == speakerID {
			isKeyed = true
		}
	}
	if !isOrganizer && !isBoundSpeaker && !isKeyed {
		http.NotFound(w, r)
		return
	}

	data, filename, err := programcalendar.SpeakerCalendar(appstate.MustGet().Snapshot(), speakerID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "private, max-age=60")
	_, _ = w.Write(data)
}

func portalUpload(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/portal-upload/"), "/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		speakerID, taskID := parts[0], parts[1]
		// Authorize before multipart parsing or creating a file. A portal
		// session is bound to exactly one speaker; organizers and chairs may
		// assist an assigned speaker, while observers remain read-only.
		// Return the same 404 for a missing speaker/task, an unassigned task,
		// or an unauthorized visitor so this endpoint cannot enumerate portal
		// identifiers or task assignments.
		state := appstate.MustGet().Snapshot()
		visitor := session.Current(r)
		if visitor.String(portalSpeakerSessionKey) != speakerID && !canMutateWorkspace(r) {
			http.NotFound(w, r)
			return
		}
		if _, found := state.Speaker(speakerID); !found {
			http.NotFound(w, r)
			return
		}
		task, found := state.Task(taskID)
		if !found || !state.TaskAssignedToSpeaker(*task, speakerID) || !uploadTaskType(*task) {
			http.NotFound(w, r)
			return
		}
		// bodyLimit (registered ahead of sessions.Protect) has already wrapped
		// r.Body in a MaxBytesReader sized to uploadBodyEnvelope, so the CSRF
		// check's ParseMultipartForm call and this one are both bounded.
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			writeMutationError(w, r, http.StatusRequestEntityTooLarge, "Upload must be smaller than 10 MB.")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeMutationError(w, r, http.StatusBadRequest, "Choose a file to upload.")
			return
		}
		defer file.Close()
		originalName := filepath.Base(header.Filename)
		if !allowedUpload(originalName) {
			writeMutationError(w, r, http.StatusUnsupportedMediaType, "Use PDF, PowerPoint, Keynote, PNG, JPEG, or WebP.")
			return
		}
		// A headshot is a public-facing image once an organizer approves it.
		// Enforce both its extension and its sniffed bytes here, before the
		// payload reaches disk, rather than trusting multipart Content-Type.
		var source io.Reader = file
		if present.IsHeadshotTask(*task) {
			if !allowedHeadshot(originalName) {
				writeMutationError(w, r, http.StatusUnsupportedMediaType, "Headshots must be PNG, JPEG, or WebP images.")
				return
			}
			prefix := make([]byte, 512)
			count, readErr := io.ReadFull(file, prefix)
			if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
				writeMutationError(w, r, http.StatusBadRequest, "Could not read the headshot image.")
				return
			}
			if !allowedHeadshotContentType(http.DetectContentType(prefix[:count])) {
				writeMutationError(w, r, http.StatusUnsupportedMediaType, "Headshots must contain a PNG, JPEG, or WebP image.")
				return
			}
			source = io.MultiReader(bytes.NewReader(prefix[:count]), file)
		}
		uploadDir := filepath.Join(root, "data", "uploads")
		if err := os.MkdirAll(uploadDir, 0o750); err != nil {
			writeMutationError(w, r, http.StatusInternalServerError, "Could not prepare upload storage.")
			return
		}
		storedName := domain.NewID("upload") + strings.ToLower(filepath.Ext(originalName))
		storedPath := filepath.Join(uploadDir, storedName)
		destination, err := os.OpenFile(storedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			writeMutationError(w, r, http.StatusInternalServerError, "Could not create upload.")
			return
		}
		// Copy one byte past the cap: reading exactly maxUploadBytes+1 without
		// error means the source held more than the cap, so treat it as
		// oversized rather than silently truncating and reporting success.
		written, copyErr := io.CopyN(destination, source, maxUploadBytes+1)
		closeErr := destination.Close()
		var maxBytesErr *http.MaxBytesError
		switch {
		case written > maxUploadBytes:
			_ = os.Remove(storedPath)
			writeMutationError(w, r, http.StatusRequestEntityTooLarge, "Upload must be smaller than 10 MB.")
			return
		case errors.As(copyErr, &maxBytesErr):
			_ = os.Remove(storedPath)
			writeMutationError(w, r, http.StatusRequestEntityTooLarge, "Upload must be smaller than 10 MB.")
			return
		case copyErr != nil && !errors.Is(copyErr, io.EOF):
			_ = os.Remove(storedPath)
			writeMutationError(w, r, http.StatusInternalServerError, "Could not store upload.")
			return
		case closeErr != nil || written == 0:
			_ = os.Remove(storedPath)
			writeMutationError(w, r, http.StatusInternalServerError, "Could not store upload.")
			return
		}
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		completionID := ""
		supersededPath := ""
		if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
			Actor:      uploadActor(r, speakerID),
			Action:     "portal.file_uploaded",
			EntityType: "task_completion",
			EntityID:   taskID + ":" + speakerID,
			Summary:    "Speaker task file submitted for review.",
			Origin:     "portal-upload",
		}, func(state *domain.State) error {
			if _, found := state.Speaker(speakerID); !found {
				return fmt.Errorf("speaker %s not found", speakerID)
			}
			task, found := state.Task(taskID)
			if !found || !state.TaskAssignedToSpeaker(*task, speakerID) || !uploadTaskType(*task) {
				return fmt.Errorf("task %s not found", taskID)
			}
			now := time.Now().UTC()
			completion, found := state.Completion(taskID, speakerID)
			if !found {
				completionID = domain.NewID("done")
				state.TaskCompletions = append(state.TaskCompletions, domain.TaskCompletion{
					ID: completionID, TaskID: taskID, SpeakerID: speakerID, Status: domain.TaskSubmitted,
					FileName: originalName, ContentType: contentType, StoredPath: filepath.ToSlash(storedPath), CompletedAt: now, UpdatedAt: now,
				})
				if present.IsHeadshotTask(*task) {
					speaker, _ := state.Speaker(speakerID)
					speaker.HeadshotURL = "/portal-file/" + completionID
				}
				return nil
			}
			completionID = completion.ID
			supersededPath = completion.StoredPath
			completion.Status = domain.TaskSubmitted
			completion.FileName = originalName
			completion.ContentType = contentType
			completion.StoredPath = filepath.ToSlash(storedPath)
			completion.CompletedAt = now
			completion.UpdatedAt = now
			if present.IsHeadshotTask(*task) {
				speaker, _ := state.Speaker(speakerID)
				speaker.HeadshotURL = "/portal-file/" + completionID
			}
			return nil
		}); err != nil {
			_ = os.Remove(storedPath)
			writeMutationError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		// The state commit is durable before cleanup begins. Remove the old
		// private upload only when it is a safe, unreferenced file inside this
		// workspace's upload directory; a failed cleanup must not turn a
		// successful submission into an error.
		removeSupersededUpload(root, supersededPath, filepath.ToSlash(storedPath))
		if present.IsHeadshotTask(*task) {
			// A re-upload returns the completion to submitted. Remove any
			// previously published image immediately so an old approved
			// headshot cannot remain directly reachable while review is pending.
			removePublicHeadshots(root, speakerID)
		}
		session.AddFlash(r, "notice", "File uploaded and submitted for review.")
		live.Broadcast("task:uploaded", map[string]string{"speaker": speakerID, "task": taskID})
		writeMutationSuccess(w, r, "File uploaded and submitted for review.", "/portal/"+speakerID+"#tasks")
	}
}

// removeSupersededUpload removes a prior private upload after a successful
// replacement. Imported or hand-edited state may contain arbitrary paths, so
// both paths are normalized and constrained to the direct children of the
// real data/uploads directory. The current state is checked first so a file
// shared by another completion is retained. Filesystem cleanup is deliberately
// best effort: the state transaction is already committed and must remain
// successful even if a file is locked or disappears concurrently.
func removeSupersededUpload(root, previousPath, replacementPath string) {
	previous, ok := privateUploadPath(root, previousPath)
	if !ok {
		return
	}
	replacement, ok := privateUploadPath(root, replacementPath)
	if !ok || filepath.Clean(previous) == filepath.Clean(replacement) {
		return
	}
	for _, completion := range appstate.MustGet().Snapshot().TaskCompletions {
		path, referenced := privateUploadPath(root, completion.StoredPath)
		if referenced && filepath.Clean(path) == filepath.Clean(previous) {
			return
		}
	}
	info, err := os.Lstat(previous)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		log.Printf("portal upload: inspect superseded private upload %s: %v", previous, err)
		return
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		log.Printf("portal upload: refusing to remove non-regular superseded upload %s", previous)
		return
	}
	if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("portal upload: remove superseded private upload %s: %v", previous, err)
	}
}

// privateUploadPath accepts only an absolute path to a direct file beneath
// data/uploads. Resolving the directory and its parent prevents a symlinked
// uploads directory or nested symlink path from turning cleanup into an
// arbitrary filesystem delete. Current portal uploads are absolute paths, so
// relative legacy/import paths are intentionally rejected rather than guessed.
func privateUploadPath(root, storedPath string) (string, bool) {
	if strings.TrimSpace(storedPath) == "" {
		return "", false
	}
	uploadDir, err := filepath.Abs(filepath.Join(root, "data", "uploads"))
	if err != nil {
		return "", false
	}
	realUploadDir, err := filepath.EvalSymlinks(uploadDir)
	if err != nil {
		return "", false
	}
	candidate := filepath.FromSlash(strings.TrimSpace(storedPath))
	if !filepath.IsAbs(candidate) {
		return "", false
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(uploadDir, candidate)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) || strings.Contains(relative, string(filepath.Separator)) {
		return "", false
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil || filepath.Clean(parent) != filepath.Clean(realUploadDir) {
		return "", false
	}
	return filepath.Join(parent, filepath.Base(candidate)), true
}

func uploadActor(r *http.Request, speakerID string) string {
	if canMutateWorkspace(r) {
		if user, ok := auth.Current(r); ok && user.ID != "" {
			return "organizer:" + user.ID
		}
		return "organizer"
	}
	return "speaker:" + speakerID
}

func uploadTaskType(task domain.Task) bool {
	return task.Type == "file" || task.Type == "headshot"
}

func allowedUpload(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf", ".ppt", ".pptx", ".key", ".png", ".jpg", ".jpeg", ".webp":
		return true
	default:
		return false
	}
}

func allowedHeadshot(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	default:
		return false
	}
}

func allowedHeadshotContentType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

// downloadContentTypes lists the MIME types portalFile trusts to send as
// Content-Type on a download. A completion's stored ContentType came from a
// client-supplied multipart header at upload time (main.go's portalUpload),
// so it is untrusted; any value outside this allowlist degrades to
// application/octet-stream. X-Content-Type-Options: nosniff is already set
// globally by securityHeaders, so a browser never sniffs past either value.
var downloadContentTypes = map[string]bool{
	"application/pdf":               true,
	"application/vnd.ms-powerpoint": true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.apple.keynote":                                             true,
	"application/x-iwork-keynote-sffkey":                                        true,
	"image/png":                                                                 true,
	"image/jpeg":                                                                true,
	"image/webp":                                                                true,
}

func downloadContentType(stored string) string {
	stored = strings.TrimSpace(stored)
	if downloadContentTypes[strings.ToLower(stored)] {
		return stored
	}
	return "application/octet-stream"
}

// sanitizeDownloadFilename returns a Content-Disposition-safe filename.
// portalUpload already ran the original name through filepath.Base and an
// extension allowlist before storing it, so this is defense in depth rather
// than the primary control: it strips any path separator, quote, or control
// character so a corrupted or hand-edited store record can never inject a
// header value, and falls back to a generic name when nothing safe remains.
func sanitizeDownloadFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "download"
	}
	var clean strings.Builder
	for _, r := range name {
		if r == '"' || r == '\\' || r < 0x20 || r == 0x7f {
			continue
		}
		clean.WriteRune(r)
	}
	if sanitized := strings.TrimSpace(clean.String()); sanitized != "" {
		return sanitized
	}
	return "download"
}

// portalFile serves GET /portal-file/{completionID}, the download route for
// files speakers upload through portalUpload (PT-1).
//
// Path traversal defense: the request path segment is never used to build a
// filesystem path. It is only compared for equality against the ID field of
// each stored TaskCompletion; the byte that reaches the filesystem is always
// completion.StoredPath, a value this process itself wrote in portalUpload,
// and even that is re-validated with filepath.Rel to stay inside
// data/uploads before opening.
//
// Authorization: the requester must either be the speaker who owns the
// completion (the portalSpeakerSessionKey session binding PT-2 sets in
// app/portal/page.server.go) or carry an organizer-facing session
// (isOrganizerSession). A missing completion and an unauthorized one both
// render the same 404 —
// no different status distinguishes "no such file" from "not yours" — so a
// stranger cannot use the response to enumerate valid completion IDs
// (mirrors loadPortal's identical treatment of unknown-speaker vs.
// missing-key in app/portal/page.server.go).
func portalFile(root string) http.HandlerFunc {
	uploadDir := filepath.Join(root, "data", "uploads")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		completionID := strings.TrimPrefix(r.URL.Path, "/portal-file/")
		if completionID == "" || strings.ContainsAny(completionID, "/\\") {
			http.NotFound(w, r)
			return
		}

		state := appstate.MustGet().Snapshot()
		var completion *domain.TaskCompletion
		for index := range state.TaskCompletions {
			if state.TaskCompletions[index].ID == completionID {
				completion = &state.TaskCompletions[index]
				break
			}
		}
		if completion == nil || completion.StoredPath == "" {
			http.NotFound(w, r)
			return
		}

		visitor := session.Current(r)
		isOwner := completion.SpeakerID != "" && visitor.String(portalSpeakerSessionKey) == completion.SpeakerID
		isOrganizer := isOrganizerSession(r)
		if !isOwner && !isOrganizer {
			http.NotFound(w, r)
			return
		}

		storedPath := filepath.FromSlash(completion.StoredPath)
		rel, err := filepath.Rel(uploadDir, storedPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			log.Printf("portal-file: completion %s stored path resolves outside data/uploads; refusing to serve", completionID)
			http.NotFound(w, r)
			return
		}

		file, err := os.Open(storedPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.Error(w, "could not read upload", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", downloadContentType(completion.ContentType))
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeDownloadFilename(completion.FileName)+`"`)
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, "", info.ModTime(), file)
	}
}

func resetDemo(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Reset wipes the workspace, so it must not be reachable by any
		// visitor who merely holds a session CSRF token (SE-8/M2). Require a
		// shared secret when RESET_SECRET is configured; refuse entirely in
		// production when none is set, so a deployed demo cannot be wiped out
		// from under a judge.
		if secret := strings.TrimSpace(getenv("RESET_SECRET", "")); secret != "" {
			provided := r.URL.Query().Get("secret")
			if provided == "" {
				provided = r.FormValue("secret")
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
				http.NotFound(w, r)
				return
			}
		} else if strings.EqualFold(getenv("APP_ENV", "development"), "production") {
			http.NotFound(w, r)
			return
		}
		if err := appstate.MustGet().Reset(); err != nil {
			writeMutationError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		// SE-8/M6: Reset only rewinds the JSONStore's in-memory and on-disk
		// state; it never touches the filesystem. Without this, a speaker's
		// uploaded file (main.go's portalUpload) would survive a reset with
		// no TaskCompletion left pointing at it -- an orphaned file outliving
		// the workspace that thinks it discarded it.
		clearUploads(root)
		session.AddFlash(r, "notice", "Workspace restored to the polished demo baseline.")
		live.Broadcast("workspace:reset", map[string]any{"at": time.Now().UTC()})
		writeMutationSuccess(w, r, "Workspace restored to the polished demo baseline.", "/organizer")
	}
}

// clearUploads removes every file under data/uploads so a workspace reset
// (SE-8/M6) discards speaker-uploaded artifacts along with the state that
// referenced them. A missing uploads directory is not an error -- a
// workspace nobody has uploaded to yet has none -- and a single file that
// resists removal is logged and skipped rather than failing the reset.
func clearUploads(root string) {
	uploadDir := filepath.Join(root, "data", "uploads")
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("reset: could not read %s: %v", uploadDir, err)
		}
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(uploadDir, entry.Name())); err != nil {
			log.Printf("reset: could not remove upload %s: %v", entry.Name(), err)
		}
	}
}

// removePublicHeadshots removes every supported public variant for a speaker.
// It is used when a speaker replaces an already-approved headshot: the new
// completion must wait for approval, so the old image may not remain publicly
// reachable merely because its deterministic static path is still on disk.
func removePublicHeadshots(root, speakerID string) {
	if speakerID == "" {
		return
	}
	dir := filepath.Join(root, "public", "headshots")
	for _, extension := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		path := filepath.Join(dir, speakerID+extension)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("portal upload: remove stale public headshot %s: %v", path, err)
		}
	}
}

func writeMutationSuccess(w http.ResponseWriter, r *http.Request, message, redirect string) {
	if requestWantsJSON(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":       true,
			"message":  message,
			"redirect": redirect,
		})
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func writeMutationError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if requestWantsJSON(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": message})
		return
	}
	http.Error(w, message, status)
}

func requestWantsJSON(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		r.Header.Get("X-Requested-With") == "XMLHttpRequest"
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
