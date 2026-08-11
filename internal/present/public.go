package present

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/token"
	decisionrules "github.com/m31-labs/rostrum/rules"
)

func SubmissionForm(state domain.State, slug string) (map[string]any, error) {
	return submissionForm(state, slug, nil)
}

// SubmissionFormWithValues renders the same public CFP with a trusted saved
// draft prefilled. Callers must authenticate ownership before passing values;
// this package deliberately does not turn a submission ID into access.
func SubmissionFormWithValues(state domain.State, slug string, values map[string]string) (map[string]any, error) {
	return submissionForm(state, slug, values)
}

func submissionForm(state domain.State, slug string, values map[string]string) (map[string]any, error) {
	form, found := state.Form(slug)
	if !found {
		return nil, fmt.Errorf("form %s not found", slug)
	}
	engine, err := decisionrules.Shared()
	if err != nil {
		return nil, err
	}
	categories := make([]map[string]string, 0, len(state.Event.Categories))
	for _, category := range state.Event.Categories {
		categories = append(categories, map[string]string{"id": category.ID, "name": category.Name, "owner": category.OwnerName})
	}
	formats := make([]map[string]string, 0, len(state.Event.Formats))
	for _, item := range state.Event.Formats {
		formats = append(formats, map[string]string{"value": item, "label": item})
	}
	levels := make([]map[string]string, 0, len(state.Event.Levels))
	for _, item := range state.Event.Levels {
		levels = append(levels, map[string]string{"value": item, "label": item})
	}
	proposalFields, err := submissionFormFieldRows(state, *form, "proposal", values, engine)
	if err != nil {
		return nil, err
	}
	participantFields, err := submissionFormFieldRows(state, *form, "participant", values, engine)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"workspace": WorkspaceIdentity(state),
		"event": map[string]any{
			"name":     state.Event.Name,
			"theme":    state.Event.Theme,
			"dates":    state.Event.StartsAt.Format("January 02") + "–" + state.Event.EndsAt.Format("02, 2006"),
			"location": state.Event.Location,
		},
		"form": map[string]any{
			"id":           form.ID,
			"slug":         form.Slug,
			"name":         form.Name,
			"title":        form.ExternalTitle,
			"heading":      form.WelcomeHeading,
			"body":         form.WelcomeBody,
			"status":       form.Status,
			"open":         form.Status == "open" && time.Now().Before(form.CloseAt),
			"close":        form.CloseAt.Format("Monday, January 02 at 15:04 MST"),
			"confirmation": form.SendConfirmation,
			"redirect":     form.RedirectToPortal,
		},
		"categories":        categories,
		"formats":           formats,
		"levels":            levels,
		"proposalFields":    proposalFields,
		"participantFields": participantFields,
	}, nil
}

// submissionFormFieldRows returns the public, schema-driven rows for one
// section. A source field owning question rules is emitted as a single
// hydrated group with its target questions; every other field stays a small
// server component. This keeps the browser work coherent and gives builder
// rules immediate, no-refresh visibility feedback.
func submissionFormFieldRows(state domain.State, form domain.SubmissionForm, section string, values map[string]string, engine *decisionrules.Engine) ([]map[string]any, error) {
	rows := make([]map[string]any, 0, len(form.Fields))
	byID := make(map[string]map[string]any)
	for _, field := range form.Fields {
		if field.Section != section {
			continue
		}
		var maxLength any
		if field.MaxLength > 0 {
			maxLength = field.MaxLength
		}
		inputType := "text"
		if field.Type == "email" {
			inputType = "email"
		}
		row := map[string]any{
			"id":        field.ID,
			"label":     field.Label,
			"type":      field.Type,
			"inputType": inputType,
			// Concrete branch flags: a string comparison like
			// props.field.type == "select" does not evaluate inside a child
			// component whose prop is a map[string]any value, so the render
			// path is decided here in Go and passed as plain bools.
			"isTextarea":  field.Type == "textarea",
			"isSelect":    field.Type == "select",
			"isInput":     field.Type != "textarea" && field.Type != "select",
			"required":    field.Required,
			"placeholder": field.Placeholder,
			"help":        field.Help,
			"maxLength":   maxLength,
			"options":     submissionFormFieldOptions(state, field),
			"value":       values[field.ID],
			"targets":     []map[string]any{},
		}
		rows = append(rows, row)
		byID[field.ID] = row
	}
	for _, rule := range form.QuestionRules {
		source, sourceOK := byID[rule.SourceFieldID]
		target, targetOK := byID[rule.TargetFieldID]
		if !sourceOK || !targetOK {
			continue
		}
		visibility, err := engine.QuestionVisibility("", rule.Value, rule.Effect, rule.TargetFieldID)
		if err != nil {
			return nil, err
		}
		target["isConditionalTarget"] = true
		target["ruleValue"] = rule.Value
		target["ruleEffect"] = rule.Effect
		target["conditionalWhy"] = visibility.Reason
		target["policyRule"] = visibility.Rule
		source["isConditionalSource"] = true
		source["targets"] = append(source["targets"].([]map[string]any), target)
	}
	return rows, nil
}

