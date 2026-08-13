package review

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

// organizerMutationStore returns the preflight snapshot, then applies a
// separate valid state transition before saveReview acquires its write lock.
// It models an organizer changing a plan, roster, or managed assignment in
// the interval between loading the review form and saving it.
type organizerMutationStore struct {
	store.StateStore
	mutate  func(*domain.State) error
	mutated bool
}

func (store *organizerMutationStore) Snapshot() domain.State {
	snapshot := store.StateStore.Snapshot()
	if !store.mutated {
		store.mutated = true
		if err := store.StateStore.Update(store.mutate); err != nil {
			panic(err)
		}
	}
	return snapshot
}

func TestSaveReviewReauthorizesTheCurrentReviewState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.State)
	}{
		{name: "closed plan", mutate: func(state *domain.State) { state.ReviewPlans[1].Status = "closed" }},
		{name: "removed roster member", mutate: func(state *domain.State) {
			state.ReviewPlans[1].ReviewerIDs = []string{"rev_ada", "rev_marcus", "rev_virtual_practitioner"}
		}},
		{name: "removed managed assignment", mutate: func(state *domain.State) {
			state.ReviewAssignments[0].RemovedAt = time.Now().UTC()
			state.ReviewAssignments[0].RemovalReason = "reviewer reassigned"
		}},
		{name: "new recusal", mutate: func(state *domain.State) {
			for index := range state.Reviewers {
				if state.Reviewers[index].ID == "rev_ines" {
					state.Reviewers[index].Company = "Coastline"
				}
			}
		}},
		{name: "changed rubric", mutate: func(state *domain.State) {
			state.ReviewPlans[1].Criteria[0].MaxScore = 4
			for index := range state.Evaluations {
				if state.Evaluations[index].PlanID == "plan_round_two" {
					state.Evaluations[index].Scores["program_fit"] = 4
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			state := fixture.Seed(now)
			state.ReviewPlans[1].AssignmentsManaged = true
			state.ReviewAssignments = []domain.ReviewAssignment{{
				ID: "assign_organizer_current_state", PlanID: "plan_round_two", SubmissionID: "sub_workspace", ReviewerID: "rev_ines",
				Source: "manual", Actor: "organizer:test", AssignedAt: now,
			}}
			workspace, err := store.Open(":memory:", state)
			if err != nil {
				t.Fatalf("open workspace: %v", err)
			}
			appstate.Set(&organizerMutationStore{
				StateStore: workspace,
				mutate: func(current *domain.State) error {
					test.mutate(current)
					return nil
				},
			})

			err = saveReview(&action.Context{
				Request: httptest.NewRequest(http.MethodPost, "/organizer/review/__actions/saveReview", nil),
				FormData: map[string]string{
					"plan_id":              "plan_round_two",
					"submission_id":        "sub_workspace",
					"reviewer_id":          "rev_ines",
					"recommendation":       "yes",
					"comments":             "This review is valid until current organizer authorization changes.",
					"score_program_fit":    "5",
					"score_evidence":       "4",
					"score_novelty":        "4",
					"score_audience_value": "5",
				},
			})
			var result *action.ResultError
			if !errors.As(err, &result) {
				t.Fatalf("saveReview error = %v, want structured validation failure", err)
			}
			for _, evaluation := range workspace.Snapshot().Evaluations {
				if evaluation.PlanID == "plan_round_two" && evaluation.SubmissionID == "sub_workspace" && evaluation.ReviewerID == "rev_ines" && evaluation.Source == "human" {
					t.Fatal("score persisted after current-state authorization changed")
				}
			}
		})
	}
}
