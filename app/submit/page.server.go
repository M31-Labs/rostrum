package submit

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/actionflow"
	"github.com/odvcencio/programma/internal/appstate"
	"github.com/odvcencio/programma/internal/domain"
	"github.com/odvcencio/programma/internal/live"
	"github.com/odvcencio/programma/internal/present"
	"github.com/odvcencio/programma/internal/ratelimit"
	decisionrules "github.com/odvcencio/programma/rules"
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

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(thisFile), "[slug]", "page.gsx")
	if err := route.RegisterFileModule(route.FileModuleFor(source, route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.SubmissionForm(appstate.MustGet().Snapshot(), ctx.Param("slug"))
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Call for speakers — Programma"}, Description: "Submit a proposal to M31 Systems Forum 2026."}, nil
		},
		Actions: route.FileActions{"submitProposal": submitProposal},
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
		state.Communications = append(state.Communications, domain.Communication{
			ID: domain.NewID("comm"), TemplateID: form.ConfirmationTemplate, SpeakerID: speakerID, Subject: "We received " + strings.TrimSpace(ctx.FormData["title"]), Status: "sent", Provider: "demo-outbox", SentAt: now,
		})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Proposal received. We sent a confirmation and opened your portal.")
	live.Broadcast("submission:created", map[string]string{"submission": submissionID, "speaker": speakerID, "queue": decision.Queue})
	actionflow.Redirect(ctx, "/portal/"+speakerID+"?submitted=1")
	return nil
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
