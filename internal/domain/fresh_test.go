package domain

import (
	"strings"
	"testing"
	"time"
)

func TestFreshStateIncludesGenericOrganizerWorkflowDefaults(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	state := FreshState(now)
	if err := state.Validate(); err != nil {
		t.Fatalf("FreshState validation: %v", err)
	}
	if _, found := state.Task("task_profile"); !found {
		t.Fatal("FreshState is missing the generic speaker profile task")
	}
	form, found := state.Form("form_cfp")
	if !found {
		t.Fatal("FreshState is missing its starter CFP")
	}
	for _, required := range CoreSubmissionFields(state.Event) {
		actual, found := formFieldByID(form.Fields, required.ID)
		if !found || !actual.Locked || required.Required && !actual.Required {
			t.Fatalf("starter core field %s = %#v, want locked parity with %#v", required.ID, actual, required)
		}
	}
	for _, templateID := range []string{AcceptanceTemplateID, PublishedInviteTemplateID} {
		found := false
		for _, template := range state.EmailTemplates {
			if template.ID == templateID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("FreshState is missing system template %s", templateID)
		}
	}
}

func formFieldByID(fields []FormField, id string) (FormField, bool) {
	for _, field := range fields {
		if field.ID == id {
			return field, true
		}
	}
	return FormField{}, false
}

func TestEmptyStateIncludesRequiredSystemTemplates(t *testing.T) {
	state := EmptyState(time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC))
	if err := state.Validate(); err != nil {
		t.Fatalf("EmptyState validation: %v", err)
	}
	if len(state.EmailTemplates) != 2 {
		t.Fatalf("EmptyState templates = %d, want acceptance and published invite", len(state.EmailTemplates))
	}
}

func TestStateValidateRejectsBrokenWorkspaceReferences(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		mutate func(*State)
		want   string
	}{
		{name: "category track", mutate: func(state *State) { state.Event.Categories[0].TrackID = "track_missing" }, want: "category general references unknown track"},
		{name: "form event", mutate: func(state *State) { state.Forms[0].EventID = "evt_missing" }, want: "submission form form_cfp references unknown event"},
		{name: "confirmation template", mutate: func(state *State) { state.Forms[0].ConfirmationTemplate = "tpl_missing" }, want: "unknown confirmation template"},
		{name: "submission track", mutate: func(state *State) {
			state.Speakers = append(state.Speakers, Speaker{ID: "spk_test", FirstName: "Test", Email: "test@example.com"})
			state.Submissions = append(state.Submissions, Submission{ID: "sub_test", EventID: state.Event.ID, FormID: state.Forms[0].ID, CategoryID: "general", TrackID: "track_missing", SpeakerIDs: []string{"spk_test"}, Status: SubmissionPending})
		}, want: "submission sub_test references unknown track"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := FreshState(now)
			test.mutate(&state)
			if err := state.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want %q", err, test.want)
			}
		})
	}
}
