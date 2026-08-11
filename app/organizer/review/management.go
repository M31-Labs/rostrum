package review

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	decisionrules "github.com/m31-labs/rostrum/rules"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/session"
)

const defaultRubric = "relevance|Relevance|Fit for the event audience and theme.|25|5\nclarity|Clarity|Problem, approach, and takeaway are specific.|25|5\nevidence|Evidence|Claims are supported by lived or measured results.|25|5\naudience_value|Audience value|Creates an actionable attendee outcome.|25|5"

type reviewPlanInput struct {
	Name               string
	Round              int
	Status             string
	Instructions       string
	DueAt              time.Time
	Anonymous          bool
	WeeklyReminders    bool
	IncludeFiles       bool
	EvaluationsPerItem int
	Criteria           []domain.RubricCriterion
}

// createReviewPlan starts a draft with an explicit, validated rubric. A draft
// has no hidden routing side effect; a chair deliberately opens it only after
// its roster and proposal set are ready.
func createReviewPlan(ctx *action.Context) error {
	input, fieldErrors := parseReviewPlanInput(ctx.FormData, appstate.MustGet().Snapshot().Event.TimeZone)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the review-plan details.", fieldErrors, ctx.FormData)
	}
	planID := domain.NewID("plan")
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.plan_created",
		EntityType: "review_plan",
		EntityID:   planID,
		Summary:    "Created a draft review plan with a validated rubric.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		if err := ensureOpenPlanAvailable(*state, "", input.Status); err != nil {
			return action.Validation("Only one review plan may be open at a time.", map[string]string{"status": err.Error()}, ctx.FormData)
		}
		now := time.Now().UTC()
		state.ReviewPlans = append(state.ReviewPlans, domain.ReviewPlan{
			ID:                 planID,
			Name:               input.Name,
			Round:              input.Round,
			Status:             input.Status,
			Instructions:       input.Instructions,
			DueAt:              input.DueAt,
			Anonymous:          input.Anonymous,
			WeeklyReminders:    input.WeeklyReminders,
			IncludeFiles:       input.IncludeFiles,
			EvaluationsPerItem: input.EvaluationsPerItem,
			Criteria:           input.Criteria,
			CreatedAt:          now,
			UpdatedAt:          now,
		})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Created draft review plan “"+input.Name+"”.")
	live.Broadcast("review:plan-created", map[string]string{"plan": planID})
	actionflow.Redirect(ctx, "/organizer/review#plan-manager")
	return nil
}

// updateReviewPlan preserves interpretation of already-recorded scores:
// operational fields may change, but a scored rubric is locked. Create a new
// round when the meaning of a criterion needs to change.
func updateReviewPlan(ctx *action.Context) error {
	planID := strings.TrimSpace(ctx.FormData["plan_id"])
	snapshot := appstate.MustGet().Snapshot()
	if _, found := snapshot.ReviewPlan(planID); !found {
		return action.Validation("Choose an existing review plan.", map[string]string{"plan_id": "Review plan not found."}, ctx.FormData)
	}
	input, fieldErrors := parseReviewPlanInput(ctx.FormData, snapshot.Event.TimeZone)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the review-plan details.", fieldErrors, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.plan_updated",
		EntityType: "review_plan",
		EntityID:   planID,
		Summary:    "Updated review-plan operations without deleting historical scores.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		plan, found := state.ReviewPlan(planID)
		if !found {
			return action.Validation("Choose an existing review plan.", map[string]string{"plan_id": "Review plan not found."}, ctx.FormData)
		}
		if err := ensureOpenPlanAvailable(*state, planID, input.Status); err != nil {
			return action.Validation("Only one review plan may be open at a time.", map[string]string{"status": err.Error()}, ctx.FormData)
		}
		if planHasEvaluation(*state, planID) && !sameCriteria(plan.Criteria, input.Criteria) {
			return action.Validation("A scored rubric is immutable.", map[string]string{"rubric": "Create a new review round to change rubric criteria, weights, or score ranges."}, ctx.FormData)
		}
		plan.Name = input.Name
		plan.Round = input.Round
		plan.Status = input.Status
		plan.Instructions = input.Instructions
		plan.DueAt = input.DueAt
		plan.Anonymous = input.Anonymous
		plan.WeeklyReminders = input.WeeklyReminders
		plan.IncludeFiles = input.IncludeFiles
		plan.EvaluationsPerItem = input.EvaluationsPerItem
		plan.Criteria = input.Criteria
		plan.UpdatedAt = time.Now().UTC()
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Updated review plan “"+input.Name+"”.")
	live.Broadcast("review:plan-updated", map[string]string{"plan": planID})
	actionflow.Redirect(ctx, "/organizer/review#plan-manager")
	return nil
}

