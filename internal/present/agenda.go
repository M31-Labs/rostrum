package present

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
	decisionrules "github.com/m31-labs/rostrum/rules"
)

// Agenda builds the organizer agenda board. dayParam is the raw "day" query
// value (format 2006-01-02); an empty or unrecognized value falls back to the
// first event day that has a scheduled session, else the event's first day.
func Agenda(state domain.State, view string, dayParam string) (map[string]any, error) {
	switch view {
	case "list", "day", "week", "track", "room":
	default:
		view = "day"
	}
	engine, err := decisionrules.Shared()
	if err != nil {
		return nil, err
	}

	conflicts := domain.DetectConflicts(state.Sessions)
	conflictRows := make([]map[string]any, 0, len(conflicts))
	hard := 0
	for _, conflict := range conflicts {
		decision, err := engine.EvaluateConflict(conflict)
		if err != nil {
			return nil, err
		}
		if !decision.Allowed {
			hard++
		}
		titles := make([]string, 0, len(conflict.SessionIDs))
		for _, id := range conflict.SessionIDs {
			if session, found := state.Session(id); found {
				titles = append(titles, session.Title)
			}
		}
		conflictRows = append(conflictRows, map[string]any{
			"id":       conflict.ID,
			"kind":     StatusLabel(conflict.Kind),
			"severity": decision.Severity,
			"tone":     conflictTone(decision.Severity),
			"sessions": strings.Join(titles, " ↔ "),
			"message":  conflict.Message,
			"reason":   decision.Reason,
			"rule":     decision.Rule,
			"action":   decision.Action,
			"trace":    strings.Join(decision.Trace, " → "),
		})
	}

	location, err := time.LoadLocation(state.Event.TimeZone)
	if err != nil {
		location = time.UTC
	}

	eventDays := agendaEventDays(state, location)
	day := agendaSelectedDay(state, eventDays, location, dayParam)
	dayKey := day.Format("2006-01-02")

	clocks := agendaSlotClocks(state, day, location)
	slots := make([]map[string]any, 0, len(clocks))
	for _, clock := range clocks {
		start := time.Date(day.Year(), day.Month(), day.Day(), clock[0], clock[1], 0, 0, location)
		cells := make([]map[string]any, 0, len(state.Event.Rooms))
		for _, room := range state.Event.Rooms {
			cards := make([]map[string]any, 0)
			for _, session := range state.Sessions {
				if session.RoomID == room.ID && session.StartsAt.Equal(start) {
					cards = append(cards, sessionCard(state, session))
				}
			}
			cells = append(cells, map[string]any{
				"roomID":   room.ID,
				"roomName": room.Name,
				"start":    start.Format("2006-01-02T15:04"),
				"sessions": cards,
			})
		}
		slots = append(slots, map[string]any{
			"label": start.Format("15:04"),
			"start": start.Format("2006-01-02T15:04"),
			"cells": cells,
		})
	}

	rooms := make([]map[string]string, 0, len(state.Event.Rooms))
	for _, room := range state.Event.Rooms {
		rooms = append(rooms, map[string]string{"id": room.ID, "name": room.Name, "capacity": fmt.Sprintf("%d seats", room.Capacity)})
	}
	tracks := make([]map[string]string, 0, len(state.Event.Tracks))
	for _, track := range state.Event.Tracks {
		tracks = append(tracks, map[string]string{"id": track.ID, "name": track.Name, "tone": track.Color})
	}
	formats := make([]map[string]string, 0, len(state.Event.Formats))
	for _, format := range state.Event.Formats {
		formats = append(formats, map[string]string{"value": format, "label": format})
	}
	speakers := make([]map[string]string, 0, len(state.Speakers))
	for _, speaker := range state.Speakers {
		speakers = append(speakers, map[string]string{"id": speaker.ID, "name": speaker.Name()})
	}
	sort.Slice(speakers, func(left, right int) bool { return speakers[left]["name"] < speakers[right]["name"] })
	times := make([]map[string]string, 0, len(slots))
	for _, slot := range slots {
		times = append(times, map[string]string{"value": slot["start"].(string), "label": slot["label"].(string)})
	}
	moveSessions := make([]map[string]any, 0, len(state.Sessions))
	for _, item := range SessionsForList(state) {
		moveSessions = append(moveSessions, sessionCard(state, item))
	}
	bank := agendaBank(state)

	return map[string]any{
		"section":   "agenda",
		"workspace": WorkspaceIdentity(state),
		"view":      view,
		"views": []map[string]string{
			agendaView("list", "List", view),
			agendaView("day", "Day", view),
			agendaView("week", "Week", view),
			agendaView("track", "Track", view),
			agendaView("room", "Room", view),
		},
		"boardView":    view == "day",
		"groups":       agendaGroups(state, view, location),
		"date":         day.Format("Monday · January 02, 2006"),
		"dayKey":       dayKey,
		"days":         agendaDayTabs(eventDays, day),
		"rooms":        rooms,
		"tracks":       tracks,
		"formats":      formats,
		"speakers":     speakers,
		"times":        times,
		"slots":        slots,
		"moveSessions": moveSessions,
		"bank":         bank,
		"bankEmpty":    len(bank) == 0,
		"conflicts":    conflictRows,
		"hardCount":    hard,
		"warnCount":    len(conflicts) - hard,
		"sessionCount": len(state.Sessions),
		"sessionLabel": Pluralize(len(state.Sessions), "session", "sessions"),
		"hardLabel":    Pluralize(hard, "hard conflict", "hard conflicts"),
		"warnLabel":    Pluralize(len(conflicts)-hard, "program warning", "program warnings"),
		"publishable":  hard == 0,
	}, nil
}

