package portal

import (
	"testing"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestValidateTaskSubmissionRequiresConfirmationAndDeclaredOptions(t *testing.T) {
	if errors := validateTaskSubmission(domain.Task{Type: "form"}, map[string]string{}); errors["task"] == "" {
		t.Fatalf("empty confirmation errors = %#v, want task confirmation error", errors)
	}
	task := domain.Task{Type: "form", FormFields: []domain.FormField{
		{ID: "choice", Type: "select", Required: true, Options: []string{"A", "B"}},
		{ID: "agree", Type: "checkbox", Required: true},
	}}
	errors := validateTaskSubmission(task, map[string]string{"choice": "forged", "agree": "yes"})
	if errors["choice"] == "" {
		t.Fatalf("invalid select errors = %#v, want choice failure", errors)
	}
	if errors := validateTaskSubmission(task, map[string]string{"choice": "A", "agree": "on"}); len(errors) != 0 {
		t.Fatalf("valid task values errors = %#v, want none", errors)
	}
}
