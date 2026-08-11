package review

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestBalancedAssignmentsHonorRecusalAndAreIdempotent(t *testing.T) {
	now := time.Now().UTC()
	state := &domain.State{
		Speakers:    []domain.Speaker{{ID: "spk_conflict", Company: "Northstar"}},
		Submissions: []domain.Submission{{ID: "sub_1", SpeakerIDs: []string{"spk_conflict"}, Status: domain.SubmissionPending}},
		Reviewers: []domain.Reviewer{
			{ID: "rev_conflict", Name: "Conflicted", Email: "conflicted@example.com", Company: "Northstar", Kind: "human"},
			{ID: "rev_safe", Name: "Safe", Email: "safe@example.com", Company: "Elsewhere", Kind: "human"},
		},
		ReviewPlans: []domain.ReviewPlan{{
			ID: "plan_1", Name: "Plan", Round: 1, Status: "open", DueAt: now.Add(time.Hour),
			ReviewerIDs: []string{"rev_conflict", "rev_safe"}, SubmissionIDs: []string{"sub_1"},
			Criteria:           []domain.RubricCriterion{{ID: "fit", Name: "Fit", Weight: 100, MaxScore: 5}},
			EvaluationsPerItem: 1,
		}},
	}
	assigned, recused, err := buildBalancedAssignments(state, "plan_1", "organizer:test")
	if err != nil {
		t.Fatalf("buildBalancedAssignments: %v", err)
	}
	if assigned != 1 || recused == 0 {
		t.Fatalf("assignment result = (%d, %d), want one safe assignment and a recusal", assigned, recused)
	}
	if len(state.ReviewAssignments) != 1 || state.ReviewAssignments[0].ReviewerID != "rev_safe" || state.ReviewAssignments[0].Source != "automatic" {
		t.Fatalf("assignments = %#v, want safe automatic assignment", state.ReviewAssignments)
	}
	if !state.ReviewPlans[0].AssignmentsManaged {
		t.Fatal("plan did not enter managed-assignment mode")
	}
	assigned, _, err = buildBalancedAssignments(state, "plan_1", "organizer:test")
	if err != nil || assigned != 0 {
		t.Fatalf("repeat balance = (%d, %v), want idempotent no-op", assigned, err)
	}
}

func TestParseRubricRejectsInvalidWeight(t *testing.T) {
	if _, err := parseRubric("fit|Fit|Program fit.|80|5\nclarity|Clarity|Clear proposal.|10|5"); err == nil {
		t.Fatal("rubric whose weights do not total 100 was accepted")
	}
}
