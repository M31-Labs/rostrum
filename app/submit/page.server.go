package submit

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	delivery "github.com/m31-labs/rostrum/internal/communications"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/mailtemplate"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/ratelimit"
	"github.com/m31-labs/rostrum/internal/token"
	decisionrules "github.com/m31-labs/rostrum/rules"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

// submissionLimiter caps the number of proposals one session may submit
// (SE-3b). GoSX establishes the session's CSRF token before this action
// runs, so a real browser submission always resolves to a stable per-session
// key; a client without cookies falls back to its IP address.
var submissionLimiter = ratelimit.NewCounter(5)

// submissionIPLimiter throttles submissions per IP address over a rolling
// hour, independent of session identity, so one address cannot bypass
// submissionLimiter by dropping cookies between requests (SE-3b).
var submissionIPLimiter = ratelimit.NewTokenBucket(10, time.Hour)

const (
	draftCreationSessionLimit = 5
	draftCreationIPLimit      = 10
)

// draftCreationLimiter caps new, unkeyed draft identities per form and request
// identity for the lifetime of this process. A signed draft update bypasses
// this guard because it updates an existing speaker and submission instead of
// growing the store.
var draftCreationLimiter = ratelimit.NewCounter(draftCreationSessionLimit)

// draftCreationIPLimiter independently smooths new draft identities per form
// and network address. It prevents a caller from evading the request-identity
// cap by discarding their session between otherwise anonymous saves.
var draftCreationIPLimiter = ratelimit.NewTokenBucket(draftCreationIPLimit, time.Hour)

// confirmationSender is the process-wide mail transport, resolved once on
// first use. It is deferred (not a plain package var) because mail.FromEnv
// reads SMTP_* from the environment, which main() loads from .env only after
// package-level initialization has already run.
var confirmationSender = sync.OnceValue(mail.FromEnv)

// publicBaseURL is the absolute base a confirmation email's portal link must
// use, because that link is followed from an inbox, outside the browser
// session. It mirrors main()'s PUBLIC_URL default.
func publicBaseURL() string {
	if base := strings.TrimSpace(os.Getenv("PUBLIC_URL")); base != "" {
		return base
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	return "http://localhost:" + port
}

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	slugDir := filepath.Join(filepath.Dir(thisFile), "[slug]")
	source := filepath.Join(slugDir, "page.gsx")
	if err := route.RegisterFileModule(route.FileModuleFor(source, route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			data, err := loadSubmissionForm(ctx)
			if err != nil {
				// An unknown or retired form slug is a routing miss, not a
				// server fault: render the branded 404 (app/not-found.gsx)
				// instead of the raw 500 a plain error would trigger.
				return nil, route.NotFound(err.Error())
			}
			return data, nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			eventName := "the event"
			if fields, ok := data.(map[string]any); ok {
				if event, ok := fields["event"].(map[string]any); ok {
					if name, ok := event["name"].(string); ok && name != "" {
						eventName = name
					}
				}
			}
			return server.Metadata{Title: server.Title{Default: "Call for speakers — Rostrum"}, Description: "Submit a proposal to " + eventName + "."}, nil
		},
		Actions: route.FileActions{
			"submitProposal": submitProposal,
			"saveDraft":      saveDraft,
		},
	})); err != nil {
		log.Fatal(err)
	}

	// FB-5: the success page submitProposal redirects to. It has no actions
	// of its own — this route is read-only, decorated only with the
	// <meta http-equiv="refresh"> head tag thanksMetadata attaches — so, like
	// app/public/page.server.go's nested routes, its module is registered
	// here from the parent package rather than from a page.server.go inside
	// the bracketed [slug] directory.
	thanksSource := filepath.Join(slugDir, "thanks", "page.gsx")
	if err := route.RegisterFileModule(route.FileModuleFor(thanksSource, route.FileModuleOptions{
		Load:     loadThanks,
		Metadata: thanksMetadata,
	})); err != nil {
		log.Fatal(err)
	}
}

func loadSubmissionForm(ctx *route.RouteContext) (map[string]any, error) {
	snapshot := appstate.MustGet().Snapshot()
	draft, speakerID, ok := accessibleDraft(snapshot, ctx.Param("slug"), ctx.Query("draft"), ctx.Query("key"))
	if ok {
		data, err := present.SubmissionFormWithValues(snapshot, ctx.Param("slug"), draftValues(snapshot, draft))
		if err != nil {
			return nil, err
		}
		data["draft"] = map[string]any{"active": true, "id": draft.ID, "key": ctx.Query("key"), "speakerID": speakerID}
		return data, nil
	}
	data, err := present.SubmissionForm(snapshot, ctx.Param("slug"))
	if err != nil {
		return nil, err
	}
	data["draft"] = map[string]any{"active": false, "id": "", "key": "", "speakerID": ""}
	return data, nil
}

