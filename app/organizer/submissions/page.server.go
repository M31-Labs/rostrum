package submissions

import (
	"fmt"
	"log"
	"strings"

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
			return present.Submissions(appstate.MustGet().Snapshot(), ctx.Query("q"), ctx.Query("status"), ctx.Query("category")), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Submissions — Rostrum"}, Description: "Filter, route, and update proposal decisions."}, nil
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
		if status == domain.SubmissionAccepted {
			state.AddSessionForSubmission(id)
			state.AssignAcceptedOnlyTasks(submission.SpeakerIDs)
			if accepted, found := state.SessionBySubmission(submission.ID); found {
				state.QueueAcceptanceCommunication(accepted.ID, submission.SpeakerIDs)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "“"+title+"” moved to "+present.StatusLabel(status)+".")
	live.Broadcast("submission:updated", map[string]string{"id": id, "status": status})
	// The list page and the detail page share this action. Send the
	// user back to the page that posted the form: the action URL is
	// "<page>/__actions/updateStatus", so the page path is its prefix.
	target := "/organizer/submissions"
	if origin := strings.TrimSuffix(ctx.Request.URL.Path, "/__actions/updateStatus"); strings.HasPrefix(origin, "/organizer/submissions/") {
		target = origin
	}
	actionflow.Redirect(ctx, target)
	return nil
}
