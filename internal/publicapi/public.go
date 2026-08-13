package publicapi

import (
	"sort"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
)

// Index is deliberately a directory, not a serialization of internal state.
// It gives integrations stable discovery links without exposing proposal text,
// reviewer records, speaker email, upload paths, or integration configuration.
func Index(state domain.State) map[string]any {
	return map[string]any{
		"name":    "Rostrum public API",
		"version": "v1",
		"event": map[string]any{
			"id": state.Event.ID, "name": state.Event.Name, "slug": state.Event.Slug,
			"theme": state.Event.Theme, "description": state.Event.Description,
			"location": state.Event.Location, "timeZone": state.Event.TimeZone,
			"startsAt": state.Event.StartsAt, "endsAt": state.Event.EndsAt,
		},
		"counts": map[string]int{
			"publishedSessions": publishedSessionCount(state),
			"publishedSpeakers": len(publicSpeakerIDs(state)),
		},
		"links": map[string]string{
			"self": "/api/v1/workspace", "schedule": "/api/v1/schedule", "speakers": "/api/v1/speakers",
			"agenda": "/public/" + state.Event.Slug + "/agenda", "gallery": "/public/" + state.Event.Slug + "/speakers",
		},
	}
}

func Speakers(state domain.State) map[string]any {
	publicIDs := publicSpeakerIDs(state)
	rows := make([]map[string]any, 0, len(publicIDs))
	for _, speaker := range state.Speakers {
		if _, ok := publicIDs[speaker.ID]; !ok {
			continue
		}
		sessionIDs := make([]string, 0)
		for _, item := range state.Sessions {
			if item.Status == "published" && contains(item.SpeakerIDs, speaker.ID) {
				sessionIDs = append(sessionIDs, item.ID)
			}
		}
		rows = append(rows, map[string]any{
			"id": speaker.ID, "name": speaker.Name(), "pronouns": speaker.Pronouns,
			"role": speaker.Role, "company": speaker.Company, "biography": speaker.Biography,
			"headshotUrl": approvedHeadshotURL(state, speaker.ID), "websiteUrl": speaker.WebsiteURL,
			"linkedInUrl": speaker.LinkedInURL, "city": speaker.City, "sessionIds": sessionIDs,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) < rows[j]["name"].(string) })
	return map[string]any{"eventId": state.Event.ID, "speakers": rows}
}

// approvedHeadshotURL mirrors the public gallery's consent boundary. The
// speaker model carries an authenticated portal-file link for its owner; the
// public API instead exposes the state-authenticated public route only after
// organizer approval, or no URL at all.
func approvedHeadshotURL(state domain.State, speakerID string) string {
	if _, found := state.ApprovedHeadshot(speakerID); !found || strings.TrimSpace(speakerID) == "" || strings.ContainsAny(speakerID, "/\\?#") {
		return ""
	}
	return "/public-headshot/" + speakerID
}

func Schedule(state domain.State) map[string]any {
	sessions := make([]domain.Session, 0)
	for _, item := range state.Sessions {
		if item.Status == "published" {
			sessions = append(sessions, item)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].StartsAt.Equal(sessions[j].StartsAt) {
			return sessions[i].Title < sessions[j].Title
		}
		return sessions[i].StartsAt.Before(sessions[j].StartsAt)
	})
	rows := make([]map[string]any, 0, len(sessions))
	for _, item := range sessions {
		speakerIDs := append([]string(nil), item.SpeakerIDs...)
		rows = append(rows, map[string]any{
			"id": item.ID, "title": item.Title, "description": item.Description,
			"format": item.Format, "trackId": item.TrackID, "track": trackName(state, item.TrackID),
			"roomId": item.RoomID, "room": roomName(state, item.RoomID), "speakerIds": speakerIDs,
			"startsAt": item.StartsAt, "endsAt": item.EndsAt, "status": item.Status,
		})
	}
	return map[string]any{"eventId": state.Event.ID, "timeZone": state.Event.TimeZone, "sessions": rows}
}

func publicSpeakerIDs(state domain.State) map[string]struct{} {
	result := make(map[string]struct{})
	for _, item := range state.Sessions {
		if item.Status != "published" {
			continue
		}
		for _, speakerID := range item.SpeakerIDs {
			result[speakerID] = struct{}{}
		}
	}
	return result
}

func publishedSessionCount(state domain.State) int {
	count := 0
	for _, item := range state.Sessions {
		if item.Status == "published" {
			count++
		}
	}
	return count
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func trackName(state domain.State, id string) string {
	for _, track := range state.Event.Tracks {
		if track.ID == id {
			return track.Name
		}
	}
	return ""
}

func roomName(state domain.State, id string) string {
	for _, room := range state.Event.Rooms {
		if room.ID == id {
			return room.Name
		}
	}
	return ""
}
