package present

import (
	"strconv"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
	decisionrules "github.com/m31-labs/rostrum/rules"
)

// FieldTypes lists the field types the builder offers. Every value here has a
// public renderer, so a builder edit cannot create an orphaned question.
var FieldTypes = []map[string]string{
	{"value": "text", "label": "Short text"},
	{"value": "textarea", "label": "Paragraph"},
	{"value": "select", "label": "Choice list"},
	{"value": "email", "label": "Email"},
}

// FieldSections are the two deliberate public-form groups. Conditional rules
// are constrained to fields in the same section so the interactive group can
// preserve schema order and a coherent accessible fieldset.
var FieldSections = []map[string]string{
	{"value": "proposal", "label": "Proposal"},
	{"value": "participant", "label": "Participant"},
}

// Forms presents the selected form's builder surface while retaining a full
// picker for workspaces that run more than one CFP. Selecting a picker link
// is a GoSX soft navigation; mutations remain managed ActionForms.
func Forms(state domain.State, selection ...string) (map[string]any, error) {
	selected := ""
	if len(selection) > 0 {
		selected = selection[0]
	}
	engine, err := decisionrules.Shared()
	if err != nil {
		return nil, err
	}

	routes := make([]map[string]any, 0, len(state.Event.Categories))
	for _, category := range state.Event.Categories {
		decision, err := engine.Route(category.ID, "Talk", "Intermediate")
		if err != nil {
			return nil, err
		}
		routes = append(routes, map[string]any{
			"category": category.Name,
			"queue":    decision.Queue,
			"owner":    decision.Owner,
			"track":    TrackName(state, decision.Track),
			"rule":     decision.Rule,
			"reason":   decision.Reason,
		})
	}

	formRows := make([]map[string]any, 0, len(state.Forms))
	for _, item := range state.Forms {
		formRows = append(formRows, map[string]any{
			"id":         item.ID,
			"name":       item.Name,
			"title":      item.ExternalTitle,
			"slug":       item.Slug,
			"kind":       StatusLabel(item.Kind),
			"status":     StatusLabel(item.Status),
			"statusTone": StatusTone(item.Status),
			"selected":   selectedFormID(state, selected) == item.ID,
			"href":       "/organizer/forms?form=" + item.ID,
			"publicURL":  "/submit/" + item.Slug,
			"fieldCount": len(item.Fields),
		})
	}

	form, found := selectedSubmissionForm(state, selected)
	if !found {
		return map[string]any{
			"section":       "forms",
			"demoMode":      DemoMode(),
			"workspace":     WorkspaceIdentity(state),
			"hasForm":       false,
			"forms":         formRows,
			"form":          emptyFormRow(),
			"fields":        []map[string]any{},
			"fieldTypes":    FieldTypes,
			"fieldSections": FieldSections,
			"ruleChoices":   []map[string]any{},
			"questionRules": []map[string]any{},
			"routes":        routes,
		}, nil
	}

	fields := formBuilderFields(form.Fields)
	ruleChoices := make([]map[string]any, 0, len(form.Fields))
	for _, field := range form.Fields {
		ruleChoices = append(ruleChoices, map[string]any{
			"id":      field.ID,
			"label":   field.Label,
			"section": field.Section,
			"locked":  field.Locked,
		})
	}

	questionRules := make([]map[string]any, 0, len(form.QuestionRules))
	for _, rule := range form.QuestionRules {
		visibility, err := engine.QuestionVisibility("", rule.Value, rule.Effect, rule.TargetFieldID)
		if err != nil {
			return nil, err
		}
		questionRules = append(questionRules, map[string]any{
			"id":          rule.ID,
			"sourceID":    rule.SourceFieldID,
			"targetID":    rule.TargetFieldID,
			"source":      formFieldLabel(form.Fields, rule.SourceFieldID),
			"target":      formFieldLabel(form.Fields, rule.TargetFieldID),
			"operator":    rule.Operator,
			"value":       rule.Value,
			"effect":      rule.Effect,
			"condition":   formFieldLabel(form.Fields, rule.SourceFieldID) + " " + rule.Operator + " “" + rule.Value + "”",
			"then":        rule.Effect + " “" + formFieldLabel(form.Fields, rule.TargetFieldID) + "”",
			"description": rule.Description,
			"policy":      visibility.Rule,
			"trace":       visibility.Reason,
		})
	}

	return map[string]any{
		"section":       "forms",
		"demoMode":      DemoMode(),
		"workspace":     WorkspaceIdentity(state),
		"hasForm":       true,
		"forms":         formRows,
		"form":          submissionFormRow(form),
		"fields":        fields,
		"fieldTypes":    FieldTypes,
		"fieldSections": FieldSections,
		"ruleChoices":   ruleChoices,
		"questionRules": questionRules,
		"routes":        routes,
	}, nil
}

