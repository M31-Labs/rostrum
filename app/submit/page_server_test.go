package submit

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/mail"
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
	testOutbox := mail.NewOutboxSender()
	confirmationSender = func() mail.Sender { return testOutbox }
	workspace, err := store.Open(":memory:", fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	return workspace
}

type submissionCaptureSender struct {
	messages []mail.Message
	err      error
	onSend   func(mail.Message)
}

func (sender *submissionCaptureSender) Send(message mail.Message) error {
	sender.messages = append(sender.messages, message)
	if sender.onSend != nil {
		sender.onSend(message)
	}
	return sender.err
}

func (sender *submissionCaptureSender) Name() string { return "test-transport" }

func validProposalData(email, title string) map[string]string {
	return map[string]string{
		"form_id":    "form_cfp_2026",
		"title":      title,
		"abstract":   "A complete proposal with enough detail for the program team to evaluate it.",
		"format":     "Talk",
		"category":   "agents",
		"level":      "Intermediate",
		"topics":     "Inspection\nDelivery\nOperations",
		"first_name": "Ada",
		"last_name":  "Lovelace",
		"email":      email,
	}
}

func confirmationCommunications(state domain.State) []domain.Communication {
	result := make([]domain.Communication, 0)
	for _, communication := range state.Communications {
		if communication.Trigger == "submission.confirmation" {
			result = append(result, communication)
		}
	}
	return result
}

func thanksDataForSpeaker(t *testing.T, slug, speakerID string) map[string]any {
	t.Helper()
	key := token.New().Sign(speakerID)
	loaded, err := loadThanks(&route.RouteContext{
		Request: httptest.NewRequest(http.MethodGet, "/submit/"+slug+"/thanks?speaker="+url.QueryEscape(speakerID)+"&key="+url.QueryEscape(key), nil),
		Params:  map[string]string{"slug": slug},
	}, route.FilePage{})
	if err != nil {
		t.Fatalf("loadThanks: %v", err)
	}
	return loaded.(map[string]any)
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
	state := fixture.Seed(now)
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

	request := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil)
	identity := ratelimit.RequestIdentity(request)
	err = submitProposal(&action.Context{
		Request: request,
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
	if count := submissionLimiter.Count(identity); count != 0 {
		t.Fatalf("stale-schema rejection consumed %d successful-submission slots", count)
	}
}

func TestSubmitProposalUsesConfiguredConfirmationTemplateAndPortalMerge(t *testing.T) {
	workspace := submissionTestState(t)
	const templateID = "tpl_custom_submission_receipt"
	if err := workspace.Update(func(state *domain.State) error {
		state.EmailTemplates = append(state.EmailTemplates, domain.EmailTemplate{
			ID: templateID, Name: "Custom receipt", Audience: "submitter",
			Subject: "Receipt: {{submission.title}} at {{event.name}}",
			Body:    "Dear {{speaker.name}},\n\nOpen your private workspace:\n{{speaker.portal_url}}",
		})
		form, found := state.Form("form_cfp_2026")
		if !found {
			return errors.New("test form not found")
		}
		form.SendConfirmation = true
		form.ConfirmationTemplate = templateID
		return nil
	}); err != nil {
		t.Fatalf("configure form: %v", err)
	}

	t.Setenv("PUBLIC_URL", "https://events.example")
	beforeSubmissions := len(workspace.Snapshot().Submissions)
	durableAtSend := false
	sender := &submissionCaptureSender{onSend: func(mail.Message) {
		durableAtSend = len(workspace.Snapshot().Submissions) == beforeSubmissions+1
	}}
	confirmationSender = func() mail.Sender { return sender }
	if err := submitProposal(&action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil),
		FormData: validProposalData("configured-template@example.com", "Inspectable delivery"),
	}); err != nil {
		t.Fatalf("submitProposal: %v", err)
	}

	if len(sender.messages) != 1 {
		t.Fatalf("delivery attempts = %d, want 1", len(sender.messages))
	}
	if !durableAtSend {
		t.Fatal("confirmation delivery ran before the proposal commit became visible")
	}
	message := sender.messages[0]
	if message.Subject != "Receipt: Inspectable delivery at M31 Systems Forum 2026" {
		t.Fatalf("subject = %q", message.Subject)
	}
	if !strings.HasPrefix(message.TextBody, "Dear Ada Lovelace,\n\nOpen your private workspace:\nhttps://events.example/portal/") {
		t.Fatalf("body did not render organizer copy and portal URL: %q", message.TextBody)
	}
	portalStart := strings.Index(message.TextBody, "https://")
	portal, err := url.Parse(strings.TrimSpace(message.TextBody[portalStart:]))
	if err != nil {
		t.Fatalf("parse rendered portal URL: %v", err)
	}
	segments := strings.Split(strings.Trim(portal.Path, "/"), "/")
	if len(segments) != 2 || segments[0] != "portal" {
		t.Fatalf("portal path = %q", portal.Path)
	}
	keySpeakerID, ok := token.New().Verify(portal.Query().Get("key"))
	if !ok || keySpeakerID != segments[1] {
		t.Fatalf("portal key subject = %q, ok=%t; want %q", keySpeakerID, ok, segments[1])
	}

	communications := confirmationCommunications(workspace.Snapshot())
	if len(communications) != 1 {
		t.Fatalf("confirmation rows = %d, want 1", len(communications))
	}
	communication := communications[0]
	if communication.TemplateID != templateID || communication.Subject != message.Subject || communication.Status != domain.CommunicationSent {
		t.Fatalf("confirmation row = %#v", communication)
	}
	if communication.Provider != "test-transport" || communication.RecipientEmail != "configured-template@example.com" || communication.SubmissionID == "" {
		t.Fatalf("confirmation delivery metadata = %#v", communication)
	}
	if communication.IdempotencyKey == "" || communication.IdempotencyKey != message.IdempotencyKey || communication.AttemptCount != 1 || communication.LastAttemptAt.IsZero() {
		t.Fatalf("confirmation attempt metadata = %#v / message=%#v", communication, message)
	}
	if status := thanksDataForSpeaker(t, "systems-forum-cfp", communication.SpeakerID)["confirmationStatus"].(string); status != domain.CommunicationSent {
		t.Fatalf("thanks confirmation status = %q, want sent", status)
	}
}

