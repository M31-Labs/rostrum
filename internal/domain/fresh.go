package domain

import "time"

// FreshState returns a minimal starter workspace for an organizer
// deployment: one placeholder event, one open call-for-proposals form, and
// nothing else. An organizer edits the event details, categories, and form
// fields from the workspace before opening the call. No submission, no
// speaker, no review, and no session exists yet; imported templates remain a
// separate, explicit initialization path.
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
	event := Event{
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
	}
	fields := CoreSubmissionFields(event)
	// The starter makes workshop logistics a conditional example without
	// changing the reusable locked core shared by every organizer-created CFP.
	fields = append(fields[:5], append([]FormField{{
		ID: "workshop_needs", Section: "proposal", Label: "Workshop logistics", Type: "textarea",
		Help: "Tell us about room setup, facilitation, materials, and participant limits.", MaxLength: 800,
	}}, fields[5:]...)...)
	state := State{
		SchemaVersion: CurrentSchemaVersion,
		Event:         event,
		Forms: []SubmissionForm{
			{
				ID:                    formID,
				EventID:               eventID,
				Name:                  "Call for proposals",
				ExternalTitle:         "Call for proposals",
				Slug:                  "call-for-proposals",
				Kind:                  "abstract",
				Status:                "open",
				WelcomeHeading:        "Tell us about your talk",
				WelcomeBody:           "Share a working title, an abstract, and how we can reach you. Edit this copy, the fields, and the categories from Forms once you are ready.",
				CloseAt:               now.AddDate(0, 2, 0),
				MaxDraftsPerSubmitter: 3,
				RedirectToPortal:      true,
				SendConfirmation:      true,
				ConfirmationTemplate:  "tpl_submission_confirmation",
				RuleFile:              "rules/form-visibility.arb",
				Fields:                fields,
				QuestionRules: []QuestionRule{
					{ID: "rule_workshop_needs", SourceFieldID: "format", Operator: "equals", Value: "Workshop", TargetFieldID: "workshop_needs", Effect: "show", Description: "Collect logistics only for workshop proposals."},
				},
			},
		},
		EmailTemplates: []EmailTemplate{
			{ID: "tpl_submission_confirmation", Name: "Submission confirmation", Audience: "submitter", Subject: "We received {{submission.title}}", Body: "Hi {{speaker.first_name}},\n\nYour proposal, {{submission.title}}, is safely in our review queue.\n\nOpen your secure speaker portal:\n{{speaker.portal_url}}\n\nProgram team", ReplyTo: "program@example.com", System: true},
			AcceptanceTemplate(),
			PublishedInviteTemplate(),
		},
		Tasks: []Task{
			{
				ID:          "task_profile",
				Title:       "Confirm your public profile",
				Description: "Review your role, organization, biography, pronouns, and public links.",
				Type:        "profile",
				Required:    true,
				DueAt:       now.AddDate(0, 2, 0),
			},
		},
		UpdatedAt: now,
	}
	return state
}

// CoreSubmissionFields is the locked, typed schema shared by the starter
// workspace and every CFP created through the organizer UI. Keeping one
// constructor prevents a fresh deployment from losing fields the submit,
// routing, review, and speaker-profile paths rely on.
func CoreSubmissionFields(event Event) []FormField {
	return []FormField{
		{ID: "title", Section: "proposal", Label: "Session title", Type: "text", Required: true, Locked: true, Placeholder: "A concrete title that tells us what changes", MaxLength: 120},
		{ID: "abstract", Section: "proposal", Label: "Abstract", Type: "textarea", Required: true, Locked: true, Help: "Describe the problem, the approach, and what attendees will be able to do afterward.", MaxLength: 1600},
		{ID: "format", Section: "proposal", Label: "Format", Type: "select", Required: true, Locked: true, Options: append([]string(nil), event.Formats...)},
		{ID: "category", Section: "proposal", Label: "Category", Type: "select", Required: true, Locked: true, Options: categoryNames(event.Categories)},
		{ID: "level", Section: "proposal", Label: "Audience level", Type: "select", Required: true, Locked: true, Options: append([]string(nil), event.Levels...)},
		{ID: "first_name", Section: "participant", Label: "First name", Type: "text", Required: true, Locked: true, MaxLength: 80},
		{ID: "last_name", Section: "participant", Label: "Last name", Type: "text", Required: true, Locked: true, MaxLength: 80},
		{ID: "email", Section: "participant", Label: "Email", Type: "email", Required: true, Locked: true, MaxLength: 254},
		{ID: "role", Section: "participant", Label: "Role", Type: "text", Locked: true, MaxLength: 160},
		{ID: "company", Section: "participant", Label: "Company or project", Type: "text", Locked: true, MaxLength: 160},
		{ID: "biography", Section: "participant", Label: "Short biography", Type: "textarea", Locked: true, MaxLength: 800},
	}
}

func categoryNames(categories []Category) []string {
	values := make([]string, 0, len(categories))
	for _, category := range categories {
		values = append(values, category.Name)
	}
	return values
}

// EmptyState returns a workspace with only a placeholder event skeleton: no
// forms, no speakers, no submissions, no call for proposals. Use
// INITIAL_WORKSPACE=empty
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
			Formats:  []string{"Talk", "Workshop"},
			Levels:   []string{"Introductory", "Intermediate", "Advanced"},
		},
		EmailTemplates: []EmailTemplate{AcceptanceTemplate(), PublishedInviteTemplate()},
		UpdatedAt:      now,
	}
}
