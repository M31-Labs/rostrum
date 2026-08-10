package submissions

import (
	"log"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

// This file registers the /organizer/submissions/{id} detail route
// (RV-1). Its page.gsx lives in the sibling "[id]" directory rather than
// here, following the same split app/portal/page.server.go and
// app/submit/page.server.go already use for their own dynamic-segment
// pages: gosx's module sync (cmd/gosx/modules_sync.go) skips any directory
// whose name starts with "[" when it discovers packages to blank-import
// into modules/modules.go, since a bracket is not a valid Go import path
// segment. A page.server.go placed inside "[id]" itself would therefore
// never run its init() and the route would never register. Registering
// here, in the same package and directory as the existing
// page.server.go (SP-1's, left untouched), keeps the route live without
// adding a new import to the generated modules package.
func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(thisFile), "[id]", "page.gsx")
	if err := route.RegisterFileModule(route.FileModuleFor(source, route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return loadSubmissionDetail(ctx)
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{
				Title:       server.Title{Default: "Submission detail — Rostrum"},
				Description: "Read the full abstract, routing trace, and evaluation history for one proposal.",
			}, nil
		},
		// updateStatus is defined in page.server.go (SP-1). Registering the
		// same function value here — rather than a copy — means accepting
		// from this page runs the identical bridge that creates the
		// unscheduled session, with no duplicated logic.
		Actions: route.FileActions{"updateStatus": updateStatus},
	})); err != nil {
		log.Fatal(err)
	}
}

// loadSubmissionDetail resolves the routed submission id against the
// current snapshot. An unknown id renders the identical friendly
// not-found state as a raw 500, matching the loadPortal idiom in
// app/portal/page.server.go.
func loadSubmissionDetail(ctx *route.RouteContext) (any, error) {
	id := strings.TrimSpace(ctx.Param("id"))
	data, err := present.SubmissionDetail(appstate.MustGet().Snapshot(), id)
	if err != nil {
		return map[string]any{"section": "submissions", "found": false}, nil
	}
	return data, nil
}
