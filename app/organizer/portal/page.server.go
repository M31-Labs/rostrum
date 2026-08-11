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
	"m31labs.dev/gosx/auth"
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
		Actions: route.FileActions{
			"approveTask": approveTask,
			"createTask":  createTask,
			"updateTask":  updateTask,
			"assignTask":  assignTask,
			"retireTask":  retireTask,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

var validOrganizerTaskTypes = map[string]bool{
	"profile":  true,
	"form":     true,
	"file":     true,
	"headshot": true,
}

type taskInput struct {
	Title        string
	Description  string
	Type         string
	DueAt        time.Time
	Required     bool
	AcceptedOnly bool
	AssignAll    bool
}

// createTask adds a new task to the portal without exposing it to anybody
// until it has an explicit assignment. The optional bulk assignment only
// includes speakers in an accepted program state; direct assignment remains
// available below for a deliberately selected speaker.
func createTask(ctx *action.Context) error {
	snapshot := appstate.MustGet().Snapshot()
	input, fieldErrors := parseTaskInput(ctx.FormData, snapshot.Event)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the task details.", fieldErrors, ctx.FormData)
	}
	taskID := domain.NewID("task")
	assigned := 0
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      portalActor(ctx),
		Action:     "portal.task_created",
		EntityType: "task",
		EntityID:   taskID,
		Summary:    "Created a speaker portal task.",
		Origin:     "organizer-portal",
	}, func(state *domain.State) error {
		now := time.Now().UTC()
		task := domain.Task{
			ID:           taskID,
			Title:        input.Title,
			Description:  input.Description,
			Type:         input.Type,
			Required:     input.Required,
			DueAt:        input.DueAt,
			AcceptedOnly: input.AcceptedOnly,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if input.AssignAll {
			for _, speaker := range state.Speakers {
				if state.SpeakerEligibleForAcceptedTasks(speaker.ID) {
					task.AssignedSpeakerIDs = append(task.AssignedSpeakerIDs, speaker.ID)
					assigned++
				}
			}
		}
		state.Tasks = append(state.Tasks, task)
		return nil
	}); err != nil {
		return err
	}
	message := "Created “" + input.Title + "”."
	if assigned > 0 {
		message = fmt.Sprintf("Created “%s” and assigned %d accepted speaker(s).", input.Title, assigned)
	}
	session.AddFlash(ctx.Request, "notice", message)
	live.Broadcast("task:created", map[string]string{"task": taskID})
	actionflow.Redirect(ctx, "/organizer/portal#task-manager")
	return nil
}

// updateTask changes configuration without erasing submissions. The delivery
// type is intentionally immutable once someone has completed the task: a
// file submission must never be silently reinterpreted as a freeform form,
// and an existing form response must not suddenly be treated as a file.
func updateTask(ctx *action.Context) error {
	taskID := strings.TrimSpace(ctx.FormData["task_id"])
	snapshot := appstate.MustGet().Snapshot()
	if _, found := snapshot.Task(taskID); !found {
		return action.Validation("Choose an existing task.", map[string]string{"task_id": "Task not found."}, ctx.FormData)
	}
	input, fieldErrors := parseTaskInput(ctx.FormData, snapshot.Event)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the task details.", fieldErrors, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      portalActor(ctx),
		Action:     "portal.task_updated",
		EntityType: "task",
		EntityID:   taskID,
		Summary:    "Updated speaker portal task configuration.",
		Origin:     "organizer-portal",
	}, func(state *domain.State) error {
		task, found := state.Task(taskID)
		if !found || !task.Active() {
			return action.Validation("This task is no longer active.", map[string]string{"task_id": "Retired tasks cannot be edited."}, ctx.FormData)
		}
		if task.Type != input.Type && taskHasCompletion(*state, taskID) {
			return action.Validation("A task with submitted work cannot change type.", map[string]string{"type": "Retire this task and create a replacement instead."}, ctx.FormData)
		}
		if input.AcceptedOnly {
			for _, speakerID := range task.AssignedSpeakerIDs {
				if !state.SpeakerEligibleForAcceptedTasks(speakerID) {
					return action.Validation("Accepted-only tasks can only keep accepted speakers.", map[string]string{"accepted_only": "Remove or retire ineligible assignments first."}, ctx.FormData)
				}
			}
		}
		task.Title = input.Title
		task.Description = input.Description
		task.Type = input.Type
		task.Required = input.Required
		task.DueAt = input.DueAt
		task.AcceptedOnly = input.AcceptedOnly
		task.UpdatedAt = time.Now().UTC()
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Updated “"+input.Title+"”.")
	live.Broadcast("task:updated", map[string]string{"task": taskID})
	actionflow.Redirect(ctx, "/organizer/portal#task-manager")
	return nil
}

