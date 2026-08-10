package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/appstate"
	programcalendar "github.com/odvcencio/programma/internal/calendar"
	"github.com/odvcencio/programma/internal/domain"
	"github.com/odvcencio/programma/internal/live"
	"github.com/odvcencio/programma/internal/present"
	"github.com/odvcencio/programma/internal/publicapi"
	"github.com/odvcencio/programma/internal/store"
	_ "github.com/odvcencio/programma/modules"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx/controller"
	"m31labs.dev/gosx/env"
	"m31labs.dev/gosx/hydrate"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

const developmentSessionSecret = "programma-development-secret-change-me"

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	root := server.ResolveAppRoot(thisFile)
	if err := env.LoadDir(root, ""); err != nil {
		log.Fatal(err)
	}

	now := time.Now().UTC()
	dataPath := getenv("DATA_PATH", filepath.Join(root, "data", "programma.json"))
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
		return server.HTMLDocument(ctx.Title("Programma"), ctx.Head(), body)
	})
	if err := router.AddDir(filepath.Join(root, "app"), route.FileRoutesOptions{}); err != nil {
		log.Fatal(err)
	}

	app := server.New()
	app.EnableGzip()
	app.Use(securityHeaders(publicBase, navigationScriptCSPHash()))
	app.Use(sessions.Middleware)
	app.Use(sessions.Protect)
	app.SetPublicDir(filepath.Join(root, "public"))
	app.API("GET /api/health", func(ctx *server.Context) (any, error) {
		ctx.CachePublic(30 * time.Second)
		return map[string]any{
			"ok":      true,
			"app":     "Programma",
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
	app.Mount("/organizer/export/submissions.csv", http.HandlerFunc(submissionsCSV))
	app.Mount("/favicon.ico", http.RedirectHandler("/favicon.svg", http.StatusTemporaryRedirect))
	app.Mount("/demo/reset", http.HandlerFunc(resetDemo))

	rootHandler, err := router.BuildChecked()
	if err != nil {
		log.Fatal(err)
	}
	app.Mount("/", rootHandler)

	log.Printf("Programma listening on %s (data: %s)", publicBase, workspace.Path())
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
			Name: "programma-workspace-preferences",
			Root: "#workspace-sidebar",
			Storage: &controller.Storage{
				Area:      "local",
				Namespace: "programma-workspace",
				Load: []controller.StorageSlot{{
					Key:    "rail-collapsed",
					Signal: "$programmaWorkspaceRail",
				}},
				Save: []controller.StorageSlot{{
					Key:    "rail-collapsed",
					Signal: "$programmaWorkspaceRail",
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
			Name: "programma-public-itinerary",
			Root: ".public-itinerary",
			Storage: &controller.Storage{
				Area:      "local",
				Namespace: "programma-itinerary-" + slug,
				Load: []controller.StorageSlot{{
					Key:    "sessions",
					Signal: "$programmaItinerary",
				}},
				Save: []controller.StorageSlot{{
					Key:    "sessions",
					Signal: "$programmaItinerary",
				}},
			},
		})
	}

	switch path {
	case "/organizer":
		ctx.Runtime().BindHub("programma-overview", "/live", refreshBindings(
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
		ctx.Runtime().BindHub("programma-portal-matrix", "/live", refreshBindings(
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
	w.Header().Set("Content-Disposition", `attachment; filename="programma-submissions.csv"`)
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
		r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
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
		written, copyErr := io.Copy(destination, io.LimitReader(file, 10<<20))
		closeErr := destination.Close()
		if copyErr != nil || closeErr != nil || written == 0 {
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

func resetDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
