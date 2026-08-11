package domain

import (
	"fmt"
	"strings"
)

// ActiveReviewPlan returns the sole review plan currently open for scoring.
// A workspace with no plan, or with two concurrently open plans, is unsafe to
// assign or decide against: the caller must make the round explicit first.
func (state State) ActiveReviewPlan() (ReviewPlan, error) {
	var active []ReviewPlan
	for _, plan := range state.ReviewPlans {
		if plan.Status == "active" || plan.Status == "open" {
			active = append(active, plan)
		}
	}
	switch len(active) {
	case 0:
		return ReviewPlan{}, fmt.Errorf("no active review plan")
	case 1:
		return active[0], nil
	default:
		return ReviewPlan{}, fmt.Errorf("multiple active review plans")
	}
}

// ActiveReviewPlanForSubmission returns the sole active plan that owns the
// submission. Final program decisions use it to avoid applying a quorum from
// a closed or unrelated round.
func (state State) ActiveReviewPlanForSubmission(submissionID string) (ReviewPlan, error) {
	var matched []ReviewPlan
	for _, plan := range state.ReviewPlans {
		if (plan.Status == "active" || plan.Status == "open") && containsReviewID(plan.SubmissionIDs, submissionID) {
			matched = append(matched, plan)
		}
	}
	switch len(matched) {
	case 0:
		return ReviewPlan{}, fmt.Errorf("submission is not assigned to an active review plan")
	case 1:
		return matched[0], nil
	default:
		return ReviewPlan{}, fmt.Errorf("submission is assigned to multiple active review plans")
	}
}

// AssignPendingToActiveReviewPlan appends each pending submission to the one
// active review plan in stable submission order. It is idempotent and never
// moves a proposal into a second open round implicitly.
func (state *State) AssignPendingToActiveReviewPlan() (planID string, assigned int, err error) {
	plan, err := state.ActiveReviewPlan()
	if err != nil {
		return "", 0, err
	}
	for index := range state.ReviewPlans {
		if state.ReviewPlans[index].ID != plan.ID {
			continue
		}
		assignedIDs := make(map[string]struct{}, len(state.ReviewPlans[index].SubmissionIDs))
		for _, submissionID := range state.ReviewPlans[index].SubmissionIDs {
			assignedIDs[submissionID] = struct{}{}
		}
		for _, submission := range state.Submissions {
			if submission.Status != SubmissionPending {
				continue
			}
			if _, exists := assignedIDs[submission.ID]; exists {
				continue
			}
			state.ReviewPlans[index].SubmissionIDs = append(state.ReviewPlans[index].SubmissionIDs, submission.ID)
			assignedIDs[submission.ID] = struct{}{}
			assigned++
		}
		return plan.ID, assigned, nil
	}
	return "", 0, fmt.Errorf("active review plan disappeared")
}

// HumanEvaluationCount counts distinct human reviewers for one assigned
// submission. Distinctness prevents an imported duplicate or a repeated
// upsert from satisfying a quorum on its own.
func (state State) HumanEvaluationCount(planID, submissionID string) int {
	reviewers := map[string]struct{}{}
	for _, evaluation := range state.Evaluations {
		if evaluation.PlanID == planID && evaluation.SubmissionID == submissionID && evaluation.Source == "human" && evaluation.ReviewerID != "" {
			reviewers[evaluation.ReviewerID] = struct{}{}
		}
	}
	return len(reviewers)
}

// ReviewerCompanyConflict reports an organization-level recusal condition.
// Company matching is only data preparation; the action it blocks is decided
// by the review-governance Arbiter policy.
func (state State) ReviewerCompanyConflict(reviewer Reviewer, submission Submission) bool {
	company := normalizeCompany(reviewer.Company)
	if company == "" {
		return false
	}
	for _, speakerID := range submission.SpeakerIDs {
		speaker, found := state.Speaker(speakerID)
		if found && normalizeCompany(speaker.Company) == company {
			return true
		}
	}
	return false
}

func containsReviewID(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func normalizeCompany(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
