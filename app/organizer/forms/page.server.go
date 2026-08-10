package forms

import (
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
			return present.Forms(appstate.MustGet().Snapshot())
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Forms & routing — Rostrum"}, Description: "Conditional CFP configuration with audited category routing."}, nil
		},
		Actions: route.FileActions{
			"toggleForm": toggleForm,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

func toggleForm(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	nextStatus := strings.TrimSpace(ctx.FormData["status"])
	if nextStatus != "open" && nextStatus != "closed" {
		return action.Validation("Choose a valid form state.", map[string]string{"status": "Use open or closed."}, ctx.FormData)
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		form.Status = nextStatus
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "CFP state changed to "+nextStatus+".")
	live.Broadcast("form:updated", map[string]string{"id": formID, "status": nextStatus})
	actionflow.Redirect(ctx, "/organizer/forms")
	return nil
}
