package present

import (
	"github.com/m31-labs/rostrum/internal/domain"
	decisionrules "github.com/m31-labs/rostrum/rules"
)

func Settings(state domain.State) map[string]any {
	fallback := map[string]string{
		"queue": "program-triage",
		"track": "the fallback track",
		"rule":  "RouteFallback",
	}
	if engine, err := decisionrules.Shared(); err == nil {
		if decision, err := engine.Route("__new_category__", "Talk", "Intermediate"); err == nil {
			fallback = map[string]string{
				"queue": decision.Queue,
				"track": TrackName(state, decision.Track),
				"rule":  decision.Rule,
			}
		}
	}
	return map[string]any{
		"section":   "settings",
		"demoMode":  DemoMode(),
		"workspace": WorkspaceIdentity(state),
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
		"tracks":           state.Event.Tracks,
		"rooms":            state.Event.Rooms,
		"categories":       state.Event.Categories,
		"trackCount":       len(state.Event.Tracks),
		"roomCount":        len(state.Event.Rooms),
		"categoryCount":    len(state.Event.Categories),
		"categoryFallback": fallback,
	}
}