// saveDraft preserves an incomplete proposal without sending mail, creating a
// review item, or consuming the final-submission rate limit. Its redirect is
// a managed GoSX navigation, so the prefilled draft view arrives without a
// document refresh and carries a signed, speaker-bound continuation key.
func saveDraft(ctx *action.Context) error {
	snapshot := appstate.MustGet().Snapshot()
	form, found := snapshot.Form(ctx.FormData["form_id"])
	if !found {
		return action.Error(404, "Submission form not found.")
	}
	if message, ok := rejectUnknownFields(ctx.FormData, form.Fields); !ok {
		return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
	}
	engine, err := decisionrules.Shared()
	if err != nil {
		return err
	}
	visibleFields, visibilityTrace, err := visibleSchemaFields(engine, form, ctx.FormData)
	if err != nil {
		return err
	}
	fieldErrors := validateDraftFields(form.Fields, snapshot, ctx.FormData, visibleFields)
	email := strings.ToLower(strings.TrimSpace(ctx.FormData["email"]))
	if !strings.Contains(email, "@") {
		fieldErrors["email"] = "Enter an email address so we can protect your draft."
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the highlighted fields before saving your draft.", fieldErrors, ctx.FormData)
	}

	draftID := strings.TrimSpace(ctx.FormData["draft_id"])
	if draftID == "" {
		// Only an unkeyed save creates a new isolated speaker and draft. Apply
		// both admission guards after schema validation but before allocating an
		// ID or entering UpdateAudit, so a rejected request cannot grow state or
		// its audit ledger. A valid signed draft update remains available even
		// after this browser has exhausted its creation budget.
		identity := ratelimit.RequestIdentity(ctx.Request)
		if !draftCreationLimiter.Allow(rateLimitFormKey(form.ID, identity)) {
			message := "You have reached the new-draft limit for this call. Continue from one of your saved draft links or try again later."
			return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
		}
		if ip := ratelimit.ClientIP(ctx.Request); ip != "" && !draftCreationIPLimiter.Allow(rateLimitFormKey(form.ID, ip)) {
			message := "Too many new drafts have been created from this network right now. Continue from a saved draft link or try again later."
			return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
		}
	}
	plannedDraftID := draftID
	if plannedDraftID == "" {
		// Allocate the identifier before UpdateAudit so the immutable audit
		// event always points at the exact draft this mutation creates.
		plannedDraftID = domain.NewID("sub")
	}
	draftSpeakerID := ""
	if draftID != "" {
		_, draftSpeakerID, found = accessibleDraft(snapshot, form.Slug, draftID, ctx.FormData["draft_key"])
		if !found {
			return action.Validation("Your saved draft link is no longer valid. Start a new draft or reopen your saved link.", map[string]string{"form": "Draft access could not be verified."}, ctx.FormData)
		}
		if speaker, speakerFound := snapshot.Speaker(draftSpeakerID); !speakerFound || !strings.EqualFold(speaker.Email, email) {
			return action.Validation("Use the email address that owns this saved draft.", map[string]string{"email": "This draft belongs to a different email address."}, ctx.FormData)
		}
	}

	speakerID := ""
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      "submitter:" + email,
		Action:     "submission.draft_saved",
		EntityType: "submission",
		EntityID:   plannedDraftID,
		Summary:    "Saved an incomplete public CFP proposal draft.",
		Origin:     "public-submission",
		Rule:       "form-visibility.arb",
		Trace:      strings.Join(visibilityTrace, " | "),
	}, func(state *domain.State) error {
		currentForm, currentFound := state.Form(form.ID)
		if !currentFound {
			return action.Error(404, "Submission form not found.")
		}
		if currentForm.Status != "open" || time.Now().After(currentForm.CloseAt) {
			return action.Validation("This call for speakers is closed.", map[string]string{"form": "The submission deadline has passed."}, ctx.FormData)
		}
		now := time.Now().UTC()
		if draftID != "" {
			draft, draftFound := state.Submission(draftID)
			if !draftFound || draft.Status != domain.SubmissionDraft || !contains(draft.SpeakerIDs, draftSpeakerID) {
				return action.Validation("This draft is no longer available.", map[string]string{"form": "Please start a new draft."}, ctx.FormData)
			}
			speaker, speakerFound := state.Speaker(draftSpeakerID)
			if !speakerFound || !strings.EqualFold(speaker.Email, email) {
				return action.Validation("Draft ownership changed unexpectedly.", map[string]string{"form": "Please reopen your saved draft link."}, ctx.FormData)
			}
			applyDraftSpeakerFields(speaker, ctx.FormData, now)
			speakerID = speaker.ID
			applyDraftSubmission(draft, state.Event.ID, currentForm.ID, speakerID, ctx.FormData, visibilityTrace, now, currentForm.Fields, visibleFields)
			return nil
		}

		// An email address entered on the public form is contact data, not proof
		// that this browser owns an existing speaker. Always isolate a new draft
		// behind a new speaker identity; only the verified draft-key branch above
		// may update an existing speaker. This prevents a known email address from
		// becoming a speaker-wide portal credential.
		speakerID = domain.NewID("spk")
		speaker := domain.Speaker{ID: speakerID, Email: email, CreatedAt: now, UpdatedAt: now}
		applyDraftSpeakerFields(&speaker, ctx.FormData, now)
		state.Speakers = append(state.Speakers, speaker)
		if draftCount(state.Submissions, currentForm.ID, speakerID) >= maxDrafts(currentForm) {
			return action.Validation("You have reached this form's saved-draft limit.", map[string]string{"form": "Submit or withdraw one of your existing drafts before creating another."}, ctx.FormData)
		}
		newDraft := domain.Submission{ID: plannedDraftID}
		applyDraftSubmission(&newDraft, state.Event.ID, currentForm.ID, speakerID, ctx.FormData, visibilityTrace, now, currentForm.Fields, visibleFields)
		draftID = newDraft.ID
		state.Submissions = append(state.Submissions, newDraft)
		return nil
	}); err != nil {
		return err
	}

	key := token.New().Sign(speakerID)
	session.AddFlash(ctx.Request, "notice", "Draft saved. Keep this link private; it is how you return to your proposal.")
	live.Broadcast("submission:draft_saved", map[string]string{"submission": draftID, "speaker": speakerID})
	actionflow.Redirect(ctx, draftURL(form.Slug, draftID, key))
	return nil
}

