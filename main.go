package main

import (
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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	programcalendar "github.com/m31-labs/rostrum/internal/calendar"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/publicapi"
	"github.com/m31-labs/rostrum/internal/store"
	_ "github.com/m31-labs/rostrum/modules"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx/controller"
	"m31labs.dev/gosx/env"
	"m31labs.dev/gosx/hydrate"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

const developmentSessionSecret = "rostrum-development-secret-change-me"

// Session keys shared with the portal route module
// (app/portal/page.server.go). portalSpeakerSessionKey mirrors that
// package's private portalSessionKey constant; keep the literal in sync if
// either side changes it. portalAdminSessionKey is new here: organizerContext
// sets it for any session that has visited the (documented, proxy-gated)
// `/organizer` surface, and portalFile accepts it as an alternative to the
// owning speaker's own binding.
const (
	portalSpeakerSessionKey = "portal_speaker"
	portalAdminSessionKey   = "portal_admin"
)

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	root := server.ResolveAppRoot(thisFile)
	if err := env.LoadDir(root, ""); err != nil {
		log.Fatal(err)
	}

	now := time.Now().UTC()
	dataPath := getenv("DATA_PATH", filepath.Join(root, "data", "rostrum.json"))
	if strings.EqualFold(getenv("DEMO_MODE", "true"), "memory") {
		dataPath = ":memory:"
	}
	workspace, err := store.Open(dataPath, domain.Seed(now))
	if err != nil {
		log.Fatal(err)
	}
	appstate.Set(workspace)

	port := getenv("PORT", "8080")
	publicBase := getenv("PUBLIC_URL", "http://localhost:"+port)
	appEnv := strings.ToLower(getenv("APP_ENV", "development"))
	sessionSecret := getenv("SESSION_SECRET", developmentSessionSecret)
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

	router := route.NewRouter()
	router.SetLayout(func(ctx *route.RouteContext, body gosx.Node) gosx.Node {
		ctx.SetMetadata(server.Metadata{
			Links: []server.LinkTag{
				{Rel: "preconnect", Href: "https://fonts.googleapis.com"},
				{Rel: "preconnect", Href: "https://fonts.gstatic.com", CrossOrigin: "anonymous"},
				{Rel: "stylesheet", Href: "/styles.css"},
				{Rel: "stylesheet", Href: "https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@500&family=Space+Grotesk:wght@500;600&family=Work+Sans:wght@400;500;600&display=swap"},
				{Rel: "icon", Href: "/favicon.svg", Type: "image/svg+xml"},
			},
		})
		ctx.AddHead(gosx.El("meta", gosx.Attrs(
			gosx.Attr("name", "theme-color"),
			gosx.Attr("content", "#f4f1e8"),
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
	app.EnableGzip()
	app.Use(securityHeaders(publicBase, navigationScriptCSPHash()))
	app.Use(sessions.Middleware)
	app.Use(organizerContext())
	app.Use(bodyLimit())
	app.Use(sessions.Protect)
	app.SetPublicDir(filepath.Join(root, "public"))
	app.API("GET /api/health", func(ctx *server.Context) (any, error) {
		ctx.CachePublic(30 * time.Second)
		return map[string]any{
			"ok":      true,
			"app":     "Rostrum",
			"version": gosx.Version,
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
	app.Mount("/live", live.Dashboard)
	app.Mount("/calendar/", http.HandlerFunc(calendarDownload))
	app.Mount("/portal-upload/", http.HandlerFunc(portalUpload(root)))
	app.Mount("/portal-file/", http.HandlerFunc(portalFile(root)))
	app.Mount("/organizer/export/submissions.csv", http.HandlerFunc(submissionsCSV))
	app.Mount("/favicon.ico", http.RedirectHandler("/favicon.svg", http.StatusTemporaryRedirect))
	app.Mount("/demo/reset", http.HandlerFunc(resetDemo))

	rootHandler, err := router.BuildChecked()
	if err != nil {
		log.Fatal(err)
	}
	app.Mount("/", rootHandler)

	log.Printf("Rostrum listening on %s (data: %s)", publicBase, workspace.Path())
	log.Fatal(app.ListenAndServe(":" + port))
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
			"event:updated",
			"form:updated",
			"integration:dry-run",
			"integration:live-sync",
			"review:updated",
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

func securityHeaders(publicBase, navigationHash string) server.Middleware {
	// GoSX islands execute the framework's compiled WebAssembly VM. Authorize
	// WebAssembly compilation without opening generic eval or inline scripts.
	scriptPolicy := "script-src 'self' 'wasm-unsafe-eval'"
	if navigationHash != "" {
		scriptPolicy += " " + navigationHash
	}
	policy := "default-src 'self'; base-uri 'self'; object-src 'none'; " + scriptPolicy + "; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self' ws: wss:; frame-src 'self' https://www.youtube-nocookie.com https://player.vimeo.com; frame-ancestors *; form-action 'self'"
	secure := strings.HasPrefix(publicBase, "https://")
	if secure {
		policy += "; upgrade-insecure-requests"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", policy)
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			if secure {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Upload body sizing. uploadBodyEnvelope bounds the whole multipart request
// (the 10 MiB file cap plus boundary and header overhead); maxUploadBytes
// bounds the stored file payload itself. defaultBodyLimit bounds every other
// route so no form endpoint spools an unbounded body to memory or disk.
const (
	uploadBodyEnvelope = 12 << 20
	maxUploadBytes     = 10 << 20
	defaultBodyLimit   = 1 << 20
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
			if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/portal-upload/") {
				limit = uploadBodyEnvelope
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// organizerContext binds a visiting session to the organizer surface. The
// `/organizer` prefix carries no application-level identity of its own —
// docs/deployment.md documents that a production deployment must gate it at
// the reverse proxy — so this middleware is the honest reflection of that
// boundary: whoever the proxy already let reach `/organizer` is trusted as
// an organizer for the rest of that session, including file downloads
// through portalFile.
func organizerContext() server.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimSuffix(r.URL.Path, "/")
			if path == "/organizer" || strings.HasPrefix(path, "/organizer/") {
				session.Current(r).Set(portalAdminSessionKey, "1")
			}
			next.ServeHTTP(w, r)
		})
	}
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

func submissionsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state := appstate.MustGet().Snapshot()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="rostrum-submissions.csv"`)
	w.Header().Set("Cache-Control", "private, no-store")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"id", "title", "status", "category", "format", "level", "speakers", "routed_owner", "submitted_at"})
	for _, submission := range state.Submissions {
		_ = writer.Write([]string{
			submission.ID, submission.Title, submission.Status, present.CategoryName(state, submission.CategoryID),
			submission.Format, submission.Level, present.SpeakerNames(state, submission.SpeakerIDs),
			submission.RoutedOwner, submission.SubmittedAt.Format(time.RFC3339),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Printf("write submissions export: %v", err)
	}
}

func calendarDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	speakerID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/calendar/"), ".ics")
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
		// bodyLimit (registered ahead of sessions.Protect) has already wrapped
		// r.Body in a MaxBytesReader sized to uploadBodyEnvelope, so the CSRF
		// check's ParseMultipartForm call and this one are both bounded.
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			writeMutationError(w, r, http.StatusRequestEntityTooLarge, "Upload must be smaller than 10 MB.")
			return
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
		written, copyErr := io.CopyN(destination, file, maxUploadBytes+1)
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
		if err := appstate.MustGet().Update(func(state *domain.State) error {
			if _, found := state.Speaker(speakerID); !found {
				return fmt.Errorf("speaker %s not found", speakerID)
			}
			if _, found := state.Task(taskID); !found {
				return fmt.Errorf("task %s not found", taskID)
			}
			now := time.Now().UTC()
			completion, found := state.Completion(taskID, speakerID)
			if !found {
				state.TaskCompletions = append(state.TaskCompletions, domain.TaskCompletion{
					ID: domain.NewID("done"), TaskID: taskID, SpeakerID: speakerID, Status: domain.TaskSubmitted,
					FileName: originalName, ContentType: contentType, StoredPath: filepath.ToSlash(storedPath), CompletedAt: now, UpdatedAt: now,
				})
				return nil
			}
			completion.Status = domain.TaskSubmitted
			completion.FileName = originalName
			completion.ContentType = contentType
			completion.StoredPath = filepath.ToSlash(storedPath)
			completion.CompletedAt = now
			completion.UpdatedAt = now
			return nil
		}); err != nil {
			_ = os.Remove(storedPath)
			writeMutationError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		session.AddFlash(r, "notice", "File uploaded and submitted for review.")
		live.Broadcast("task:uploaded", map[string]string{"speaker": speakerID, "task": taskID})
		writeMutationSuccess(w, r, "File uploaded and submitted for review.", "/portal/"+speakerID+"#tasks")
	}
}

func allowedUpload(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf", ".ppt", ".pptx", ".key", ".png", ".jpg", ".jpeg", ".webp":
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
// app/portal/page.server.go) or an organizer (portalAdminSessionKey, set by
// organizerContext above for any session that has reached `/organizer`).
// A missing completion and an unauthorized one both render the same 404 —
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
		isOrganizer := visitor.String(portalAdminSessionKey) == "1"
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

func resetDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Reset wipes the workspace, so it must not be reachable by any visitor
	// who merely holds a session CSRF token (SE-8/M2). Require a shared secret
	// when RESET_SECRET is configured; refuse entirely in production when none
	// is set, so a deployed demo cannot be wiped out from under a judge.
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
	session.AddFlash(r, "notice", "Workspace restored to the polished demo baseline.")
	live.Broadcast("workspace:reset", map[string]any{"at": time.Now().UTC()})
	writeMutationSuccess(w, r, "Workspace restored to the polished demo baseline.", "/organizer")
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
