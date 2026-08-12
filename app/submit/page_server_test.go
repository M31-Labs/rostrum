package submit

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/ratelimit"
	"github.com/m31-labs/rostrum/internal/store"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
)

func submissionTestState(t *testing.T) *store.JSONStore {
	t.Helper()
	// Package-level limiters intentionally live for the process lifetime in the
	// application. Reset them between these serial unit tests so one scenario's
	// intake budget cannot make a later scenario order-dependent.
	submissionLimiter = ratelimit.NewCounter(5)
	submissionIPLimiter = ratelimit.NewTokenBucket(10, time.Hour)
	draftCreationLimiter = ratelimit.NewCounter(draftCreationSessionLimit)
	draftCreationIPLimiter = ratelimit.NewTokenBucket(draftCreationIPLimit, time.Hour)
	workspace, err := store.Open(":memory:", domain.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	return workspace
}

type mutateAfterSnapshotStore struct {
	store.StateStore
	mutate  func(*domain.State) error
	mutated bool
}

func (store *mutateAfterSnapshotStore) Snapshot() domain.State {
	snapshot := store.StateStore.Snapshot()
	if !store.mutated {
		store.mutated = true
		if err := store.StateStore.Update(store.mutate); err != nil {
			panic(err)
		}
	}
	return snapshot
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

func TestSaveDraftDoesNotClaimExistingSpeakerByEmail(t *testing.T) {
	workspace := submissionTestState(t)
	before := workspace.Snapshot()
	victim, found := before.Speaker("spk_maya")
	if !found {
		t.Fatal("seed speaker spk_maya not found")
	}
	victimBefore := *victim

	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/saveDraft", nil),
		FormData: map[string]string{
			"form_id":    "form_cfp_2026",
			"email":      victim.Email,
			"first_name": "Unverified",
			"last_name":  "Claimant",
			"format":     "Talk",
			"title":      "An isolated draft",
		},
	}
	if err := saveDraft(ctx); err != nil {
		t.Fatalf("saveDraft: %v", err)
	}

	after := workspace.Snapshot()
	victimAfter, found := after.Speaker(victimBefore.ID)
	if !found || !reflect.DeepEqual(*victimAfter, victimBefore) {
		t.Fatalf("existing speaker changed from unverified email claim:\nbefore=%#v\nafter=%#v", victimBefore, victimAfter)
	}
	if got, want := len(after.Speakers), len(before.Speakers)+1; got != want {
		t.Fatalf("speaker count = %d, want %d isolated identities", got, want)
	}
	if got, want := len(after.Submissions), len(before.Submissions)+1; got != want {
		t.Fatalf("submission count = %d, want %d", got, want)
	}
	draft := after.Submissions[len(after.Submissions)-1]
	if len(draft.SpeakerIDs) != 1 || draft.SpeakerIDs[0] == victimBefore.ID {
		t.Fatalf("draft speakers = %#v, must not claim existing speaker %s", draft.SpeakerIDs, victimBefore.ID)
	}
	isolatedID := draft.SpeakerIDs[0]
	if portal := thanksPortalURL(victimBefore.ID, token.New().Sign(isolatedID)); portal != "" {
		t.Fatalf("isolated draft key authorized existing speaker portal: %s", portal)
	}
}

func TestSaveDraftBoundsRepeatedUnkeyedCreationAndAllowsOwnedUpdates(t *testing.T) {
	workspace := submissionTestState(t)
	before := workspace.Snapshot()
	const email = "bounded-drafts@example.com"

	for attempt := 0; attempt < draftCreationSessionLimit; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/saveDraft", nil)
		request.RemoteAddr = "203.0.113.42:443"
		err := saveDraft(&action.Context{
			Request: request,
			FormData: map[string]string{
				"form_id":    "form_cfp_2026",
				"email":      email,
				"first_name": "Bounded",
				"last_name":  "Author",
				"format":     "Talk",
				"title":      "Isolated draft",
			},
		})
		if err != nil {
			t.Fatalf("unkeyed save %d of %d: %v", attempt+1, draftCreationSessionLimit, err)
		}
	}

	rejectedRequest := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/saveDraft", nil)
	rejectedRequest.RemoteAddr = "203.0.113.42:443"
	rejected := saveDraft(&action.Context{
		Request: rejectedRequest,
		FormData: map[string]string{
			"form_id": "form_cfp_2026",
			"email":   email,
			"format":  "Talk",
			"title":   "One draft too many",
		},
	})
	var result *action.ResultError
	if !errors.As(rejected, &result) {
		t.Fatalf("save beyond creation cap = %v, want structured rate-limit validation", rejected)
	}
	if message := result.Result.FieldErrors["form"]; !strings.Contains(message, "new-draft limit") {
		t.Fatalf("rate-limit field error = %q, want new-draft explanation", message)
	}

	afterLimit := workspace.Snapshot()
	if got, want := len(afterLimit.Speakers), len(before.Speakers)+draftCreationSessionLimit; got != want {
		t.Fatalf("speaker count after repeated unkeyed saves = %d, want bounded count %d", got, want)
	}
	if got, want := len(afterLimit.Submissions), len(before.Submissions)+draftCreationSessionLimit; got != want {
		t.Fatalf("submission count after repeated unkeyed saves = %d, want bounded count %d", got, want)
	}
	if got, want := len(afterLimit.AuditEvents), len(before.AuditEvents)+draftCreationSessionLimit; got != want {
		t.Fatalf("audit count after rejected save = %d, want only %d committed draft events", got, want)
	}

	ownedDraft := afterLimit.Submissions[len(afterLimit.Submissions)-1]
	if len(ownedDraft.SpeakerIDs) != 1 {
		t.Fatalf("owned draft speaker IDs = %#v, want exactly one", ownedDraft.SpeakerIDs)
	}
	ownedSpeakerID := ownedDraft.SpeakerIDs[0]
	ownedUpdateRequest := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/saveDraft", nil)
	ownedUpdateRequest.RemoteAddr = "203.0.113.42:443"
	if err := saveDraft(&action.Context{
		Request: ownedUpdateRequest,
		FormData: map[string]string{
			"form_id":   "form_cfp_2026",
			"draft_id":  ownedDraft.ID,
			"draft_key": token.New().Sign(ownedSpeakerID),
			"email":     email,
			"format":    "Talk",
			"title":     "Updated through the owned draft link",
		},
	}); err != nil {
		t.Fatalf("signed update after exhausting creation budget: %v", err)
	}

	afterUpdate := workspace.Snapshot()
	if got, want := len(afterUpdate.Speakers), len(afterLimit.Speakers); got != want {
		t.Fatalf("signed update grew speaker count to %d, want %d", got, want)
	}
	if got, want := len(afterUpdate.Submissions), len(afterLimit.Submissions); got != want {
		t.Fatalf("signed update grew submission count to %d, want %d", got, want)
	}
	updatedDraft, found := afterUpdate.Submission(ownedDraft.ID)
	if !found || updatedDraft.Title != "Updated through the owned draft link" {
		t.Fatalf("signed draft update = %#v, want the same record with updated title", updatedDraft)
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

func TestSubmitProposalDoesNotClaimExistingSpeakerByEmail(t *testing.T) {
	workspace := submissionTestState(t)
	before := workspace.Snapshot()
	victim, found := before.Speaker("spk_maya")
	if !found {
		t.Fatal("seed speaker spk_maya not found")
	}
	victimBefore := *victim

	ctx := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil),
		FormData: map[string]string{
			"form_id":    "form_cfp_2026",
			"title":      "An isolated proposal",
			"abstract":   "A complete proposal submitted with an email address that belongs to an existing speaker.",
			"format":     "Talk",
			"category":   "agents",
			"level":      "Intermediate",
			"topics":     "Identity\nVerification\nIsolation",
			"first_name": "Unverified",
			"last_name":  "Claimant",
			"email":      victim.Email,
		},
	}
	if err := submitProposal(ctx); err != nil {
		t.Fatalf("submitProposal: %v", err)
	}

	after := workspace.Snapshot()
	victimAfter, found := after.Speaker(victimBefore.ID)
	if !found || !reflect.DeepEqual(*victimAfter, victimBefore) {
		t.Fatalf("existing speaker changed from unverified email claim:\nbefore=%#v\nafter=%#v", victimBefore, victimAfter)
	}
	if got, want := len(after.Speakers), len(before.Speakers)+1; got != want {
		t.Fatalf("speaker count = %d, want %d isolated identities", got, want)
	}
	proposal := after.Submissions[len(after.Submissions)-1]
	if len(proposal.SpeakerIDs) != 1 || proposal.SpeakerIDs[0] == victimBefore.ID {
		t.Fatalf("proposal speakers = %#v, must not claim existing speaker %s", proposal.SpeakerIDs, victimBefore.ID)
	}
	isolatedID := proposal.SpeakerIDs[0]
	if portal := thanksPortalURL(victimBefore.ID, token.New().Sign(isolatedID)); portal != "" {
		t.Fatalf("isolated proposal key authorized existing speaker portal: %s", portal)
	}
}

