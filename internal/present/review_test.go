package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

// TestReviewHumanCoverageAndAggregateSkipUnscoredCandidate asserts that a
// candidate with no recorded human evaluation contributes zero to coverage
// and renders an em-dash aggregate score, rather than a false zero.
func TestReviewHumanCoverageAndAggregateSkipUnscoredCandidate(t *testing.T) {
	view := Review(domain.Seed(time.Now().UTC()))
	plans := view["plans"].([]map[string]any)
	active := plans[len(plans)-1]
	if got := active["coverage"].(int); got != 7 {
		t.Fatalf("human coverage = %d%%, want 7%%", got)
	}

	candidates := view["candidates"].([]map[string]any)
	for _, candidate := range candidates {
		if candidate["id"] != "sub_ai_review" {
			continue
		}
		if got := candidate["evaluationCount"].(int); got != 0 {
			t.Fatalf("human evaluation count = %d, want 0", got)
		}
		if got := candidate["score"].(string); got != "—" {
			t.Fatalf("human aggregate = %q, want em dash", got)
		}
		return
	}
	t.Fatal("sub_ai_review candidate not found")
}
