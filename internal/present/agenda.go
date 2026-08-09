package present

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/domain"
	decisionrules "github.com/odvcencio/programma/rules"
)

func Agenda(state domain.State, view string) (map[string]any, error) {
	switch view {
	case "list", "day", "week", "track", "room":
	default:
		view = "day"
	}
	engine, err := decisionrules.New()
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
	day := state.Event.StartsAt.In(location).AddDate(0, 0, 1)
	slotClock := [][2]int{{9, 0}, {10, 0}, {10, 30}, {11, 30}, {12, 30}, {13, 30}, {14, 30}, {16, 0}}
	slots := make([]map[string]any, 0, len(slotClock))
	for _, clock := range slotClock {
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
	times := make([]map[string]string, 0, len(slotClock))
	for _, slot := range slots {
		times = append(times, map[string]string{"value": slot["start"].(string), "label": slot["label"].(string)})
	}
	moveSessions := make([]map[string]any, 0, len(state.Sessions))
	for _, item := range SessionsForList(state) {
		moveSessions = append(moveSessions, sessionCard(state, item))
	}

	return map[string]any{
		"section": "agenda",
		"view":    view,
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
		"rooms":        rooms,
		"tracks":       tracks,
		"times":        times,
		"slots":        slots,
		"moveSessions": moveSessions,
		"conflicts":    conflictRows,
		"hardCount":    hard,
		"warnCount":    len(conflicts) - hard,
		"sessionCount": len(state.Sessions),
		"publishable":  hard == 0,
	}, nil
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
	return map[string]any{
		"id":          session.ID,
		"title":       session.Title,
		"time":        TimeRange(session.StartsAt, session.EndsAt),
		"start":       session.StartsAt.Format("2006-01-02T15:04"),
		"date":        session.StartsAt.Format("Mon, Jan 02"),
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
