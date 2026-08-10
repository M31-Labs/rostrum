package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestSubmissionDetailIncludesTheFullAbstract(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	view, err := SubmissionDetail(state, "sub_memory")
	if err != nil {
		t.Fatalf("SubmissionDetail: %v", err)
	}
	submission, found := state.Submission("sub_memory")
	if !found {
		t.Fatal("seed is missing sub_memory")
	}
	if got := view["abstract"].(string); got != submission.Abstract || got == "" {
		t.Fatalf("abstract = %q, want the full seeded abstract %q", got, submission.Abstract)
	}
	if got := view["title"].(string); got != submission.Title {
		t.Fatalf("title = %q, want %q", got, submission.Title)
	}
	trace := view["ruleTrace"].([]string)
	if len(trace) == 0 {
		t.Fatal("expected a non-empty rule trace")
	}
	evaluations := view["evaluations"].([]map[string]any)
	if len(evaluations) < 2 {
		t.Fatalf("evaluations = %d, want at least 2 seeded rows for sub_memory", len(evaluations))
	}
}

func TestSubmissionDetailReportsNotFound(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	if _, err := SubmissionDetail(state, "sub_does_not_exist"); err == nil {
		t.Fatal("expected an error for an unknown submission id")
	}
}
