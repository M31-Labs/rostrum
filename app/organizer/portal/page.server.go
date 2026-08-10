package portal

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.PortalOperations(appstate.MustGet().Snapshot()), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Portal & tasks — Rostrum"}, Description: "Real-time speaker onboarding and resource operations."}, nil
		},
		Actions: route.FileActions{"approveTask": approveTask},
	}); err != nil {
		log.Fatal(err)
	}
}

func approveTask(ctx *action.Context) error {
	taskID := strings.TrimSpace(ctx.FormData["task_id"])
	speakerID := strings.TrimSpace(ctx.FormData["speaker_id"])
	if taskID == "" || speakerID == "" {
		return action.Validation("Choose a submitted task to approve.", nil, ctx.FormData)
	}
	name := ""
	taskTitle := ""
	// publish, when set, copies the just-approved headshot upload into
	// public/headshots/ so the public gallery (internal/present/public_event.go)
	// can link an unauthenticated <img> to it. It runs after Update returns,
	// not inside the closure, so the store's write lock never waits on disk
	// I/O for the copy.
	var publish func() error
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		speaker, found := state.Speaker(speakerID)
		if !found {
			return fmt.Errorf("speaker %s not found", speakerID)
		}
		name = speaker.Name()
		completion, found := state.Completion(taskID, speakerID)
		if !found {
			return fmt.Errorf("task submission not found")
		}
		if completion.Status != domain.TaskSubmitted {
			return action.Validation("That task is no longer awaiting approval.", nil, ctx.FormData)
		}
		task, found := state.Task(taskID)
		if !found {
			return fmt.Errorf("task %s not found", taskID)
		}
		taskTitle = task.Title
		completion.Status = domain.TaskApproved
		completion.UpdatedAt = time.Now().UTC()
		if present.IsHeadshotTask(*task) && completion.StoredPath != "" {
			publish = publishHeadshotCopy(speakerID, completion.StoredPath, completion.FileName)
		}
		return nil
	}); err != nil {
		return err
	}
	if publish != nil {
		if err := publish(); err != nil {
			// A failed publish must not fail the approval itself: the
			// completion is already approved and the organizer's action
			// succeeded. Log so the gap is visible without turning a
			// filesystem hiccup into a lost approval.
			log.Printf("approve task: publish public headshot for speaker %s: %v", speakerID, err)
		}
	}
	session.AddFlash(ctx.Request, "notice", "Approved “"+taskTitle+"” from "+name+".")
	live.Broadcast("task:approved", map[string]string{"task": taskID, "speaker": speakerID})
	actionflow.Redirect(ctx, "/organizer/portal")
	return nil
}

// publishHeadshotCopy returns a closure that copies the approved upload at
// storedPath into public/headshots/<speakerID><ext>, the path
// internal/present/public_event.go's publicHeadshotURL derives for the
// public gallery. Approval is the publication consent gate (PT-3): nothing
// copies into public/ before an organizer approves the task, and
// /portal-file/ itself — storedPath's origin — stays authenticated.
func publishHeadshotCopy(speakerID, storedPath, fileName string) func() error {
	return func() error {
		source, err := os.Open(storedPath)
		if err != nil {
			return fmt.Errorf("open approved headshot upload: %w", err)
		}
		defer source.Close()

		dir := publicHeadshotDir()
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("prepare public headshot directory: %w", err)
		}
		destination := filepath.Join(dir, speakerID+headshotExtension(fileName))
		temporary, err := os.CreateTemp(dir, ".headshot-*.tmp")
		if err != nil {
			return fmt.Errorf("create temporary public headshot: %w", err)
		}
		temporaryPath := temporary.Name()
		cleanup := func() {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
		if _, err := io.Copy(temporary, source); err != nil {
			cleanup()
			return fmt.Errorf("copy public headshot: %w", err)
		}
		if err := temporary.Close(); err != nil {
			cleanup()
			return fmt.Errorf("close temporary public headshot: %w", err)
		}
		if err := os.Chmod(temporaryPath, 0o644); err != nil {
			cleanup()
			return fmt.Errorf("set public headshot permissions: %w", err)
		}
		if err := os.Rename(temporaryPath, destination); err != nil {
			cleanup()
			return fmt.Errorf("publish public headshot: %w", err)
		}
		return nil
	}
}

// publicHeadshotDir resolves the on-disk public/headshots directory.
// approveTask runs outside main.go's own root resolution, and
// server.ResolveAppRoot falls back to the process's working directory when
// neither the executable path nor this file's directory looks like an app
// root — not reliable for a package that does not live at the repo root.
// Instead this walks up from this source file's own build-time location
// (app/organizer/portal/page.server.go is four directories below root),
// the same runtime.Caller(0) pattern app/portal/page.server.go's init
// already uses to locate its page.gsx relative to itself.
func publicHeadshotDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	return filepath.Join(root, "public", "headshots")
}

// headshotExtension returns the lowercased, dotted extension of fileName
// (for example ".jpg"), defaulting to ".jpg" when fileName has none. Mirrors
// internal/present/public_event.go's fileExtension so both sides of the
// approve-time copy agree on the same public URL.
func headshotExtension(fileName string) string {
	if dot := strings.LastIndex(fileName, "."); dot >= 0 {
		return strings.ToLower(fileName[dot:])
	}
	return ".jpg"
}