func selectedSubmissionForm(state domain.State, selected string) (domain.SubmissionForm, bool) {
	if strings.TrimSpace(selected) != "" {
		if form, found := state.Form(selected); found {
			return *form, true
		}
	}
	if len(state.Forms) == 0 {
		return domain.SubmissionForm{}, false
	}
	return state.Forms[0], true
}

func selectedFormID(state domain.State, selected string) string {
	if form, found := selectedSubmissionForm(state, selected); found {
		return form.ID
	}
	return ""
}

func emptyFormRow() map[string]any {
	return map[string]any{
		"id": "", "name": "", "title": "", "status": "", "statusValue": "", "statusTone": "neutral",
		"close": "Not scheduled", "closeISO": "", "redirect": false, "confirmation": false, "ruleFile": "rules/form-visibility.arb",
		"publicURL": "", "welcomeHeading": "", "welcomeBody": "", "fieldCount": 0, "conditionalCount": 0,
		"maxDraftsPerSubmitter": "3",
	}
}

func submissionFormRow(form domain.SubmissionForm) map[string]any {
	row := emptyFormRow()
	row["id"] = form.ID
	row["name"] = form.Name
	row["title"] = form.ExternalTitle
	row["status"] = StatusLabel(form.Status)
	row["statusValue"] = form.Status
	row["statusTone"] = StatusTone(form.Status)
	row["close"] = DateTime(form.CloseAt)
	row["closeISO"] = form.CloseAt.Format("2006-01-02T15:04")
	row["redirect"] = form.RedirectToPortal
	row["confirmation"] = form.SendConfirmation
	row["ruleFile"] = form.RuleFile
	row["publicURL"] = "/submit/" + form.Slug
	row["welcomeHeading"] = form.WelcomeHeading
	row["welcomeBody"] = form.WelcomeBody
	row["fieldCount"] = len(form.Fields)
	row["conditionalCount"] = len(form.QuestionRules)
	row["maxDraftsPerSubmitter"] = strconv.Itoa(draftLimit(form))
	return row
}

func formBuilderFields(formFields []domain.FormField) []map[string]any {
	fields := make([]map[string]any, 0, len(formFields))
	for index, field := range formFields {
		fields = append(fields, map[string]any{
			"index":        index + 1,
			"id":           field.ID,
			"section":      StatusLabel(field.Section),
			"sectionValue": field.Section,
			"label":        field.Label,
			"kind":         StatusLabel(field.Type),
			"typeValue":    field.Type,
			"required":     field.Required,
			"locked":       field.Locked,
			"requirement":  requirementLabel(field.Required),
			"placeholder":  field.Placeholder,
			"help":         field.Help,
			"options":      strings.Join(field.Options, ", "),
			"maxLength":    maxLengthValue(field.MaxLength),
			"first":        index == 0,
			"last":         index == len(formFields)-1,
		})
	}
	return fields
}

func formFieldLabel(fields []domain.FormField, id string) string {
	for _, field := range fields {
		if field.ID == id {
			return field.Label
		}
	}
	return id
}

func draftLimit(form domain.SubmissionForm) int {
	if form.MaxDraftsPerSubmitter > 0 {
		return form.MaxDraftsPerSubmitter
	}
	return 3
}

func requirementLabel(required bool) string {
	if required {
		return "Required"
	}
	return "Optional"
}

func maxLengthValue(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}
