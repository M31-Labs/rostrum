package submissions

import (
	"fmt"
	"log"
	"strings"

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
			return present.Submissions(appstate.MustGet().Snapshot(), ctx.Query("q"), ctx.Query("status"), ctx.Query("category")), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Submissions — Programma"}, Description: "Filter, route, and update proposal decisions."}, nil
		},
		Actions: route.FileActions{"updateStatus": updateStatus},
	}); err != nil {
		log.Fatal(err)
	}
}

func updateStatus(ctx *action.Context) error {
	id := strings.TrimSpace(ctx.FormData["submission_id"])
	status := strings.TrimSpace(ctx.FormData["status"])
	valid := false
	for _, candidate := range domain.SubmissionStatuses {
		if candidate == status {
			valid = true
			break
		}
	}
	if !valid {
		return action.Validation("Choose a valid status.", map[string]string{"status": "Unknown submission status."}, ctx.FormData)
	}
	title := ""
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		submission, found := state.Submission(id)
		if !found {
			return fmt.Errorf("submission %s not found", id)
		}
		submission.Status = status
		title = submission.Title
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "“"+title+"” moved to "+present.StatusLabel(status)+".")
	live.Broadcast("submission:updated", map[string]string{"id": id, "status": status})
	actionflow.Redirect(ctx, "/organizer/submissions")
	return nil
}
