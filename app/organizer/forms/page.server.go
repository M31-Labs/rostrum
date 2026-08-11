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
	"m31labs.dev/gosx/auth"
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
			return present.Forms(appstate.MustGet().Snapshot(), ctx.Query("form"))
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Forms & routing — Rostrum"}, Description: "Conditional CFP configuration with audited category routing."}, nil
		},
		Actions: route.FileActions{
			"createForm":         createForm,
			"toggleForm":         toggleForm,
			"addField":           addField,
			"updateField":        updateField,
			"removeField":        removeField,
			"moveField":          moveField,
			"setFormSchedule":    setFormSchedule,
			"addQuestionRule":    addQuestionRule,
			"removeQuestionRule": removeQuestionRule,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

// createForm makes a separately addressable public CFP. New forms begin
// closed and receive the locked core schema that preserves typed submission
// fields and routing; organizers deliberately open them only after their
// wording, schedule, and conditional questions are ready.
func createForm(ctx *action.Context) error {
	name := strings.TrimSpace(ctx.FormData["name"])
	title := strings.TrimSpace(ctx.FormData["title"])
	slug := domain.Slugify(ctx.FormData["slug"])
	kind := strings.TrimSpace(ctx.FormData["kind"])
	fieldErrors := map[string]string{}
	if name == "" {
		fieldErrors["name"] = "Enter an internal form name."
	}
	if title == "" {
		fieldErrors["title"] = "Enter the public form title."
	}
	if slug == "" {
		slug = domain.Slugify(title)
	}
	if slug == "" || slug != strings.TrimSpace(ctx.FormData["slug"]) && strings.TrimSpace(ctx.FormData["slug"]) != "" {
		fieldErrors["slug"] = "Use lowercase letters, numbers, and hyphens."
	}
	if kind == "" {
		kind = "abstract"
	}
	if kind != "abstract" && kind != "proposal" && kind != "application" {
		fieldErrors["kind"] = "Choose abstract, proposal, or application."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the form details.", fieldErrors, ctx.FormData)
	}

	formID := uniqueFormID(appstate.MustGet().Snapshot().Forms, "form_"+slug)
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.created",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Created a new closed public submission form.",
		Origin:     "organizer-forms",
	}, func(state *domain.State) error {
		for _, existing := range state.Forms {
			if existing.Slug == slug {
				return action.Validation("Choose a unique public slug.", map[string]string{"slug": "That public form URL is already in use."}, ctx.FormData)
			}
		}
		// The form ID was resolved against the snapshot immediately before the
		// mutation. A concurrent form creation with the same slug is rejected
		// above, preserving the audit EntityID chosen for this mutation.
		if formIDTaken(state.Forms, formID) {
			return action.Validation("Choose a unique public slug.", map[string]string{"slug": "That public form URL is already in use."}, ctx.FormData)
		}
		state.Forms = append(state.Forms, domain.SubmissionForm{
			ID:                    formID,
			EventID:               state.Event.ID,
			Name:                  name,
			ExternalTitle:         title,
			Slug:                  slug,
			Kind:                  kind,
			Status:                "closed",
			WelcomeHeading:        "Tell us what you have learned.",
			WelcomeBody:           "Start a draft whenever you like; the program team will open this call when it is ready for submissions.",
			CloseAt:               time.Now().Add(30 * 24 * time.Hour).UTC(),
			MaxDraftsPerSubmitter: 3,
			RedirectToPortal:      true,
			SendConfirmation:      true,
			ConfirmationTemplate:  "tpl_submission_confirmation",
			RuleFile:              "rules/form-visibility.arb",
			Fields:                coreFields(state.Event),
		})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Created a closed form. Configure it, then open it when ready.")
	live.Broadcast("form:created", map[string]string{"id": formID, "slug": slug})
	actionflow.Redirect(ctx, formURL(formID))
	return nil
}

func toggleForm(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	nextStatus := strings.TrimSpace(ctx.FormData["status"])
	if nextStatus != "open" && nextStatus != "closed" {
		return action.Validation("Choose a valid form state.", map[string]string{"status": "Use open or closed."}, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.status_changed",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Changed the public submission form state to " + nextStatus + ".",
		Origin:     "organizer-forms",
	}, func(state *domain.State) error {
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
	actionflow.Redirect(ctx, formURL(formID))
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
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.field_added",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Added a custom question to the submission form.",
		Origin:     "organizer-forms",
	}, func(state *domain.State) error {
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
	actionflow.Redirect(ctx, formURL(formID))
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

	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.field_updated",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Updated a custom question on the submission form.",
		Origin:     "organizer-forms",
	}, func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		for index := range form.Fields {
			if form.Fields[index].ID != fieldID {
				continue
			}
			if form.Fields[index].Locked {
				return action.Validation("Core fields are locked.", map[string]string{"field": "Locked fields keep routing and speaker identity stable."}, ctx.FormData)
			}
			if questionRuleNeedsSection(form.QuestionRules, fieldID) && form.Fields[index].Section != section {
				return action.Validation("Remove or update this question's conditional rule first.", map[string]string{"section": "A conditional source and target must stay in the same section."}, ctx.FormData)
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
	actionflow.Redirect(ctx, formURL(formID))
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
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.field_removed",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Removed a custom question from the submission form.",
		Origin:     "organizer-forms",
	}, func(state *domain.State) error {
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
			if questionRuleReferences(form.QuestionRules, fieldID) {
				return action.Validation("Remove the conditional rule first.", map[string]string{"field": "This question is used by a conditional rule."}, ctx.FormData)
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
	actionflow.Redirect(ctx, formURL(formID))
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
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.field_reordered",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Reordered a submission form question.",
		Origin:     "organizer-forms",
	}, func(state *domain.State) error {
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
	actionflow.Redirect(ctx, formURL(formID))
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
	draftLimitValue := strings.TrimSpace(ctx.FormData["max_drafts_per_submitter"])
	if closeAtValue == "" {
		return action.Validation("Choose a close date and time.", map[string]string{"close_at": "Required."}, ctx.FormData)
	}
	draftLimit := 3
	if draftLimitValue != "" {
		parsed, parseErr := strconv.Atoi(draftLimitValue)
		if parseErr != nil || parsed < 1 || parsed > 10 {
			return action.Validation("Choose a draft limit between 1 and 10.", map[string]string{"max_drafts_per_submitter": "Use a whole number from 1 to 10."}, ctx.FormData)
		}
		draftLimit = parsed
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.schedule_updated",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Updated the CFP close date and draft limit.",
		Origin:     "organizer-forms",
	}, func(state *domain.State) error {
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
		form.MaxDraftsPerSubmitter = draftLimit
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Updated the CFP close date.")
	live.Broadcast("form:updated", map[string]string{"id": formID})
	actionflow.Redirect(ctx, formURL(formID))
	return nil
}

// addQuestionRule creates one constrained, explainable visibility rule. The
// application validates the builder-selected field references; Arbiter owns
// the resulting equality decision at runtime (rules/form-visibility.arb).
func addQuestionRule(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	sourceID := strings.TrimSpace(ctx.FormData["source_field_id"])
	targetID := strings.TrimSpace(ctx.FormData["target_field_id"])
	value := strings.TrimSpace(ctx.FormData["value"])
	fieldErrors := map[string]string{}
	if sourceID == "" {
		fieldErrors["source_field_id"] = "Choose the question that controls visibility."
	}
	if targetID == "" {
		fieldErrors["target_field_id"] = "Choose the question to reveal."
	}
	if value == "" {
		fieldErrors["value"] = "Enter the answer that reveals the question."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the conditional rule.", fieldErrors, ctx.FormData)
	}

	ruleID := ""
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.question_rule_added",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Added a governed conditional question rule.",
		Origin:     "organizer-forms",
		Rule:       "form-visibility.arb",
	}, func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		source, sourceFound := formField(form.Fields, sourceID)
		target, targetFound := formField(form.Fields, targetID)
		if !sourceFound {
			return action.Validation("Choose a valid source question.", map[string]string{"source_field_id": "That question is no longer on this form."}, ctx.FormData)
		}
		if !targetFound {
			return action.Validation("Choose a valid target question.", map[string]string{"target_field_id": "That question is no longer on this form."}, ctx.FormData)
		}
		if source.ID == target.ID {
			return action.Validation("Choose two different questions.", map[string]string{"target_field_id": "A question cannot control itself."}, ctx.FormData)
		}
		if source.Section != target.Section {
			return action.Validation("Keep conditional questions in one section.", map[string]string{"target_field_id": "Choose a target in the same form section as the source."}, ctx.FormData)
		}
		if target.Locked {
			return action.Validation("Core questions cannot be conditional.", map[string]string{"target_field_id": "Choose an unlocked custom question."}, ctx.FormData)
		}
		if questionRuleReferencesTarget(form.QuestionRules, source.ID) || questionRuleReferencesSource(form.QuestionRules, target.ID) {
			return action.Validation("Keep conditional rules to one layer.", map[string]string{"target_field_id": "A conditional target cannot also control another question."}, ctx.FormData)
		}
		if questionRuleTargetTaken(form.QuestionRules, target.ID) {
			return action.Validation("That question already has a visibility rule.", map[string]string{"target_field_id": "Remove its existing rule before changing the source."}, ctx.FormData)
		}
		if source.Type == "select" && !formFieldAllowsValue(state, source, value) {
			return action.Validation("Choose an answer offered by the source question.", map[string]string{"value": "This must match one of the source question's options."}, ctx.FormData)
		}
		ruleID = uniqueQuestionRuleID(form.QuestionRules, "rule_"+source.ID+"_"+target.ID)
		form.QuestionRules = append(form.QuestionRules, domain.QuestionRule{
			ID:            ruleID,
			SourceFieldID: source.ID,
			Operator:      "equals",
			Value:         value,
			TargetFieldID: target.ID,
			Effect:        "show",
			Description:   "Show “" + target.Label + "” when “" + source.Label + "” is “" + value + "”.",
		})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Added the conditional question rule.")
	live.Broadcast("form:updated", map[string]string{"id": formID, "rule": ruleID})
	actionflow.Redirect(ctx, formURL(formID))
	return nil
}

func removeQuestionRule(ctx *action.Context) error {
	formID := strings.TrimSpace(ctx.FormData["form_id"])
	ruleID := strings.TrimSpace(ctx.FormData["rule_id"])
	if ruleID == "" {
		return action.Validation("Choose a rule to remove.", map[string]string{"rule": "Rule identity is missing."}, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      formsActor(ctx),
		Action:     "form.question_rule_removed",
		EntityType: "submission_form",
		EntityID:   formID,
		Summary:    "Removed a governed conditional question rule.",
		Origin:     "organizer-forms",
		Rule:       "form-visibility.arb",
	}, func(state *domain.State) error {
		form, found := state.Form(formID)
		if !found {
			return action.Error(404, "Form not found.")
		}
		for index, rule := range form.QuestionRules {
			if rule.ID == ruleID {
				form.QuestionRules = append(form.QuestionRules[:index], form.QuestionRules[index+1:]...)
				return nil
			}
		}
		return action.Error(404, "Question rule not found.")
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Removed the conditional question rule.")
	live.Broadcast("form:updated", map[string]string{"id": formID, "rule": ruleID})
	actionflow.Redirect(ctx, formURL(formID))
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

func uniqueFormID(forms []domain.SubmissionForm, base string) string {
	if base == "form_" || base == "" {
		base = "form"
	}
	candidate := base
	for suffix := 2; formIDTaken(forms, candidate); suffix++ {
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
	return candidate
}

func formIDTaken(forms []domain.SubmissionForm, id string) bool {
	for _, form := range forms {
		if form.ID == id {
			return true
		}
	}
	return false
}

// coreFields is the minimal, locked schema every new CFP receives. The
// underlying typed Submission and Speaker records only need these fields to
// preserve routing, review, speaker identity, and a usable public workflow.
func coreFields(event domain.Event) []domain.FormField {
	return []domain.FormField{
		{ID: "title", Section: "proposal", Label: "Session title", Type: "text", Required: true, Locked: true, Placeholder: "A concrete title that tells us what changes", MaxLength: 120},
		{ID: "abstract", Section: "proposal", Label: "Abstract", Type: "textarea", Required: true, Locked: true, Help: "Describe the problem, the approach, and what attendees will be able to do afterward.", MaxLength: 1600},
		{ID: "format", Section: "proposal", Label: "Format", Type: "select", Required: true, Locked: true, Options: append([]string(nil), event.Formats...)},
		{ID: "category", Section: "proposal", Label: "Category", Type: "select", Required: true, Locked: true, Options: categoryNames(event.Categories)},
		{ID: "level", Section: "proposal", Label: "Audience level", Type: "select", Required: true, Locked: true, Options: append([]string(nil), event.Levels...)},
		{ID: "first_name", Section: "participant", Label: "First name", Type: "text", Required: true, Locked: true, MaxLength: 80},
		{ID: "last_name", Section: "participant", Label: "Last name", Type: "text", Required: true, Locked: true, MaxLength: 80},
		{ID: "email", Section: "participant", Label: "Email", Type: "email", Required: true, Locked: true, MaxLength: 254},
		{ID: "role", Section: "participant", Label: "Role", Type: "text", Required: false, Locked: true, MaxLength: 160},
		{ID: "company", Section: "participant", Label: "Company or project", Type: "text", Required: false, Locked: true, MaxLength: 160},
		{ID: "biography", Section: "participant", Label: "Short biography", Type: "textarea", Required: false, Locked: true, MaxLength: 800},
	}
}

func categoryNames(categories []domain.Category) []string {
	values := make([]string, 0, len(categories))
	for _, category := range categories {
		values = append(values, category.Name)
	}
	return values
}

func formField(fields []domain.FormField, id string) (domain.FormField, bool) {
	for _, field := range fields {
		if field.ID == id {
			return field, true
		}
	}
	return domain.FormField{}, false
}

func formFieldAllowsValue(state *domain.State, field domain.FormField, value string) bool {
	for _, option := range present.FormFieldOptionValues(*state, field) {
		if option == value {
			return true
		}
	}
	return false
}

func questionRuleTargetTaken(rules []domain.QuestionRule, targetID string) bool {
	for _, rule := range rules {
		if rule.TargetFieldID == targetID {
			return true
		}
	}
	return false
}

func questionRuleReferences(rules []domain.QuestionRule, fieldID string) bool {
	for _, rule := range rules {
		if rule.SourceFieldID == fieldID || rule.TargetFieldID == fieldID {
			return true
		}
	}
	return false
}

func questionRuleReferencesTarget(rules []domain.QuestionRule, fieldID string) bool {
	for _, rule := range rules {
		if rule.TargetFieldID == fieldID {
			return true
		}
	}
	return false
}

func questionRuleReferencesSource(rules []domain.QuestionRule, fieldID string) bool {
	for _, rule := range rules {
		if rule.SourceFieldID == fieldID {
			return true
		}
	}
	return false
}

func questionRuleNeedsSection(rules []domain.QuestionRule, fieldID string) bool {
	return questionRuleReferences(rules, fieldID)
}

func uniqueQuestionRuleID(rules []domain.QuestionRule, base string) string {
	if base == "" {
		base = "rule"
	}
	candidate := base
	for suffix := 2; questionRuleIDTaken(rules, candidate); suffix++ {
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
	return candidate
}

func questionRuleIDTaken(rules []domain.QuestionRule, id string) bool {
	for _, rule := range rules {
		if rule.ID == id {
			return true
		}
	}
	return false
}

func formURL(formID string) string {
	return "/organizer/forms?form=" + formID
}

func formsActor(ctx *action.Context) string {
	if ctx != nil && ctx.Request != nil {
		if user, ok := auth.Current(ctx.Request); ok && strings.TrimSpace(user.ID) != "" {
			return "organizer:" + strings.TrimSpace(user.ID)
		}
	}
	return "organizer"
}
