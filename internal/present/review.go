package present

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func Review(state domain.State) map[string]any {
	plans := make([]map[string]any, 0, len(state.ReviewPlans))
	for _, plan := range state.ReviewPlans {
		completed := humanEvaluations(evaluationsForPlan(state, plan.ID))
		expected := len(plan.SubmissionIDs) * plan.EvaluationsPerItem
		activeAssignments := activeAssignmentsForPlan(state, plan.ID)
		criteria := make([]map[string]any, 0, len(plan.Criteria))
		for _, criterion := range plan.Criteria {
			criteria = append(criteria, map[string]any{
				"id":          criterion.ID,
				"name":        criterion.Name,
				"description": criterion.Description,
				"weight":      fmt.Sprintf("%.0f%%", criterion.Weight),
				"max":         criterion.MaxScore,
			})
		}
		plans = append(plans, map[string]any{
			"id":                  plan.ID,
			"round":               plan.Round,
			"name":                plan.Name,
			"status":              StatusLabel(plan.Status),
			"statusValue":         plan.Status,
			"tone":                StatusTone(plan.Status),
			"due":                 DateTime(plan.DueAt),
			"dueInput":            reviewInputDate(state, plan.DueAt),
			"instructions":        plan.Instructions,
			"anonymous":           plan.Anonymous,
			"weeklyReminders":     plan.WeeklyReminders,
			"includeFiles":        plan.IncludeFiles,
			"reviewers":           len(plan.ReviewerIDs),
			"proposals":           len(plan.SubmissionIDs),
			"completed":           len(completed),
			"expected":            expected,
			"coverage":            Percent(len(completed), expected),
			"coverageStyle":       fmt.Sprintf("%d%%", Percent(len(completed), expected)),
			"criteria":            criteria,
			"rubric":              rubricInput(plan.Criteria),
			"evaluationsPerItem":  plan.EvaluationsPerItem,
			"assignmentsManaged":  plan.AssignmentsManaged,
			"activeAssignments":   len(activeAssignments),
			"unfilledAssignments": maxInt(0, expected-len(activeAssignments)),
			"recusalConflicts":    planRecusalConflictCount(state, plan),
			"hasScores":           len(completed) > 0,
			"editable":            plan.Status != "closed",
			"roster":              planRosterRows(state, plan),
			"assignments":         assignmentRows(state, activeAssignments),
		})
	}

	// A fresh workspace has no review plan yet; fall back to a zero plan so the
	// page renders its empty state instead of panicking on an empty slice.
	var active domain.ReviewPlan
	hasActivePlan := false
	if candidate, err := state.ActiveReviewPlan(); err == nil {
		active = candidate
		hasActivePlan = true
	} else if len(state.ReviewPlans) > 0 {
		active = state.ReviewPlans[len(state.ReviewPlans)-1]
	}
	candidates := make([]map[string]any, 0, len(active.SubmissionIDs))
	for _, submissionID := range active.SubmissionIDs {
		submission, found := state.Submission(submissionID)
		if !found {
			continue
		}
		evaluations := evaluationsFor(state, active.ID, submissionID)
		humanReviews := humanEvaluations(evaluations)
		score := weightedAverage(active, humanReviews)
		recommendations := make([]string, 0, len(humanReviews))
		for _, evaluation := range humanReviews {
			recommendations = append(recommendations, StatusLabel(evaluation.Recommendation))
		}
		candidates = append(candidates, map[string]any{
			"id":              submission.ID,
			"title":           submission.Title,
			"speaker":         SpeakerNames(state, submission.SpeakerIDs),
			"category":        CategoryName(state, submission.CategoryID),
			"evaluationCount": len(humanReviews),
			"targetCount":     active.EvaluationsPerItem,
			"score":           scoreLabel(score, len(humanReviews)),
			"scoreValue":      score,
			"recommendations": strings.Join(recommendations, " · "),
			"status":          StatusLabel(submission.Status),
			"tone":            StatusTone(submission.Status),
			"assignmentCount": state.ActiveReviewAssignmentCount(active.ID, submission.ID),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i]["scoreValue"].(float64) > candidates[j]["scoreValue"].(float64)
	})

	reviewers := make([]map[string]any, 0, len(state.Reviewers))
	humanReviewers := make([]map[string]any, 0, len(state.Reviewers))
	activeReviewers := make([]map[string]any, 0, len(state.Reviewers))
	retiredReviewers := make([]map[string]any, 0)
	for _, reviewer := range state.Reviewers {
		completed := 0
		for _, evaluation := range state.Evaluations {
			if evaluation.ReviewerID == reviewer.ID {
				completed++
			}
		}
		row := map[string]any{
			"id":             reviewer.ID,
			"name":           reviewer.Name,
			"kind":           StatusLabel(reviewer.Kind),
			"kindValue":      reviewer.Kind,
			"email":          reviewer.Email,
			"company":        reviewer.Company,
			"expertise":      strings.Join(reviewer.Expertise, " · "),
			"expertiseInput": strings.Join(reviewer.Expertise, ", "),
			"completed":      completed,
			"initials":       nameInitials(reviewer.Name),
			"active":         reviewer.Active(),
			"retired":        !reviewer.Active(),
			"retiredAt":      DateTime(reviewer.RetiredAt),
		}
		reviewers = append(reviewers, row)
		if reviewer.Active() {
			activeReviewers = append(activeReviewers, row)
		} else {
			retiredReviewers = append(retiredReviewers, row)
		}
		if reviewer.Active() && reviewer.Kind == "human" && containsID(active.ReviewerIDs, reviewer.ID) {
			humanReviewers = append(humanReviewers, row)
		}
	}
	activeCriteria := make([]map[string]any, 0, len(active.Criteria))
	for _, criterion := range active.Criteria {
		activeCriteria = append(activeCriteria, map[string]any{"id": criterion.ID, "name": criterion.Name, "description": criterion.Description, "weight": fmt.Sprintf("%.0f%%", criterion.Weight), "max": criterion.MaxScore})
	}

	return map[string]any{
		"section":             "review",
		"workspace":           WorkspaceIdentity(state),
		"plans":               plans,
		"activePlan":          map[string]any{"id": active.ID, "name": active.Name, "criteria": activeCriteria, "assignmentsManaged": active.AssignmentsManaged},
		"hasActivePlan":       hasActivePlan,
		"candidates":          candidates,
		"reviewers":           reviewers,
		"activeReviewers":     activeReviewers,
		"retiredReviewers":    retiredReviewers,
		"hasRetiredReviewers": len(retiredReviewers) > 0,
		"humanReviewers":      humanReviewers,
		"reviewerCount":       len(activeReviewers),
		"reviewerTotal":       len(reviewers),
		"defaultRubric":       defaultReviewRubric(),
	}
}

