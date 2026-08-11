package agenda

import (
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	delivery "github.com/m31-labs/rostrum/internal/communications"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/present"
	decisionrules "github.com/m31-labs/rostrum/rules"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

// agendaSender is resolved after main loads .env, while still avoiding a
// transport lookup for every publish action.
var agendaSender = sync.OnceValue(mail.FromEnv)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Agenda(appstate.MustGet().Snapshot(), ctx.Query("view"), ctx.Query("day"))
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Agenda — Rostrum"}, Description: "A conflict-aware, drag-and-drop program scheduler."}, nil
		},
		Actions: route.FileActions{
			"createSession":     createSession,
			"moveSession":       moveSession,
			"unscheduleSession": unscheduleSession,
			"publishAgenda":     publishAgenda,
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

	engine, err := decisionrules.Shared()
	if err != nil {
		return err
	}
	warnings := 0
	title := ""
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      agendaActor(ctx),
		Action:     "agenda.session_moved",
		EntityType: "session",
		EntityID:   sessionID,
		Summary:    "Placed a session on the organizer agenda.",
		Origin:     "organizer-agenda",
	}, func(state *domain.State) error {
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
		duration := item.Duration()
		item.RoomID = roomID
		item.TrackID = trackID
		item.StartsAt = startsAt
		item.EndsAt = startsAt.Add(duration)
		item.DurationMinutes = int(duration / time.Minute)
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
	// installed even when the user drops onto the currently visible day view,
	// and keep the organizer on the day they just dropped onto.
	redirect := "/organizer/agenda?view=day&moved=" + url.QueryEscape(sessionID)
	if len(startsAtValue) >= len("2006-01-02") {
		redirect += "&day=" + url.QueryEscape(startsAtValue[:len("2006-01-02")])
	}
	actionflow.Redirect(ctx, redirect)
	return nil
}

// unscheduleSession returns a placed session to the unscheduled bank: it
// zeroes the times and marks the session unscheduled again, undoing a
// mis-drop without touching its room or track assignment.
func unscheduleSession(ctx *action.Context) error {
	sessionID := strings.TrimSpace(ctx.FormData["session_id"])
	if sessionID == "" {
		return action.Validation("Choose a session to return to the bank.", map[string]string{"session_id": "A session is required."}, ctx.FormData)
	}

	title := ""
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      agendaActor(ctx),
		Action:     "agenda.session_unscheduled",
		EntityType: "session",
		EntityID:   sessionID,
		Summary:    "Returned a session to the unscheduled agenda bank.",
		Origin:     "organizer-agenda",
	}, func(state *domain.State) error {
		item, found := state.Session(sessionID)
		if !found {
			return fmt.Errorf("session %s not found", sessionID)
		}
		item.DurationMinutes = int(item.Duration() / time.Minute)
		item.StartsAt = time.Time{}
		item.EndsAt = time.Time{}
		item.Status = "unscheduled"
		title = item.Title
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Returned “"+title+"” to the unscheduled bank.")
	live.Broadcast("agenda:moved", map[string]string{"session": sessionID, "room": "", "startsAt": ""})
	day := strings.TrimSpace(ctx.FormData["day"])
	redirect := "/organizer/agenda?view=day"
	if day != "" {
		redirect += "&day=" + url.QueryEscape(day)
	}
	actionflow.Redirect(ctx, redirect)
	return nil
}

func publishAgenda(ctx *action.Context) error {
	eventID := appstate.MustGet().Snapshot().Event.ID
	now := time.Now().UTC()
	var publishedSessionIDs []string
	queuedInvites := 0
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      agendaActor(ctx),
		Action:     "agenda.published",
		EntityType: "event",
		EntityID:   eventID,
		Summary:    "Published every scheduled session after conflict checks.",
		Origin:     "organizer-agenda",
	}, func(state *domain.State) error {
		for _, conflict := range domain.DetectConflicts(state.Sessions) {
			if conflict.Severity == domain.SeverityHard {
				return action.Validation("Resolve hard conflicts before publishing.", map[string]string{"agenda": "Speaker and room collisions remain."}, ctx.FormData)
			}
		}
		for index := range state.Sessions {
			// M5: only a scheduled session (non-zero start and end) goes
			// public. An unscheduled bank session keeps its status, so it
			// never leaks into the public schedule with a zero-value date.
			if !state.Sessions[index].Scheduled() {
				continue
			}
			state.Sessions[index].Status = "published"
			state.Sessions[index].LastPublishedAt = now
			publishedSessionIDs = append(publishedSessionIDs, state.Sessions[index].ID)
		}
		queuedInvites = state.QueuePublishedInviteCommunications(publishedSessionIDs, now)
		return nil
	}); err != nil {
		return err
	}

	deliveryReport := delivery.Report{}
	if len(publishedSessionIDs) > 0 {
		var err error
		deliveryReport, err = (delivery.Runner{Store: appstate.MustGet(), Sender: agendaSender()}).RunDue()
		if err != nil {
			return err
		}
		live.Broadcast("communication:outbox_run", map[string]int{
			"sent":       deliveryReport.Sent,
			"retried":    deliveryReport.Retried,
			"failed":     deliveryReport.Failed,
			"suppressed": deliveryReport.Suppressed,
		})
	}
	notice := "Published the conflict-free agenda to portals, embeds, and API consumers."
	if queuedInvites > 0 {
		notice += fmt.Sprintf(" Queued %d speaker calendar invitation(s); %d sent.", queuedInvites, deliveryReport.Sent)
	}
	session.AddFlash(ctx.Request, "notice", notice)
	live.Broadcast("agenda:published", map[string]any{"at": time.Now().UTC()})
	actionflow.Redirect(ctx, "/organizer/agenda")
	return nil
}

