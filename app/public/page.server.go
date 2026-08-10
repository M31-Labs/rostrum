package public

import (
	"log"
	"path/filepath"
	"runtime"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Dir(thisFile)
	register(filepath.Join(root, "[slug]", "agenda", "page.gsx"), "agenda")
	register(filepath.Join(root, "[slug]", "speakers", "page.gsx"), "speakers")
}

func register(source, kind string) {
	if err := route.RegisterFileModule(route.FileModuleFor(source, route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			state := appstate.MustGet().Snapshot()
			if kind == "agenda" {
				return present.PublicAgenda(state, ctx.Param("slug"), ctx.Query("embed") == "1")
			}
			return present.PublicSpeakers(state, ctx.Param("slug"), ctx.Query("embed") == "1")
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			title := "Event agenda — Rostrum"
			description := "Browse the event schedule and build a personal itinerary."
			if kind == "speakers" {
				title = "Speakers — Rostrum"
				description = "Meet the event speakers and explore their sessions."
			}
			return server.Metadata{Title: server.Title{Default: title}, Description: description}, nil
		},
	})); err != nil {
		log.Fatal(err)
	}
}
