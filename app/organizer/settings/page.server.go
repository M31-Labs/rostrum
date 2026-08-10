package settings

import (
	"fmt"
	"log"
	"strings"
	"time"

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
			return present.Settings(appstate.MustGet().Snapshot()), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Event settings — Rostrum"}, Description: "Event identity, dates, venue, tracks, and rooms."}, nil
		},
		Actions: route.FileActions{"saveEvent": saveEvent},
	}); err != nil {
		log.Fatal(err)
	}
}

func saveEvent(ctx *action.Context) error {
	fieldErrors := map[string]string{}
	for _, field := range []string{"name", "slug", "location", "timezone", "starts_at", "ends_at"} {
		if strings.TrimSpace(ctx.FormData[field]) == "" {
			fieldErrors[field] = "Required."
		}
	}
	location, err := time.LoadLocation(ctx.FormData["timezone"])
	if err != nil {
		fieldErrors["timezone"] = "Use an IANA timezone such as America/Los_Angeles."
		location = time.UTC
	}
	startsAt, startErr := time.ParseInLocation("2006-01-02T15:04", ctx.FormData["starts_at"], location)
	endsAt, endErr := time.ParseInLocation("2006-01-02T15:04", ctx.FormData["ends_at"], location)
	if startErr != nil {
		fieldErrors["starts_at"] = "Choose a valid start date."
	}
	if endErr != nil || !endsAt.After(startsAt) {
		fieldErrors["ends_at"] = "End must be after start."
	}
	if domain.Slugify(ctx.FormData["slug"]) != ctx.FormData["slug"] {
		fieldErrors["slug"] = "Use lowercase letters, numbers, and hyphens."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the event settings.", fieldErrors, ctx.FormData)
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		if state.Event.ID == "" {
			return fmt.Errorf("event is missing")
		}
		state.Event.Name = strings.TrimSpace(ctx.FormData["name"])
		state.Event.Slug = strings.TrimSpace(ctx.FormData["slug"])
		state.Event.Type = strings.TrimSpace(ctx.FormData["type"])
		state.Event.WebsiteURL = strings.TrimSpace(ctx.FormData["website"])
		state.Event.Location = strings.TrimSpace(ctx.FormData["location"])
		state.Event.TimeZone = strings.TrimSpace(ctx.FormData["timezone"])
		state.Event.StartsAt = startsAt
		state.Event.EndsAt = endsAt
		state.Event.Theme = strings.TrimSpace(ctx.FormData["theme"])
		state.Event.Description = strings.TrimSpace(ctx.FormData["description"])
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Event settings saved.")
	live.Broadcast("event:updated", map[string]string{"name": ctx.FormData["name"]})
	actionflow.Redirect(ctx, "/organizer/settings")
	return nil
}