func createReviewer(ctx *action.Context) error {
	input, fieldErrors := parseReviewerInput(ctx.FormData)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the reviewer details.", fieldErrors, ctx.FormData)
	}
	reviewerID := domain.NewID("rev")
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.reviewer_created",
		EntityType: "reviewer",
		EntityID:   reviewerID,
		Summary:    "Added a reviewer to the program roster.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		if reviewerEmailTaken(*state, input.Email, "") {
			return action.Validation("Use a reviewer email that is not already in the roster.", map[string]string{"email": "That email is already in use."}, ctx.FormData)
		}
		now := time.Now().UTC()
		state.Reviewers = append(state.Reviewers, domain.Reviewer{
			ID: reviewerID, Name: input.Name, Email: input.Email, Company: input.Company,
			Expertise: input.Expertise, Kind: input.Kind, CreatedAt: now, UpdatedAt: now,
		})
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Added reviewer “"+input.Name+"”.")
	live.Broadcast("review:reviewer-created", map[string]string{"reviewer": reviewerID})
	actionflow.Redirect(ctx, "/organizer/review#reviewer-manager")
	return nil
}

func updateReviewer(ctx *action.Context) error {
	reviewerID := strings.TrimSpace(ctx.FormData["reviewer_id"])
	input, fieldErrors := parseReviewerInput(ctx.FormData)
	if len(fieldErrors) > 0 {
		return action.Validation("Correct the reviewer details.", fieldErrors, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.reviewer_updated",
		EntityType: "reviewer",
		EntityID:   reviewerID,
		Summary:    "Updated an active reviewer profile.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		reviewer, found := state.Reviewer(reviewerID)
		if !found || !reviewer.Active() {
			return action.Validation("This reviewer is no longer active.", map[string]string{"reviewer_id": "Choose an active reviewer."}, ctx.FormData)
		}
		if reviewerEmailTaken(*state, input.Email, reviewerID) {
			return action.Validation("Use a reviewer email that is not already in the roster.", map[string]string{"email": "That email is already in use."}, ctx.FormData)
		}
		if reviewerHasEvaluation(*state, reviewerID) && (reviewer.Name != input.Name || reviewer.Email != input.Email || reviewer.Company != input.Company || reviewer.Kind != input.Kind) {
			return action.Validation("A scored reviewer identity is immutable.", map[string]string{"name": "Retire this reviewer and add a replacement instead."}, ctx.FormData)
		}
		reviewer.Name = input.Name
		reviewer.Email = input.Email
		reviewer.Company = input.Company
		reviewer.Expertise = input.Expertise
		reviewer.Kind = input.Kind
		reviewer.UpdatedAt = time.Now().UTC()
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Updated reviewer “"+input.Name+"”.")
	live.Broadcast("review:reviewer-updated", map[string]string{"reviewer": reviewerID})
	actionflow.Redirect(ctx, "/organizer/review#reviewer-manager")
	return nil
}

// retireReviewer removes a reviewer from open rosters and deactivates their
// current assignments. It deliberately leaves evaluations and historical
// assignments in place, so a roster cleanup can never rewrite who scored.
func retireReviewer(ctx *action.Context) error {
	reviewerID := strings.TrimSpace(ctx.FormData["reviewer_id"])
	name := ""
	removedAssignments := 0
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.reviewer_retired",
		EntityType: "reviewer",
		EntityID:   reviewerID,
		Summary:    "Retired reviewer and preserved existing scores and provenance.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		reviewer, found := state.Reviewer(reviewerID)
		if !found || !reviewer.Active() {
			return action.Validation("This reviewer is already inactive.", map[string]string{"reviewer_id": "Choose an active reviewer."}, ctx.FormData)
		}
		now := time.Now().UTC()
		for index := range state.ReviewPlans {
			plan := &state.ReviewPlans[index]
			if plan.Status == "active" || plan.Status == "open" {
				plan.ReviewerIDs = withoutReviewID(plan.ReviewerIDs, reviewerID)
				plan.UpdatedAt = now
			}
		}
		for index := range state.ReviewAssignments {
			assignment := &state.ReviewAssignments[index]
			if assignment.ReviewerID == reviewerID && assignment.Active() {
				assignment.RemovedAt = now
				assignment.RemovalReason = "reviewer retired"
				removedAssignments++
			}
		}
		reviewer.RetiredAt = now
		reviewer.UpdatedAt = now
		name = reviewer.Name
		return nil
	}); err != nil {
		return err
	}
	message := "Retired reviewer “" + name + "”; historical scores remain."
	if removedAssignments > 0 {
		message = fmt.Sprintf("Retired reviewer “%s” and deactivated %d current assignment(s); historical scores remain.", name, removedAssignments)
	}
	session.AddFlash(ctx.Request, "notice", message)
	live.Broadcast("review:reviewer-retired", map[string]string{"reviewer": reviewerID})
	actionflow.Redirect(ctx, "/organizer/review#reviewer-manager")
	return nil
}

