package portal

import (
	"fmt"
	"log"
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
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.PortalOperations(appstate.MustGet().Snapshot()), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Portal & tasks — Programma"}, Description: "Real-time speaker onboarding and resource operations."}, nil
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
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Approved “"+taskTitle+"” from "+name+".")
	live.Broadcast("task:approved", map[string]string{"task": taskID, "speaker": speakerID})
	actionflow.Redirect(ctx, "/organizer/portal")
	return nil
}
