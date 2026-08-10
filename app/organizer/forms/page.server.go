package forms

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

// validFieldTypes are the field types app/submit/[slug]/page.gsx's
// FormFieldRow (FB-1) knows how to render on the public form. addField and
// updateField reject anything outside this set so the builder can never
// produce a field with nowhere to render.
var validFieldTypes = map[string]bool{"text": true, "textarea": true, "select": true, "email": true}

// validFieldSections are the two sections present.SubmissionForm groups
// fields into for the public form's two-fieldset layout (FB-1).
var validFieldSections = map[string]bool{"proposal": true, "participant": true}

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Forms(appstate.MustGet().Snapshot())
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Forms & routing — Rostrum"}, Description: "Conditional CFP configuration with audited category routing."}, nil
		},
		Actions: route.FileActions{
			"toggleForm":      toggleForm,
			"addField":        addField,
			"updateField":     updateField,
			"removeField":     removeField,
			"moveField":       moveField,
			"setFormSchedule": setFormSchedule,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

func toggleForm(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	nextStatus := strings.TrimSpace(ctx.FormData["status"])
	if nextStatus != "open" && nextStatus != "closed" {
		return action.Validation("Choose a valid form state.", map[string]string{"status": "Use open or closed."}, ctx.FormData)
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		form.Status = nextStatus
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "CFP state changed to "+nextStatus+".")
	live.Broadcast("form:updated", map[string]string{"id": formID, "status": nextStatus})
	actionflow.Redirect(ctx, "/organizer/forms")
	return nil
}

// addField appends a new FormField to the form's schema (FB-3). Because
// app/submit/[slug]/page.gsx renders app/submit/page.server.go's
// present.SubmissionForm straight from form.Fields, the new field appears on
// the public form on its next load, with no other change required.
func addField(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	label, fieldType, section, options, maxLength, fieldErrors := parseFieldInput(ctx.FormData)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the highlighted fields.", fieldErrors, ctx.FormData)
	}

	newID := ""
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		newID = uniqueFieldID(form.Fields, domain.Slugify(label))
		form.Fields = append(form.Fields, domain.FormField{
			ID:          newID,
			Section:     section,
			Label:       label,
			Type:        fieldType,
			Required:    ctx.FormData["required"] == "yes",
			Placeholder: strings.TrimSpace(ctx.FormData["placeholder"]),
			Help:        strings.TrimSpace(ctx.FormData["help"]),
			Options:     options,
			MaxLength:   maxLength,
		})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Added the "+label+" field.")
	live.Broadcast("form:updated", map[string]string{"id": formID, "field": newID})
	actionflow.Redirect(ctx, "/organizer/forms")
	return nil
}

// updateField edits an existing field's label, type, section, requirement,
// placeholder, help text, options, and max length (FB-3). It never changes
// the field's ID: IDs feed Arbiter facts and other fields (for example the
// ConditionalFormatFields island's "workshop_needs" reference) key off them,
// so an ID stays stable once addField creates it.
func updateField(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	fieldID := strings.TrimSpace(ctx.FormData["field_id"])
	label, fieldType, section, options, maxLength, fieldErrors := parseFieldInput(ctx.FormData)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the highlighted fields.", fieldErrors, ctx.FormData)
	}

	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		for index := range form.Fields {
			if form.Fields[index].ID != fieldID {
				continue
			}
			form.Fields[index].Label = label
			form.Fields[index].Type = fieldType
			form.Fields[index].Section = section
			form.Fields[index].Required = ctx.FormData["required"] == "yes"
			form.Fields[index].Placeholder = strings.TrimSpace(ctx.FormData["placeholder"])
			form.Fields[index].Help = strings.TrimSpace(ctx.FormData["help"])
			form.Fields[index].Options = options
			form.Fields[index].MaxLength = maxLength
			return nil
		}
		return action.Error(404, "Field not found.")
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Updated the "+label+" field.")
	live.Broadcast("form:updated", map[string]string{"id": formID, "field": fieldID})
	actionflow.Redirect(ctx, "/organizer/forms")
	return nil
}

// removeField deletes a field the builder no longer wants. A Locked field
// (category, format, level, and the other routing/core fields FB-1 keeps
// present) cannot be removed: those IDs are what the routing engine, the
// speaker record, and Submission's typed columns key off.
func removeField(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	fieldID := strings.TrimSpace(ctx.FormData["field_id"])
	label := ""
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		for index, field := range form.Fields {
			if field.ID != fieldID {
				continue
			}
			if field.Locked {
				return action.Validation("This field is locked and cannot be removed.", map[string]string{"field": "Locked fields power required routing and cannot be deleted."}, ctx.FormData)
			}
			label = field.Label
			form.Fields = append(form.Fields[:index], form.Fields[index+1:]...)
			return nil
		}
		return action.Error(404, "Field not found.")
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Removed the "+label+" field.")
	live.Broadcast("form:updated", map[string]string{"id": formID, "field": fieldID})
	actionflow.Redirect(ctx, "/organizer/forms")
	return nil
}