func reviewInputDate(state domain.State, value time.Time) string {
	location, err := time.LoadLocation(state.Event.TimeZone)
	if err != nil {
		location = time.UTC
	}
	return value.In(location).Format("2006-01-02T15:04")
}

func rubricInput(criteria []domain.RubricCriterion) string {
	rows := make([]string, 0, len(criteria))
	for _, criterion := range criteria {
		rows = append(rows, fmt.Sprintf("%s|%s|%s|%g|%g", criterion.ID, criterion.Name, criterion.Description, criterion.Weight, criterion.MaxScore))
	}
	return strings.Join(rows, "\n")
}

func defaultReviewRubric() string {
	return "relevance|Relevance|Fit for the event audience and theme.|25|5\nclarity|Clarity|Problem, approach, and takeaway are specific.|25|5\nevidence|Evidence|Claims are supported by lived or measured results.|25|5\naudience_value|Audience value|Creates an actionable attendee outcome.|25|5"
}

func activeAssignmentsForPlan(state domain.State, planID string) []domain.ReviewAssignment {
	result := make([]domain.ReviewAssignment, 0)
	for _, assignment := range state.ReviewAssignments {
		if assignment.PlanID == planID && assignment.Active() {
			result = append(result, assignment)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SubmissionID == result[j].SubmissionID {
			return result[i].ReviewerID < result[j].ReviewerID
		}
		return result[i].SubmissionID < result[j].SubmissionID
	})
	return result
}