// agendaEventDays returns every calendar day from Event.StartsAt through
// Event.EndsAt, inclusive, in the given location. It always returns at least
// one day so the board never has nothing to render.
func agendaEventDays(state domain.State, location *time.Location) []time.Time {
	start := state.Event.StartsAt.In(location)
	end := state.Event.EndsAt.In(location)
	if end.Before(start) {
		end = start
	}
	days := make([]time.Time, 0, 4)
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		days = append(days, cursor)
	}
	if len(days) == 0 {
		days = append(days, start)
	}
	return days
}

// agendaSelectedDay picks the board day: dayParam if it names one of
// eventDays, else the first event day carrying a scheduled session, else the
// first event day.
func agendaSelectedDay(state domain.State, eventDays []time.Time, location *time.Location, dayParam string) time.Time {
	if dayParam != "" {
		for _, candidate := range eventDays {
			if candidate.Format("2006-01-02") == dayParam {
				return candidate
			}
		}
	}
	for _, candidate := range eventDays {
		for _, item := range state.Sessions {
			if !item.Scheduled() {
				continue
			}
			when := item.StartsAt.In(location)
			if when.Year() == candidate.Year() && when.YearDay() == candidate.YearDay() {
				return candidate
			}
		}
	}
	return eventDays[0]
}

// agendaDayTabs renders one tab per event day, linking to the day board.
func agendaDayTabs(eventDays []time.Time, selected time.Time) []map[string]string {
	selectedKey := selected.Format("2006-01-02")
	tabs := make([]map[string]string, 0, len(eventDays))
	for _, candidate := range eventDays {
		key := candidate.Format("2006-01-02")
		className := "view-tab"
		if key == selectedKey {
			className += " active"
		}
		tabs = append(tabs, map[string]string{
			"id":    key,
			"label": candidate.Format("Mon, Jan 02"),
			"href":  "/organizer/agenda?view=day&day=" + key,
			"class": className,
		})
	}
	return tabs
}

// agendaSlotClocks returns the half-hour board grid from 08:00 through 18:00,
// trimmed to the event start on its first day and to clocks strictly before
// the event end on its last day. It also retains one clock for every scheduled
// session on the selected day, including historical off-grid or out-of-window
// placements, so an existing record never disappears from the board.
func agendaSlotClocks(state domain.State, day time.Time, location *time.Location) [][2]int {
	clocks := make([][2]int, 0, 24)
	seen := make(map[[2]int]bool, 24)
	addClock := func(clock [2]int) {
		if !seen[clock] {
			seen[clock] = true
			clocks = append(clocks, clock)
		}
	}
	eventStart := state.Event.StartsAt.In(location)
	eventEnd := state.Event.EndsAt.In(location)
	firstDay := eventStart.Year() == day.Year() && eventStart.YearDay() == day.YearDay()
	lastDay := eventEnd.Year() == day.Year() && eventEnd.YearDay() == day.YearDay()
	for hour := 8; hour <= 18; hour++ {
		for _, minute := range []int{0, 30} {
			if hour == 18 && minute == 30 {
				continue
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, location)
			if firstDay && candidate.Before(eventStart) {
				continue
			}
			if lastDay && !candidate.Before(eventEnd) {
				continue
			}
			addClock([2]int{hour, minute})
		}
	}
	for _, item := range state.Sessions {
		if !item.Scheduled() {
			continue
		}
		when := item.StartsAt.In(location)
		if when.Year() != day.Year() || when.YearDay() != day.YearDay() {
			continue
		}
		addClock([2]int{when.Hour(), when.Minute()})
	}
	sort.Slice(clocks, func(i, j int) bool {
		left, right := clocks[i], clocks[j]
		if left[0] != right[0] {
			return left[0] < right[0]
		}
		return left[1] < right[1]
	})
	return clocks
}

