package present

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
	decisionrules "github.com/m31-labs/rostrum/rules"
)

func SubmissionForm(state domain.State, slug string) (map[string]any, error) {
	form, found := state.Form(slug)
	if !found {
		return nil, fmt.Errorf("form %s not found", slug)
	}
	engine, err := decisionrules.New()
	if err != nil {
		return nil, err
	}
	workshop, err := engine.FieldVisibility("Workshop", "")
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
	return map[string]any{
		"event": map[string]any{
			"name":     state.Event.Name,
			"theme":    state.Event.Theme,
			"dates":    state.Event.StartsAt.Format("January 02") + "–" + state.Event.EndsAt.Format("02, 2006"),
			"location": state.Event.Location,
		},
		"form": map[string]any{
			"id":              form.ID,
			"slug":            form.Slug,
			"name":            form.Name,
			"title":           form.ExternalTitle,
			"heading":         form.WelcomeHeading,
			"body":            form.WelcomeBody,
			"status":          form.Status,
			"open":            form.Status == "open" && time.Now().Before(form.CloseAt),
			"close":           form.CloseAt.Format("Monday, January 02 at 15:04 MST"),
			"confirmation":    form.SendConfirmation,
			"redirect":        form.RedirectToPortal,
			"conditionalRule": workshop.Rule,
			"conditionalWhy":  workshop.Reason,
		},
		"categories": categories,
		"formats":    formats,
		"levels":     levels,
		// proposalFields and participantFields render the public form from
		// form.Fields, grouped by the two sections the layout keeps. The
		// format field stays in the list so it renders in schema order, but
		// the ConditionalFormatFields island — not this list — draws it and
		// the workshop_needs field beside it, so workshop_needs is omitted
		// here to avoid rendering it twice.
		"proposalFields":    submissionFormFieldRows(state, form.Fields, "proposal"),
		"participantFields": submissionFormFieldRows(state, form.Fields, "participant"),
	}, nil
}

// submissionFormFieldRows returns the public, schema-driven rows for one
// section of a submission form. Removing a field from form.Fields removes it
// from this list, and therefore from the rendered page, on the next load.
func submissionFormFieldRows(state domain.State, fields []domain.FormField, section string) []map[string]any {
	rows := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		if field.Section != section || field.ID == "workshop_needs" {
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
		rows = append(rows, map[string]any{
			"id":          field.ID,
			"label":       field.Label,
			"type":        field.Type,
			"inputType":   inputType,
			"required":    field.Required,
			"placeholder": field.Placeholder,
			"help":        field.Help,
			"maxLength":   maxLength,
			"options":     submissionFormFieldOptions(state, field),
		})
	}
	return rows
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
			proposals = append(proposals, map[string]any{
				"title":     item.Title,
				"status":    StatusLabel(item.Status),
				"tone":      StatusTone(item.Status),
				"submitted": item.SubmittedAt.Format("January 02, 2006"),
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
		"event": map[string]any{
			"name":     state.Event.Name,
			"dates":    state.Event.StartsAt.Format("January 02") + "–" + state.Event.EndsAt.Format("02, 2006"),
			"location": state.Event.Location,
		},
		"speaker": map[string]any{
			"id":        speaker.ID,
			"name":      speaker.Name(),
			"firstName": speaker.FirstName,
			"lastName":  speaker.LastName,
			"initials":  speaker.Initials(),
			"email":     speaker.Email,
			"pronouns":  speaker.Pronouns,
			"role":      speaker.Role,
			"company":   speaker.Company,
			"biography": speaker.Biography,
			"city":      speaker.City,
			"linkedIn":  speaker.LinkedInURL,
			"website":   speaker.WebsiteURL,
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
