package app

import (
	"log"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

// init registers app/not-found.gsx as the process-wide 404: gosx's
// file-based router treats a root "not-found.gsx" as the global not-found
// page (route.FileRoutes.NotFound), rendered with the same app/layout.gsx
// global header every other page uses (M1).
func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return map[string]any{
				"workspace": present.WorkspaceIdentity(appstate.MustGet().Snapshot()),
			}, nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Page not found — Rostrum"}}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}
}
