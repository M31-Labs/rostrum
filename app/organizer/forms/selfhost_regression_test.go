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

func TestUpdateFormDetailsPersistsPublicCopyAndHandoffFlags(t *testing.T) {
	state := domain.FreshState(time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC))
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open fresh workspace: %v", err)
	}
	appstate.Set(workspace)

	initial := workspace.Snapshot()
	before, found := initial.Form("form_cfp")
	if !found || !before.SendConfirmation || !before.RedirectToPortal {
		t.Fatalf("fresh form handoff flags = %#v, want both enabled before update", before)
	}

	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/forms/__actions/updateFormDetails", nil),
		FormData: map[string]string{
			"form_id":         "form_cfp",
			"name":            "Community proposals",
			"title":           "Share a session with our community",
			"welcome_heading": "Bring us one useful idea",
			"welcome_body":    "Tell the program team what attendees will learn and how you will help them practice it.",
			"success_heading": "Your proposal is safely with us",
			"success_body":    "We will review it carefully and follow up with clear next steps.",
			// Omitted checkboxes are the browser's representation of false.
		},
	}
	if err := updateFormDetails(ctx); err != nil {
		t.Fatalf("updateFormDetails: %v", err)
	}

	snapshot := workspace.Snapshot()
	form, found := snapshot.Form("form_cfp")
	if !found {
		t.Fatal("updated form is missing")
	}
	if form.Name != "Community proposals" || form.ExternalTitle != "Share a session with our community" {
		t.Fatalf("updated form identity = %#v", form)
	}
	if form.WelcomeHeading != "Bring us one useful idea" || form.WelcomeBody != "Tell the program team what attendees will learn and how you will help them practice it." {
		t.Fatalf("updated welcome copy = %#v", form)
	}
	if form.SuccessHeading != "Your proposal is safely with us" || form.SuccessBody != "We will review it carefully and follow up with clear next steps." {
		t.Fatalf("updated success copy = %#v", form)
	}
	if form.SendConfirmation || form.RedirectToPortal {
		t.Fatalf("updated handoff flags = confirmation:%t redirect:%t, want both disabled", form.SendConfirmation, form.RedirectToPortal)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("updated workspace validation: %v", err)
	}
	audit := snapshot.AuditEvents[len(snapshot.AuditEvents)-1]
	if audit.Action != "form.details_updated" || audit.EntityID != form.ID {
		t.Fatalf("details audit = %#v, want form.details_updated for %s", audit, form.ID)
	}
}
