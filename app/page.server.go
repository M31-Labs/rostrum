package app

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			state := appstate.MustGet().Snapshot()
			return landingData(state), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{
				Title:       server.Title{Default: "Rostrum — governed program operations"},
				Description: "Open program operations for calls for proposals, review, speaker onboarding, scheduling, and public event experiences.",
			}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}
}

func landingData(state domain.State) map[string]any {
	conflicts := domain.DetectConflicts(state.Sessions)
	hard := 0
	for _, conflict := range conflicts {
		if conflict.Severity == domain.SeverityHard {
			hard++
		}
	}
	accepted := 0
	for _, submission := range state.Submissions {
		if submission.Status == domain.SubmissionAccepted {
			accepted++
		}
	}
	return map[string]any{
		"demoMode":  present.DemoMode(),
		"workspace": present.WorkspaceIdentity(state),
		"event": map[string]any{
			"name":     state.Event.Name,
			"location": state.Event.Location,
			"dates":    state.Event.StartsAt.Format("Jan 02") + "–" + state.Event.EndsAt.Format("02, 2006"),
		},
		"stats": []map[string]any{
			{"value": len(state.Submissions), "label": "proposals", "detail": acceptedString(accepted)},
			{"value": len(state.Speakers), "label": "participants", "detail": "self-service profiles"},
			{"value": len(state.Sessions), "label": "sessions", "detail": conflictString(hard)},
		},
		"surfaces": []map[string]string{
			{"number": "01", "title": "Collect", "body": "Conditional CFPs route every proposal into a visible, owned queue.", "href": "/organizer/forms"},
			{"number": "02", "title": "Decide", "body": "Multi-round rubrics keep human and optional AI review comparable and attributable.", "href": "/organizer/review"},
			{"number": "03", "title": "Publish", "body": "Conflict-aware scheduling feeds speaker portals, embeds, calendar files, and Accelevents.", "href": "/organizer/agenda"},
		},
		"updated": state.UpdatedAt.In(time.Local).Format("15:04 MST"),
	}
}

func acceptedString(count int) string {
	return formatCount(count) + " accepted"
}

func conflictString(count int) string {
	return present.Pluralize(count, "hard conflict flagged", "hard conflicts flagged")
}

// formatCount zero-pads count to two digits below 100 and returns the plain
// decimal at or above it, so a landing-page stat never prints a punctuation
// character in place of a digit: the old rune-arithmetic version added
// count/10 to the byte '0', which passes '9' into ':', ';', and so on for
// any count of 100 or more.
func formatCount(count int) string {
	if count < 0 {
		count = 0
	}
	if count < 100 {
		return fmt.Sprintf("%02d", count)
	}
	return strconv.Itoa(count)
}
