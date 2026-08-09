package agenda

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/actionflow"
	"github.com/odvcencio/programma/internal/appstate"
	"github.com/odvcencio/programma/internal/domain"
	"github.com/odvcencio/programma/internal/live"
	"github.com/odvcencio/programma/internal/present"
	decisionrules "github.com/odvcencio/programma/rules"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Agenda(appstate.MustGet().Snapshot(), ctx.Query("view"))
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Agenda — Programma"}, Description: "A conflict-aware, drag-and-drop program scheduler."}, nil
		},
		Actions: route.FileActions{
			"moveSession":   moveSession,
			"publishAgenda": publishAgenda,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

func moveSession(ctx *action.Context) error {
	sessionID := strings.TrimSpace(ctx.FormData["session_id"])
	roomID := strings.TrimSpace(ctx.FormData["room_id"])
	trackID := strings.TrimSpace(ctx.FormData["track_id"])
	startsAtValue := strings.TrimSpace(ctx.FormData["starts_at"])
	if sessionID == "" || roomID == "" || trackID == "" || startsAtValue == "" {
		return action.Validation("Choose a session, time, room, and track.", map[string]string{"starts_at": "All placement fields are required."}, ctx.FormData)
	}

	engine, err := decisionrules.New()
	if err != nil {
		return err
	}
	warnings := 0
	title := ""
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		item, found := state.Session(sessionID)
		if !found {
			return fmt.Errorf("session %s not found", sessionID)
		}
		if _, found := state.Room(roomID); !found {
			return fmt.Errorf("room %s not found", roomID)
		}
		if _, found := state.Track(trackID); !found {
			return fmt.Errorf("track %s not found", trackID)
		}
		location, loadErr := time.LoadLocation(state.Event.TimeZone)
		if loadErr != nil {
			location = time.UTC
		}
		startsAt, parseErr := time.ParseInLocation("2006-01-02T15:04", startsAtValue, location)
		if parseErr != nil {
			return action.Validation("Choose a valid agenda time.", map[string]string{"starts_at": "Invalid date or time."}, ctx.FormData)
		}
		duration := item.EndsAt.Sub(item.StartsAt)
		if duration <= 0 {
			duration = 45 * time.Minute
		}
		item.RoomID = roomID
		item.TrackID = trackID
		item.StartsAt = startsAt
		item.EndsAt = startsAt.Add(duration)
		item.Status = "draft"
		title = item.Title

		for _, conflict := range domain.ConflictsForSession(state.Sessions, sessionID) {
			decision, decisionErr := engine.EvaluateConflict(conflict)
			if decisionErr != nil {
				return decisionErr
			}
			if !decision.Allowed {
				return action.Validation("That move is blocked by "+decision.Rule+".", map[string]string{"starts_at": decision.Reason}, ctx.FormData)
			}
			warnings++
		}
		return nil
	}); err != nil {
		return err
	}
	message := "Moved “" + title + "” with no conflicts."
	if warnings > 0 {
		message = fmt.Sprintf("Moved “%s” with %d program-shape warning(s).", title, warnings)
	}
	session.AddFlash(ctx.Request, "notice", message)
	live.Broadcast("agenda:moved", map[string]string{"session": sessionID, "room": roomID, "startsAt": startsAtValue})
	// Give managed navigation a distinct URL so the freshly rendered board is
	// installed even when the user drops onto the currently visible day view.
	actionflow.Redirect(ctx, "/organizer/agenda?view=day&moved="+url.QueryEscape(sessionID))
	return nil
}

func publishAgenda(ctx *action.Context) error {
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		for _, conflict := range domain.DetectConflicts(state.Sessions) {
			if conflict.Severity == domain.SeverityHard {
				return action.Validation("Resolve hard conflicts before publishing.", map[string]string{"agenda": "Speaker and room collisions remain."}, ctx.FormData)
			}
		}
		now := time.Now().UTC()
		for index := range state.Sessions {
			state.Sessions[index].Status = "published"
			state.Sessions[index].LastPublishedAt = now
		}
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Published the conflict-free agenda to portals, embeds, and API consumers.")
	live.Broadcast("agenda:published", map[string]any{"at": time.Now().UTC()})
	actionflow.Redirect(ctx, "/organizer/agenda")
	return nil
}
