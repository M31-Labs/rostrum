package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

// TestSpeakersHeadshotFallback covers PT-3's organizer readiness view: the
// speaker card row surfaces Speaker.HeadshotURL when set (an organizer's
// session already carries the portal_admin flag, so the authenticated
// /portal-file/ link resolves for them too), and reports hasHeadshot false
// otherwise, so the template can fall back to initials.
func TestSpeakersHeadshotFallback(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	rows := Speakers(state, "")["rows"].([]map[string]any)

	byID := map[string]map[string]any{}
	for _, row := range rows {
		byID[row["id"].(string)] = row
	}
	maya, found := byID["spk_maya"]
	if !found {
		t.Fatal("expected spk_maya in the speaker rows")
	}
	if maya["hasHeadshot"].(bool) {
		t.Fatal("hasHeadshot = true, want false: the seed never sets Speaker.HeadshotURL")
	}
	if maya["headshotURL"] != "" {
		t.Fatalf("headshotURL = %q, want empty", maya["headshotURL"])
	}

	speaker, found := state.Speaker("spk_maya")
	if !found {
		t.Fatal("seed is missing spk_maya")
	}
	speaker.HeadshotURL = "/portal-file/done_maya_headshot"
	rows = Speakers(state, "")["rows"].([]map[string]any)
	for _, row := range rows {
		byID[row["id"].(string)] = row
	}
	maya = byID["spk_maya"]
	if !maya["hasHeadshot"].(bool) {
		t.Fatal("hasHeadshot = false, want true once Speaker.HeadshotURL is set")
	}
	if maya["headshotURL"] != "/portal-file/done_maya_headshot" {
		t.Fatalf("headshotURL = %q, want the speaker's HeadshotURL passed through unchanged", maya["headshotURL"])
	}
}
