package portal

import (
	"errors"
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
	delivery "github.com/m31-labs/rostrum/internal/communications"
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

	// Stage the bytes before the state mutation. The final rename happens
	// inside the mutation closure and a failed staging/rename aborts approval,
	// so a completion can never claim "approved" while its public headshot is
	// missing. The copy itself is outside the store lock; only the atomic
	// rename is performed while the private state copy is being finalized.
	var staged *stagedHeadshot
	snapshot := appstate.MustGet().Snapshot()
	if task, taskFound := snapshot.Task(taskID); taskFound && present.IsHeadshotTask(*task) {
		completion, completionFound := snapshot.Completion(taskID, speakerID)
		if !completionFound || completion.StoredPath == "" {
			return action.Validation("A headshot file must be uploaded before approval.", nil, ctx.FormData)
		}
		var stageErr error
		staged, stageErr = stageHeadshotCopy(speakerID, completion.StoredPath, completion.FileName)
		if stageErr != nil {
			return action.Error(500, "Could not prepare the public headshot. The task remains awaiting approval.")
		}
	}
	committedPublicCopy := false
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      "organizer",
		Action:     "portal.task_approved",
		EntityType: "task_completion",
		EntityID:   taskID + ":" + speakerID,
		Summary:    "Speaker task approved by program operations.",
		Origin:     "organizer-portal",
	}, func(state *domain.State) error {
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
		if present.IsHeadshotTask(*task) {
			if staged == nil {
				return action.Validation("A headshot file must be uploaded before approval.", nil, ctx.FormData)
			}
			if err := staged.Commit(); err != nil {
				return fmt.Errorf("publish public headshot: %w", err)
			}
			committedPublicCopy = true
			speaker.HeadshotURL = "/portal-file/" + completion.ID
		}
		completion.Status = domain.TaskApproved
		completion.UpdatedAt = time.Now().UTC()
		delivery.EnqueueNotificationRules(state, delivery.Trigger{Name: "task.approved", TaskID: taskID, SpeakerID: speakerID}, completion.UpdatedAt)
		return nil
	}); err != nil {
		if staged != nil {
			staged.Discard(committedPublicCopy)
		}
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Approved “"+taskTitle+"” from "+name+".")
	live.Broadcast("task:approved", map[string]string{"task": taskID, "speaker": speakerID})
	actionflow.Redirect(ctx, "/organizer/portal")
	return nil
}

// stagedHeadshot keeps an inaccessible temporary file until the state update
// confirms the task is still eligible for approval. Commit swaps it into the
// deterministic static location that publicHeadshotURL derives.
type stagedHeadshot struct {
	temporary   string
	destination string
	speakerID   string
}

// resolvePublicHeadshotDir is a seam for the file-publication test; production
// always uses publicHeadshotDir below.
var resolvePublicHeadshotDir = publicHeadshotDir

func stageHeadshotCopy(speakerID, storedPath, fileName string) (*stagedHeadshot, error) {
	source, err := os.Open(storedPath)
	if err != nil {
		return nil, fmt.Errorf("open approved headshot upload: %w", err)
	}
	defer source.Close()

	dir := resolvePublicHeadshotDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("prepare public headshot directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".headshot-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temporary public headshot: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if _, err := io.Copy(temporary, source); err != nil {
		cleanup()
		return nil, fmt.Errorf("copy public headshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("close temporary public headshot: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		cleanup()
		return nil, fmt.Errorf("set public headshot permissions: %w", err)
	}
	return &stagedHeadshot{
		temporary:   temporaryPath,
		destination: filepath.Join(dir, speakerID+headshotExtension(fileName)),
		speakerID:   speakerID,
	}, nil
}

func (stage *stagedHeadshot) Commit() error {
	if stage == nil {
		return errors.New("headshot stage is missing")
	}
	// Remove every supported older extension before the rename: otherwise a
	// replaced .jpg can remain directly reachable after a new .webp wins.
	removePublishedHeadshots(filepath.Dir(stage.destination), stage.speakerID)
	if err := os.Rename(stage.temporary, stage.destination); err != nil {
		return err
	}
	stage.temporary = ""
	return nil
}

// Discard removes a staged file, and optionally a file that was committed
// immediately before the durable state write failed. It makes the failure
// path fail closed: no public byte survives a rejected approval.
func (stage *stagedHeadshot) Discard(removePublished bool) {
	if stage == nil {
		return
	}
	if stage.temporary != "" {
		_ = os.Remove(stage.temporary)
	}
	if removePublished {
		removePublishedHeadshots(filepath.Dir(stage.destination), stage.speakerID)
	}
}

func removePublishedHeadshots(dir, speakerID string) {
	for _, extension := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		if err := os.Remove(filepath.Join(dir, speakerID+extension)); err != nil && !os.IsNotExist(err) {
			log.Printf("headshot publication: remove stale image for %s: %v", speakerID, err)
		}
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
