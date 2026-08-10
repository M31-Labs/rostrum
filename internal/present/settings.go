package present

import "github.com/m31-labs/rostrum/internal/domain"

func Settings(state domain.State) map[string]any {
	return map[string]any{
		"section": "settings",
		"event": map[string]any{
			"id":          state.Event.ID,
			"name":        state.Event.Name,
			"slug":        state.Event.Slug,
			"type":        state.Event.Type,
			"website":     state.Event.WebsiteURL,
			"location":    state.Event.Location,
			"timezone":    state.Event.TimeZone,
			"starts":      state.Event.StartsAt.Format("2006-01-02T15:04"),
			"ends":        state.Event.EndsAt.Format("2006-01-02T15:04"),
			"theme":       state.Event.Theme,
			"description": state.Event.Description,
		},
		"tracks":     state.Event.Tracks,
		"rooms":      state.Event.Rooms,
		"trackCount": len(state.Event.Tracks),
		"roomCount":  len(state.Event.Rooms),
	}
}
