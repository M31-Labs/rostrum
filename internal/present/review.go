package present

import (
	"fmt"
	"sort"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
)

func Review(state domain.State) map[string]any {
	plans := make([]map[string]any, 0, len(state.ReviewPlans))
	for _, plan := range state.ReviewPlans {
		completed := humanEvaluations(evaluationsForPlan(state, plan.ID))
		expected := len(plan.SubmissionIDs) * plan.EvaluationsPerItem
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
			"id":            plan.ID,
			"round":         plan.Round,
			"name":          plan.Name,
			"status":        StatusLabel(plan.Status),
			"tone":          StatusTone(plan.Status),
			"due":           DateTime(plan.DueAt),
			"instructions":  plan.Instructions,
			"anonymous":     plan.Anonymous,
			"aiAssist":      plan.AIAssist,
			"reviewers":     len(plan.ReviewerIDs),
			"proposals":     len(plan.SubmissionIDs),
			"completed":     len(completed),
			"expected":      expected,
			"coverage":      Percent(len(completed), expected),
			"coverageStyle": fmt.Sprintf("%d%%", Percent(len(completed), expected)),
			"criteria":      criteria,
		})
	}

	active := state.ReviewPlans[len(state.ReviewPlans)-1]
	candidates := make([]map[string]any, 0, len(active.SubmissionIDs))
	for _, submissionID := range active.SubmissionIDs {
		submission, found := state.Submission(submissionID)
		if !found {
			continue
		}
		evaluations := evaluationsFor(state, active.ID, submissionID)
		humanReviews := humanEvaluations(evaluations)
		score := weightedAverage(active, humanReviews)
		hasAI := false
		assistModel := ""
		assistSource := ""
		recommendations := make([]string, 0, len(evaluations))
		for _, evaluation := range evaluations {
			if evaluation.Source != "" && evaluation.Source != "human" {
				hasAI = true
				assistModel = evaluation.Model
				assistSource = StatusLabel(evaluation.Source)
			}
			if evaluation.Source == "human" {
				recommendations = append(recommendations, StatusLabel(evaluation.Recommendation))
			}
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
			"hasAI":           hasAI,
			"assistModel":     assistModel,
			"assistSource":    assistSource,
			"recommendations": strings.Join(recommendations, " · "),
			"status":          StatusLabel(submission.Status),
			"tone":            StatusTone(submission.Status),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i]["scoreValue"].(float64) > candidates[j]["scoreValue"].(float64)
	})

	reviewers := make([]map[string]any, 0, len(state.Reviewers))
	humanReviewers := make([]map[string]any, 0, len(state.Reviewers))
	for _, reviewer := range state.Reviewers {
		completed := 0
		for _, evaluation := range state.Evaluations {
			if evaluation.ReviewerID == reviewer.ID {
				completed++
			}
		}
		row := map[string]any{
			"id":        reviewer.ID,
			"name":      reviewer.Name,
			"kind":      StatusLabel(reviewer.Kind),
			"expertise": strings.Join(reviewer.Expertise, " · "),
			"completed": completed,
			"initials":  nameInitials(reviewer.Name),
		}
		reviewers = append(reviewers, row)
		if reviewer.Kind == "human" && containsID(active.ReviewerIDs, reviewer.ID) {
			humanReviewers = append(humanReviewers, row)
		}
	}
	activeCriteria := make([]map[string]any, 0, len(active.Criteria))
	for _, criterion := range active.Criteria {
		activeCriteria = append(activeCriteria, map[string]any{"id": criterion.ID, "name": criterion.Name, "description": criterion.Description, "weight": fmt.Sprintf("%.0f%%", criterion.Weight), "max": criterion.MaxScore})
	}

	return map[string]any{
		"section":        "review",
		"plans":          plans,
		"activePlan":     map[string]any{"id": active.ID, "name": active.Name, "aiAssist": active.AIAssist, "criteria": activeCriteria},
		"candidates":     candidates,
		"reviewers":      reviewers,
		"humanReviewers": humanReviewers,
		"reviewerCount":  len(reviewers),
	}
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