// moveField reorders one field up or down by one position (FB-3). This is
// the "two small move buttons" the spec asks for in place of a drag handle;
// a drag-reorder island stays a later polish item, not part of this unit.
func moveField(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	fieldID := strings.TrimSpace(ctx.FormData["field_id"])
	direction := strings.TrimSpace(ctx.FormData["direction"])
	if direction != "up" && direction != "down" {
		return action.Validation("Choose a direction to move the field.", map[string]string{"direction": "Use up or down."}, ctx.FormData)
	}
	moved := false
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		index := -1
		for candidate := range form.Fields {
			if form.Fields[candidate].ID == fieldID {
				index = candidate
				break
			}
		}
		if index == -1 {
			return action.Error(404, "Field not found.")
		}
		target := index - 1
		if direction == "down" {
			target = index + 1
		}
		if target < 0 || target >= len(form.Fields) {
			return nil
		}
		form.Fields[index], form.Fields[target] = form.Fields[target], form.Fields[index]
		moved = true
		return nil
	}); err != nil {
		return err
	}
	if moved {
		session.AddFlash(ctx.Request, "notice", "Reordered the fields.")
		live.Broadcast("form:updated", map[string]string{"id": formID, "field": fieldID})
	}
	actionflow.Redirect(ctx, "/organizer/forms")
	return nil
}

// setFormSchedule saves the CFP close date and time (FB-3). This is what
// makes app/organizer/forms/page.gsx's close-date input a working control:
// app/submit/page.server.go's submitProposal already blocks a submission
// once time.Now() passes form.CloseAt, so saving a new CloseAt here changes
// live, server-enforced behavior on the public form, not just display copy.
func setFormSchedule(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	closeAtValue := strings.TrimSpace(ctx.FormData["close_at"])
	if closeAtValue == "" {
		return action.Validation("Choose a close date and time.", map[string]string{"close_at": "Required."}, ctx.FormData)
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		location, loadErr := time.LoadLocation(state.Event.TimeZone)
		if loadErr != nil {
			location = time.UTC
		}
		closeAt, parseErr := time.ParseInLocation("2006-01-02T15:04", closeAtValue, location)
		if parseErr != nil {
			return action.Validation("Choose a valid close date and time.", map[string]string{"close_at": "Invalid date or time."}, ctx.FormData)
		}
		form.CloseAt = closeAt
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Updated the CFP close date.")
	live.Broadcast("form:updated", map[string]string{"id": formID})
	actionflow.Redirect(ctx, "/organizer/forms")
	return nil
}

// parseFieldInput validates the fields addField and updateField share: a
// non-empty label, a known type, a known section, options (required, and
// only meaningful, for a "select" field), and an optional non-negative max
// length. It returns the parsed values plus a fieldErrors map that is empty
// when every input is valid.
func parseFieldInput(formData map[string]string) (label, fieldType, section string, options []string, maxLength int, fieldErrors map[string]string) {
	fieldErrors = map[string]string{}
	label = strings.TrimSpace(formData["label"])
	if label == "" {
		fieldErrors["label"] = "Enter a label for the field."
	}
	fieldType = strings.TrimSpace(formData["type"])
	if !validFieldTypes[fieldType] {
		fieldErrors["type"] = "Choose a field type."
	}
	section = strings.TrimSpace(formData["section"])
	if !validFieldSections[section] {
		fieldErrors["section"] = "Choose a section."
	}
	options = parseFieldOptions(formData["options"])
	if fieldType == "select" && len(options) == 0 {
		fieldErrors["options"] = "Add at least one option, separated by commas."
	}
	var maxLengthErr error
	maxLength, maxLengthErr = parseMaxLength(formData["max_length"])
	if maxLengthErr != nil {
		fieldErrors["max_length"] = maxLengthErr.Error()
	}
	return label, fieldType, section, options, maxLength, fieldErrors
}

// parseFieldOptions splits the builder's single comma-separated options
// input into the trimmed, non-empty list domain.FormField.Options stores.
func parseFieldOptions(value string) []string {
	parts := strings.Split(value, ",")
	options := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			options = append(options, trimmed)
		}
	}
	return options
}

// parseMaxLength parses the builder's optional max-length input. A blank
// input means no limit (0). A non-numeric or negative input is a validation
// error rather than a silent 0, so a typo cannot quietly remove a field's
// length guard.
func parseMaxLength(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("enter a whole number of characters, or leave it blank for no limit")
	}
	return parsed, nil
}

// uniqueFieldID returns base, or base suffixed with -2, -3, and so on, until
// it finds an ID none of fields already uses. Field IDs are slug-safe and
// stable once created (they feed Arbiter facts), so a new field's ID must
// never collide with one already on the form, even when two fields share a
// label.
func uniqueFieldID(fields []domain.FormField, base string) string {
	if base == "" {
		base = "field"
	}
	candidate := base
	for suffix := 2; fieldIDTaken(fields, candidate); suffix++ {
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
	return candidate
}

func fieldIDTaken(fields []domain.FormField, id string) bool {
	for _, field := range fields {
		if field.ID == id {
			return true
		}
	}
	return false
}
