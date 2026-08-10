package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestAllowedResourceEmbed(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "privacy enhanced youtube", value: "https://www.youtube-nocookie.com/embed/example", want: true},
		{name: "vimeo player", value: "https://player.vimeo.com/video/123", want: true},
		{name: "plain http", value: "http://www.youtube-nocookie.com/embed/example", want: false},
		{name: "lookalike host", value: "https://www.youtube-nocookie.com.example.test/embed/example", want: false},
		{name: "javascript", value: "javascript:alert(1)", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := allowedResourceEmbed(test.value); got != test.want {
				t.Fatalf("allowedResourceEmbed(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

// TestSpeakerPortalRendersTaskFormFields covers PT-3's first acceptance
// criterion: a task with declared FormFields (the seeded task_av) renders
// its real fields — select, checkbox, textarea — instead of the generic
// confirmation checkbox, and a task with zero fields still falls back to
// that checkbox.
func TestSpeakerPortalRendersTaskFormFields(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	view, err := SpeakerPortal(state, "spk_theo", false)
	if err != nil {
		t.Fatalf("SpeakerPortal: %v", err)
	}
	tasks := view["tasks"].([]map[string]any)

	var avTask map[string]any
	var headshotTask map[string]any
	for _, task := range tasks {
		switch task["id"] {
		case "task_av":
			avTask = task
		case "task_headshot":
			headshotTask = task
		}
	}
	if avTask == nil {
		t.Fatal("expected spk_theo to carry the seeded task_av")
	}
	if headshotTask == nil {
		t.Fatal("expected spk_theo to carry the seeded task_headshot")
	}

	if !avTask["hasFields"].(bool) {
		t.Fatal("task_av has three FormFields, want hasFields true")
	}
	if headshotTask["hasFields"].(bool) {
		t.Fatal("task_headshot has zero FormFields, want hasFields false so the generic checkbox still renders")
	}

	fields := avTask["fields"].([]map[string]any)
	if len(fields) != 3 {
		t.Fatalf("task_av fields = %d, want 3 (microphone, demo, notes)", len(fields))
	}
	byID := map[string]map[string]any{}
	for _, field := range fields {
		byID[field["id"].(string)] = field
	}

	microphone, ok := byID["microphone"]
	if !ok {
		t.Fatal("expected a microphone field")
	}
	if microphone["type"] != "select" || microphone["isText"].(bool) {
		t.Fatalf("microphone field = %+v, want type select and isText false", microphone)
	}
	options := microphone["options"].([]map[string]string)
	if len(options) != 3 {
		t.Fatalf("microphone options = %d, want 3", len(options))
	}

	demo, ok := byID["demo"]
	if !ok {
		t.Fatal("expected a demo field")
	}
	if demo["type"] != "checkbox" || demo["isText"].(bool) {
		t.Fatalf("demo field = %+v, want type checkbox and isText false", demo)
	}

	notes, ok := byID["notes"]
	if !ok {
		t.Fatal("expected a notes field")
	}
	if notes["type"] != "textarea" || notes["isText"].(bool) {
		t.Fatalf("notes field = %+v, want type textarea and isText false", notes)
	}
}

// TestTaskFieldRowsPreFillsFromCompletionValues covers the second half of
// PT-3's first acceptance criterion: a resubmitted task's fields carry the
// values completeTask previously stored, so the card shows the submitted
// answers back (a checked checkbox and a selected/typed value), not blank
// inputs.
func TestTaskFieldRowsPreFillsFromCompletionValues(t *testing.T) {
	fields := []domain.FormField{
		{ID: "microphone", Type: "select", Options: []string{"Lavalier", "Handheld"}},
		{ID: "demo", Type: "checkbox"},
		{ID: "notes", Type: "textarea"},
	}
	values := map[string]string{"microphone": "Handheld", "demo": "yes", "notes": "Bring a clicker."}
	rows := taskFieldRows(fields, values)
	byID := map[string]map[string]any{}
	for _, row := range rows {
		byID[row["id"].(string)] = row
	}
	if got := byID["microphone"]["value"]; got != "Handheld" {
		t.Fatalf("microphone value = %v, want Handheld", got)
	}
	if checked := byID["demo"]["checked"].(bool); !checked {
		t.Fatal("demo checked = false, want true for a stored value of \"yes\"")
	}
	if got := byID["notes"]["value"]; got != "Bring a clicker." {
		t.Fatalf("notes value = %v, want the stored note", got)
	}
}

// TestSpeakerPortalHeadshotFallback covers PT-3's headshot display: the
// portal profile header surfaces Speaker.HeadshotURL when set, and reports
// hasHeadshot false (the template's initials-fallback signal) when it is
// empty, as it is for every seeded speaker until the upload path sets it.
func TestSpeakerPortalHeadshotFallback(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	view, err := SpeakerPortal(state, "spk_maya", false)
	if err != nil {
		t.Fatalf("SpeakerPortal: %v", err)
	}
	speaker := view["speaker"].(map[string]any)
	if speaker["hasHeadshot"].(bool) {
		t.Fatal("hasHeadshot = true, want false: no seeded speaker carries Speaker.HeadshotURL")
	}
	if speaker["headshotURL"] != "" {
		t.Fatalf("headshotURL = %q, want empty", speaker["headshotURL"])
	}

	maya, found := state.Speaker("spk_maya")
	if !found {
		t.Fatal("seed is missing spk_maya")
	}
	maya.HeadshotURL = "/portal-file/done_maya_headshot"
	view, err = SpeakerPortal(state, "spk_maya", false)
	if err != nil {
		t.Fatalf("SpeakerPortal: %v", err)
	}
	speaker = view["speaker"].(map[string]any)
	if !speaker["hasHeadshot"].(bool) {
		t.Fatal("hasHeadshot = false, want true once Speaker.HeadshotURL is set")
	}
	if speaker["headshotURL"] != "/portal-file/done_maya_headshot" {
		t.Fatalf("headshotURL = %q, want the speaker's HeadshotURL passed through unchanged", speaker["headshotURL"])
	}
}