func addPlanReviewer(ctx *action.Context) error {
	planID := strings.TrimSpace(ctx.FormData["plan_id"])
	reviewerID := strings.TrimSpace(ctx.FormData["reviewer_id"])
	if planID == "" || reviewerID == "" {
		return action.Validation("Choose a review plan and reviewer.", map[string]string{"reviewer_id": "Select a reviewer."}, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.roster_added",
		EntityType: "review_plan",
		EntityID:   planID + ":" + reviewerID,
		Summary:    "Added reviewer to review-plan roster.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		plan, found := state.ReviewPlan(planID)
		if !found || plan.Status == "closed" {
			return action.Validation("Choose a draft or open review plan.", map[string]string{"plan_id": "Closed plans cannot change roster."}, ctx.FormData)
		}
		reviewer, found := state.Reviewer(reviewerID)
		if !found || !reviewer.Active() {
			return action.Validation("Choose an active reviewer.", map[string]string{"reviewer_id": "Reviewer is inactive."}, ctx.FormData)
		}
		if containsReviewID(plan.ReviewerIDs, reviewerID) {
			return action.Validation("That reviewer is already on the roster.", map[string]string{"reviewer_id": "Already included."}, ctx.FormData)
		}
		plan.ReviewerIDs = append(plan.ReviewerIDs, reviewerID)
		plan.UpdatedAt = time.Now().UTC()
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Added reviewer to the plan roster.")
	live.Broadcast("review:roster-added", map[string]string{"plan": planID, "reviewer": reviewerID})
	actionflow.Redirect(ctx, "/organizer/review#plan-manager")
	return nil
}

func removePlanReviewer(ctx *action.Context) error {
	planID := strings.TrimSpace(ctx.FormData["plan_id"])
	reviewerID := strings.TrimSpace(ctx.FormData["reviewer_id"])
	removedAssignments := 0
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.roster_removed",
		EntityType: "review_plan",
		EntityID:   planID + ":" + reviewerID,
		Summary:    "Removed reviewer from active roster while preserving scores.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		plan, found := state.ReviewPlan(planID)
		if !found || plan.Status == "closed" || !containsReviewID(plan.ReviewerIDs, reviewerID) {
			return action.Validation("This reviewer is not on an editable roster.", map[string]string{"reviewer_id": "Choose a current roster member."}, ctx.FormData)
		}
		now := time.Now().UTC()
		plan.ReviewerIDs = withoutReviewID(plan.ReviewerIDs, reviewerID)
		plan.UpdatedAt = now
		for index := range state.ReviewAssignments {
			assignment := &state.ReviewAssignments[index]
			if assignment.PlanID == planID && assignment.ReviewerID == reviewerID && assignment.Active() {
				assignment.RemovedAt = now
				assignment.RemovalReason = "removed from plan roster"
				removedAssignments++
			}
		}
		return nil
	}); err != nil {
		return err
	}
	message := "Removed reviewer from the plan roster; recorded scores remain."
	if removedAssignments > 0 {
		message = fmt.Sprintf("Removed reviewer and deactivated %d current assignment(s); recorded scores remain.", removedAssignments)
	}
	session.AddFlash(ctx.Request, "notice", message)
	live.Broadcast("review:roster-removed", map[string]string{"plan": planID, "reviewer": reviewerID})
	actionflow.Redirect(ctx, "/organizer/review#plan-manager")
	return nil
}

