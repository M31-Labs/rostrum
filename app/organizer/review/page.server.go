package review

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			snapshot := appstate.MustGet().Snapshot()
			data := present.Review(snapshot)
			data["reviewerLinks"] = reviewerLinkRows(snapshot)
			return data, nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Review — Rostrum"}, Description: "Multi-round rubric review with attributable scoring."}, nil
		},
		Actions: route.FileActions{"saveReview": saveReview},
	}); err != nil {
		log.Fatal(err)
	}
}

// reviewerLinkRows builds the RV-2 "Copy review link" list: one row per
// reviewer, carrying a signed /review/{token} URL for a human reviewer, or
// none for the virtual practitioner (an AI reviewer never signs in). The
// token itself comes from internal/token's reviewer-kind signer
// (token.NewReviewer, internal/token/reviewer.go) — a distinct key from the
// speaker/portal token, so this link can never double as a portal link.
func reviewerLinkRows(state domain.State) []map[string]any {
	rows := make([]map[string]any, 0, len(state.Reviewers))
	for _, reviewer := range state.Reviewers {
		canReview := reviewer.Kind == "human"
		link := ""
		if canReview {
			link = reviewerLinkURL(reviewer.ID)
		}
		rows = append(rows, map[string]any{
			"id":        reviewer.ID,
			"name":      reviewer.Name,
			"initials":  reviewerLinkInitials(reviewer.Name),
			"kind":      present.StatusLabel(reviewer.Kind),
			"canReview": canReview,
			"link":      link,
		})
	}
	return rows
}

// reviewerLinkInitials mirrors present's unexported nameInitials
// (internal/present/review.go), duplicated here in ~8 lines because that
// helper is unexported and this package does not own internal/present.
func reviewerLinkInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "—"
	}
	result := string([]rune(parts[0])[0])
	if len(parts) > 1 {
		result += string([]rune(parts[len(parts)-1])[0])
	}
	return strings.ToUpper(result)
}

// reviewerLinkURL builds the absolute /review/{token} URL an organizer's
// "Copy review link" button copies, so the copied text is ready to paste
// into an email or chat message with no domain to fill in by hand.
func reviewerLinkURL(reviewerID string) string {
	return publicBaseURL() + "/review/" + token.NewReviewer().SignReviewer(reviewerID)
}

// publicBaseURL mirrors app/submit/page.server.go's helper of the same
// name: the absolute base a reviewer link must use, because it is followed
// from an inbox or chat client outside the browser session that copied it.
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

func saveReview(ctx *action.Context) error {
	planID := strings.TrimSpace(ctx.FormData["plan_id"])
	submissionID := strings.TrimSpace(ctx.FormData["submission_id"])
	reviewerID := strings.TrimSpace(ctx.FormData["reviewer_id"])
	recommendation := strings.TrimSpace(ctx.FormData["recommendation"])
	comments := strings.TrimSpace(ctx.FormData["comments"])

	snapshot := appstate.MustGet().Snapshot()
	plan, found := snapshot.ReviewPlan(planID)
	if !found || (plan.Status != "active" && plan.Status != "open") {
		return action.Validation("Choose the active review round.", map[string]string{"plan_id": "The review plan is not active."}, ctx.FormData)
	}
	if _, found := snapshot.Submission(submissionID); !found || !containsReviewID(plan.SubmissionIDs, submissionID) {
		return action.Validation("Choose a proposal assigned to this round.", map[string]string{"submission_id": "Proposal is not assigned."}, ctx.FormData)
	}
	reviewer, reviewerFound := reviewParticipant(snapshot, reviewerID)
	if !reviewerFound || reviewer.Kind != "human" || !containsReviewID(plan.ReviewerIDs, reviewerID) {
		return action.Validation("Choose a human reviewer assigned to this round.", map[string]string{"reviewer_id": "Reviewer is not assigned."}, ctx.FormData)
	}
	validRecommendation := false
	for _, candidate := range []string{"strong_yes", "yes", "maybe", "no", "strong_no"} {
		if recommendation == candidate {
			validRecommendation = true
			break
		}
	}
	fieldErrors := map[string]string{}
	if !validRecommendation {
		fieldErrors["recommendation"] = "Choose a recommendation."
	}
	if len([]rune(comments)) < 20 {
		fieldErrors["comments"] = "Add at least 20 characters of decision-relevant context."
	}
	scores := make(map[string]float64, len(plan.Criteria))
	for _, criterion := range plan.Criteria {
		field := "score_" + criterion.ID
		score, err := strconv.ParseFloat(strings.TrimSpace(ctx.FormData[field]), 64)
		if err != nil || score < 0 || score > criterion.MaxScore {
			fieldErrors[field] = fmt.Sprintf("Use a score from 0 to %.0f.", criterion.MaxScore)
			continue
		}
		scores[criterion.ID] = score
	}
	if len(fieldErrors) > 0 {
		return action.Validation("Complete every rubric criterion.", fieldErrors, ctx.FormData)
	}

	created := false
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		now := time.Now().UTC()
		for index := range state.Evaluations {
			evaluation := &state.Evaluations[index]
			if evaluation.PlanID == planID && evaluation.SubmissionID == submissionID && evaluation.ReviewerID == reviewerID && evaluation.Source == "human" {
				evaluation.Scores = scores
				evaluation.Comments = comments
				evaluation.Recommendation = recommendation
				evaluation.UpdatedAt = now
				return nil
			}
		}
		state.Evaluations = append(state.Evaluations, domain.Evaluation{
			ID: domain.NewID("eval"), PlanID: planID, SubmissionID: submissionID, ReviewerID: reviewerID,
			Scores: scores, Comments: comments, Recommendation: recommendation, Source: "human", CreatedAt: now, UpdatedAt: now,
		})
		created = true
		return nil
	}); err != nil {
		return err
	}
	verb := "Updated"
	if created {
		verb = "Recorded"
	}
	session.AddFlash(ctx.Request, "notice", verb+" a human rubric review from "+reviewer.Name+".")
	live.Broadcast("review:updated", map[string]string{"submission": submissionID, "source": "human"})
	actionflow.Redirect(ctx, "/organizer/review#candidates")
	return nil
}

func containsReviewID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func reviewParticipant(state domain.State, id string) (domain.Reviewer, bool) {
	for _, reviewer := range state.Reviewers {
		if reviewer.ID == id {
			return reviewer, true
		}
	}
	return domain.Reviewer{}, false
}