// assignTask gives one selected speaker a task. It never accepts a raw
// speaker ID as authority: the workspace must know that speaker, the task
// must remain active, and an accepted-only task rechecks the speaker's
// current program status inside the audited state mutation.
func assignTask(ctx *action.Context) error {
	taskID := strings.TrimSpace(ctx.FormData["task_id"])
	speakerID := strings.TrimSpace(ctx.FormData["speaker_id"])
	if taskID == "" || speakerID == "" {
		return action.Validation("Choose a task and speaker.", map[string]string{"speaker_id": "Select a speaker."}, ctx.FormData)
	}
	taskTitle := ""
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      portalActor(ctx),
		Action:     "portal.task_assigned",
		EntityType: "task_assignment",
		EntityID:   taskID + ":" + speakerID,
		Summary:    "Assigned a speaker portal task.",
		Origin:     "organizer-portal",
	}, func(state *domain.State) error {
		task, found := state.Task(taskID)
		if !found || !task.Active() {
			return action.Validation("This task is no longer active.", map[string]string{"task_id": "Choose an active task."}, ctx.FormData)
		}
		if _, found := state.Speaker(speakerID); !found {
			return action.Validation("Choose an existing speaker.", map[string]string{"speaker_id": "Speaker not found."}, ctx.FormData)
		}
		if task.AcceptedOnly && !state.SpeakerEligibleForAcceptedTasks(speakerID) {
			return action.Validation("This task is limited to accepted speakers.", map[string]string{"speaker_id": "That speaker is not yet accepted."}, ctx.FormData)
		}
		if containsSpeaker(task.AssignedSpeakerIDs, speakerID) {
			return action.Validation("That speaker already has this task.", map[string]string{"speaker_id": "Already assigned."}, ctx.FormData)
		}
		task.AssignedSpeakerIDs = append(task.AssignedSpeakerIDs, speakerID)
		task.UpdatedAt = time.Now().UTC()
		taskTitle = task.Title
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Assigned “"+taskTitle+"”.")
	live.Broadcast("task:assigned", map[string]string{"task": taskID, "speaker": speakerID})
	actionflow.Redirect(ctx, "/organizer/portal#task-manager")
	return nil
}

// retireTask preserves the task and every completion for audit/export, while
// making the task unavailable in portals and rejecting any later submission
// or upload by its old URL.
func retireTask(ctx *action.Context) error {
	taskID := strings.TrimSpace(ctx.FormData["task_id"])
	if taskID == "" {
		return action.Validation("Choose a task to retire.", map[string]string{"task_id": "Task is required."}, ctx.FormData)
	}
	title := ""
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      portalActor(ctx),
		Action:     "portal.task_retired",
		EntityType: "task",
		EntityID:   taskID,
		Summary:    "Retired a speaker portal task without deleting its history.",
		Origin:     "organizer-portal",
	}, func(state *domain.State) error {
		task, found := state.Task(taskID)
		if !found {
			return action.Validation("Choose an existing task.", map[string]string{"task_id": "Task not found."}, ctx.FormData)
		}
		if !task.Active() {
			return action.Validation("That task is already retired.", map[string]string{"task_id": "Already retired."}, ctx.FormData)
		}
		now := time.Now().UTC()
		task.RetiredAt = now
		task.UpdatedAt = now
		title = task.Title
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Retired “"+title+"”; existing submissions remain archived.")
	live.Broadcast("task:retired", map[string]string{"task": taskID})
	actionflow.Redirect(ctx, "/organizer/portal#task-manager")
	return nil
}

func parseTaskInput(values map[string]string, event domain.Event) (taskInput, map[string]string) {
	input := taskInput{
		Title:        strings.TrimSpace(values["title"]),
		Description:  strings.TrimSpace(values["description"]),
		Type:         strings.ToLower(strings.TrimSpace(values["type"])),
		Required:     checked(values["required"]),
		AcceptedOnly: checked(values["accepted_only"]),
		AssignAll:    checked(values["assign_all_accepted"]),
	}
	fieldErrors := map[string]string{}
	if input.Title == "" || len([]rune(input.Title)) > 160 {
		fieldErrors["title"] = "Use a task title of 1–160 characters."
	}
	if len([]rune(input.Description)) > 2_000 {
		fieldErrors["description"] = "Keep the task description to 2,000 characters or fewer."
	}
	if !validOrganizerTaskTypes[input.Type] {
		fieldErrors["type"] = "Choose profile, form, file, or headshot."
	}
	location, err := time.LoadLocation(event.TimeZone)
	if err != nil {
		location = time.UTC
	}
	dueAt, err := time.ParseInLocation("2006-01-02T15:04", strings.TrimSpace(values["due_at"]), location)
	if err != nil || dueAt.IsZero() {
		fieldErrors["due_at"] = "Choose a valid due date and time."
	} else {
		input.DueAt = dueAt
	}
	return input, fieldErrors
}

func taskHasCompletion(state domain.State, taskID string) bool {
	for _, completion := range state.TaskCompletions {
		if completion.TaskID == taskID {
			return true
		}
	}
	return false
}

func containsSpeaker(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func checked(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func portalActor(ctx *action.Context) string {
	if user, ok := auth.Current(ctx.Request); ok && strings.TrimSpace(user.ID) != "" {
		return "organizer:" + strings.TrimSpace(user.ID)
	}
	return "organizer"
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