// autoAssignReviewers turns a plan's legacy shared pool into explicit,
// balanced reviewer/proposal assignments. It backfills provenance for
// existing human scores, honors Arbiter recusal policy for every new
// assignment, and is idempotent on repeat.
func autoAssignReviewers(ctx *action.Context) error {
	planID := strings.TrimSpace(ctx.FormData["plan_id"])
	assigned, recused := 0, 0
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.assignments_balanced",
		EntityType: "review_plan",
		EntityID:   planID,
		Summary:    "Created explicit balanced reviewer assignments with provenance.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		var err error
		assigned, recused, err = buildBalancedAssignments(state, planID, reviewActor(ctx))
		return err
	}); err != nil {
		return err
	}
	message := "No additional eligible reviewer assignments were needed."
	if assigned > 0 {
		message = fmt.Sprintf("Created %d explicit reviewer assignment(s).", assigned)
	}
	if recused > 0 {
		message += fmt.Sprintf(" %d conflict/recusal pairing(s) were excluded.", recused)
	}
	session.AddFlash(ctx.Request, "notice", message)
	live.Broadcast("review:assignments-balanced", map[string]string{"plan": planID})
	actionflow.Redirect(ctx, "/organizer/review#plan-manager")
	return nil
}

func assignReview(ctx *action.Context) error {
	planID := strings.TrimSpace(ctx.FormData["plan_id"])
	submissionID := strings.TrimSpace(ctx.FormData["submission_id"])
	reviewerID := strings.TrimSpace(ctx.FormData["reviewer_id"])
	if planID == "" || submissionID == "" || reviewerID == "" {
		return action.Validation("Choose a plan, proposal, and reviewer.", map[string]string{"reviewer_id": "All assignment fields are required."}, ctx.FormData)
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.assignment_created",
		EntityType: "review_assignment",
		EntityID:   planID + ":" + submissionID + ":" + reviewerID,
		Summary:    "Created an explicit manual review assignment.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		return createReviewAssignment(state, planID, submissionID, reviewerID, "manual", reviewActor(ctx))
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Assigned reviewer to proposal.")
	live.Broadcast("review:assignment-created", map[string]string{"plan": planID, "submission": submissionID, "reviewer": reviewerID})
	actionflow.Redirect(ctx, "/organizer/review#plan-manager")
	return nil
}

// unassignReview performs a non-destructive reassignment step. The history of
// the assignment and any score remains; a replacement reviewer can then be
// assigned manually or by the balanced assignment action.
func unassignReview(ctx *action.Context) error {
	assignmentID := strings.TrimSpace(ctx.FormData["assignment_id"])
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      reviewActor(ctx),
		Action:     "review.assignment_removed",
		EntityType: "review_assignment",
		EntityID:   assignmentID,
		Summary:    "Deactivated a review assignment without deleting historical score evidence.",
		Origin:     "organizer-review",
	}, func(state *domain.State) error {
		for index := range state.ReviewAssignments {
			assignment := &state.ReviewAssignments[index]
			if assignment.ID != assignmentID {
				continue
			}
			if !assignment.Active() {
				return action.Validation("That assignment is already inactive.", map[string]string{"assignment_id": "Already reassigned or removed."}, ctx.FormData)
			}
			assignment.RemovedAt = time.Now().UTC()
			assignment.RemovalReason = "manual reassignment"
			return nil
		}
		return action.Validation("Choose an active assignment.", map[string]string{"assignment_id": "Assignment not found."}, ctx.FormData)
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Removed the active assignment; its history and any score remain.")
	live.Broadcast("review:assignment-removed", map[string]string{"assignment": assignmentID})
	actionflow.Redirect(ctx, "/organizer/review#plan-manager")
	return nil
}

func buildBalancedAssignments(state *domain.State, planID, actor string) (assigned, recused int, err error) {
	plan, found := state.ReviewPlan(planID)
	if !found || (plan.Status != "active" && plan.Status != "open") {
		return 0, 0, action.Validation("Open a review plan before assigning reviewers.", map[string]string{"plan_id": "Choose an active review plan."}, nil)
	}
	engine, err := decisionrules.Shared()
	if err != nil {
		return 0, 0, fmt.Errorf("load review governance: %w", err)
	}
	// Preserve assignment provenance for existing scores first. A legacy score
	// is never deleted or recast as a new automatic decision.
	now := time.Now().UTC()
	for _, evaluation := range state.Evaluations {
		if evaluation.PlanID != plan.ID || evaluation.Source != "human" || evaluation.ReviewerID == "" || state.ReviewAssignmentActive(plan.ID, evaluation.SubmissionID, evaluation.ReviewerID) {
			continue
		}
		state.ReviewAssignments = append(state.ReviewAssignments, domain.ReviewAssignment{
			ID: domain.NewID("assign"), PlanID: plan.ID, SubmissionID: evaluation.SubmissionID, ReviewerID: evaluation.ReviewerID,
			Source: "legacy", Actor: "migration:managed-assignments", AssignedAt: evaluation.CreatedAt,
		})
	}
	load := map[string]int{}
	for _, assignment := range state.ReviewAssignments {
		if assignment.PlanID == plan.ID && assignment.Active() {
			load[assignment.ReviewerID]++
		}
	}
	for _, submissionID := range plan.SubmissionIDs {
		submission, found := state.Submission(submissionID)
		if !found {
			continue
		}
		for state.ActiveReviewAssignmentCount(plan.ID, submissionID) < plan.EvaluationsPerItem {
			candidates := make([]assignmentCandidate, 0, len(plan.ReviewerIDs))
			for _, reviewerID := range plan.ReviewerIDs {
				reviewer, found := state.Reviewer(reviewerID)
				if !found || !reviewer.Active() || reviewer.Kind != "human" || state.ReviewAssignmentActive(plan.ID, submissionID, reviewerID) {
					continue
				}
				decision, decisionErr := engine.EvaluateReviewGovernance(decisionrules.ReviewGovernanceInput{
					Operation:       "score",
					CompanyConflict: state.ReviewerCompanyConflict(*reviewer, *submission),
				})
				if decisionErr != nil {
					return assigned, recused, decisionErr
				}
				if !decision.Allowed {
					recused++
					continue
				}
				candidates = append(candidates, assignmentCandidate{reviewerID: reviewerID, load: load[reviewerID], rule: decision.Rule, trace: decision.Trace})
			}
			if len(candidates) == 0 {
				break
			}
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].load == candidates[j].load {
					return candidates[i].reviewerID < candidates[j].reviewerID
				}
				return candidates[i].load < candidates[j].load
			})
			candidate := candidates[0]
			state.ReviewAssignments = append(state.ReviewAssignments, domain.ReviewAssignment{
				ID: domain.NewID("assign"), PlanID: plan.ID, SubmissionID: submissionID, ReviewerID: candidate.reviewerID,
				Source: "automatic", Actor: actor, Rule: candidate.rule, Trace: append([]string(nil), candidate.trace...), AssignedAt: now,
			})
			load[candidate.reviewerID]++
			assigned++
		}
	}
	plan.AssignmentsManaged = true
	plan.UpdatedAt = now
	return assigned, recused, nil
}

