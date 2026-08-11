// Package review is RV-2's reviewer-facing surface: it lets a reviewer sign
// in with nothing but a signed link and score their assigned proposals,
// closing the impersonation gap where an organizer had to score on a
// reviewer's behalf (app/organizer/review/page.server.go's saveReview).
//
// The identity travels in the URL path, not a query key: an organizer's
// "Copy review link" (app/organizer/review/page.server.go) emits
// /review/{token}, and the token itself — internal/token's ReviewerToken —
// carries the reviewer's ID for every request against this route, GET or
// POST. That is what lets loadReview and scoreSubmission both trust
// ctx.Request.PathValue("token") as the sole source of reviewer identity:
// unlike the speaker portal's one-time ?key= (bound into a GoSX session on
// first visit), a reviewer link stays self-authenticating on every reload
// with no session state to lose or forge, so an action never needs to trust
// a posted reviewer_id.
package review

import (
	"fmt"
	"log"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/token"
	decisionrules "github.com/m31-labs/rostrum/rules"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(thisFile), "[token]", "page.gsx")
	if err := route.RegisterFileModule(route.FileModuleFor(source, route.FileModuleOptions{
		Load: loadReview,
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{
				Title:       server.Title{Default: "Reviewer link — Rostrum"},
				Description: "Score your assigned proposals against the weighted rubric.",
			}, nil
		},
		Actions: route.FileActions{"scoreSubmission": scoreSubmission},
	})); err != nil {
		log.Fatal(err)
	}
}

// loadReview verifies the token in the URL path and, when valid, renders
// the reviewer's assigned proposals for the active review round.
//
// An unknown or tampered token and an unknown or non-human reviewer ID both
// render the identical friendly "check your link" page (L8, the same
// oracle-proofing loadPortal already does for speaker IDs in
// app/portal/page.server.go): this route never lets a visitor tell a valid
// but unassigned reviewer ID apart from a garbage one.
func loadReview(ctx *route.RouteContext, page route.FilePage) (any, error) {
	snapshot := appstate.MustGet().Snapshot()
	reviewerID, ok := token.NewReviewer().VerifyReviewer(ctx.Param("token"))
	if !ok {
		return reviewUnavailable(snapshot), nil
	}
	reviewer, found := findReviewer(snapshot, reviewerID)
	if !found || !reviewer.Active() || reviewer.Kind != "human" {
		return reviewUnavailable(snapshot), nil
	}

	plan, err := snapshot.ActiveReviewPlan()
	if err != nil {
		return map[string]any{
			"available":       true,
			"workspace":       present.WorkspaceIdentity(snapshot),
			"reviewer":        reviewerSummary(reviewer),
			"hasActiveRound":  false,
			"submissions":     []map[string]any{},
			"submissionCount": 0,
		}, nil
	}

	submissions := reviewerSubmissionRows(snapshot, plan, reviewer)
	return map[string]any{
		"available":      true,
		"workspace":      present.WorkspaceIdentity(snapshot),
		"reviewer":       reviewerSummary(reviewer),
		"hasActiveRound": true,
		"plan": map[string]any{
			"id":           plan.ID,
			"name":         plan.Name,
			"round":        plan.Round,
			"instructions": plan.Instructions,
			"due":          present.DateTime(plan.DueAt),
			"anonymous":    plan.Anonymous,
		},
		"submissions":     submissions,
		"submissionCount": len(submissions),
	}, nil
}

// reviewUnavailable is the identical friendly response for a missing or
// invalid token and an unknown or non-human reviewer ID. It still carries
// "workspace" so app/layout.gsx's global nav renders a live Submit and
// public-agenda link on this page too, not the fixed demo slug.
func reviewUnavailable(state domain.State) map[string]any {
	return map[string]any{"available": false, "workspace": present.WorkspaceIdentity(state)}
}

func reviewerSummary(reviewer domain.Reviewer) map[string]any {
	return map[string]any{"id": reviewer.ID, "name": reviewer.Name, "initials": reviewerInitials(reviewer.Name)}
}