func TestThanksRequiresKeyBoundToSpeaker(t *testing.T) {
	submissionTestState(t)
	validKey := token.New().Sign("spk_maya")

	tests := []struct {
		name       string
		query      string
		wantPortal bool
		wantTitle  bool
	}{
		{name: "no key is generic", query: "?speaker=spk_maya"},
		{name: "cross-speaker key is generic", query: "?speaker=spk_maya&key=" + token.New().Sign("spk_theo")},
		{name: "matching key reveals owned receipt", query: "?speaker=spk_maya&key=" + validKey, wantPortal: true, wantTitle: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := &route.RouteContext{
				Request: httptest.NewRequest(http.MethodGet, "/submit/systems-forum-cfp/thanks"+test.query, nil),
				Params:  map[string]string{"slug": "systems-forum-cfp"},
			}
			loaded, err := loadThanks(ctx, route.FilePage{})
			if err != nil {
				t.Fatalf("loadThanks: %v", err)
			}
			data := loaded.(map[string]any)
			if got := data["hasPortal"].(bool); got != test.wantPortal {
				t.Fatalf("hasPortal = %t, want %t", got, test.wantPortal)
			}
			if got := data["portalURL"].(string); (got != "") != test.wantPortal {
				t.Fatalf("portalURL = %q, want portal=%t", got, test.wantPortal)
			}
			submission := data["submission"].(map[string]any)
			if got := submission["title"].(string); (got != "") != test.wantTitle {
				t.Fatalf("submission title = %q, want visible=%t", got, test.wantTitle)
			}
		})
	}
}