// rateLimitFormKey scopes a public-intake identity to one CFP without turning
// an unavailable request identity into a shared non-empty key. Real inbound
// requests always have a remote address; preserving the empty value keeps the
// ratelimit package's documented fallback behavior intact in direct tests.
func rateLimitFormKey(formID, identity string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ""
	}
	return strings.TrimSpace(formID) + ":" + identity
}

func submitProposal(ctx *action.Context) error {
	snapshot := appstate.MustGet().Snapshot()
	engine, err := decisionrules.Shared()
	if err != nil {
		return err
	}
	validated, err := validateSubmittedProposal(snapshot, ctx.FormData["form_id"], ctx.FormData, engine, time.Now().UTC())
	if err != nil {
		return err
	}
	form := validated.Form
	draftID := strings.TrimSpace(ctx.FormData["draft_id"])
	plannedSubmissionID := draftID
	if plannedSubmissionID == "" {
		// As with drafts, choose the ID outside the transaction so the audit
		// trail cannot contain a blank entity reference for a new proposal.
		plannedSubmissionID = domain.NewID("sub")
	}
	draftSpeakerID := ""
	found := false
	if draftID != "" {
		_, draftSpeakerID, found = accessibleDraft(snapshot, form.Slug, draftID, ctx.FormData["draft_key"])
		if !found {
			return action.Validation("Your saved draft link is no longer valid. Start a new draft or reopen your saved link.", map[string]string{"form": "Draft access could not be verified."}, ctx.FormData)
		}
		if speaker, speakerFound := snapshot.Speaker(draftSpeakerID); !speakerFound || !strings.EqualFold(speaker.Email, strings.TrimSpace(ctx.FormData["email"])) {
			return action.Validation("Use the email address that owns this saved draft.", map[string]string{"email": "This draft belongs to a different email address."}, ctx.FormData)
		}
	}

	requestIdentity := ratelimit.RequestIdentity(ctx.Request)
	clientIP := ratelimit.ClientIP(ctx.Request)
	speakerID := ""
	submissionID := ""
	submissionQueue := ""
	var submittedSpeaker domain.Speaker
	var submittedRecord domain.Submission
	var confirmationTemplate domain.EmailTemplate
	auditAction := "submission.created"
	auditSummary := "Submitted a proposal through the public CFP."
	if draftID != "" {
		auditAction = "submission.draft_submitted"
		auditSummary = "Submitted a previously saved proposal draft."
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      "submitter:" + strings.ToLower(strings.TrimSpace(ctx.FormData["email"])),
		Action:     auditAction,
		EntityType: "submission",
		EntityID:   plannedSubmissionID,
		Summary:    auditSummary,
		Origin:     "public-submission",
		Rule:       validated.Decision.Rule,
		Trace:      strings.Join(append(append([]string(nil), validated.Decision.Trace...), validated.VisibilityTrace...), " | "),
	}, func(state *domain.State) error {
		current, err := validateSubmittedProposal(*state, ctx.FormData["form_id"], ctx.FormData, engine, time.Now().UTC())
		if err != nil {
			return err
		}
		form = current.Form
		submissionQueue = current.Decision.Queue
		now := time.Now().UTC()
		email := strings.ToLower(strings.TrimSpace(ctx.FormData["email"]))
		if draftID != "" {
			// Recheck ownership and draft state against the same current clone the
			// schema validator just interpreted, before spending admission budget.
			speaker, speakerFound := state.Speaker(draftSpeakerID)
			draft, draftFound := state.Submission(draftID)
			if !speakerFound || !strings.EqualFold(speaker.Email, email) || !draftFound || draft.Status != domain.SubmissionDraft || !contains(draft.SpeakerIDs, draftSpeakerID) {
				return action.Validation("This draft is no longer available.", map[string]string{"form": "Please reopen your saved draft link."}, ctx.FormData)
			}
		}
		if form.SendConfirmation {
			var templateFound bool
			confirmationTemplate, templateFound = submissionEmailTemplate(*state, form.ConfirmationTemplate)
			if !templateFound {
				return fmt.Errorf("submission form %s references missing confirmation template %s", form.ID, form.ConfirmationTemplate)
			}
			if err := mailtemplate.Validate(confirmationTemplate.Name, confirmationTemplate.Audience, confirmationTemplate.Subject, confirmationTemplate.Body, confirmationTemplate.ReplyTo); err != nil {
				return fmt.Errorf("submission form %s has an invalid confirmation template: %w", form.ID, err)
			}
		}
		// Admission budgets are spent only after the current transactional
		// schema, conditional visibility, routing, and draft ownership checks
		// all pass. A typo or concurrently edited field therefore cannot consume
		// one of a speaker's five successful-submission slots.
		if !submissionLimiter.Allow(requestIdentity) {
			message := "You have reached the submission limit for this session. Contact the program team if you need to submit another proposal."
			return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
		}
		if clientIP != "" && !submissionIPLimiter.Allow(clientIP) {
			message := "Too many submissions from this network right now. Please try again in a little while."
			return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
		}
		if draftID != "" {
			// accessibleDraft verified a signed key for draftSpeakerID before this
			// transaction. That possession proof, rather than the posted email, is
			// what authorizes updating the draft's existing speaker identity.
			speaker, found := state.Speaker(draftSpeakerID)
			if !found || !strings.EqualFold(speaker.Email, email) {
				return action.Validation("Draft ownership changed unexpectedly.", map[string]string{"form": "Please reopen your saved draft link."}, ctx.FormData)
			}
			speakerID = speaker.ID
			applySubmittedSpeakerFields(speaker, ctx.FormData, now)
		} else {
			// A direct public submission has supplied no ownership proof. Even when
			// its email matches a known speaker, create an isolated identity so the
			// browser-visible portal key can never authorize the known speaker's
			// profile, tasks, files, or earlier proposals.
			speakerID = domain.NewID("spk")
			speaker := domain.Speaker{ID: speakerID, Email: email, CreatedAt: now, UpdatedAt: now}
			applySubmittedSpeakerFields(&speaker, ctx.FormData, now)
			state.Speakers = append(state.Speakers, speaker)
		}
		if draftID != "" && speakerID != draftSpeakerID {
			return action.Validation("Draft ownership changed unexpectedly.", map[string]string{"form": "Please reopen your saved draft link."}, ctx.FormData)
		}
		if draftID != "" {
			draft, draftFound := state.Submission(draftID)
			if !draftFound || draft.Status != domain.SubmissionDraft || !contains(draft.SpeakerIDs, speakerID) {
				return action.Validation("This draft is no longer available.", map[string]string{"form": "Please start a new draft."}, ctx.FormData)
			}
			submissionID = draft.ID
			applySubmittedSubmission(draft, state.Event.ID, form.ID, speakerID, ctx.FormData, current.Decision, current.VisibilityTrace, now, form.Fields, current.VisibleFields)
		} else {
			submissionID = plannedSubmissionID
			submission := domain.Submission{ID: submissionID}
			applySubmittedSubmission(&submission, state.Event.ID, form.ID, speakerID, ctx.FormData, current.Decision, current.VisibilityTrace, now, form.Fields, current.VisibleFields)
			state.Submissions = append(state.Submissions, submission)
		}
		storedSpeaker, speakerFound := state.Speaker(speakerID)
		storedSubmission, submissionFound := state.Submission(submissionID)
		if !speakerFound || !submissionFound {
			return fmt.Errorf("submitted speaker or proposal disappeared before commit")
		}
		submittedSpeaker = *storedSpeaker
		submittedRecord = *storedSubmission
		// Administrator notification rules enqueue only stable IDs while this
		// same audited submission transaction commits. A later outbox runner
		// resolves current merge data and performs the external delivery.
		delivery.EnqueueNotificationRules(state, delivery.Trigger{
			Name: "submission.created", SubmissionID: submissionID, SpeakerID: speakerID,
		}, now)
		for index := range state.Tasks {
			if state.Tasks[index].ID == "task_profile" && state.Tasks[index].Active() && !state.Tasks[index].AcceptedOnly && !contains(state.Tasks[index].AssignedSpeakerIDs, speakerID) {
				state.Tasks[index].AssignedSpeakerIDs = append(state.Tasks[index].AssignedSpeakerIDs, speakerID)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// A form may deliberately disable confirmation delivery. When enabled,
	// render the organizer-selected template from committed records and send
	// outside the store lock. Only an actual attempt earns a Communication row.
	confirmationAttempted := false
	confirmationDelivered := false
	if form.SendConfirmation {
		confirmationAttempted = true
		confirmationID := domain.NewID("comm")
		portalKey := token.New().Sign(speakerID)
		portalURL := mail.PortalURL(publicBaseURL(), speakerID, portalKey)
		subject, body := present.RenderCommunicationContextWithPortalURL(
			appstate.MustGet().Snapshot(), confirmationTemplate, submittedSpeaker,
			domain.Session{}, submittedRecord, domain.Task{}, portalURL,
		)
		sender := confirmationSender()
		attemptedAt := time.Now().UTC()
		provider := "custom"
		var sendErr error
		if sender == nil {
			sendErr = fmt.Errorf("confirmation sender is unavailable")
		} else {
			if named, ok := sender.(mail.Named); ok && strings.TrimSpace(named.Name()) != "" {
				provider = strings.TrimSpace(named.Name())
			}
			sendErr = sender.Send(mail.Message{
				To: submittedSpeaker.Email, ToName: submittedSpeaker.Name(),
				Subject: subject, TextBody: body, IdempotencyKey: confirmationID,
			})
		}
		status := domain.CommunicationSent
		failureCategory := ""
		if sendErr != nil {
			status = domain.CommunicationFailed
			failureCategory = "delivery_failed"
			log.Printf("submit: confirmation delivery for speaker %s failed: %v", speakerID, sendErr)
		} else {
			confirmationDelivered = true
		}
		if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
			Actor:      "system:mail",
			Action:     "communication.recorded",
			EntityType: "communication",
			EntityID:   confirmationID,
			Summary:    "Recorded the public-submission confirmation delivery outcome.",
			Origin:     "public-submission",
		}, func(state *domain.State) error {
			state.Communications = append(state.Communications, domain.Communication{
				ID: confirmationID, TemplateID: confirmationTemplate.ID,
				SpeakerID: speakerID, SubmissionID: submissionID,
				RecipientEmail: submittedSpeaker.Email, RecipientName: submittedSpeaker.Name(),
				Subject: subject, Status: status, Provider: provider,
				DeliveryMode: domain.DeliveryAutomatic, Trigger: "submission.confirmation",
				IdempotencyKey: confirmationID, LastAttemptAt: attemptedAt,
				SentAt: attemptedAt, CreatedAt: attemptedAt,
				AttemptCount: 1, MaxAttempts: 1, Error: failureCategory,
			})
			return nil
		}); err != nil {
			// The proposal is already durable and delivery has already been
			// attempted. Do not turn a bookkeeping failure into a retry prompt
			// that could create a duplicate proposal or message.
			log.Printf("submit: could not record confirmation attempt %s: %v", confirmationID, err)
		}
		live.Broadcast("communication:queued", map[string]string{"speaker": speakerID, "provider": provider})
	}

	notice := "Proposal received."
	if confirmationAttempted && confirmationDelivered {
		notice += " Your confirmation was handed to the configured mail service."
	} else if confirmationAttempted {
		notice += " The confirmation could not be delivered just now, but your proposal is safely stored."
	}
	if form.RedirectToPortal {
		notice += " Your secure speaker portal is ready."
	}
	session.AddFlash(ctx.Request, "notice", notice)
	live.Broadcast("submission:created", map[string]string{"submission": submissionID, "speaker": speakerID, "queue": submissionQueue})
	// Every successful proposal lands on the organizer-customizable receipt.
	// RedirectToPortal controls only the safe keyed follow-on, never whether
	// the browser is sent to an unkeyed private route.
	actionflow.Redirect(ctx, thanksRedirectURL(form.Slug, speakerID))
	return nil
}

func submissionEmailTemplate(state domain.State, templateID string) (domain.EmailTemplate, bool) {
	for _, template := range state.EmailTemplates {
		if template.ID == templateID {
			return template, true
		}
	}
	return domain.EmailTemplate{}, false
}

// submittedProposalValidation is the complete, current form interpretation
// used to persist a public submission. Re-running it inside UpdateAudit keeps
// a concurrently edited CFP from committing answers, visibility, or routing
// decisions computed from an earlier schema snapshot.
type submittedProposalValidation struct {
	Form            domain.SubmissionForm
	Decision        decisionrules.RoutingDecision
	VisibilityTrace []string
	VisibleFields   map[string]bool
}

func validateSubmittedProposal(state domain.State, formID string, formData map[string]string, engine *decisionrules.Engine, now time.Time) (submittedProposalValidation, error) {
	form, found := state.Form(formID)
	if !found {
		return submittedProposalValidation{}, fmt.Errorf("submission form not found")
	}
	if form.Status != "open" || now.After(form.CloseAt) {
		return submittedProposalValidation{}, action.Validation("This call for speakers is closed.", map[string]string{"form": "The submission deadline has passed."}, formData)
	}
	if message, ok := rejectUnknownFields(formData, form.Fields); !ok {
		return submittedProposalValidation{}, action.Validation(message, map[string]string{"form": message}, formData)
	}
	visibleFields, visibilityTrace, err := visibleSchemaFields(engine, form, formData)
	if err != nil {
		return submittedProposalValidation{}, err
	}
	fieldErrors := validateSchemaFields(form.Fields, state, formData, visibleFields)
	if !strings.Contains(formData["email"], "@") {
		fieldErrors["email"] = "Enter a valid email address."
	}
	if _, found := state.Category(formData["category"]); !found {
		fieldErrors["category"] = "Unknown category."
	}
	if len(fieldErrors) > 0 {
		return submittedProposalValidation{}, action.Validation("Correct the highlighted fields and submit again.", fieldErrors, formData)
	}
	decision, err := engine.Route(formData["category"], formData["format"], formData["level"])
	if err != nil {
		return submittedProposalValidation{}, err
	}
	if decision.Track != "" {
		if _, found := state.Track(decision.Track); !found {
			return submittedProposalValidation{}, fmt.Errorf("routing policy selected unknown track %q", decision.Track)
		}
	}
	return submittedProposalValidation{
		Form:            *form,
		Decision:        decision,
		VisibilityTrace: visibilityTrace,
		VisibleFields:   visibleFields,
	}, nil
}

// thanksRedirectURL builds the /submit/{slug}/thanks target submitProposal
// redirects to on success. The fresh PT-2 token proves which receipt may name
// the submitted proposal. When RedirectToPortal is enabled, that same proof
// also powers the thanks page's safe follow-on link and meta refresh.
func thanksRedirectURL(formSlug, speakerID string) string {
	target := url.URL{Path: "/submit/" + formSlug + "/thanks"}
	values := url.Values{}
	values.Set("speaker", speakerID)
	values.Set("key", token.New().Sign(speakerID))
	target.RawQuery = values.Encode()
	return target.String()
}

// loadThanks renders the FB-5 success page. It resolves the form's
// customizable success copy (domain.SubmissionForm.SuccessPageHeading and
// SuccessPageBody). It resolves the proposal title and portal URL only when
// ?key= verifies to the same subject as ?speaker=. A direct, unkeyed, invalid,
// or mismatched visit still renders the branded success page with generic copy
// and no speaker-specific data or navigation.
func loadThanks(ctx *route.RouteContext, page route.FilePage) (any, error) {
	snapshot := appstate.MustGet().Snapshot()
	form, found := snapshot.Form(ctx.Param("slug"))
	if !found {
		// Matches the [slug] loader: an unknown or retired form slug is a
		// routing miss, so this renders the branded 404, not a 500.
		return nil, route.NotFound("submission form not found")
	}
	speakerID, hasReceipt := verifiedThanksSpeaker(ctx.Query("speaker"), ctx.Query("key"))
	portalURL := ""
	if hasReceipt && form.RedirectToPortal {
		portalURL = thanksPortalURL(speakerID, ctx.Query("key"))
	}
	hasPortal := portalURL != ""
	if !hasReceipt {
		// A bare public speaker ID is enumerable and conveys no authority. Keep
		// the response generic unless the key proves the same speaker subject.
		speakerID = ""
	}
	submission, hasSubmission := latestSubmission(snapshot, form.ID, speakerID)
	confirmationStatus := "disabled"
	if form.SendConfirmation {
		confirmationStatus = "enabled"
	}
	if hasSubmission {
		for _, communication := range snapshot.Communications {
			if communication.SubmissionID == submission.ID && communication.Trigger == "submission.confirmation" {
				confirmationStatus = communication.Status
			}
		}
	}
	return map[string]any{
		"workspace": present.WorkspaceIdentity(snapshot),
		"form": map[string]any{
			"heading": form.SuccessPageHeading(),
			"body":    form.SuccessPageBody(),
		},
		"submission":          map[string]any{"title": submission.Title},
		"formSlug":            form.Slug,
		"portalURL":           portalURL,
		"hasPortal":           hasPortal,
		"confirmationEnabled": form.SendConfirmation,
		"confirmationStatus":  confirmationStatus,
	}, nil
}

// thanksMetadata attaches the FB-5 redirect: a declarative
// <meta http-equiv="refresh"> head tag that carries the browser on to the
// speaker's keyed portal URL about ten seconds after the success page renders.
// It emits the tag only for a key verified to the requested speaker. This is
// document metadata, not a <script>, so it holds the zero-bespoke-JavaScript
// invariant and needs no CSP change.
func thanksMetadata(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
	description := "Your proposal has been received."
	fields, _ := data.(map[string]any)
	portalURL, _ := fields["portalURL"].(string)
	hasPortal, _ := fields["hasPortal"].(bool)
	if hasPortal && portalURL != "" {
		ctx.AddHead(gosx.El("meta", gosx.Attrs(
			gosx.Attr("http-equiv", "refresh"),
			gosx.Attr("content", "10;url="+portalURL),
		)))
		description = "Your proposal is in, and your secure speaker portal is one click away."
	}
	return server.Metadata{Title: server.Title{Default: "Proposal received — Rostrum"}, Description: description}, nil
}

// thanksPortalURL builds the keyed portal URL the success page's meta refresh
// and visible link target, but only when key verifies to the same speakerID.
// A speaker ID by itself is public metadata, not authorization, so invalid,
// absent, and cross-speaker keys all produce an empty URL.
func thanksPortalURL(speakerID, key string) string {
	speakerID, ok := verifiedThanksSpeaker(speakerID, key)
	if !ok {
		return ""
	}
	target := url.URL{Path: "/portal/" + speakerID}
	values := url.Values{}
	values.Set("key", strings.TrimSpace(key))
	target.RawQuery = values.Encode()
	return target.String()
}

func verifiedThanksSpeaker(speakerID, key string) (string, bool) {
	speakerID = strings.TrimSpace(speakerID)
	keySpeakerID, ok := token.New().Verify(strings.TrimSpace(key))
	if speakerID == "" || !ok || keySpeakerID != speakerID {
		return "", false
	}
	return speakerID, true
}

// latestSubmission returns the most recently submitted proposal speakerID
// has on formID. The thanks page uses only its title and stable ID (to find
// the matching confirmation outcome); it never renders answers or routing.
func latestSubmission(state domain.State, formID, speakerID string) (domain.Submission, bool) {
	if speakerID == "" {
		return domain.Submission{}, false
	}
	var latest domain.Submission
	found := false
	for _, submission := range state.Submissions {
		if submission.FormID != formID || !contains(submission.SpeakerIDs, speakerID) {
			continue
		}
		if !found || submission.SubmittedAt.After(latest.SubmittedAt) {
			latest = submission
			found = true
		}
	}
	return latest, found
}

// accessibleDraft requires both a draft-shaped record and a valid signed key
// for one of its speakers. A bare database ID is never sufficient to prefill
// personally identifying proposal content on the public route.
func accessibleDraft(state domain.State, formSlug, draftID, key string) (domain.Submission, string, bool) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return domain.Submission{}, "", false
	}
	form, formFound := state.Form(formSlug)
	draft, draftFound := state.Submission(draftID)
	speakerID, keyOK := token.New().Verify(strings.TrimSpace(key))
	if !formFound || !draftFound || !keyOK || draft.Status != domain.SubmissionDraft || draft.FormID != form.ID || !contains(draft.SpeakerIDs, speakerID) {
		return domain.Submission{}, "", false
	}
	return *draft, speakerID, true
}

