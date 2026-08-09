package present

import (
	"github.com/odvcencio/programma/internal/domain"
	decisionrules "github.com/odvcencio/programma/rules"
)

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
			"index":       index + 1,
			"id":          field.ID,
			"section":     StatusLabel(field.Section),
			"label":       field.Label,
			"kind":        StatusLabel(field.Type),
			"required":    field.Required,
			"locked":      field.Locked,
			"requirement": requirementLabel(field.Required),
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
