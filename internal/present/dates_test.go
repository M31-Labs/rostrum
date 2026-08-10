package present

import (
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestPresenterDatesFollowConfiguredCalendarValues(t *testing.T) {
	state := domain.Seed(time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC))
	state.Event.StartsAt = time.Date(2027, time.March, 4, 9, 0, 0, 0, time.UTC)
	state.Event.EndsAt = time.Date(2027, time.March, 6, 17, 0, 0, 0, time.UTC)
	state.Tasks[0].DueAt = time.Date(2027, time.November, 8, 17, 0, 0, 0, time.UTC)

	overview := Overview(state)
	event := overview["event"].(map[string]any)
	if got := event["dates"].(string); got != "Mar 04–06, 2027" {
		t.Fatalf("overview dates = %q", got)
	}

	portal, err := SpeakerPortal(state, "spk_maya", false)
	if err != nil {
		t.Fatal(err)
	}
	tasks := portal["tasks"].([]map[string]any)
	foundTask := false
	for _, task := range tasks {
		if task["id"] == state.Tasks[0].ID {
			foundTask = task["due"].(string) == "Nov 08"
		}
	}
	if !foundTask {
		t.Fatal("speaker portal did not render the configured November task date")
	}

	state.Sessions[0].StartsAt = time.Date(2027, time.April, 5, 10, 0, 0, 0, time.UTC)
	state.Sessions[0].EndsAt = state.Sessions[0].StartsAt.Add(45 * time.Minute)
	state.Sessions[0].Status = "published"
	publicAgenda, err := PublicAgenda(state, state.Event.Slug, false)
	if err != nil {
		t.Fatal(err)
	}
	sessions := publicAgenda["sessions"].([]map[string]any)
	found := false
	for _, session := range sessions {
		if session["id"] == state.Sessions[0].ID {
			found = strings.Contains(session["date"].(string), "April 05")
		}
	}
	if !found {
		t.Fatal("public agenda did not render the configured April session date")
	}
}
