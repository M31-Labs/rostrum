package settings

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

func TestOrganizerCanUpdateAndRetireUnusedCategory(t *testing.T) {
	state := domain.FreshState(time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC))
	state.Event.Tracks = []domain.Track{{ID: "track-platform", Name: "Platform", Color: "teal"}}
	state.Event.Categories = append(state.Event.Categories, domain.Category{ID: "unused", Name: "Unused"})
	workspace := openCategoryWorkspace(t, state)

	if err := updateCategory(categoryActionContext("updateCategory", map[string]string{
		"category_id": "general",
		"name":        "Platform engineering",
		"owner_name":  "Alex Rivera",
		"owner_email": " ALEX@EXAMPLE.COM ",
		"track_id":    "track-platform",
	})); err != nil {
		t.Fatalf("updateCategory: %v", err)
	}
	if err := retireCategory(categoryActionContext("retireCategory", map[string]string{"category_id": "unused"})); err != nil {
		t.Fatalf("retireCategory: %v", err)
	}

	snapshot := workspace.Snapshot()
	category, found := snapshot.Category("general")
	if !found || category.Name != "Platform engineering" || category.OwnerName != "Alex Rivera" || category.OwnerEmail != "alex@example.com" || category.TrackID != "track-platform" {
		t.Fatalf("updated category = %#v", category)
	}
	if _, found := snapshot.Category("unused"); found || len(snapshot.Event.Categories) != 1 {
		t.Fatalf("categories after retirement = %#v, want only the updated category", snapshot.Event.Categories)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("workspace after category changes: %v", err)
	}
	if got := snapshot.AuditEvents[len(snapshot.AuditEvents)-2].Action; got != "event.category_updated" {
		t.Fatalf("update audit action = %q", got)
	}
	if got := snapshot.AuditEvents[len(snapshot.AuditEvents)-1].Action; got != "event.category_retired" {
		t.Fatalf("retire audit action = %q", got)
	}
}

func TestRetireCategoryProtectsProposalHistoryAndLastCategory(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	t.Run("category with proposal history", func(t *testing.T) {
		state := domain.FreshState(now)
		state.Event.Categories = append(state.Event.Categories, domain.Category{ID: "replacement", Name: "Replacement"})
		state.Speakers = append(state.Speakers, domain.Speaker{ID: "spk_history", FirstName: "History", LastName: "Keeper", Email: "history@example.com", CreatedAt: now, UpdatedAt: now})
		state.Submissions = append(state.Submissions, domain.Submission{
			ID: "sub_history", EventID: state.Event.ID, FormID: state.Forms[0].ID,
			Title: "A proposal with durable history", CategoryID: "general", SpeakerIDs: []string{"spk_history"},
			Status: domain.SubmissionPending, SubmittedAt: now, UpdatedAt: now,
		})
		workspace := openCategoryWorkspace(t, state)
		beforeAudit := len(workspace.Snapshot().AuditEvents)

		err := retireCategory(categoryActionContext("retireCategory", map[string]string{"category_id": "general"}))
		assertCategoryValidation(t, err, "category")
		snapshot := workspace.Snapshot()
		if _, found := snapshot.Category("general"); !found {
			t.Fatal("in-use category was retired")
		}
		if len(snapshot.AuditEvents) != beforeAudit {
			t.Fatalf("rejected retirement added an audit event: %#v", snapshot.AuditEvents)
		}
	})

	t.Run("last category", func(t *testing.T) {
		workspace := openCategoryWorkspace(t, domain.FreshState(now))
		beforeAudit := len(workspace.Snapshot().AuditEvents)

		err := retireCategory(categoryActionContext("retireCategory", map[string]string{"category_id": "general"}))
		assertCategoryValidation(t, err, "category")
		snapshot := workspace.Snapshot()
		if len(snapshot.Event.Categories) != 1 || snapshot.Event.Categories[0].ID != "general" {
			t.Fatalf("last category changed after rejected retirement: %#v", snapshot.Event.Categories)
		}
		if len(snapshot.AuditEvents) != beforeAudit {
			t.Fatalf("rejected retirement added an audit event: %#v", snapshot.AuditEvents)
		}
	})
}

func openCategoryWorkspace(t *testing.T, state domain.State) *store.JSONStore {
	t.Helper()
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open category workspace: %v", err)
	}
	appstate.Set(workspace)
	return workspace
}

func categoryActionContext(name string, formData map[string]string) *action.Context {
	return &action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/organizer/settings/__actions/"+name, nil),
		FormData: formData,
	}
}

func assertCategoryValidation(t *testing.T, err error, field string) {
	t.Helper()
	var result *action.ResultError
	if !errors.As(err, &result) {
		t.Fatalf("error = %v, want structured validation failure", err)
	}
	if result.Result.FieldErrors[field] == "" {
		t.Fatalf("field errors = %#v, want %s failure", result.Result.FieldErrors, field)
	}
}