// reviewerSubmissionRows lists the active plan's assigned proposals for
// reviewer, each carrying its own rubric score inputs pre-filled from any
// evaluation this reviewer already recorded. It returns an empty slice
// (never an error) when the plan does not assign this reviewer at all, so
// the page renders a normal "nothing assigned yet" empty state rather than
// a not-found page — the reviewer's identity is still valid.
//
// Anonymous plans (ReviewPlan.Anonymous) hide speaker names here, matching
// the same rule the human rubric review already observes for round one.
func reviewerSubmissionRows(state domain.State, plan domain.ReviewPlan, reviewer domain.Reviewer) []map[string]any {
	if !containsID(plan.ReviewerIDs, reviewer.ID) {
		return []map[string]any{}
	}
	rows := make([]map[string]any, 0, len(plan.SubmissionIDs))
	for _, submissionID := range plan.SubmissionIDs {
		submission, found := state.Submission(submissionID)
		if !found {
			continue
		}
		// Recused reviewers receive no proposal card at all. The score action
		// independently rechecks the same fact and policy, so a stale page or
		// forged post cannot bypass recusal.
		if state.ReviewerCompanyConflict(reviewer, *submission) {
			continue
		}
		if plan.AssignmentsManaged && !state.ReviewAssignmentActive(plan.ID, submission.ID, reviewer.ID) {
			continue
		}
		existing, hasExisting := reviewerEvaluation(state, plan.ID, submission.ID, reviewer.ID)
		speaker := "Hidden for anonymous review"
		if !plan.Anonymous {
			speaker = present.SpeakerNames(state, submission.SpeakerIDs)
		}
		rows = append(rows, map[string]any{
			"id":             submission.ID,
			"title":          submission.Title,
			"abstract":       submission.Abstract,
			"format":         submission.Format,
			"level":          submission.Level,
			"category":       present.CategoryName(state, submission.CategoryID),
			"speaker":        speaker,
			"scores":         scoreInputRows(plan, existing),
			"comments":       existing.Comments,
			"recommendation": existing.Recommendation,
			"reviewed":       hasExisting,
			"updated":        present.DateTime(existing.UpdatedAt),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["title"].(string) < rows[j]["title"].(string) })
	return rows
}

// scoreInputRows builds one row per rubric criterion, carrying the
// reviewer's previously saved score (as a string, ready for an <input
// value=...>) when one exists, so re-opening a review link to edit a score
// shows what was already recorded rather than a blank form.
func scoreInputRows(plan domain.ReviewPlan, existing domain.Evaluation) []map[string]any {
	rows := make([]map[string]any, 0, len(plan.Criteria))
	for _, criterion := range plan.Criteria {
		value := ""
		if score, ok := existing.Scores[criterion.ID]; ok {
			value = strconv.FormatFloat(score, 'f', -1, 64)
		}
		rows = append(rows, map[string]any{
			"id":          criterion.ID,
			"name":        criterion.Name,
			"description": criterion.Description,
			"weight":      fmt.Sprintf("%.0f%%", criterion.Weight),
			"max":         criterion.MaxScore,
			"value":       value,
		})
	}
	return rows
}

func reviewerEvaluation(state domain.State, planID, submissionID, reviewerID string) (domain.Evaluation, bool) {
	for _, evaluation := range state.Evaluations {
		if evaluation.PlanID == planID && evaluation.SubmissionID == submissionID && evaluation.ReviewerID == reviewerID && evaluation.Source == "human" {
			return evaluation, true
		}
	}
	return domain.Evaluation{}, false
}

func findReviewer(state domain.State, id string) (domain.Reviewer, bool) {
	for _, reviewer := range state.Reviewers {
		if reviewer.ID == id {
			return reviewer, true
		}
	}
	return domain.Reviewer{}, false
}

func containsID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// reviewerInitials mirrors present's unexported nameInitials
// (internal/present/review.go): two-letter initials from a display name,
// duplicated here in ~8 lines because present's helper is unexported and
// this package does not own internal/present.
func reviewerInitials(name string) string {
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

// reviewScoreEligibility performs the current-state authorization checks for
// a reviewer score. It is intentionally called both before and inside the
// mutation: the preflight gives the reviewer immediate feedback, while the
// transaction-time call makes roster, assignment, and recusal changes take
// effect before any score is persisted.
func reviewScoreEligibility(ctx *action.Context, state domain.State, planID, submissionID, reviewerID string, engine *decisionrules.Engine) (domain.ReviewPlan, decisionrules.ReviewGovernanceDecision, error) {
	plan, found := state.ReviewPlan(planID)
	if !found || (plan.Status != "active" && plan.Status != "open") {
		return domain.ReviewPlan{}, decisionrules.ReviewGovernanceDecision{}, action.Validation("This review round is no longer open.", map[string]string{"plan_id": "The review plan is not active."}, ctx.FormData)
	}
	submission, found := state.Submission(submissionID)
	if !found || !containsID(plan.SubmissionIDs, submissionID) {
		return domain.ReviewPlan{}, decisionrules.ReviewGovernanceDecision{}, action.Validation("Choose a proposal assigned to this round.", map[string]string{"submission_id": "Proposal is not assigned."}, ctx.FormData)
	}
	reviewer, reviewerFound := findReviewer(state, reviewerID)
	if !reviewerFound || !reviewer.Active() || reviewer.Kind != "human" || !containsID(plan.ReviewerIDs, reviewerID) {
		return domain.ReviewPlan{}, decisionrules.ReviewGovernanceDecision{}, action.Validation("Your reviewer link is not assigned to this round.", nil, ctx.FormData)
	}
	if plan.AssignmentsManaged && !state.ReviewAssignmentActive(planID, submissionID, reviewerID) {
		return domain.ReviewPlan{}, decisionrules.ReviewGovernanceDecision{}, action.Validation("This proposal is not assigned to your reviewer link.", nil, ctx.FormData)
	}
	governance, err := engine.EvaluateReviewGovernance(decisionrules.ReviewGovernanceInput{
		Operation:       "score",
		CompanyConflict: state.ReviewerCompanyConflict(reviewer, *submission),
	})
	if err != nil {
		return domain.ReviewPlan{}, decisionrules.ReviewGovernanceDecision{}, fmt.Errorf("evaluate review governance: %w", err)
	}
	if !governance.Allowed {
		return domain.ReviewPlan{}, decisionrules.ReviewGovernanceDecision{}, action.Validation("This review cannot be recorded.", map[string]string{"submission_id": governance.Reason}, ctx.FormData)
	}
	return *plan, governance, nil
}

func parseReviewScoreInput(ctx *action.Context, plan domain.ReviewPlan, recommendation, comments string) (map[string]float64, error) {
	fieldErrors := map[string]string{}
	if !domain.EvaluationRecommendationKnown(recommendation) {
		fieldErrors["recommendation"] = "Choose a recommendation."
	}
	if len([]rune(comments)) < 20 {
		fieldErrors["comments"] = "Add at least 20 characters of decision-relevant context."
	}
	scores := make(map[string]float64, len(plan.Criteria))
	for _, criterion := range plan.Criteria {
		field := "score_" + criterion.ID
		score, err := strconv.ParseFloat(strings.TrimSpace(ctx.FormData[field]), 64)
		if err != nil || math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > criterion.MaxScore {
			fieldErrors[field] = fmt.Sprintf("Use a score from 0 to %.0f.", criterion.MaxScore)
			continue
		}
		scores[criterion.ID] = score
	}
	if len(fieldErrors) > 0 {
		return nil, action.Validation("Complete every rubric criterion.", fieldErrors, ctx.FormData)
	}
	return scores, nil
}

// scoreSubmission is the reviewer-facing counterpart to
// app/organizer/review/page.server.go's saveReview. It applies the
// identical rubric validation and upsert-by-(plan, submission, reviewer)
// logic, with one deliberate difference (the RV-2 fix this whole route
// exists for): reviewerID comes from the verified path token, never from
// ctx.FormData. A posted reviewer_id would be impossible to spoof through
// even if a caller added one, because nothing here reads it.
func scoreSubmission(ctx *action.Context) error {
	reviewerID, ok := token.NewReviewer().VerifyReviewer(ctx.Request.PathValue("token"))
	if !ok {
		return action.Validation("Your review link is no longer valid. Ask the program team for a new one.", nil, ctx.FormData)
	}

	planID := strings.TrimSpace(ctx.FormData["plan_id"])
	submissionID := strings.TrimSpace(ctx.FormData["submission_id"])
	recommendation := strings.TrimSpace(ctx.FormData["recommendation"])
	comments := strings.TrimSpace(ctx.FormData["comments"])

	engine, err := decisionrules.Shared()
	if err != nil {
		return fmt.Errorf("load review governance: %w", err)
	}
	snapshot := appstate.MustGet().Snapshot()
	plan, governance, err := reviewScoreEligibility(ctx, snapshot, planID, submissionID, reviewerID, engine)
	if err != nil {
		return err
	}
	if _, err := parseReviewScoreInput(ctx, plan, recommendation, comments); err != nil {
		return err
	}

	created := false
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      "reviewer:" + reviewerID,
		Action:     "review.scored",
		EntityType: "evaluation",
		EntityID:   planID + ":" + submissionID + ":" + reviewerID,
		Summary:    "human review recorded by assigned reviewer",
		Origin:     "reviewer-link",
		Rule:       governance.Rule,
		Trace:      strings.Join(governance.Trace, "; "),
	}, func(state *domain.State) error {
		currentPlan, _, err := reviewScoreEligibility(ctx, *state, planID, submissionID, reviewerID, engine)
		if err != nil {
			return err
		}
		currentScores, err := parseReviewScoreInput(ctx, currentPlan, recommendation, comments)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for index := range state.Evaluations {
			evaluation := &state.Evaluations[index]
			if evaluation.PlanID == planID && evaluation.SubmissionID == submissionID && evaluation.ReviewerID == reviewerID && evaluation.Source == "human" {
				evaluation.Scores = currentScores
				evaluation.Comments = comments
				evaluation.Recommendation = recommendation
				evaluation.UpdatedAt = now
				return nil
			}
		}
		state.Evaluations = append(state.Evaluations, domain.Evaluation{
			ID: domain.NewID("eval"), PlanID: planID, SubmissionID: submissionID, ReviewerID: reviewerID,
			Scores: currentScores, Comments: comments, Recommendation: recommendation, Source: "human", CreatedAt: now, UpdatedAt: now,
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
	session.AddFlash(ctx.Request, "notice", verb+" your review of this proposal.")
	live.Broadcast("review:updated", map[string]string{"submission": submissionID, "source": "human"})
	actionflow.Redirect(ctx, "/review/"+ctx.Request.PathValue("token")+"#submissions")
	return nil
}