func assignmentRows(state domain.State, assignments []domain.ReviewAssignment) []map[string]any {
	rows := make([]map[string]any, 0, len(assignments))
	for _, assignment := range assignments {
		submission, submissionFound := state.Submission(assignment.SubmissionID)
		reviewer, reviewerFound := state.Reviewer(assignment.ReviewerID)
		if !submissionFound || !reviewerFound {
			continue
		}
		rows = append(rows, map[string]any{
			"id":         assignment.ID,
			"proposal":   submission.Title,
			"reviewer":   reviewer.Name,
			"source":     StatusLabel(assignment.Source),
			"assignedAt": DateTime(assignment.AssignedAt),
		})
	}
	return rows
}

func planRosterRows(state domain.State, plan domain.ReviewPlan) []map[string]any {
	rows := make([]map[string]any, 0, len(plan.ReviewerIDs))
	for _, reviewerID := range plan.ReviewerIDs {
		reviewer, found := state.Reviewer(reviewerID)
		if !found {
			continue
		}
		scoreCount := 0
		assignmentCount := 0
		for _, evaluation := range state.Evaluations {
			if evaluation.PlanID == plan.ID && evaluation.ReviewerID == reviewerID && evaluation.Source == "human" {
				scoreCount++
			}
		}
		for _, assignment := range state.ReviewAssignments {
			if assignment.PlanID == plan.ID && assignment.ReviewerID == reviewerID && assignment.Active() {
				assignmentCount++
			}
		}
		rows = append(rows, map[string]any{
			"id":          reviewer.ID,
			"name":        reviewer.Name,
			"kind":        StatusLabel(reviewer.Kind),
			"scoreCount":  scoreCount,
			"assignments": assignmentCount,
			"active":      reviewer.Active(),
		})
	}
	return rows
}

func planRecusalConflictCount(state domain.State, plan domain.ReviewPlan) int {
	count := 0
	for _, reviewerID := range plan.ReviewerIDs {
		reviewer, found := state.Reviewer(reviewerID)
		if !found || reviewer.Kind != "human" {
			continue
		}
		for _, submissionID := range plan.SubmissionIDs {
			submission, found := state.Submission(submissionID)
			if found && state.ReviewerCompanyConflict(*reviewer, *submission) {
				count++
			}
		}
	}
	return count
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func humanEvaluations(evaluations []domain.Evaluation) []domain.Evaluation {
	result := make([]domain.Evaluation, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if evaluation.Source == "human" {
			result = append(result, evaluation)
		}
	}
	return result
}

func evaluationsForPlan(state domain.State, planID string) []domain.Evaluation {
	result := make([]domain.Evaluation, 0)
	for _, evaluation := range state.Evaluations {
		if evaluation.PlanID == planID {
			result = append(result, evaluation)
		}
	}
	return result
}

func evaluationsFor(state domain.State, planID, submissionID string) []domain.Evaluation {
	result := make([]domain.Evaluation, 0)
	for _, evaluation := range state.Evaluations {
		if evaluation.PlanID == planID && evaluation.SubmissionID == submissionID {
			result = append(result, evaluation)
		}
	}
	return result
}

func weightedAverage(plan domain.ReviewPlan, evaluations []domain.Evaluation) float64 {
	if len(evaluations) == 0 {
		return 0
	}
	total := 0.0
	for _, evaluation := range evaluations {
		evaluationScore := 0.0
		for _, criterion := range plan.Criteria {
			evaluationScore += evaluation.Scores[criterion.ID] * criterion.Weight / 100
		}
		total += evaluationScore
	}
	return total / float64(len(evaluations))
}

func scoreLabel(score float64, evaluations int) string {
	if evaluations == 0 {
		return "—"
	}
	return Score(score)
}

func nameInitials(name string) string {
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
