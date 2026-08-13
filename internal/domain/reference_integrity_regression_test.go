package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFreshStateCoreFieldsExactlyMatchOrganizerCreatedForms(t *testing.T) {
	state := FreshState(time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC))
	form, found := state.Form("form_cfp")
	if !found {
		t.Fatal("fresh starter form is missing")
	}
	actualByID := make(map[string]FormField, len(form.Fields))
	for _, field := range form.Fields {
		actualByID[field.ID] = field
	}
	for _, expected := range CoreSubmissionFields(state.Event) {
		actual, found := actualByID[expected.ID]
		if !found {
			t.Fatalf("fresh starter form is missing core field %s", expected.ID)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("fresh core field %s = %#v, want exact organizer-created field %#v", expected.ID, actual, expected)
		}
	}
}

func TestStateValidateRejectsBrokenOrganizerWorkspaceReferences(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*State)
		want   string
	}{
		{name: "category track", mutate: func(state *State) { state.Event.Categories[0].TrackID = "track_missing" }, want: "category general references unknown track track_missing"},
		{name: "form event", mutate: func(state *State) { state.Forms[0].EventID = "evt_missing" }, want: "submission form form_cfp references unknown event evt_missing"},
		{name: "form confirmation template", mutate: func(state *State) { state.Forms[0].ConfirmationTemplate = "tpl_missing" }, want: "submission form form_cfp references unknown confirmation template tpl_missing"},
		{name: "submission event", mutate: func(state *State) { state.Submissions[0].EventID = "evt_missing" }, want: "submission sub_reference references unknown event evt_missing"},
		{name: "submission form", mutate: func(state *State) { state.Submissions[0].FormID = "form_missing" }, want: "submission sub_reference references unknown form form_missing"},
		{name: "submission category", mutate: func(state *State) { state.Submissions[0].CategoryID = "category_missing" }, want: "submission sub_reference references unknown category category_missing"},
		{name: "submission track", mutate: func(state *State) { state.Submissions[0].TrackID = "track_missing" }, want: "submission sub_reference references unknown track track_missing"},
		{name: "submission speaker", mutate: func(state *State) { state.Submissions[0].SpeakerIDs[0] = "spk_missing" }, want: "submission sub_reference references unknown speaker spk_missing"},
		{name: "session event", mutate: func(state *State) { state.Sessions[0].EventID = "evt_missing" }, want: "session ses_reference references unknown event evt_missing"},
		{name: "session submission", mutate: func(state *State) { state.Sessions[0].SubmissionID = "sub_missing" }, want: "session ses_reference references unknown submission sub_missing"},
		{name: "session track", mutate: func(state *State) { state.Sessions[0].TrackID = "track_missing" }, want: "session ses_reference references unknown track track_missing"},
		{name: "session room", mutate: func(state *State) { state.Sessions[0].RoomID = "room_missing" }, want: "session ses_reference references unknown room room_missing"},
		{name: "session speaker", mutate: func(state *State) { state.Sessions[0].SpeakerIDs[0] = "spk_missing" }, want: "session ses_reference references unknown speaker spk_missing"},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			state := referenceRichFreshState(t)
			test.mutate(&state)
			if err := state.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want %q", err, test.want)
			}
		})
	}
}

func referenceRichFreshState(t *testing.T) State {
	t.Helper()
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	state := FreshState(now)
	state.Event.Tracks = []Track{{ID: "track-reference", Name: "Reference", Color: "blue"}}
	state.Event.Rooms = []Room{{ID: "room-reference", Name: "Reference Hall", Capacity: 120}}
	state.Event.Categories[0].TrackID = "track-reference"
	state.Speakers = []Speaker{{
		ID: "spk_reference", FirstName: "Reference", LastName: "Speaker", Email: "reference@example.com",
		CreatedAt: now, UpdatedAt: now,
	}}
	state.Submissions = []Submission{{
		ID: "sub_reference", EventID: state.Event.ID, FormID: state.Forms[0].ID,
		Title: "Reference proposal", Format: "Talk", CategoryID: "general", TrackID: "track-reference", Level: "Intermediate",
		SpeakerIDs: []string{"spk_reference"}, Status: SubmissionPending, SubmittedAt: now, UpdatedAt: now,
	}}
	state.Sessions = []Session{{
		ID: "ses_reference", EventID: state.Event.ID, SubmissionID: "sub_reference", Title: "Reference proposal",
		TrackID: "track-reference", RoomID: "room-reference", SpeakerIDs: []string{"spk_reference"},
		StartsAt: now.AddDate(0, 3, 0), EndsAt: now.AddDate(0, 3, 0).Add(time.Hour), DurationMinutes: 60,
	}}
	if err := state.Validate(); err != nil {
		t.Fatalf("reference-rich baseline is invalid: %v", err)
	}
	return state
}
