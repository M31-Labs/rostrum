package portal

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/actionflow"
	"github.com/odvcencio/programma/internal/appstate"
	"github.com/odvcencio/programma/internal/domain"
	"github.com/odvcencio/programma/internal/live"
	"github.com/odvcencio/programma/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(thisFile), "[speaker]", "page.gsx")
	if err := route.RegisterFileModule(route.FileModuleFor(source, route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.SpeakerPortal(appstate.MustGet().Snapshot(), ctx.Param("speaker"), ctx.Query("submitted") == "1")
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Speaker portal — Programma"}, Description: "Update your profile, complete event tasks, and review your schedule."}, nil
		},
		Actions: route.FileActions{
			"updateProfile": updateProfile,
			"completeTask":  completeTask,
		},
	})); err != nil {
		log.Fatal(err)
	}
}

func updateProfile(ctx *action.Context) error {
	speakerID := strings.TrimSpace(ctx.FormData["speaker_id"])
	if len(strings.TrimSpace(ctx.FormData["biography"])) < 40 {
		return action.Validation("Add a little more detail to your biography.", map[string]string{"biography": "Use at least 40 characters."}, ctx.FormData)
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		speaker, found := state.Speaker(speakerID)
		if !found {
			return fmt.Errorf("speaker %s not found", speakerID)
		}
		speaker.Pronouns = strings.TrimSpace(ctx.FormData["pronouns"])
		speaker.Role = strings.TrimSpace(ctx.FormData["role"])
		speaker.Company = strings.TrimSpace(ctx.FormData["company"])
		speaker.Biography = strings.TrimSpace(ctx.FormData["biography"])
		speaker.City = strings.TrimSpace(ctx.FormData["city"])
		speaker.LinkedInURL = strings.TrimSpace(ctx.FormData["linkedin"])
		speaker.WebsiteURL = strings.TrimSpace(ctx.FormData["website"])
		speaker.UpdatedAt = time.Now().UTC()
		upsertCompletion(state, "task_profile", speakerID, domain.TaskSubmitted, nil)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Your public profile is updated and awaiting program approval.")
	live.Broadcast("speaker:updated", map[string]string{"speaker": speakerID})
	actionflow.Redirect(ctx, "/portal/"+speakerID)
	return nil
}

func completeTask(ctx *action.Context) error {
	speakerID := strings.TrimSpace(ctx.FormData["speaker_id"])
	taskID := strings.TrimSpace(ctx.FormData["task_id"])
	if speakerID == "" || taskID == "" {
		return action.Validation("Task identity is missing.", map[string]string{"task": "Reload the portal and try again."}, ctx.FormData)
	}
	values := make(map[string]string)
	for key, value := range ctx.FormData {
		if key != "csrf_token" && key != "speaker_id" && key != "task_id" {
			values[key] = value
		}
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		if _, found := state.Speaker(speakerID); !found {
			return fmt.Errorf("speaker %s not found", speakerID)
		}
		if _, found := state.Task(taskID); !found {
			return fmt.Errorf("task %s not found", taskID)
		}
		upsertCompletion(state, taskID, speakerID, domain.TaskSubmitted, values)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Task submitted. The program team can review it now.")
	live.Broadcast("task:submitted", map[string]string{"speaker": speakerID, "task": taskID})
	actionflow.Redirect(ctx, "/portal/"+speakerID+"#tasks")
	return nil
}

func upsertCompletion(state *domain.State, taskID, speakerID, status string, values map[string]string) {
	if item, found := state.Completion(taskID, speakerID); found {
		item.Status = status
		item.Values = values
		item.UpdatedAt = time.Now().UTC()
		if status == domain.TaskSubmitted || status == domain.TaskApproved {
			item.CompletedAt = time.Now().UTC()
		}
		return
	}
	now := time.Now().UTC()
	state.TaskCompletions = append(state.TaskCompletions, domain.TaskCompletion{ID: domain.NewID("done"), TaskID: taskID, SpeakerID: speakerID, Status: status, Values: values, CompletedAt: now, UpdatedAt: now})
}