func draftURL(formSlug, draftID, key string) string {
	target := url.URL{Path: "/submit/" + formSlug}
	values := url.Values{}
	values.Set("draft", draftID)
	values.Set("key", key)
	target.RawQuery = values.Encode()
	return target.String()
}

func draftValues(state domain.State, draft domain.Submission) map[string]string {
	values := make(map[string]string, len(draft.Answers)+12)
	values["title"] = draft.Title
	values["abstract"] = draft.Abstract
	values["format"] = draft.Format
	values["category"] = draft.CategoryID
	values["level"] = draft.Level
	for key, value := range draft.Answers {
		values[key] = value
	}
	if len(draft.SpeakerIDs) > 0 {
		if speaker, found := state.Speaker(draft.SpeakerIDs[0]); found {
			values["first_name"] = speaker.FirstName
			values["last_name"] = speaker.LastName
			values["email"] = speaker.Email
			values["role"] = speaker.Role
			values["company"] = speaker.Company
			values["biography"] = speaker.Biography
		}
	}
	return values
}

func applyDraftSpeakerFields(speaker *domain.Speaker, formData map[string]string, now time.Time) {
	if speaker == nil {
		return
	}
	if value := strings.TrimSpace(formData["first_name"]); value != "" {
		speaker.FirstName = value
	}
	if value := strings.TrimSpace(formData["last_name"]); value != "" {
		speaker.LastName = value
	}
	if value := strings.TrimSpace(formData["role"]); value != "" {
		speaker.Role = value
	}
	if value := strings.TrimSpace(formData["company"]); value != "" {
		speaker.Company = value
	}
	if value := strings.TrimSpace(formData["biography"]); value != "" {
		speaker.Biography = value
	}
	speaker.UpdatedAt = now
}

