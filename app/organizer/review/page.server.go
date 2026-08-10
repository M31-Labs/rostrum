package review

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/reviewassist"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Review(appstate.MustGet().Snapshot()), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Review — Rostrum"}, Description: "Multi-round rubric review with attributable AI assistance."}, nil
		},
		Actions: route.FileActions{"aiAssist": addAIAssist, "saveReview": saveReview},
	}); err != nil {
		log.Fatal(err)
	}
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

func addAIAssist(ctx *action.Context) error {
	submissionID := strings.TrimSpace(ctx.FormData["submission_id"])
	if submissionID == "" {
		return fmt.Errorf("submission is required")
	}

	snapshot := appstate.MustGet().Snapshot()
	plan, found := snapshot.ReviewPlan("plan_round_two")
	if !found {
		return fmt.Errorf("active review plan not found")
	}
	submission, found := snapshot.Submission(submissionID)
	if !found {
		return fmt.Errorf("submission %s not found", submissionID)
	}
	for _, evaluation := range snapshot.Evaluations {
		if evaluation.PlanID == plan.ID && evaluation.SubmissionID == submissionID && evaluation.Source != "human" {
			session.AddFlash(ctx.Request, "notice", "An assisted evaluation already exists for “"+submission.Title+"”.")
			actionflow.Redirect(ctx, "/organizer/review")
			return nil
		}
	}

	assessment, err := reviewassist.NewFromEnv().Evaluate(ctx.Request.Context(), *plan, *submission)
	if err != nil {
		session.AddFlash(ctx.Request, "notice", "Review assist could not run: "+err.Error())
		actionflow.Redirect(ctx, "/organizer/review")
		return nil
	}

	added := false
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		currentPlan, found := state.ReviewPlan(plan.ID)
		if !found {
			return fmt.Errorf("active review plan not found")
		}
		if _, found := state.Submission(submissionID); !found {
			return fmt.Errorf("submission %s not found", submissionID)
		}
		for _, evaluation := range state.Evaluations {
			if evaluation.PlanID == currentPlan.ID && evaluation.SubmissionID == submissionID && evaluation.Source != "human" {
				return nil
			}
		}
		now := time.Now().UTC()
		state.Evaluations = append(state.Evaluations, domain.Evaluation{
			ID:             domain.NewID("eval"),
			PlanID:         currentPlan.ID,
			SubmissionID:   submissionID,
			ReviewerID:     "rev_virtual_practitioner",
			Scores:         assessment.Scores,
			Comments:       assessment.Comments,
			Recommendation: assessment.Recommendation,
			Source:         assessment.Provider,
			Model:          assessment.Model,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		added = true
		return nil
	}); err != nil {
		return err
	}
	message := "An assisted evaluation already exists for “" + submission.Title + "”."
	if added {
		message = "Added a rubric-grounded second opinion for “" + submission.Title + "” via " + assessment.Provider + "."
	}
	session.AddFlash(ctx.Request, "notice", message)
	live.Broadcast("review:updated", map[string]string{"submission": submissionID, "source": assessment.Provider})
	actionflow.Redirect(ctx, "/organizer/review")
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
