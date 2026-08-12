package publicapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestPublicEndpointsDoNotExposePrivateWorkspaceFields(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	state.TaskCompletions[0].StoredPath = "/private/uploads/agreement.pdf"

	for name, payload := range map[string]any{
		"index": Index(state), "speakers": Speakers(state), "schedule": Schedule(state),
	} {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, secret := range []string{"maya@example.com", "agreement.pdf", "ownerEmail", "reviewPlans", "evaluations"} {
			if strings.Contains(text, secret) {
				t.Errorf("%s API exposed %q", name, secret)
			}
		}
	}
}

func TestScheduleIncludesOnlyPublishedSessions(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	payload := Schedule(state)
	sessions := payload["sessions"].([]map[string]any)
	if len(sessions) != 6 {
		t.Fatalf("got %d published sessions, want 6", len(sessions))
	}
	for _, session := range sessions {
		if session["status"] != "published" {
			t.Fatalf("unpublished session in API: %#v", session)
		}
	}
}

func TestSpeakersExposeOnlyApprovedPublicHeadshotProjection(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	for index := range state.Speakers {
		state.Speakers[index].HeadshotURL = "/portal-file/private-completion"
	}
	payload := Speakers(state)
	rows := payload["speakers"].([]map[string]any)
	byID := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		byID[row["id"].(string)] = row
	}
	if got := byID["spk_maya"]["headshotUrl"]; got != "/demo-headshots/spk_maya.webp" {
		t.Fatalf("approved public headshot URL = %q", got)
	}
	if got := byID["spk_lina"]["headshotUrl"]; got != "" {
		t.Fatalf("declined headshot leaked as %q", got)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "/portal-file/") {
		t.Fatalf("public speaker API exposed authenticated portal URL: %s", data)
	}
}

func TestSpeakersApprovedReplacementOverridesSeedPortrait(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	for index := range state.TaskCompletions {
		completion := &state.TaskCompletions[index]
		if completion.TaskID == "task_headshot" && completion.SpeakerID == "spk_maya" {
			completion.FileName = "maya-replacement.PNG"
			completion.StoredPath = "/private/uploads/maya-replacement"
		}
	}
	rows := Speakers(state)["speakers"].([]map[string]any)
	for _, row := range rows {
		if row["id"] == "spk_maya" && row["headshotUrl"] != "/headshots/spk_maya.png" {
			t.Fatalf("replacement headshotUrl = %q, want approved upload", row["headshotUrl"])
		}
	}
}