func applySubmittedSpeakerFields(speaker *domain.Speaker, formData map[string]string, now time.Time) {
	if speaker == nil {
		return
	}
	speaker.FirstName = strings.TrimSpace(formData["first_name"])
	speaker.LastName = strings.TrimSpace(formData["last_name"])
	// Optional profile information is intentionally additive. A proposal form
	// can omit or leave optional participant fields blank without erasing a
	// speaker profile captured in a previous submission or portal edit.
	applyDraftSpeakerFields(speaker, formData, now)
}

func applyDraftSubmission(submission *domain.Submission, eventID, formID, speakerID string, formData map[string]string, visibilityTrace []string, now time.Time, fields []domain.FormField, visible map[string]bool) {
	submission.EventID = eventID
	submission.FormID = formID
	submission.Title = strings.TrimSpace(formData["title"])
	submission.Abstract = strings.TrimSpace(formData["abstract"])
	submission.Format = strings.TrimSpace(formData["format"])
	submission.CategoryID = strings.TrimSpace(formData["category"])
	submission.Level = strings.TrimSpace(formData["level"])
	submission.SpeakerIDs = []string{speakerID}
	submission.Status = domain.SubmissionDraft
	submission.TrackID = ""
	submission.RoutedQueue = ""
	submission.RoutedOwner = ""
	submission.RuleTrace = append([]string(nil), visibilityTrace...)
	submission.Answers = submissionAnswers(fields, formData, visible)
	submission.WithdrawalReason = ""
	submission.WithdrawnAt = time.Time{}
	submission.SubmittedAt = time.Time{}
	submission.UpdatedAt = now
}

