package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/domain"
)

// TestPublicSpeakersHeadshotRequiresApproval covers PT-3's headshot
// acceptance criterion for the public gallery: only an approved headshot
// task completion with a stored file produces a /public-headshot/ URL, using
// the seed's mixed
// approved/declined/submitted task_headshot completions across speakers who
// all have a published session and so already appear in the gallery.
func TestPublicSpeakersHeadshotRequiresApproval(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	for index := range state.Tasks {
		if state.Tasks[index].ID == "task_headshot" {
			state.Tasks[index].Type = "headshot"
		}
	}
	for index := range state.TaskCompletions {
		completion := &state.TaskCompletions[index]
		if completion.TaskID == "task_headshot" && completion.Status == domain.TaskApproved {
			completion.StoredPath = "/durable/uploads/" + completion.SpeakerID + ".webp"
		}
	}
	view, err := PublicSpeakers(state, state.Event.Slug, false)
	if err != nil {
		t.Fatalf("PublicSpeakers: %v", err)
	}
	speakers := view["speakers"].([]map[string]any)
	byID := map[string]map[string]any{}
	for _, speaker := range speakers {
		byID[speaker["id"].(string)] = speaker
	}

	tests := []struct {
		id, want string
	}{
		{"spk_maya", "/public-headshot/spk_maya"},
		{"spk_theo", "/public-headshot/spk_theo"},
		{"spk_priya", "/public-headshot/spk_priya"},
		{"spk_lina", ""},   // TaskDeclined
		{"spk_elliot", ""}, // TaskSubmitted, not yet approved
	}
	for _, test := range tests {
		speaker, found := byID[test.id]
		if !found {
			t.Fatalf("expected %s in the public gallery (it has a published session)", test.id)
		}
		if got := speaker["headshotURL"]; got != test.want {
			t.Fatalf("%s headshotURL = %q, want %q", test.id, got, test.want)
		}
		if got := speaker["hasHeadshot"].(bool); got != (test.want != "") {
			t.Fatalf("%s hasHeadshot = %v, want %v", test.id, got, test.want != "")
		}
	}
}

func TestPublicSpeakersApprovedCompletionUsesStateAuthenticatedRoute(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	for index := range state.Tasks {
		if state.Tasks[index].ID == "task_headshot" {
			state.Tasks[index].Type = "headshot"
		}
	}
	for index := range state.TaskCompletions {
		completion := &state.TaskCompletions[index]
		if completion.TaskID == "task_headshot" && completion.SpeakerID == "spk_maya" {
			completion.FileName = "maya-replacement.PNG"
			completion.StoredPath = "/private/uploads/maya-replacement"
		}
	}
	view, err := PublicSpeakers(state, state.Event.Slug, false)
	if err != nil {
		t.Fatalf("PublicSpeakers: %v", err)
	}
	for _, speaker := range view["speakers"].([]map[string]any) {
		if speaker["id"] == "spk_maya" && speaker["headshotURL"] != "/public-headshot/spk_maya" {
			t.Fatalf("replacement headshotURL = %q, want approved upload", speaker["headshotURL"])
		}
	}
}

func TestIsHeadshotTask(t *testing.T) {
	tests := []struct {
		name string
		task domain.Task
		want bool
	}{
		{"canonical PT-4 type", domain.Task{ID: "task_new_headshot", Type: "headshot"}, true},
		{"name alone is not semantic", domain.Task{ID: "task_headshot", Type: "file"}, false},
		{"unrelated task", domain.Task{ID: "task_slides", Type: "file"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsHeadshotTask(test.task); got != test.want {
				t.Fatalf("IsHeadshotTask(%+v) = %v, want %v", test.task, got, test.want)
			}
		})
	}
}

func TestPublicHeadshotURLRequiresStoredFile(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	for index := range state.Tasks {
		if state.Tasks[index].ID == "task_headshot" {
			state.Tasks[index].Type = "headshot"
		}
	}
	if got := publicHeadshotURL(state, "spk_maya"); got != "" {
		t.Fatalf("approved completion without stored path produced %q", got)
	}
}
