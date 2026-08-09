package embeds

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
			return present.EmbedAdmin(appstate.MustGet().Snapshot()), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Embeds — Programma"}, Description: "Preview mobile agenda and speaker gallery embeds."}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}
}
