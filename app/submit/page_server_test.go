package submit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/action"
)

func submissionTestState(t *testing.T) *store.JSONStore {
	t.Helper()
	workspace, err := store.Open(":memory:", domain.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	return workspace
}

func TestSaveDraftCreatesAuditedPrivateDraft(t *testing.T) {
	workspace := submissionTestState(t)
	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/saveDraft", nil),
		FormData: map[string]string{
			"form_id":        "form_cfp_2026",
			"email":          "draft@example.com",
			"first_name":     "Draft",
			"last_name":      "Author",
			"format":         "Talk",
			"workshop_needs": "A stale hidden value must not be stored.",
		},
	}
	if err := saveDraft(ctx); err != nil {
		t.Fatalf("saveDraft: %v", err)
	}

	snapshot := workspace.Snapshot()
	var draft domain.Submission
	for _, submission := range snapshot.Submissions {
		if submission.Title == "" && submission.Status == domain.SubmissionDraft {
			for _, speakerID := range submission.SpeakerIDs {
				speaker, found := snapshot.Speaker(speakerID)
				if found && speaker.Email == "draft@example.com" {
					draft = submission
				}
			}
		}
	}
	if draft.ID == "" || draft.Status != domain.SubmissionDraft {
		t.Fatalf("saved draft = %#v, want a new draft", draft)
	}
	if _, stored := draft.Answers["workshop_needs"]; stored {
		t.Fatalf("hidden conditional answer persisted in draft: %#v", draft.Answers)
	}
	if len(snapshot.AuditEvents) == 0 {
		t.Fatal("no audit event recorded for saved draft")
	}
	audit := snapshot.AuditEvents[len(snapshot.AuditEvents)-1]
	if audit.Action != "submission.draft_saved" || audit.EntityID != draft.ID {
		t.Fatalf("draft audit = %#v, want entity %s", audit, draft.ID)
	}
}

func TestSubmittingDraftPromotesTheSameAuditedRecord(t *testing.T) {
	workspace := submissionTestState(t)
	draftContext := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/saveDraft", nil),
		FormData: map[string]string{
			"form_id":    "form_cfp_2026",
			"email":      "promote@example.com",
			"first_name": "Promote",
			"last_name":  "Author",
			"title":      "A durable draft",
			"format":     "Workshop",
		},
	}
	if err := saveDraft(draftContext); err != nil {
		t.Fatalf("saveDraft: %v", err)
	}

	snapshot := workspace.Snapshot()
	var draft domain.Submission
	var speaker domain.Speaker
	for _, candidate := range snapshot.Submissions {
		if candidate.Status != domain.SubmissionDraft {
			continue
		}
		for _, speakerID := range candidate.SpeakerIDs {
			if found, ok := snapshot.Speaker(speakerID); ok && found.Email == "promote@example.com" {
				draft = candidate
				speaker = *found
			}
		}
	}
	if draft.ID == "" || speaker.ID == "" {
		t.Fatalf("draft/speaker not found: %#v / %#v", draft, speaker)
	}

	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil),
		FormData: map[string]string{
			"form_id":        "form_cfp_2026",
			"draft_id":       draft.ID,
			"draft_key":      token.New().Sign(speaker.ID),
			"title":          "A durable draft",
			"abstract":       "A complete proposal that promotes the saved record into review.",
			"format":         "Workshop",
			"category":       "agents",
			"level":          "Intermediate",
			"topics":         "Drafts\nAudits\nReview",
			"workshop_needs": "A round table and two microphones.",
			"first_name":     "Promote",
			"last_name":      "Author",
			"email":          "promote@example.com",
		},
	}
	if err := submitProposal(ctx); err != nil {
		t.Fatalf("submitProposal: %v", err)
	}

	final := workspace.Snapshot()
	submission, found := final.Submission(draft.ID)
	if !found || submission.Status != domain.SubmissionPending {
		t.Fatalf("promoted submission = %#v, want pending record %s", submission, draft.ID)
	}
	if submission.Answers["workshop_needs"] == "" {
		t.Fatalf("visible conditional answer missing after promotion: %#v", submission.Answers)
	}
	audit := final.AuditEvents[len(final.AuditEvents)-2]
	// A confirmation communication audit follows the submission audit.
	if audit.Action != "submission.draft_submitted" || audit.EntityID != draft.ID {
		t.Fatalf("promotion audit = %#v, want draft entity %s", audit, draft.ID)
	}
}
