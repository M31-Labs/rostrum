package tour

import (
	"log"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: loadTour,
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{
				Title:       server.Title{Default: "Product tour — Rostrum"},
				Description: "Walk Rostrum from call for proposals to a published, conflict-checked program.",
			}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}
}

func loadTour(ctx *route.RouteContext, page route.FilePage) (any, error) {
	// Demo persona links are bearer credentials. Do not let browsers or
	// intermediaries retain a rendered copy of the tour page.
	ctx.NoStore()
	return tourData(appstate.MustGet().Snapshot(), present.ReadOnlyDemoMode()), nil
}

func tourData(state domain.State, readOnlyDemo bool) map[string]any {
	workspace := present.WorkspaceIdentity(state)
	reviewerHref := "/organizer/review"
	reviewerAction := "Inspect review operations"
	speakerHref := "/organizer/portal"
	speakerAction := "Inspect speaker operations"
	if readOnlyDemo {
		if reviewerID := firstHumanReviewer(state); reviewerID != "" {
			reviewerHref = "/review/" + token.NewReviewer().SignReviewerDemo(reviewerID)
			reviewerAction = "Open the reviewer desk"
		}
		if speakerID := present.FirstPortalSpeaker(state); speakerID != "" {
			speakerHref = "/portal/" + speakerID + "?key=" + token.New().SignDemo(speakerID)
			speakerAction = "Open the speaker portal"
		}
	}

	return map[string]any{
		"workspace":    workspace,
		"readOnlyDemo": readOnlyDemo,
		"eventName":    state.Event.Name,
		"sourceURL":    "https://github.com/M31-Labs/rostrum",
		"docsURL":      "https://m31-labs.github.io/rostrum/",
		"personas": []map[string]any{
			{
				"number": "01", "role": "Organizer", "label": "Command the program",
				"body": "See proposal health, routing traces, review coverage, onboarding, and schedule risk in one operational workspace.",
				"href": "/organizer", "action": "Enter the command center", "tone": "organizer",
			},
			{
				"number": "02", "role": "Submitter", "label": "Follow the conditional CFP",
				"body": "Choose a session format and watch the form reveal only the questions that matter, with server-side routing preserved in the audit trail.",
				"href": workspace["cfpHref"], "action": "Walk the submission path", "tone": "submitter",
			},
			{
				"number": "03", "role": "Reviewer", "label": "Score with a governed rubric",
				"body": "Review assigned work against weighted criteria while anonymity, recusal, and company-conflict rules remain enforceable and explainable.",
				"href": reviewerHref, "action": reviewerAction, "tone": "reviewer",
			},
			{
				"number": "04", "role": "Speaker", "label": "Finish every handoff",
				"body": "Update a profile, track approvals, deliver files, read event resources, and keep the personal calendar aligned from one signed-link portal.",
				"href": speakerHref, "action": speakerAction, "tone": "speaker",
			},
			{
				"number": "05", "role": "Attendee", "label": "Build a personal itinerary",
				"body": "Browse the published program, save sessions locally, meet the speakers, and consume the same records through embeds or the public JSON API.",
				"href": workspace["publicAgendaHref"], "action": "Explore the public program", "tone": "attendee",
			},
		},
	}
}

func firstHumanReviewer(state domain.State) string {
	if plan, err := state.ActiveReviewPlan(); err == nil {
		for _, reviewerID := range plan.ReviewerIDs {
			for _, reviewer := range state.Reviewers {
				if reviewer.ID == reviewerID && reviewer.Kind == "human" && reviewer.Active() {
					return reviewer.ID
				}
			}
		}
	}
	for _, reviewer := range state.Reviewers {
		if reviewer.Kind == "human" && reviewer.Active() {
			return reviewer.ID
		}
	}
	return ""
}