func TestSubmitProposalSkipsDisabledConfirmation(t *testing.T) {
	workspace := submissionTestState(t)
	if err := workspace.Update(func(state *domain.State) error {
		form, found := state.Form("form_cfp_2026")
		if !found {
			return errors.New("test form not found")
		}
		form.SendConfirmation = false
		return nil
	}); err != nil {
		t.Fatalf("disable confirmation: %v", err)
	}
	sender := &submissionCaptureSender{}
	confirmationSender = func() mail.Sender { return sender }

	if err := submitProposal(&action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil),
		FormData: validProposalData("no-confirmation@example.com", "No automatic receipt"),
	}); err != nil {
		t.Fatalf("submitProposal: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("disabled confirmation attempted %d deliveries", len(sender.messages))
	}
	if rows := confirmationCommunications(workspace.Snapshot()); len(rows) != 0 {
		t.Fatalf("disabled confirmation recorded rows: %#v", rows)
	}
	snapshot := workspace.Snapshot()
	created := snapshot.Submissions[len(snapshot.Submissions)-1]
	if status := thanksDataForSpeaker(t, "systems-forum-cfp", created.SpeakerIDs[0])["confirmationStatus"].(string); status != "disabled" {
		t.Fatalf("thanks confirmation status = %q, want disabled", status)
	}
}

func TestSubmitProposalRecordsFailedAttemptWithoutProviderError(t *testing.T) {
	workspace := submissionTestState(t)
	sender := &submissionCaptureSender{err: errors.New("provider secret and internal host must never persist")}
	confirmationSender = func() mail.Sender { return sender }

	if err := submitProposal(&action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil),
		FormData: validProposalData("failed-confirmation@example.com", "Durable before delivery"),
	}); err != nil {
		t.Fatalf("submitProposal: %v", err)
	}
	rows := confirmationCommunications(workspace.Snapshot())
	if len(rows) != 1 || rows[0].Status != domain.CommunicationFailed || rows[0].AttemptCount != 1 {
		t.Fatalf("failed confirmation rows = %#v", rows)
	}
	if rows[0].Error != "delivery_failed" || strings.Contains(rows[0].Error, "provider secret") {
		t.Fatalf("stored failure category = %q", rows[0].Error)
	}
	if status := thanksDataForSpeaker(t, "systems-forum-cfp", rows[0].SpeakerID)["confirmationStatus"].(string); status != domain.CommunicationFailed {
		t.Fatalf("thanks confirmation status = %q, want failed", status)
	}
	snapshot := workspace.Snapshot()
	if submission, found := snapshot.Submission(rows[0].SubmissionID); !found || submission.Status != domain.SubmissionPending {
		t.Fatalf("proposal was not durable before failed delivery: %#v", submission)
	}
}