// submissionFormFieldOptions resolves the value/label pairs a select field
// renders publicly. The category field keeps the event's category IDs as
// values, because the routing engine and state.Category both key on ID, not
// on the field's display-name options; every other select field renders its
// own declared options as both the value and the label.
func submissionFormFieldOptions(state domain.State, field domain.FormField) []map[string]string {
	if field.ID == "category" {
		options := make([]map[string]string, 0, len(state.Event.Categories))
		for _, category := range state.Event.Categories {
			options = append(options, map[string]string{"value": category.ID, "label": category.Name})
		}
		return options
	}
	options := make([]map[string]string, 0, len(field.Options))
	for _, option := range field.Options {
		options = append(options, map[string]string{"value": option, "label": option})
	}
	return options
}

// FormFieldOptionValues returns the set of values a select field accepts on
// submission. It mirrors submissionFormFieldOptions's category special case
// so app/submit/page.server.go validates against exactly what the public
// form actually offered.
func FormFieldOptionValues(state domain.State, field domain.FormField) []string {
	options := submissionFormFieldOptions(state, field)
	values := make([]string, 0, len(options))
	for _, option := range options {
		values = append(values, option["value"])
	}
	return values
}

// taskFieldRows renders a task's declared FormFields as the rows the
// portal's completeTask ActionForm draws (PT-3): select, checkbox, textarea,
// and a plain text row for any other declared type. values carries the
// speaker's most recent completion.Values for this task, so a resubmit
// pre-fills every field with what was submitted before — that pre-fill is
// how the card displays the submitted values back, matching a checked
// checkbox, a selected option, or filled text to what completeTask stored.
func taskFieldRows(fields []domain.FormField, values map[string]string) []map[string]any {
	rows := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		value := values[field.ID]
		options := make([]map[string]string, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, map[string]string{"value": option, "label": option})
		}
		rows = append(rows, map[string]any{
			"id":          field.ID,
			"label":       field.Label,
			"type":        field.Type,
			"required":    field.Required,
			"placeholder": field.Placeholder,
			"help":        field.Help,
			"options":     options,
			"value":       value,
			"checked":     value == "yes",
			// isText marks the plain-text-input fallback row: any declared
			// type other than checkbox, select, or textarea. Computed here,
			// not as a long negated condition in the template, so the
			// portal's TaskFieldRow component stays a simple type switch.
			"isText": field.Type != "checkbox" && field.Type != "select" && field.Type != "textarea",
		})
	}
	return rows
}