func TestSubmitProposalRevalidatesTheCurrentFormSchema(t *testing.T) {
	now := time.Now().UTC()
	state := domain.Seed(now)
	state.Forms[0].CloseAt = now.Add(time.Hour)
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	before := len(workspace.Snapshot().Submissions)
	appstate.Set(&mutateAfterSnapshotStore{
		StateStore: workspace,
		mutate: func(current *domain.State) error {
			form, found := current.Form("form_cfp_2026")
			if !found {
				return errors.New("form disappeared")
			}
			for index := range form.Fields {
				if form.Fields[index].ID == "topics" {
					form.Fields[index].MaxLength = 8
					return nil
				}
			}
			return errors.New("topics field disappeared")
		},
	})

	err = submitProposal(&action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil),
		FormData: map[string]string{
			"form_id":    "form_cfp_2026",
			"title":      "A current schema must win",
			"abstract":   "This proposal is valid under the first form snapshot but not the current form schema.",
			"format":     "Talk",
			"category":   "agents",
			"level":      "Intermediate",
			"topics":     "A durable set of useful takeaways.",
			"first_name": "Current",
			"last_name":  "Schema",
			"email":      "current-schema@example.com",
		},
	})
	var result *action.ResultError
	if !errors.As(err, &result) {
		t.Fatalf("submitProposal error = %v, want structured validation failure", err)
	}
	if result.Result.FieldErrors["topics"] == "" {
		t.Fatalf("field errors = %#v, want current topics constraint", result.Result.FieldErrors)
	}
	if after := len(workspace.Snapshot().Submissions); after != before {
		t.Fatalf("submissions after stale-schema rejection = %d, want %d", after, before)
	}
}
