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
	decisionrules "github.com/odvcencio/programma/rules"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

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
	required := []string{"form_id", "title", "abstract", "format", "category", "level", "topics", "first_name", "last_name", "email"}
	fieldErrors := make(map[string]string)
	for _, field := range required {
		if strings.TrimSpace(ctx.FormData[field]) == "" {
			fieldErrors[field] = "This field is required."
		}
	}
	if !strings.Contains(ctx.FormData["email"], "@") {
		fieldErrors["email"] = "Enter a valid email address."
	}
	if ctx.FormData["format"] == "Workshop" && strings.TrimSpace(ctx.FormData["workshop_needs"]) == "" {
		fieldErrors["workshop_needs"] = "Tell us what the workshop needs."
	}
	if len([]rune(ctx.FormData["title"])) > 120 {
		fieldErrors["title"] = "Keep the title to 120 characters."
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
		answers := make(map[string]string)
		for key, value := range ctx.FormData {
			if key != "csrf_token" {
				answers[key] = value
			}
		}
		state.Submissions = append(state.Submissions, domain.Submission{
			ID: submissionID, EventID: state.Event.ID, FormID: form.ID, Title: strings.TrimSpace(ctx.FormData["title"]), Abstract: strings.TrimSpace(ctx.FormData["abstract"]),
			Format: ctx.FormData["format"], CategoryID: ctx.FormData["category"], TrackID: decision.Track, Level: ctx.FormData["level"], SpeakerIDs: []string{speakerID},
			Status: domain.SubmissionPending, RoutedQueue: decision.Queue, RoutedOwner: decision.Owner, RuleTrace: append([]string{decision.Rule + ": " + decision.Reason}, decision.Trace...),
			Answers: answers, SubmittedAt: now, UpdatedAt: now,
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

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
