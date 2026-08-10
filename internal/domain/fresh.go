package domain

import "time"

// FreshState returns a minimal starter workspace for a real, non-demo
// deployment: one placeholder event, one open call-for-proposals form, and
// nothing else. An organizer edits the event details, categories, and form
// fields from the workspace before opening the call. No submission, no
// speaker, no review, and no session exists yet — unlike Seed, FreshState
// never impersonates real workspace activity.
//
// The call for proposals declares the fields app/submit/page.server.go
// already reads by name (title, abstract, category, format, first_name,
// last_name, email) plus workshop_needs, because the submit handler and the
// public form's ConditionalFormatFields island always render and validate a
// workshop_needs answer whenever Workshop is an offered format. Leaving it
// out of the schema would reject every workshop submission as an unknown
// field.
func FreshState(now time.Time) State {
	eventID := "evt_your_event"
	formID := "form_cfp"
	return State{
		SchemaVersion: CurrentSchemaVersion,
		Event: Event{
			ID:          eventID,
			Name:        "Your Event",
			Slug:        "your-event",
			Type:        "Conference",
			Location:    "Set your venue and city",
			TimeZone:    "America/Los_Angeles",
			StartsAt:    now.AddDate(0, 3, 0),
			EndsAt:      now.AddDate(0, 3, 0).Add(48 * time.Hour),
			Description: "Describe your event for speakers and attendees.",
			Categories:  []Category{{ID: "general", Name: "General"}},
			Formats:     []string{"Talk", "Workshop"},
			Levels:      []string{"Introductory", "Intermediate", "Advanced"},
		},
		Forms: []SubmissionForm{
			{
				ID:               formID,
				EventID:          eventID,
				Name:             "Call for proposals",
				ExternalTitle:    "Call for proposals",
				Slug:             "call-for-proposals",
				Kind:             "abstract",
				Status:           "open",
				WelcomeHeading:   "Tell us about your talk",
				WelcomeBody:      "Share a working title, an abstract, and how we can reach you. Edit this copy, the fields, and the categories from Forms once you are ready.",
				CloseAt:          now.AddDate(0, 2, 0),
				RedirectToPortal: true,
				SendConfirmation: true,
				Fields: []FormField{
					{ID: "title", Section: "proposal", Label: "Session title", Type: "text", Required: true, Locked: true, Placeholder: "A concrete title that tells us what changes", MaxLength: 120},
					{ID: "abstract", Section: "proposal", Label: "Abstract", Type: "textarea", Required: true, Help: "Describe the problem, the approach, and what attendees will be able to do afterward.", MaxLength: 1600},
					{ID: "category", Section: "proposal", Label: "Category", Type: "select", Required: true},
					{ID: "format", Section: "proposal", Label: "Format", Type: "select", Required: true, Options: []string{"Talk", "Workshop"}},
					{ID: "workshop_needs", Section: "proposal", Label: "Workshop logistics", Type: "textarea", Required: false, Help: "Tell us about room setup, facilitation, materials, and participant limits.", MaxLength: 800},
					{ID: "first_name", Section: "participant", Label: "First name", Type: "text", Required: true, Locked: true, MaxLength: 80},
					{ID: "last_name", Section: "participant", Label: "Last name", Type: "text", Required: true, Locked: true, MaxLength: 80},
					{ID: "email", Section: "participant", Label: "Email", Type: "email", Required: true, Locked: true, MaxLength: 160},
				},
				QuestionRules: []QuestionRule{
					{ID: "rule_workshop_needs", SourceFieldID: "format", Operator: "equals", Value: "Workshop", TargetFieldID: "workshop_needs", Effect: "show", Description: "Collect logistics only for workshop proposals."},
				},
			},
		},
		UpdatedAt: now,
	}
}

// EmptyState returns a workspace with only a placeholder event skeleton: no
// forms, no speakers, no submissions, no call for proposals. Use SEED=empty
// for a deployment that provisions its own call for proposals entirely
// through the organizer UI before opening it to speakers.
func EmptyState(now time.Time) State {
	return State{
		SchemaVersion: CurrentSchemaVersion,
		Event: Event{
			ID:       "evt_your_event",
			Name:     "Your Event",
			Slug:     "your-event",
			Type:     "Conference",
			TimeZone: "America/Los_Angeles",
			StartsAt: now.AddDate(0, 3, 0),
			EndsAt:   now.AddDate(0, 3, 0).Add(48 * time.Hour),
		},
		UpdatedAt: now,
	}
}