func applySubmittedSubmission(submission *domain.Submission, eventID, formID, speakerID string, formData map[string]string, decision decisionrules.RoutingDecision, visibilityTrace []string, now time.Time, fields []domain.FormField, visible map[string]bool) {
	submission.EventID = eventID
	submission.FormID = formID
	submission.Title = strings.TrimSpace(formData["title"])
	submission.Abstract = strings.TrimSpace(formData["abstract"])
	submission.Format = strings.TrimSpace(formData["format"])
	submission.CategoryID = strings.TrimSpace(formData["category"])
	submission.TrackID = decision.Track
	submission.Level = strings.TrimSpace(formData["level"])
	submission.SpeakerIDs = []string{speakerID}
	submission.Status = domain.SubmissionPending
	submission.RoutedQueue = decision.Queue
	submission.RoutedOwner = decision.Owner
	submission.RuleTrace = append(append([]string{decision.Rule + ": " + decision.Reason}, decision.Trace...), visibilityTrace...)
	submission.Answers = submissionAnswers(fields, formData, visible)
	submission.WithdrawalReason = ""
	submission.WithdrawnAt = time.Time{}
	submission.SubmittedAt = now
	submission.UpdatedAt = now
}

func validateDraftFields(fields []domain.FormField, state domain.State, formData map[string]string, visible map[string]bool) map[string]string {
	fieldErrors := make(map[string]string)
	for _, field := range fields {
		if !visible[field.ID] {
			continue
		}
		value := strings.TrimSpace(formData[field.ID])
		if value == "" {
			continue
		}
		if field.MaxLength > 0 && len([]rune(value)) > field.MaxLength {
			fieldErrors[field.ID] = fmt.Sprintf("Keep this field to %d characters.", field.MaxLength)
			continue
		}
		if field.Type == "select" && !contains(present.FormFieldOptionValues(state, field), value) {
			fieldErrors[field.ID] = "Choose a valid option."
		}
	}
	return fieldErrors
}

