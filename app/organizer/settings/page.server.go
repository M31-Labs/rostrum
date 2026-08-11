package settings

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/auth"
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
		Actions: route.FileActions{
			"saveEvent":   saveEvent,
			"addRoom":     addRoom,
			"addTrack":    addTrack,
			"addCategory": addCategory,
		},
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
	// SE-4: reject any scheme other than http or https at write time, so a
	// javascript: URL saved here can never reach an href on the public event
	// page. A blank value stays allowed -- it clears the link.
	website := strings.TrimSpace(ctx.FormData["website"])
	if !present.AllowedURL(website) {
		fieldErrors["website"] = "Use a link that starts with http:// or https://."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the event settings.", fieldErrors, ctx.FormData)
	}
	eventID := appstate.MustGet().Snapshot().Event.ID
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      settingsActor(ctx),
		Action:     "event.updated",
		EntityType: "event",
		EntityID:   eventID,
		Summary:    "Updated event identity, dates, timezone, and public details.",
		Origin:     "organizer-settings",
	}, func(state *domain.State) error {
		if state.Event.ID == "" {
			return fmt.Errorf("event is missing")
		}
		rebaseScheduledSessions(state.Sessions, state.Event.StartsAt, state.Event.TimeZone, startsAt, strings.TrimSpace(ctx.FormData["timezone"]))
		state.Event.Name = strings.TrimSpace(ctx.FormData["name"])
		state.Event.Slug = strings.TrimSpace(ctx.FormData["slug"])
		state.Event.Type = strings.TrimSpace(ctx.FormData["type"])
		state.Event.WebsiteURL = website
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

func addRoom(ctx *action.Context) error {
	name := strings.TrimSpace(ctx.FormData["name"])
	capacity, capacityErr := strconv.Atoi(strings.TrimSpace(ctx.FormData["capacity"]))
	fieldErrors := map[string]string{}
	if name == "" {
		fieldErrors["name"] = "Enter a room name."
	}
	if capacityErr != nil || capacity < 1 || capacity > 100_000 {
		fieldErrors["capacity"] = "Enter a capacity between 1 and 100,000."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the room details.", fieldErrors, ctx.FormData)
	}
	snapshot := appstate.MustGet().Snapshot()
	roomID := uniqueProgramID("room-", name, roomIDs(snapshot.Event.Rooms))
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      settingsActor(ctx),
		Action:     "event.room_created",
		EntityType: "room",
		EntityID:   roomID,
		Summary:    "Added a room to the event configuration.",
		Origin:     "organizer-settings",
	}, func(state *domain.State) error {
		if roomNameTaken(state.Event.Rooms, name) {
			return action.Validation("Use a room name that is not already in this event.", map[string]string{"name": "That room already exists."}, ctx.FormData)
		}
		state.Event.Rooms = append(state.Event.Rooms, domain.Room{ID: roomID, Name: name, Capacity: capacity})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Added room “"+name+"”.")
	live.Broadcast("event:room-created", map[string]string{"room": roomID})
	actionflow.Redirect(ctx, "/organizer/settings")
	return nil
}

var validTrackColors = map[string]bool{"blue": true, "teal": true, "violet": true, "ochre": true}

func addTrack(ctx *action.Context) error {
	name := strings.TrimSpace(ctx.FormData["name"])
	color := strings.TrimSpace(ctx.FormData["color"])
	description := strings.TrimSpace(ctx.FormData["description"])
	fieldErrors := map[string]string{}
	if name == "" {
		fieldErrors["name"] = "Enter a track name."
	}
	if !validTrackColors[color] {
		fieldErrors["color"] = "Choose one of the supported track colors."
	}
	if len([]rune(description)) > 500 {
		fieldErrors["description"] = "Keep the description to 500 characters or fewer."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the track details.", fieldErrors, ctx.FormData)
	}
	snapshot := appstate.MustGet().Snapshot()
	trackID := uniqueProgramID("track-", name, trackIDs(snapshot.Event.Tracks))
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      settingsActor(ctx),
		Action:     "event.track_created",
		EntityType: "track",
		EntityID:   trackID,
		Summary:    "Added a track to the event configuration.",
		Origin:     "organizer-settings",
	}, func(state *domain.State) error {
		if trackNameTaken(state.Event.Tracks, name) {
			return action.Validation("Use a track name that is not already in this event.", map[string]string{"name": "That track already exists."}, ctx.FormData)
		}
		state.Event.Tracks = append(state.Event.Tracks, domain.Track{ID: trackID, Name: name, Color: color, Description: description})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Added track “"+name+"”.")
	live.Broadcast("event:track-created", map[string]string{"track": trackID})
	actionflow.Redirect(ctx, "/organizer/settings")
	return nil
}

func addCategory(ctx *action.Context) error {
	name := strings.TrimSpace(ctx.FormData["name"])
	ownerName := strings.TrimSpace(ctx.FormData["owner_name"])
	ownerEmail := strings.ToLower(strings.TrimSpace(ctx.FormData["owner_email"]))
	trackID := strings.TrimSpace(ctx.FormData["track_id"])
	fieldErrors := map[string]string{}
	if name == "" {
		fieldErrors["name"] = "Enter a category name."
	}
	if ownerName == "" {
		fieldErrors["owner_name"] = "Enter the category owner's name."
	}
	if !strings.Contains(ownerEmail, "@") {
		fieldErrors["owner_email"] = "Enter a valid category-owner email."
	}
	if trackID == "" {
		fieldErrors["track_id"] = "Choose the track this category belongs to."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the category details.", fieldErrors, ctx.FormData)
	}
	snapshot := appstate.MustGet().Snapshot()
	categoryID := uniqueProgramID("", name, categoryIDs(snapshot.Event.Categories))
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      settingsActor(ctx),
		Action:     "event.category_created",
		EntityType: "category",
		EntityID:   categoryID,
		Summary:    "Added a CFP category and category-owner assignment.",
		Origin:     "organizer-settings",
	}, func(state *domain.State) error {
		if categoryNameTaken(state.Event.Categories, name) {
			return action.Validation("Use a category name that is not already in this event.", map[string]string{"name": "That category already exists."}, ctx.FormData)
		}
		if _, found := state.Track(trackID); !found {
			return action.Validation("Choose a configured program track.", map[string]string{"track_id": "That track no longer exists."}, ctx.FormData)
		}
		state.Event.Categories = append(state.Event.Categories, domain.Category{
			ID: categoryID, Name: name, OwnerName: ownerName, OwnerEmail: ownerEmail, TrackID: trackID,
		})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Added category “"+name+"”. It will use the routing fallback until a policy rule names it.")
	live.Broadcast("event:category-created", map[string]string{"category": categoryID})
	actionflow.Redirect(ctx, "/organizer/settings")
	return nil
}

func rebaseScheduledSessions(sessions []domain.Session, oldEventStart time.Time, oldZone string, newEventStart time.Time, newZone string) {
	oldLocation := locationOrUTC(oldZone)
	newLocation := locationOrUTC(newZone)
	oldLocalStart := oldEventStart.In(oldLocation)
	newLocalStart := newEventStart.In(newLocation)
	dayShift := civilDayDelta(oldLocalStart, newLocalStart)
	for index := range sessions {
		if !sessions[index].Scheduled() {
			continue
		}
		sessions[index].StartsAt = rebaseAgendaTime(sessions[index].StartsAt, oldLocation, newLocation, dayShift)
		sessions[index].EndsAt = rebaseAgendaTime(sessions[index].EndsAt, oldLocation, newLocation, dayShift)
	}
}

func locationOrUTC(name string) *time.Location {
	if location, err := time.LoadLocation(strings.TrimSpace(name)); err == nil {
		return location
	}
	return time.UTC
}

func civilDayDelta(oldValue, newValue time.Time) int {
	oldDate := time.Date(oldValue.Year(), oldValue.Month(), oldValue.Day(), 0, 0, 0, 0, time.UTC)
	newDate := time.Date(newValue.Year(), newValue.Month(), newValue.Day(), 0, 0, 0, 0, time.UTC)
	return int(newDate.Sub(oldDate).Hours() / 24)
}

func rebaseAgendaTime(value time.Time, oldLocation, newLocation *time.Location, dayShift int) time.Time {
	local := value.In(oldLocation).AddDate(0, 0, dayShift)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), newLocation)
}

