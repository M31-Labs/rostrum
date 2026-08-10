package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

// TestPublicSpeakersHeadshotRequiresApproval covers PT-3's headshot
// acceptance criterion for the public gallery: only an approved headshot
// task completion produces a /headshots/ URL, using the seed's mixed
// approved/declined/submitted task_headshot completions across speakers who
// all have a published session and so already appear in the gallery.
func TestPublicSpeakersHeadshotRequiresApproval(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
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
		{"spk_maya", "/headshots/spk_maya.jpg"},   // TaskApproved, maya-chen-headshot.jpg
		{"spk_theo", "/headshots/spk_theo.png"},   // TaskApproved, theo-okafor.png
		{"spk_priya", "/headshots/spk_priya.jpg"}, // TaskApproved, priya-nair.jpg
		{"spk_lina", ""},                          // TaskDeclined
		{"spk_elliot", ""},                        // TaskSubmitted, not yet approved
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

func TestIsHeadshotTask(t *testing.T) {
	tests := []struct {
		name string
		task domain.Task
		want bool
	}{
		{"canonical PT-4 type", domain.Task{ID: "task_new_headshot", Type: "headshot"}, true},
		{"seeded bridge id", domain.Task{ID: "task_headshot", Type: "file"}, true},
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

func TestFileExtension(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"maya-chen-headshot.jpg", ".jpg"},
		{"theo-okafor.PNG", ".png"},
		{"no-extension", ".jpg"},
		{"", ".jpg"},
	}
	for _, test := range tests {
		if got := fileExtension(test.name); got != test.want {
			t.Fatalf("fileExtension(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}
