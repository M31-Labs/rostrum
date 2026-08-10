package present

import (
	"fmt"
	"sort"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
)

func PublicAgenda(state domain.State, slug string, embedded bool) (map[string]any, error) {
	if state.Event.Slug != slug {
		return nil, fmt.Errorf("event %s not found", slug)
	}
	sessions := make([]map[string]any, 0, len(state.Sessions))
	for _, item := range SortedSessions(state.Sessions) {
		if item.Status != "published" {
			continue
		}
		sessions = append(sessions, map[string]any{
			"id":              item.ID,
			"title":           item.Title,
			"description":     item.Description,
			"date":            item.StartsAt.Format("Monday, January 02"),
			"time":            TimeRange(item.StartsAt, item.EndsAt),
			"timeShort":       item.StartsAt.Format("15:04"),
			"room":            RoomName(state, item.RoomID),
			"track":           TrackName(state, item.TrackID),
			"trackTone":       trackTone(state, item.TrackID),
			"format":          item.Format,
			"speakers":        SpeakerNames(state, item.SpeakerIDs),
			"speakerInitials": initialSet(state, item.SpeakerIDs),
		})
	}
	tracks := make([]map[string]string, 0, len(state.Event.Tracks))
	for _, track := range state.Event.Tracks {
		tracks = append(tracks, map[string]string{"id": track.ID, "name": track.Name, "tone": track.Color})
	}
	pageClass := "public-event-main"
	if embedded {
		pageClass += " embed-mode"
	}
	return map[string]any{
		"embed":        embedded,
		"pageClass":    pageClass,
		"section":      "agenda",
		"event":        publicEvent(state),
		"tracks":       tracks,
		"sessions":     sessions,
		"sessionCount": len(sessions),
	}, nil
}

func PublicSpeakers(state domain.State, slug string, embedded bool) (map[string]any, error) {
	if state.Event.Slug != slug {
		return nil, fmt.Errorf("event %s not found", slug)
	}
	active := make(map[string]struct{})
	for _, item := range state.Sessions {
		if item.Status != "published" {
			continue
		}
		for _, id := range item.SpeakerIDs {
			active[id] = struct{}{}
		}
	}
	headshot := findHeadshotTask(state)
	speakers := make([]map[string]any, 0, len(active))
	for _, speaker := range state.Speakers {
		if _, found := active[speaker.ID]; !found {
			continue
		}
		talks := make([]map[string]string, 0)
		for _, item := range state.Sessions {
			if item.Status == "published" && containsID(item.SpeakerIDs, speaker.ID) {
				talks = append(talks, map[string]string{"title": item.Title, "time": TimeRange(item.StartsAt, item.EndsAt), "room": RoomName(state, item.RoomID)})
			}
		}
		headshotURL := publicHeadshotURL(state, headshot, speaker.ID)
		speakers = append(speakers, map[string]any{
			"id":          speaker.ID,
			"name":        speaker.Name(),
			"initials":    speaker.Initials(),
			"role":        speaker.Role,
			"company":     speaker.Company,
			"biography":   speaker.Biography,
			"city":        speaker.City,
			"website":     speaker.WebsiteURL,
			"linkedin":    speaker.LinkedInURL,
			"talks":       talks,
			"headshotURL": headshotURL,
			"hasHeadshot": headshotURL != "",
		})
	}
	sort.Slice(speakers, func(i, j int) bool { return speakers[i]["name"].(string) < speakers[j]["name"].(string) })
	pageClass := "public-event-main"
	if embedded {
		pageClass += " embed-mode"
	}
	return map[string]any{
		"embed":        embedded,
		"pageClass":    pageClass,
		"section":      "speakers",
		"event":        publicEvent(state),
		"speakers":     speakers,
		"speakerCount": len(speakers),
	}, nil
}

func EmbedAdmin(state domain.State) map[string]any {
	base := "/public/" + state.Event.Slug
	agendaSource := base + "/agenda?embed=1"
	speakerSource := base + "/speakers?embed=1"
	return map[string]any{
		"section":  "embeds",
		"demoMode": DemoMode(),
		"event":    publicEvent(state),
		"agenda": map[string]any{
			"src":  agendaSource,
			"code": `<iframe src="` + agendaSource + `" title="` + state.Event.Name + ` agenda" loading="lazy" style="width:100%;min-height:720px;border:0"></iframe>`,
		},
		"speakers": map[string]any{
			"src":  speakerSource,
			"code": `<iframe src="` + speakerSource + `" title="` + state.Event.Name + ` speakers" loading="lazy" style="width:100%;min-height:720px;border:0"></iframe>`,
		},
		"api": "/api/v1/workspace",
	}
}

func publicEvent(state domain.State) map[string]any {
	return map[string]any{
		"name":        state.Event.Name,
		"slug":        state.Event.Slug,
		"theme":       state.Event.Theme,
		"description": state.Event.Description,
		"dates":       state.Event.StartsAt.Format("January 02") + "–" + state.Event.EndsAt.Format("02, 2006"),
		"location":    state.Event.Location,
		"agendaURL":   "/public/" + state.Event.Slug + "/agenda",
		"speakersURL": "/public/" + state.Event.Slug + "/speakers",
	}
}

func initialSet(state domain.State, ids []string) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, Initials(state, id))
	}
	return strings.Join(values, " · ")
}

// findHeadshotTask returns the event's headshot-upload task, or nil when no
// task is assigned that role.
func findHeadshotTask(state domain.State) *domain.Task {
	for index := range state.Tasks {
		if IsHeadshotTask(state.Tasks[index]) {
			return &state.Tasks[index]
		}
	}
	return nil
}

// IsHeadshotTask reports whether task is the speaker headshot-upload task.
// "headshot" is the PT-4 task-create type; the seeded task_headshot
// predates that vocabulary and still carries Type "file", so this also
// matches its well-known ID until the seed is updated to Type "headshot"
// (see the PT-3 unit report). Exported so app/organizer/portal's
// approveTask (the headshot publish-on-approve step) shares one
// definition with the gallery lookup here instead of drifting apart.
func IsHeadshotTask(task domain.Task) bool {
	return task.Type == "headshot" || task.ID == "task_headshot"
}

// publicHeadshotURL returns the unauthenticated gallery image path for
// speakerID, or empty when the speaker has no approved headshot.
//
// The public gallery must not link the authenticated /portal-file/ route
// (an anonymous visitor cannot open it), so this never reuses
// Speaker.HeadshotURL. Instead it derives the path organizer approveTask
// (app/organizer/portal/page.server.go) copies the approved upload bytes
// to: public/headshots/<speakerID><ext>, served statically at
// /headshots/<speakerID><ext>. Approval is the publication consent gate —
// this returns empty for any completion that is not domain.TaskApproved —
// and both sides derive the same extension from the completion's stored
// FileName, so no filesystem probe is needed here.
func publicHeadshotURL(state domain.State, headshotTask *domain.Task, speakerID string) string {
	if headshotTask == nil {
		return ""
	}
	item, found := completion(state, headshotTask.ID, speakerID)
	if !found || item.Status != domain.TaskApproved || item.FileName == "" {
		return ""
	}
	return "/headshots/" + speakerID + fileExtension(item.FileName)
}

// fileExtension returns the lowercased, dotted extension of name (for
// example ".jpg"), defaulting to ".jpg" when name has none. Shared naming
// convention with organizer approveTask's public headshot copy.
func fileExtension(name string) string {
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		return strings.ToLower(name[dot:])
	}
	return ".jpg"
}
