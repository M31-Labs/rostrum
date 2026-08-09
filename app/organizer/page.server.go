package organizer

import (
	"log"

	"github.com/odvcencio/programma/internal/appstate"
	"github.com/odvcencio/programma/internal/present"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Overview(appstate.MustGet().Snapshot()), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{
				Title:       server.Title{Default: "Overview — Programma"},
				Description: "A real-time command surface for program operations.",
			}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}
}