func TestSubmitProposalRejectsUnsafeImportedConfirmationBeforeQuota(t *testing.T) {
	workspace := submissionTestState(t)
	if err := workspace.Update(func(state *domain.State) error {
		form, found := state.Form("form_cfp_2026")
		if !found {
			return errors.New("test form not found")
		}
		for index := range state.EmailTemplates {
			if state.EmailTemplates[index].ID == form.ConfirmationTemplate {
				state.EmailTemplates[index].Subject = "Receipt\nBcc: attacker@example.com"
				return nil
			}
		}
		return errors.New("confirmation template not found")
	}); err != nil {
		t.Fatalf("install unsafe imported template: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil)
	identity := ratelimit.RequestIdentity(request)
	before := len(workspace.Snapshot().Submissions)
	err := submitProposal(&action.Context{Request: request, FormData: validProposalData("unsafe-template@example.com", "Safe boundary")})
	if err == nil || !strings.Contains(err.Error(), "invalid confirmation template") {
		t.Fatalf("unsafe confirmation template error = %v", err)
	}
	if got := len(workspace.Snapshot().Submissions); got != before {
		t.Fatalf("unsafe template changed submission count to %d, want %d", got, before)
	}
	if count := submissionLimiter.Count(identity); count != 0 {
		t.Fatalf("unsafe template consumed %d successful-submission slots", count)
	}
}

func TestInvalidProposalDoesNotConsumeSuccessfulSubmissionBudget(t *testing.T) {
	workspace := submissionTestState(t)
	const remoteAddress = "203.0.113.77:443"
	identityRequest := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil)
	identityRequest.RemoteAddr = remoteAddress
	identity := ratelimit.RequestIdentity(identityRequest)

	invalid := validProposalData("quota@example.com", "Incomplete proposal")
	delete(invalid, "abstract")
	for attempt := 0; attempt < 7; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil)
		request.RemoteAddr = remoteAddress
		err := submitProposal(&action.Context{Request: request, FormData: invalid})
		var result *action.ResultError
		if !errors.As(err, &result) || result.Result.FieldErrors["abstract"] == "" {
			t.Fatalf("invalid attempt %d = %v, want abstract validation", attempt+1, err)
		}
	}
	if count := submissionLimiter.Count(identity); count != 0 {
		t.Fatalf("invalid proposals consumed %d successful-submission slots", count)
	}

	before := len(workspace.Snapshot().Submissions)
	for attempt := 0; attempt < 5; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil)
		request.RemoteAddr = remoteAddress
		data := validProposalData("quota@example.com", fmt.Sprintf("Valid proposal %d", attempt+1))
		if err := submitProposal(&action.Context{Request: request, FormData: data}); err != nil {
			t.Fatalf("valid attempt %d: %v", attempt+1, err)
		}
	}
	if count := submissionLimiter.Count(identity); count != 5 {
		t.Fatalf("successful submission count = %d, want 5", count)
	}
	if got := len(workspace.Snapshot().Submissions); got != before+5 {
		t.Fatalf("submission count = %d, want %d", got, before+5)
	}

	request := httptest.NewRequest(http.MethodPost, "/submit/systems-forum-cfp/__actions/submitProposal", nil)
	request.RemoteAddr = remoteAddress
	err := submitProposal(&action.Context{Request: request, FormData: validProposalData("quota@example.com", "One too many")})
	var result *action.ResultError
	if !errors.As(err, &result) || !strings.Contains(result.Result.FieldErrors["form"], "submission limit") {
		t.Fatalf("sixth valid submission = %v, want limit validation", err)
	}
}

func TestThanksReceiptRemainsKeyedWhenPortalRedirectIsDisabled(t *testing.T) {
	workspace := submissionTestState(t)
	if err := workspace.Update(func(state *domain.State) error {
		form, found := state.Form("form_cfp_2026")
		if !found {
			return errors.New("test form not found")
		}
		form.RedirectToPortal = false
		return nil
	}); err != nil {
		t.Fatalf("disable portal redirect: %v", err)
	}

	target, err := url.Parse(thanksRedirectURL("systems-forum-cfp", "spk_maya"))
	if err != nil {
		t.Fatalf("parse thanks redirect: %v", err)
	}
	if target.Path != "/submit/systems-forum-cfp/thanks" {
		t.Fatalf("thanks redirect path = %q", target.Path)
	}
	if subject, ok := token.New().Verify(target.Query().Get("key")); !ok || subject != "spk_maya" {
		t.Fatalf("thanks receipt key subject = %q, ok=%t", subject, ok)
	}
	ctx := &route.RouteContext{
		Request: httptest.NewRequest(http.MethodGet, target.String(), nil),
		Params:  map[string]string{"slug": "systems-forum-cfp"},
	}
	loaded, err := loadThanks(ctx, route.FilePage{})
	if err != nil {
		t.Fatalf("loadThanks: %v", err)
	}
	data := loaded.(map[string]any)
	if data["hasPortal"].(bool) || data["portalURL"].(string) != "" {
		t.Fatalf("portal redirect disabled but thanks exposed portal: %#v", data)
	}
	if title := data["submission"].(map[string]any)["title"].(string); title == "" {
		t.Fatal("keyed receipt did not retain its owned proposal title")
	}
}
