package present

import (
	"strconv"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
	decisionrules "github.com/m31-labs/rostrum/rules"
)

// FieldTypes lists the field types the FB-3 builder offers when adding or
// editing a field. Every value here is one app/submit/[slug]/page.gsx's
// FormFieldRow already knows how to render on the public form (FB-1), so a
// builder edit is guaranteed to have somewhere to land on the next load.
var FieldTypes = []map[string]string{
	{"value": "text", "label": "Short text"},
	{"value": "textarea", "label": "Paragraph"},
	{"value": "select", "label": "Choice list"},
	{"value": "email", "label": "Email"},
}

// FieldSections lists the two sections the public submission form groups
// fields into (FB-1's two-section layout: proposal, then participant).
var FieldSections = []map[string]string{
	{"value": "proposal", "label": "Proposal"},
	{"value": "participant", "label": "Participant"},
}

func Forms(state domain.State) (map[string]any, error) {
	engine, err := decisionrules.New()
	if err != nil {
		return nil, err
	}
	form := state.Forms[0]
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

	fields := make([]map[string]any, 0, len(form.Fields))
	for index, field := range form.Fields {
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
			// options is the comma-joined form the FB-3 builder's single
			// options input edits; addField/updateField in
			// app/organizer/forms/page.server.go split it back apart on save.
			"options":   strings.Join(field.Options, ", "),
			"maxLength": maxLengthValue(field.MaxLength),
			"first":     index == 0,
			"last":      index == len(form.Fields)-1,
		})
	}

	questionRules := make([]map[string]any, 0, len(form.QuestionRules))
	for _, rule := range form.QuestionRules {
		visibility, err := engine.FieldVisibility(rule.Value, "")
		if err != nil {
			return nil, err
		}
		questionRules = append(questionRules, map[string]any{
			"condition":   rule.SourceFieldID + " " + rule.Operator + " “" + rule.Value + "”",
			"then":        rule.Effect + " “" + rule.TargetFieldID + "”",
			"description": rule.Description,
			"policy":      visibility.Rule,
			"trace":       visibility.Reason,
		})
	}

	return map[string]any{
		"section": "forms",
		"form": map[string]any{
			"id":               form.ID,
			"name":             form.Name,
			"title":            form.ExternalTitle,
			"status":           StatusLabel(form.Status),
			"statusValue":      form.Status,
			"statusTone":       StatusTone(form.Status),
			"close":            DateTime(form.CloseAt),
			"closeISO":         form.CloseAt.Format("2006-01-02T15:04"),
			"redirect":         form.RedirectToPortal,
			"confirmation":     form.SendConfirmation,
			"ruleFile":         form.RuleFile,
			"publicURL":        "/submit/" + form.Slug,
			"welcomeHeading":   form.WelcomeHeading,
			"welcomeBody":      form.WelcomeBody,
			"fieldCount":       len(form.Fields),
			"conditionalCount": len(form.QuestionRules),
		},
		"fields":        fields,
		"fieldTypes":    FieldTypes,
		"fieldSections": FieldSections,
		"questionRules": questionRules,
		"routes":        routes,
	}, nil
}

func requirementLabel(required bool) string {
	if required {
		return "Required"
	}
	return "Optional"
}

// maxLengthValue renders a FormField.MaxLength for the builder's max-length
// input: blank for "no limit" (0 or less) rather than a literal "0", so a
// freshly seeded field without a limit does not look like a zero-character
// field.
func maxLengthValue(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}