func draftCount(submissions []domain.Submission, formID, speakerID string) int {
	count := 0
	for _, submission := range submissions {
		if submission.FormID == formID && submission.Status == domain.SubmissionDraft && contains(submission.SpeakerIDs, speakerID) {
			count++
		}
	}
	return count
}

func maxDrafts(form *domain.SubmissionForm) int {
	if form != nil && form.MaxDraftsPerSubmitter > 0 {
		return form.MaxDraftsPerSubmitter
	}
	return 3
}

// coreSubmissionFields lists the schema field IDs that already have a typed
// home on domain.Submission (title, abstract, format, category, level) or
// domain.Speaker (first_name, last_name, email, role, company, biography).
// Only a field outside this set lands in Submission.Answers, so a schema
// change can never grow Answers with data a typed column already carries.
var coreSubmissionFields = map[string]bool{
	"title": true, "abstract": true, "format": true, "category": true, "level": true,
	"first_name": true, "last_name": true, "email": true, "role": true, "company": true, "biography": true,
}

// submissionAnswers stores only the non-core schema fields the form defines
// (for example topics and workshop_needs), each already bounded by
// validateSchemaFields. This is the other half of the C2 fix: an unknown key
// never reaches this map, because rejectUnknownFields ran first, and a core
// field is never duplicated into it.
func submissionAnswers(fields []domain.FormField, formData map[string]string, visible map[string]bool) map[string]string {
	answers := make(map[string]string)
	for _, field := range fields {
		if coreSubmissionFields[field.ID] || !visible[field.ID] {
			continue
		}
		if value := strings.TrimSpace(formData[field.ID]); value != "" {
			answers[field.ID] = value
		}
	}
	return answers
}