type assignmentCandidate struct {
	reviewerID string
	load       int
	rule       string
	trace      []string
}

func createReviewAssignment(state *domain.State, planID, submissionID, reviewerID, source, actor string) error {
	plan, found := state.ReviewPlan(planID)
	if !found || (plan.Status != "active" && plan.Status != "open") {
		return action.Validation("Choose an active review plan.", map[string]string{"plan_id": "Review plan is not open."}, nil)
	}
	if !containsReviewID(plan.SubmissionIDs, submissionID) {
		return action.Validation("Choose a proposal assigned to this plan.", map[string]string{"submission_id": "Proposal is not in this plan."}, nil)
	}
	reviewer, found := state.Reviewer(reviewerID)
	if !found || !reviewer.Active() || reviewer.Kind != "human" || !containsReviewID(plan.ReviewerIDs, reviewerID) {
		return action.Validation("Choose an active human reviewer on this roster.", map[string]string{"reviewer_id": "Reviewer is not eligible."}, nil)
	}
	submission, found := state.Submission(submissionID)
	if !found {
		return action.Validation("Choose an existing proposal.", map[string]string{"submission_id": "Proposal not found."}, nil)
	}
	if state.ReviewAssignmentActive(planID, submissionID, reviewerID) {
		return action.Validation("That reviewer already has this proposal.", map[string]string{"reviewer_id": "Already assigned."}, nil)
	}
	engine, err := decisionrules.Shared()
	if err != nil {
		return fmt.Errorf("load review governance: %w", err)
	}
	decision, err := engine.EvaluateReviewGovernance(decisionrules.ReviewGovernanceInput{
		Operation:       "score",
		CompanyConflict: state.ReviewerCompanyConflict(*reviewer, *submission),
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return action.Validation("This reviewer cannot be assigned to that proposal.", map[string]string{"reviewer_id": decision.Reason}, nil)
	}
	state.ReviewAssignments = append(state.ReviewAssignments, domain.ReviewAssignment{
		ID: domain.NewID("assign"), PlanID: planID, SubmissionID: submissionID, ReviewerID: reviewerID,
		Source: source, Actor: actor, Rule: decision.Rule, Trace: append([]string(nil), decision.Trace...), AssignedAt: time.Now().UTC(),
	})
	plan.AssignmentsManaged = true
	plan.UpdatedAt = time.Now().UTC()
	return nil
}

type reviewerInput struct {
	Name      string
	Email     string
	Company   string
	Expertise []string
	Kind      string
}

func parseReviewerInput(values map[string]string) (reviewerInput, map[string]string) {
	input := reviewerInput{
		Name:      strings.TrimSpace(values["name"]),
		Email:     strings.ToLower(strings.TrimSpace(values["email"])),
		Company:   strings.TrimSpace(values["company"]),
		Expertise: splitExpertise(values["expertise"]),
		Kind:      strings.ToLower(strings.TrimSpace(values["kind"])),
	}
	fieldErrors := map[string]string{}
	if input.Name == "" || len([]rune(input.Name)) > 160 {
		fieldErrors["name"] = "Use a reviewer name of 1–160 characters."
	}
	if input.Kind != "human" && input.Kind != "virtual" {
		fieldErrors["kind"] = "Choose human or virtual."
	}
	if input.Kind == "human" && !strings.Contains(input.Email, "@") {
		fieldErrors["email"] = "Enter a valid reviewer email."
	}
	if len(input.Expertise) > 12 {
		fieldErrors["expertise"] = "Use no more than 12 expertise tags."
	}
	return input, fieldErrors
}

func parseReviewPlanInput(values map[string]string, timezone string) (reviewPlanInput, map[string]string) {
	input := reviewPlanInput{
		Name:            strings.TrimSpace(values["name"]),
		Status:          strings.ToLower(strings.TrimSpace(values["status"])),
		Instructions:    strings.TrimSpace(values["instructions"]),
		Anonymous:       checkbox(values["anonymous"]),
		WeeklyReminders: checkbox(values["weekly_reminders"]),
		IncludeFiles:    checkbox(values["include_files"]),
	}
	fieldErrors := map[string]string{}
	round, err := strconv.Atoi(strings.TrimSpace(values["round"]))
	if err != nil || round < 1 || round > 99 {
		fieldErrors["round"] = "Use a round number from 1–99."
	} else {
		input.Round = round
	}
	if input.Name == "" || len([]rune(input.Name)) > 160 {
		fieldErrors["name"] = "Use a plan name of 1–160 characters."
	}
	if input.Status == "" {
		input.Status = "draft"
	}
	if input.Status != "draft" && input.Status != "open" && input.Status != "active" && input.Status != "closed" {
		fieldErrors["status"] = "Choose draft, open, or closed."
	}
	if len([]rune(input.Instructions)) > 4_000 {
		fieldErrors["instructions"] = "Keep instructions to 4,000 characters or fewer."
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	dueAt, err := time.ParseInLocation("2006-01-02T15:04", strings.TrimSpace(values["due_at"]), location)
	if err != nil || dueAt.IsZero() {
		fieldErrors["due_at"] = "Choose a valid due date and time."
	} else {
		input.DueAt = dueAt
	}
	target, err := strconv.Atoi(strings.TrimSpace(values["evaluations_per_item"]))
	if err != nil || target < 1 || target > 20 {
		fieldErrors["evaluations_per_item"] = "Use 1–20 evaluations per proposal."
	} else {
		input.EvaluationsPerItem = target
	}
	criteria, criteriaErr := parseRubric(values["rubric"])
	if criteriaErr != nil {
		fieldErrors["rubric"] = criteriaErr.Error()
	} else {
		input.Criteria = criteria
	}
	return input, fieldErrors
}

func parseRubric(raw string) ([]domain.RubricCriterion, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 1 || len(lines) > 8 {
		return nil, fmt.Errorf("Use 1–8 rubric lines.")
	}
	criteria := make([]domain.RubricCriterion, 0, len(lines))
	ids := map[string]bool{}
	weight := 0.0
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) != 5 {
			return nil, fmt.Errorf("Each rubric line must be id|name|description|weight|max.")
		}
		id := strings.ToLower(strings.TrimSpace(parts[0]))
		if !validRubricID(id) || ids[id] {
			return nil, fmt.Errorf("Use unique lowercase rubric IDs with letters, numbers, underscores, or hyphens.")
		}
		name := strings.TrimSpace(parts[1])
		description := strings.TrimSpace(parts[2])
		itemWeight, weightErr := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		maxScore, scoreErr := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if name == "" || len([]rune(name)) > 100 || len([]rune(description)) > 1_000 || weightErr != nil || itemWeight <= 0 || scoreErr != nil || maxScore <= 0 || maxScore > 10 {
			return nil, fmt.Errorf("Each rubric line needs a name, valid description, positive weight, and max score up to 10.")
		}
		ids[id] = true
		weight += itemWeight
		criteria = append(criteria, domain.RubricCriterion{ID: id, Name: name, Description: description, Weight: itemWeight, MaxScore: maxScore})
	}
	if weight < 99.99 || weight > 100.01 {
		return nil, fmt.Errorf("Rubric weights must total 100.")
	}
	return criteria, nil
}

