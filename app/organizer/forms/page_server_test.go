package forms

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

func formsTestWorkspace(t *testing.T) *store.JSONStore {
	t.Helper()
	workspace, err := store.Open(":memory:", domain.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	return workspace
}

func TestCreateFormProvidesIndependentLockedCoreSchema(t *testing.T) {
	workspace := formsTestWorkspace(t)
	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/forms/__actions/createForm", nil),
		FormData: map[string]string{
			"name":  "Scholarship applications",
			"title": "Apply for a scholarship",
			"slug":  "scholarship-application",
			"kind":  "application",
		},
	}
	if err := createForm(ctx); err != nil {
		t.Fatalf("createForm: %v", err)
	}

	snapshot := workspace.Snapshot()
	form, found := snapshot.Form("form_scholarship-application")
	if !found {
		t.Fatalf("created form not found in %#v", snapshot.Forms)
	}
	if form.Status != "closed" || form.MaxDraftsPerSubmitter != 3 || form.Slug != "scholarship-application" {
		t.Fatalf("created form = %#v", form)
	}
	locked := map[string]bool{}
	for _, field := range form.Fields {
		locked[field.ID] = field.Locked
	}
	for _, id := range []string{"title", "abstract", "format", "category", "level", "first_name", "last_name", "email"} {
		if !locked[id] {
			t.Fatalf("core field %s is not locked: %#v", id, form.Fields)
		}
	}
	audit := snapshot.AuditEvents[len(snapshot.AuditEvents)-1]
	if audit.Action != "form.created" || audit.EntityID != form.ID {
		t.Fatalf("create audit = %#v, want form entity %s", audit, form.ID)
	}
}

func TestAddQuestionRuleUsesGenericPolicyShape(t *testing.T) {
	workspace := formsTestWorkspace(t)
	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/forms/__actions/addQuestionRule", nil),
		FormData: map[string]string{
			"form_id":         "form_cfp_2026",
			"source_field_id": "format",
			"value":           "Panel",
			"target_field_id": "topics",
		},
	}
	if err := addQuestionRule(ctx); err != nil {
		t.Fatalf("addQuestionRule: %v", err)
	}
	snapshot := workspace.Snapshot()
	form, found := snapshot.Form("form_cfp_2026")
	if !found || len(form.QuestionRules) != 2 {
		t.Fatalf("question rules = %#v", form)
	}
	rule := form.QuestionRules[len(form.QuestionRules)-1]
	if rule.SourceFieldID != "format" || rule.TargetFieldID != "topics" || rule.Operator != "equals" || rule.Effect != "show" {
		t.Fatalf("generic question rule = %#v", rule)
	}
	audit := snapshot.AuditEvents
	if audit[len(audit)-1].Action != "form.question_rule_added" || audit[len(audit)-1].Rule != "form-visibility.arb" {
		t.Fatalf("question rule audit = %#v", audit[len(audit)-1])
	}
}
