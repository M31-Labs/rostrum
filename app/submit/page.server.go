package submit

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/mail"
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
			return present.SubmissionForm(appstate.MustGet().Snapshot(), ctx.Param("slug"))
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Call for speakers — Rostrum"}, Description: "Submit a proposal to M31 Systems Forum 2026."}, nil
		},
		Actions: route.FileActions{"submitProposal": submitProposal},
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

func submitProposal(ctx *action.Context) error {
	// Rate caps come first (SE-3b): reject before touching the store or the
	// schema, so a caller past their cap costs one map lookup, not a clone.
	if !submissionLimiter.Allow(ratelimit.RequestIdentity(ctx.Request)) {
		message := "You have reached the submission limit for this session. Contact the program team if you need to submit another proposal."
		return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
	}
	if ip := ratelimit.ClientIP(ctx.Request); ip != "" && !submissionIPLimiter.Allow(ip) {
		message := "Too many submissions from this network right now. Please try again in a little while."
		return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
	}

	snapshot := appstate.MustGet().Snapshot()
	form, found := snapshot.Form(ctx.FormData["form_id"])
	if !found {
		return fmt.Errorf("submission form not found")
	}

	if message, ok := rejectUnknownFields(ctx.FormData, form.Fields); !ok {
		return action.Validation(message, map[string]string{"form": message}, ctx.FormData)
	}

	fieldErrors := validateSchemaFields(form.Fields, snapshot, ctx.FormData)
	if !strings.Contains(ctx.FormData["email"], "@") {
		fieldErrors["email"] = "Enter a valid email address."
	}
	// workshop_needs is required only when the chosen format is "Workshop",
	// so its requirement cannot come from the schema's Required flag the way
	// every other field's does; validateSchemaFields skips it and this checks
	// it directly. FB-4 generalizes this through engine.FieldVisibility.
	workshopValue := strings.TrimSpace(ctx.FormData["workshop_needs"])
	if ctx.FormData["format"] == "Workshop" && workshopValue == "" {
		fieldErrors["workshop_needs"] = "Tell us what the workshop needs."
	} else if workshopField, ok := fieldByID(form.Fields, "workshop_needs"); ok && workshopField.MaxLength > 0 && len([]rune(workshopValue)) > workshopField.MaxLength {
		fieldErrors["workshop_needs"] = fmt.Sprintf("Keep this field to %d characters.", workshopField.MaxLength)
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the highlighted fields and submit again.", fieldErrors, ctx.FormData)
	}

	engine, err := decisionrules.New()
	if err != nil {
		return err
	}
	decision, err := engine.Route(ctx.FormData["category"], ctx.FormData["format"], ctx.FormData["level"])
	if err != nil {
		return err
	}
	speakerID := ""
	submissionID := ""
	speakerEmail := ""
	speakerName := ""
	title := strings.TrimSpace(ctx.FormData["title"])
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		form, found := state.Form(ctx.FormData["form_id"])
		if !found {
			return fmt.Errorf("submission form not found")
		}
		if form.Status != "open" || time.Now().After(form.CloseAt) {
			return action.Validation("This call for speakers is closed.", map[string]string{"form": "The submission deadline has passed."}, ctx.FormData)
		}
		if _, found := state.Category(ctx.FormData["category"]); !found {
			return action.Validation("Choose a valid category.", map[string]string{"category": "Unknown category."}, ctx.FormData)
		}
		email := strings.ToLower(strings.TrimSpace(ctx.FormData["email"]))
		for _, speaker := range state.Speakers {
			if strings.EqualFold(speaker.Email, email) {
				speakerID = speaker.ID
				speakerEmail = speaker.Email
				speakerName = strings.TrimSpace(speaker.FirstName + " " + speaker.LastName)
				break
			}
		}
		now := time.Now().UTC()
		if speakerID == "" {
			speakerID = domain.NewID("spk")
			state.Speakers = append(state.Speakers, domain.Speaker{
				ID: speakerID, FirstName: strings.TrimSpace(ctx.FormData["first_name"]), LastName: strings.TrimSpace(ctx.FormData["last_name"]),
				Email: email, Role: strings.TrimSpace(ctx.FormData["role"]), Company: strings.TrimSpace(ctx.FormData["company"]), Biography: strings.TrimSpace(ctx.FormData["biography"]),
				CreatedAt: now, UpdatedAt: now,
			})
			speakerEmail = email
			speakerName = strings.TrimSpace(ctx.FormData["first_name"] + " " + ctx.FormData["last_name"])
		}
		submissionID = domain.NewID("sub")
		state.Submissions = append(state.Submissions, domain.Submission{
			ID: submissionID, EventID: state.Event.ID, FormID: form.ID, Title: strings.TrimSpace(ctx.FormData["title"]), Abstract: strings.TrimSpace(ctx.FormData["abstract"]),
			Format: ctx.FormData["format"], CategoryID: ctx.FormData["category"], TrackID: decision.Track, Level: ctx.FormData["level"], SpeakerIDs: []string{speakerID},
			Status: domain.SubmissionPending, RoutedQueue: decision.Queue, RoutedOwner: decision.Owner, RuleTrace: append([]string{decision.Rule + ": " + decision.Reason}, decision.Trace...),
			Answers: submissionAnswers(form.Fields, ctx.FormData), SubmittedAt: now, UpdatedAt: now,
		})
		for index := range state.Tasks {
			if state.Tasks[index].ID == "task_profile" && !contains(state.Tasks[index].AssignedSpeakerIDs, speakerID) {
				state.Tasks[index].AssignedSpeakerIDs = append(state.Tasks[index].AssignedSpeakerIDs, speakerID)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Send the confirmation email outside the store lock — external I/O must
	// never run under Update. The keyed portal link (PT-2 token) lets the
	// speaker open their portal straight from the inbox. Record the true
	// outcome as a Communication row in a second, small mutation; never store
	// the raw provider error on the row (M8), only a sent/failed status.
	sender := confirmationSender()
	sendErr := mail.SendConfirmation(sender, publicBaseURL(), mail.Recipient{
		SpeakerID: speakerID, Name: speakerName, Email: speakerEmail,
	}, mail.Submission{Title: title}, token.New().Sign(speakerID))
	sendStatus := "sent"
	if sendErr != nil {
		sendStatus = "failed"
		log.Printf("submit: confirmation email to speaker %s failed: %v", speakerID, sendErr)
	}
	provider := "demo-outbox"
	if named, ok := sender.(mail.Named); ok {
		provider = named.Name()
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		state.Communications = append(state.Communications, domain.Communication{
			ID: domain.NewID("comm"), TemplateID: form.ConfirmationTemplate, SpeakerID: speakerID,
			Subject: "We received " + title, Status: sendStatus, Provider: provider, SentAt: time.Now().UTC(),
		})
		return nil
	}); err != nil {
		return err
	}

	session.AddFlash(ctx.Request, "notice", "Proposal received. We sent a confirmation and opened your portal.")
	live.Broadcast("submission:created", map[string]string{"submission": submissionID, "speaker": speakerID, "queue": decision.Queue})
	live.Broadcast("communication:queued", map[string]string{"speaker": speakerID})
	if form.RedirectToPortal {
		// FB-5: land on the customizable success page first. Its meta
		// refresh (thanksMetadata) carries the visitor on to the portal
		// after ~10s; the signed PT-2 token travels in the URL so that
		// later hop authenticates the portal session on arrival.
		actionflow.Redirect(ctx, thanksRedirectURL(form.Slug, speakerID))
	} else {
		actionflow.Redirect(ctx, "/portal/"+speakerID+"?submitted=1")
	}
	return nil
}

// thanksRedirectURL builds the /submit/{slug}/thanks target submitProposal
// redirects to on success. The key is a fresh PT-2 portal token
// (token.New().Sign) bound to speakerID, so the thanks page's meta refresh
// and its visible portal link both carry an authenticated session forward —
// with no separate sign-in step and no reliance on a cookie the redirect
// response itself does not set.
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
// SuccessPageBody), the most recently submitted title for the speaker named
// by ?speaker=, and the keyed portal URL the visible link points to. A
// direct visit with no ?speaker= still renders the page; it just omits the
// portal link and countdown copy, since there is no session to carry there.
func loadThanks(ctx *route.RouteContext, page route.FilePage) (any, error) {
	snapshot := appstate.MustGet().Snapshot()
	form, found := snapshot.Form(ctx.Param("slug"))
	if !found {
		return nil, fmt.Errorf("submission form not found")
	}
	speakerID := strings.TrimSpace(ctx.Query("speaker"))
	return map[string]any{
		"form": map[string]any{
			"heading": form.SuccessPageHeading(),
			"body":    form.SuccessPageBody(),
		},
		"submission": map[string]any{"title": latestSubmissionTitle(snapshot, form.ID, speakerID)},
		"formSlug":   form.Slug,
		"portalURL":  thanksPortalURL(speakerID, ctx.Query("key")),
		"hasPortal":  speakerID != "",
	}, nil
}

// thanksMetadata attaches the FB-5 redirect: a declarative
// <meta http-equiv="refresh"> head tag that carries the browser on to the
// speaker's keyed portal URL about ten seconds after the success page
// renders. This is document metadata, not a <script>, so it holds the
// zero-bespoke-JavaScript invariant and needs no CSP change.
func thanksMetadata(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
	speakerID := strings.TrimSpace(ctx.Query("speaker"))
	if speakerID != "" {
		ctx.AddHead(gosx.El("meta", gosx.Attrs(
			gosx.Attr("http-equiv", "refresh"),
			gosx.Attr("content", "10;url="+thanksPortalURL(speakerID, ctx.Query("key"))),
		)))
	}
	return server.Metadata{Title: server.Title{Default: "Proposal received — Rostrum"}, Description: "Your proposal is in, and your speaker portal is one click away."}, nil
}

// thanksPortalURL builds the keyed portal URL the success page's meta
// refresh and its visible "Go to your portal now" link both target. key is
// the PT-2 token (token.New().Sign) thanksRedirectURL already signed for
// this speaker, so following either target authenticates the portal session
// on arrival with no separate sign-in step.
func thanksPortalURL(speakerID, key string) string {
	target := url.URL{Path: "/portal/" + speakerID}
	if key != "" {
		values := url.Values{}
		values.Set("key", key)
		target.RawQuery = values.Encode()
	}
	return target.String()
}

// latestSubmissionTitle returns the title of the most recently submitted
// proposal speakerID has on formID, or "" when speakerID is empty or has no
// matching submission. The thanks page uses this only to name the proposal
// in its confirmation copy; it never uses the returned submission's other
// fields, so no other Answers or routing data leaves this function.
func latestSubmissionTitle(state domain.State, formID, speakerID string) string {
	if speakerID == "" {
		return ""
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
	return latest.Title
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
func submissionAnswers(fields []domain.FormField, formData map[string]string) map[string]string {
	answers := make(map[string]string)
	for _, field := range fields {
		if coreSubmissionFields[field.ID] {
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
	allowed := map[string]bool{"csrf_token": true, "form_id": true}
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

// validateSchemaFields checks every schema field's Required flag, MaxLength,
// and — for a select field — option membership against the posted value.
// workshop_needs is skipped here because its requirement depends on the
// chosen format, not the schema's Required flag; submitProposal checks it
// separately, right after calling this.
func validateSchemaFields(fields []domain.FormField, state domain.State, formData map[string]string) map[string]string {
	fieldErrors := make(map[string]string)
	for _, field := range fields {
		if field.ID == "workshop_needs" {
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