func validRubricID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, runeValue := range value {
		if !(runeValue >= 'a' && runeValue <= 'z') && !(runeValue >= '0' && runeValue <= '9') && runeValue != '_' && runeValue != '-' {
			return false
		}
	}
	return true
}

func sameCriteria(left, right []domain.RubricCriterion) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func ensureOpenPlanAvailable(state domain.State, currentID, status string) error {
	if status != "open" && status != "active" {
		return nil
	}
	for _, plan := range state.ReviewPlans {
		if plan.ID != currentID && (plan.Status == "open" || plan.Status == "active") {
			return fmt.Errorf("“%s” is already open", plan.Name)
		}
	}
	return nil
}

func reviewerEmailTaken(state domain.State, email, skipID string) bool {
	if email == "" {
		return false
	}
	for _, reviewer := range state.Reviewers {
		if reviewer.ID != skipID && strings.EqualFold(strings.TrimSpace(reviewer.Email), email) {
			return true
		}
	}
	return false
}

func reviewerHasEvaluation(state domain.State, reviewerID string) bool {
	for _, evaluation := range state.Evaluations {
		if evaluation.ReviewerID == reviewerID {
			return true
		}
	}
	return false
}

func planHasEvaluation(state domain.State, planID string) bool {
	for _, evaluation := range state.Evaluations {
		if evaluation.PlanID == planID {
			return true
		}
	}
	return false
}

func withoutReviewID(ids []string, want string) []string {
	result := ids[:0]
	for _, id := range ids {
		if id != want {
			result = append(result, id)
		}
	}
	return result
}

func splitExpertise(raw string) []string {
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func checkbox(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func reviewActor(ctx *action.Context) string {
	if user, ok := auth.Current(ctx.Request); ok && strings.TrimSpace(user.ID) != "" {
		return "organizer:" + strings.TrimSpace(user.ID)
	}
	return "organizer"
}
