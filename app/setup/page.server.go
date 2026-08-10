// Package setup renders /setup, the break-glass bootstrap for a fresh
// self-host with no ORGANIZER_EMAILS configured and no stored organizer
// Principal (internal/identity/setup.go). GET verifies the one-time setup
// token; POST consumes it, creates the first organizer Principal, and signs
// the browser in (directly, or through a mailed magic link when a real mail
// transport is configured). Neither step is reachable once the token has
// been used — the loader's route.NotFound is permanent for this process.
package setup

import (
	"log"
	"strings"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/identity"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: loadSetup,
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Finish setup — Rostrum"}}, nil
		},
		Actions: route.FileActions{"completeSetup": completeSetup},
	}); err != nil {
		log.Fatal(err)
	}
}

func loadSetup(ctx *route.RouteContext, page route.FilePage) (any, error) {
	token := strings.TrimSpace(ctx.Query("token"))
	manager := identity.CurrentSetup()
	if manager == nil || !manager.Verify(token) {
		return nil, route.NotFound("setup is not available")
	}
	return map[string]any{"token": token}, nil
}

func completeSetup(ctx *action.Context) error {
	manager := identity.CurrentSetup()
	if manager == nil {
		return action.Validation("Setup is not available.", nil, ctx.FormData)
	}
	redirect, err := manager.Complete(ctx.Request, ctx.FormData["token"], ctx.FormData["email"], ctx.FormData["name"])
	if err != nil {
		return action.Validation(err.Error(), map[string]string{"email": err.Error()}, ctx.FormData)
	}
	actionflow.Redirect(ctx, redirect)
	return nil
}