// createSession adds an organizer-authored session to the unscheduled bank.
// Placement remains a separate moveSession action, ensuring every placement
// still crosses the same conflict-policy boundary as accepted submissions.
func createSession(ctx *action.Context) error {
	title := strings.TrimSpace(ctx.FormData["title"])
	description := strings.TrimSpace(ctx.FormData["description"])
	format := strings.TrimSpace(ctx.FormData["format"])
	trackID := strings.TrimSpace(ctx.FormData["track_id"])
	durationMinutes, durationErr := strconv.Atoi(strings.TrimSpace(ctx.FormData["duration_minutes"]))
	fieldErrors := map[string]string{}
	if title == "" {
		fieldErrors["title"] = "Enter a session title."
	} else if len([]rune(title)) > 160 {
		fieldErrors["title"] = "Keep the title to 160 characters or fewer."
	}
	if len([]rune(description)) > 8_000 {
		fieldErrors["description"] = "Keep the description to 8,000 characters or fewer."
	}
	if durationErr != nil || durationMinutes < 5 || durationMinutes > 8*60 {
		fieldErrors["duration_minutes"] = "Choose a duration between 5 and 480 minutes."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the session details.", fieldErrors, ctx.FormData)
	}

	speakerIDs := selectedSpeakerIDs(ctx.FormData)
	sessionID := domain.NewID("ses")
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      agendaActor(ctx),
		Action:     "agenda.session_created",
		EntityType: "session",
		EntityID:   sessionID,
		Summary:    "Created an organizer-authored session in the unscheduled agenda bank.",
		Origin:     "organizer-agenda",
	}, func(state *domain.State) error {
		if !containsString(state.Event.Formats, format) {
			return action.Validation("Choose a configured session format.", map[string]string{"format": "Choose a format from this event."}, ctx.FormData)
		}
		if _, found := state.Track(trackID); !found {
			return action.Validation("Choose a configured program track.", map[string]string{"track_id": "Choose a track from this event."}, ctx.FormData)
		}
		for _, speakerID := range speakerIDs {
			if _, found := state.Speaker(speakerID); !found {
				return action.Validation("Choose only speakers in this workspace.", map[string]string{"speakers": "One selected speaker no longer exists."}, ctx.FormData)
			}
		}
		state.Sessions = append(state.Sessions, domain.Session{
			ID:              sessionID,
			EventID:         state.Event.ID,
			Title:           title,
			Description:     description,
			Format:          format,
			TrackID:         trackID,
			SpeakerIDs:      speakerIDs,
			DurationMinutes: durationMinutes,
			Status:          "unscheduled",
		})
		return nil
	}); err != nil {
		return err
	}

	session.AddFlash(ctx.Request, "notice", "Added “"+title+"” to the unscheduled bank.")
	live.Broadcast("session:created", map[string]string{"session": sessionID})
	redirect := "/organizer/agenda?view=day"
	if day := strings.TrimSpace(ctx.FormData["day"]); day != "" {
		redirect += "&day=" + url.QueryEscape(day)
	}
	actionflow.Redirect(ctx, redirect)
	return nil
}

func selectedSpeakerIDs(formData map[string]string) []string {
	const prefix = "speaker_"
	ids := make([]string, 0)
	for name, value := range formData {
		if !strings.HasPrefix(name, prefix) || strings.TrimSpace(value) == "" {
			continue
		}
		ids = append(ids, strings.TrimPrefix(name, prefix))
	}
	sort.Strings(ids)
	return ids
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func agendaActor(ctx *action.Context) string {
	if ctx != nil && ctx.Request != nil {
		if user, ok := auth.Current(ctx.Request); ok && strings.TrimSpace(user.ID) != "" {
			return "organizer:" + user.ID
		}
	}
	return "organizer"
}