func SpeakerPortal(state domain.State, speakerID string, submitted bool) (map[string]any, error) {
	speaker, found := state.Speaker(speakerID)
	if !found {
		return nil, fmt.Errorf("speaker %s not found", speakerID)
	}
	tasks := state.SpeakerTasks(speaker.ID)
	taskRows := make([]map[string]any, 0, len(tasks))
	complete := 0
	for _, task := range tasks {
		completion, found := completion(state, task.ID, speaker.ID)
		status := domain.TaskOutstanding
		fileName := ""
		fileURL := ""
		if found {
			status = completion.Status
			fileName = completion.FileName
			if fileName != "" {
				fileURL = "/portal-file/" + completion.ID
			}
		}
		if status == domain.TaskApproved || status == domain.TaskSubmitted {
			complete++
		}
		taskRows = append(taskRows, map[string]any{
			"id":          task.ID,
			"title":       task.Title,
			"description": task.Description,
			"kind":        task.Type,
			"type":        StatusLabel(task.Type),
			"due":         task.DueAt.Format("Jan 02"),
			"status":      StatusLabel(status),
			"statusValue": status,
			"tone":        StatusTone(status),
			"fileName":    fileName,
			"hasFile":     fileName != "",
			"fileURL":     fileURL,
			"complete":    status == domain.TaskApproved || status == domain.TaskSubmitted,
			"fields":      taskFieldRows(task.FormFields, completion.Values),
			"hasFields":   len(task.FormFields) > 0,
		})
	}

	sessions := make([]map[string]any, 0)
	for _, item := range SortedSessions(state.Sessions) {
		if !containsID(item.SpeakerIDs, speaker.ID) {
			continue
		}
		sessions = append(sessions, map[string]any{
			"id":        item.ID,
			"title":     item.Title,
			"date":      item.StartsAt.Format("Monday, January 02"),
			"time":      TimeRange(item.StartsAt, item.EndsAt),
			"room":      RoomName(state, item.RoomID),
			"track":     TrackName(state, item.TrackID),
			"trackTone": trackTone(state, item.TrackID),
			"status":    StatusLabel(item.Status),
			"tone":      StatusTone(item.Status),
		})
	}

	proposals := make([]map[string]any, 0)
	for _, item := range state.Submissions {
		if containsID(item.SpeakerIDs, speaker.ID) {
			form, formFound := state.Form(item.FormID)
			resumeURL := ""
			if item.Status == domain.SubmissionDraft && formFound {
				resumeURL = "/submit/" + form.Slug + "?draft=" + item.ID + "&key=" + token.New().Sign(speaker.ID)
			}
			_, scheduled := state.SessionBySubmission(item.ID)
			proposals = append(proposals, map[string]any{
				"id":          item.ID,
				"title":       item.Title,
				"status":      StatusLabel(item.Status),
				"tone":        StatusTone(item.Status),
				"submitted":   submissionDateLabel(item),
				"canWithdraw": canWithdrawSubmission(item.Status),
				"scheduled":   scheduled,
				"hasResume":   resumeURL != "",
				"resumeURL":   resumeURL,
			})
		}
	}

	resources := make([]map[string]any, 0, len(state.Resources))
	for _, resource := range state.Resources {
		kind := resource.Kind
		embedURL := resource.EmbedURL
		if kind == "embed" && !allowedResourceEmbed(embedURL) {
			kind = "blocked"
			embedURL = ""
		}
		resources = append(resources, map[string]any{
			"id":        resource.ID,
			"title":     resource.Title,
			"kind":      kind,
			"kindLabel": StatusLabel(kind),
			"summary":   resource.Summary,
			"body":      resource.Body,
			"url":       resource.URL,
			"embedURL":  embedURL,
			"order":     resource.SortOrder,
		})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i]["order"].(int) < resources[j]["order"].(int) })

	return map[string]any{
		"submitted": submitted,
		"workspace": WorkspaceIdentity(state),
		"event": map[string]any{
			"name":     state.Event.Name,
			"dates":    state.Event.StartsAt.Format("January 02") + "–" + state.Event.EndsAt.Format("02, 2006"),
			"location": state.Event.Location,
		},
		"speaker": map[string]any{
			"id":          speaker.ID,
			"name":        speaker.Name(),
			"firstName":   speaker.FirstName,
			"lastName":    speaker.LastName,
			"initials":    speaker.Initials(),
			"email":       speaker.Email,
			"pronouns":    speaker.Pronouns,
			"role":        speaker.Role,
			"company":     speaker.Company,
			"biography":   speaker.Biography,
			"city":        speaker.City,
			"linkedIn":    speaker.LinkedInURL,
			"website":     speaker.WebsiteURL,
			"emailOptOut": speaker.EmailOptOut,
			// headshotURL is the authenticated /portal-file/ link a speaker's
			// own session (and an organizer's portal_admin session) may open;
			// it is empty until an upload sets Speaker.HeadshotURL (PT-3
			// upload hook — see the unit report). hasHeadshot drives the
			// template's initials fallback.
			"headshotURL": speaker.HeadshotURL,
			"hasHeadshot": speaker.HeadshotURL != "",
		},
		"readiness": map[string]any{
			"complete": complete,
			"total":    len(tasks),
			"percent":  Percent(complete, len(tasks)),
			"style":    fmt.Sprintf("%d%%", Percent(complete, len(tasks))),
		},
		"tasks":        taskRows,
		"sessions":     sessions,
		"proposals":    proposals,
		"resources":    resources,
		"calendarURL":  "/calendar/" + speaker.ID + ".ics",
		"sessionCount": len(sessions),
	}, nil
}

func submissionDateLabel(submission domain.Submission) string {
	if submission.Status == domain.SubmissionDraft {
		return "Draft saved " + submission.UpdatedAt.Format("January 02, 2006")
	}
	if submission.Status == domain.SubmissionWithdrawn {
		return "Withdrawn " + submission.WithdrawnAt.Format("January 02, 2006")
	}
	return "Submitted " + submission.SubmittedAt.Format("January 02, 2006")
}

func canWithdrawSubmission(status string) bool {
	switch status {
	case domain.SubmissionDraft, domain.SubmissionPending, domain.SubmissionAcceptedQueue, domain.SubmissionAccepted, domain.SubmissionDeclineQueue:
		return true
	default:
		return false
	}
}

func allowedResourceEmbed(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "www.youtube-nocookie.com", "player.vimeo.com":
		return true
	default:
		return false
	}
}

func FirstPortalSpeaker(state domain.State) string {
	for _, speaker := range state.Speakers {
		if len(state.SpeakerTasks(speaker.ID)) > 0 {
			return speaker.ID
		}
	}
	return ""
}