func uniqueProgramID(prefix, name string, existing []string) string {
	base := domain.Slugify(name)
	if base == "" {
		base = "item"
	}
	base = prefix + base
	candidate := base
	for suffix := 2; containsString(existing, candidate); suffix++ {
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
	return candidate
}

func roomIDs(rooms []domain.Room) []string {
	ids := make([]string, 0, len(rooms))
	for _, room := range rooms {
		ids = append(ids, room.ID)
	}
	return ids
}

func trackIDs(tracks []domain.Track) []string {
	ids := make([]string, 0, len(tracks))
	for _, track := range tracks {
		ids = append(ids, track.ID)
	}
	return ids
}

func categoryIDs(categories []domain.Category) []string {
	ids := make([]string, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
	}
	return ids
}

func roomNameTaken(rooms []domain.Room, name string) bool {
	for _, room := range rooms {
		if strings.EqualFold(strings.TrimSpace(room.Name), name) {
			return true
		}
	}
	return false
}

func trackNameTaken(tracks []domain.Track, name string) bool {
	for _, track := range tracks {
		if strings.EqualFold(strings.TrimSpace(track.Name), name) {
			return true
		}
	}
	return false
}

func categoryNameTaken(categories []domain.Category, name string) bool {
	for _, category := range categories {
		if strings.EqualFold(strings.TrimSpace(category.Name), name) {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func settingsActor(ctx *action.Context) string {
	if ctx != nil && ctx.Request != nil {
		if user, ok := auth.Current(ctx.Request); ok && strings.TrimSpace(user.ID) != "" {
			return "organizer:" + user.ID
		}
	}
	return "organizer"
}