// agendaBank lists every unscheduled (accepted-but-unplaced) session, ordered
// by title, for the drag-from-bank region of the board.
func agendaBank(state domain.State) []map[string]any {
	unscheduled := make([]domain.Session, 0)
	for _, item := range state.Sessions {
		if !item.Scheduled() && item.Status != "cancelled" {
			unscheduled = append(unscheduled, item)
		}
	}
	sort.SliceStable(unscheduled, func(i, j int) bool { return unscheduled[i].Title < unscheduled[j].Title })
	cards := make([]map[string]any, 0, len(unscheduled))
	for _, item := range unscheduled {
		cards = append(cards, sessionCard(state, item))
	}
	return cards
}

func agendaView(id, label, active string) map[string]string {
	className := "view-tab"
	if id == active {
		className += " active"
	}
	return map[string]string{
		"id": id, "label": label, "href": "/organizer/agenda?view=" + id, "class": className,
	}
}

func agendaGroups(state domain.State, view string, location *time.Location) []map[string]any {
	if view == "day" {
		return nil
	}
	sessions := SessionsForList(state)
	makeCards := func(items []domain.Session) []map[string]any {
		cards := make([]map[string]any, 0, len(items))
		for _, item := range items {
			cards = append(cards, sessionCard(state, item))
		}
		return cards
	}
	group := func(id, label, detail string, items []domain.Session) map[string]any {
		return map[string]any{
			"id": id, "label": label, "detail": detail, "count": len(items),
			"empty": len(items) == 0, "sessions": makeCards(items),
		}
	}

	switch view {
	case "list":
		return []map[string]any{group("all", "All scheduled sessions", "Chronological program view", sessions)}
	case "week":
		result := make([]map[string]any, 0, 3)
		for day := state.Event.StartsAt.In(location); !day.After(state.Event.EndsAt.In(location)); day = day.AddDate(0, 0, 1) {
			items := filterSessions(sessions, func(item domain.Session) bool {
				when := item.StartsAt.In(location)
				return when.Year() == day.Year() && when.YearDay() == day.YearDay()
			})
			result = append(result, group(day.Format("2006-01-02"), day.Format("Monday, January 02"), "Event day", items))
		}
		return result
	case "track":
		result := make([]map[string]any, 0, len(state.Event.Tracks))
		for _, track := range state.Event.Tracks {
			items := filterSessions(sessions, func(item domain.Session) bool { return item.TrackID == track.ID })
			result = append(result, group(track.ID, track.Name, track.Description, items))
		}
		return result
	case "room":
		result := make([]map[string]any, 0, len(state.Event.Rooms))
		for _, room := range state.Event.Rooms {
			items := filterSessions(sessions, func(item domain.Session) bool { return item.RoomID == room.ID })
			result = append(result, group(room.ID, room.Name, fmt.Sprintf("%d seats", room.Capacity), items))
		}
		return result
	default:
		return nil
	}
}

func filterSessions(sessions []domain.Session, keep func(domain.Session) bool) []domain.Session {
	result := make([]domain.Session, 0)
	for _, item := range sessions {
		if keep(item) {
			result = append(result, item)
		}
	}
	return result
}

func sessionCard(state domain.State, session domain.Session) map[string]any {
	date := session.StartsAt.Format("Mon, Jan 02")
	if !session.Scheduled() {
		date = "Unscheduled"
	}
	return map[string]any{
		"id":          session.ID,
		"title":       session.Title,
		"time":        TimeRange(session.StartsAt, session.EndsAt),
		"start":       session.StartsAt.Format("2006-01-02T15:04"),
		"date":        date,
		"roomID":      session.RoomID,
		"room":        RoomName(state, session.RoomID),
		"trackID":     session.TrackID,
		"track":       TrackName(state, session.TrackID),
		"trackTone":   trackTone(state, session.TrackID),
		"speakers":    SpeakerNames(state, session.SpeakerIDs),
		"status":      StatusLabel(session.Status),
		"statusValue": session.Status,
		"tone":        StatusTone(session.Status),
		"duration":    int(session.EndsAt.Sub(session.StartsAt).Minutes()),
	}
}

func trackTone(state domain.State, trackID string) string {
	if track, ok := state.Track(trackID); ok {
		return track.Color
	}
	return "blue"
}

func conflictTone(severity string) string {
	if severity == domain.SeverityHard {
		return "critical"
	}
	return "accent"
}

func SessionsForList(state domain.State) []domain.Session {
	result := SortedSessions(state.Sessions)
	sort.SliceStable(result, func(i, j int) bool { return result[i].StartsAt.Before(result[j].StartsAt) })
	return result
}