// rejectUnknownFields reports the first posted key that is neither a schema
// field ID nor a framework key (csrf_token, form_id). A non-empty message
// means the caller must reject the submission outright: this closes the
// unbounded-key half of C2, since a posted key the schema never declared can
// no longer reach the store.
func rejectUnknownFields(formData map[string]string, fields []domain.FormField) (string, bool) {
	allowed := map[string]bool{"csrf_token": true, "form_id": true, "draft_id": true, "draft_key": true}
	for _, field := range fields {
		allowed[field.ID] = true
	}
	for key := range formData {
		if !allowed[key] {
			return "That submission included a field we do not recognize.", false
		}
	}
	return "", true
}

// validateSchemaFields checks every *visible* schema field's Required flag,
// MaxLength, and — for a select field — option membership. Hidden questions
// are disabled by the hydrated form and are also ignored server-side, so a
// stale browser cannot be forced to satisfy a now-inapplicable requirement.
func validateSchemaFields(fields []domain.FormField, state domain.State, formData map[string]string, visible map[string]bool) map[string]string {
	fieldErrors := make(map[string]string)
	for _, field := range fields {
		if !visible[field.ID] {
			continue
		}
		value := strings.TrimSpace(formData[field.ID])
		if value == "" {
			if field.Required {
				fieldErrors[field.ID] = "This field is required."
			}
			continue
		}
		if field.MaxLength > 0 && len([]rune(value)) > field.MaxLength {
			fieldErrors[field.ID] = fmt.Sprintf("Keep this field to %d characters.", field.MaxLength)
			continue
		}
		if field.Type == "select" && !contains(present.FormFieldOptionValues(state, field), value) {
			fieldErrors[field.ID] = "Choose a valid option."
		}
	}
	return fieldErrors
}

// visibleSchemaFields evaluates every persisted question rule through
// Arbiter. The builder limits rules to `equals → show`, but this function
// repeats the shape validation at the trust boundary so a restored archive or
// handcrafted request cannot turn a generic host conditional into policy.
func visibleSchemaFields(engine *decisionrules.Engine, form *domain.SubmissionForm, formData map[string]string) (map[string]bool, []string, error) {
	visible := make(map[string]bool, len(form.Fields))
	for _, field := range form.Fields {
		visible[field.ID] = true
	}
	trace := make([]string, 0, len(form.QuestionRules))
	for _, rule := range form.QuestionRules {
		if rule.Operator != "equals" || rule.Effect != "show" {
			return nil, nil, fmt.Errorf("unsupported question rule %s", rule.ID)
		}
		if _, found := fieldByID(form.Fields, rule.SourceFieldID); !found {
			return nil, nil, fmt.Errorf("question rule %s has an unknown source field", rule.ID)
		}
		if _, found := fieldByID(form.Fields, rule.TargetFieldID); !found {
			return nil, nil, fmt.Errorf("question rule %s has an unknown target field", rule.ID)
		}
		decision, err := engine.QuestionVisibility(strings.TrimSpace(formData[rule.SourceFieldID]), rule.Value, rule.Effect, rule.TargetFieldID)
		if err != nil {
			return nil, nil, err
		}
		visible[rule.TargetFieldID] = decision.Visible
		trace = append(trace, decision.Rule+": "+rule.TargetFieldID+"="+strconv.FormatBool(decision.Visible))
	}
	return visible, trace, nil
}

// fieldByID finds a schema field by ID.
func fieldByID(fields []domain.FormField, id string) (domain.FormField, bool) {
	for _, field := range fields {
		if field.ID == id {
			return field, true
		}
	}
	return domain.FormField{}, false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
