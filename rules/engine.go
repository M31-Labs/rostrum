package rules

import (
	"embed"
	"fmt"
	"sort"
	"sync"

	"github.com/m31-labs/rostrum/internal/domain"
	"m31labs.dev/arbiter"
	"m31labs.dev/arbiter/vm"
)

// Sources keeps the decision policies inside the application binary. The same
// files remain readable by organizers and independently checkable by Arbiter.
//
//go:embed *.arb
var Sources embed.FS

type Engine struct {
	routing    *arbiter.Program
	visibility *arbiter.Program
	conflicts  *arbiter.Program
	review     *arbiter.Program
}

type RoutingDecision struct {
	Queue  string
	Owner  string
	Track  string
	Reason string
	Rule   string
	Trace  []string
}

type VisibilityDecision struct {
	Field   string
	Visible bool
	Reason  string
	Rule    string
	Trace   []string
}

type ConflictDecision struct {
	Allowed  bool
	Severity string
	Reason   string
	Rule     string
	Action   string
	Trace    []string
}

// ReviewGovernanceInput carries normalized facts for a review score or final
// program decision. It deliberately contains no comments, speaker identity,
// or other review content that could become an audit leak.
type ReviewGovernanceInput struct {
	Operation             string
	CompanyConflict       bool
	HumanEvaluations      int
	RequiredEvaluations   int
	ChairOverride         bool
	OverrideReasonPresent bool
}

type ReviewGovernanceDecision struct {
	Allowed bool
	Reason  string
	Rule    string
	Action  string
	Trace   []string
}

func New() (*Engine, error) {
	routing, err := compile("cfp-routing.arb")
	if err != nil {
		return nil, err
	}
	visibility, err := compile("form-visibility.arb")
	if err != nil {
		return nil, err
	}
	conflicts, err := compile("schedule-conflicts.arb")
	if err != nil {
		return nil, err
	}
	review, err := compile("review-governance.arb")
	if err != nil {
		return nil, err
	}
	return &Engine{routing: routing, visibility: visibility, conflicts: conflicts, review: review}, nil
}

var shared = sync.OnceValues(New)

// Shared returns the process-wide, immutable policy engine. Compiling policy
// once ensures every HTTP action evaluates the same reviewed source without
// repeatedly parsing it on a request path.
func Shared() (*Engine, error) {
	return shared()
}

func (e *Engine) Route(category, format, level string) (RoutingDecision, error) {
	match, trace, err := evaluate(e.routing, map[string]any{
		"submission": map[string]any{
			"category": category,
			"format":   format,
			"level":    level,
		},
	})
	if err != nil {
		return RoutingDecision{}, err
	}
	if match.Action != "RouteSubmission" {
		return RoutingDecision{}, fmt.Errorf("unexpected routing action %q", match.Action)
	}
	return RoutingDecision{
		Queue:  stringParam(match, "queue"),
		Owner:  stringParam(match, "owner"),
		Track:  stringParam(match, "track"),
		Reason: stringParam(match, "reason"),
		Rule:   match.Name,
		Trace:  trace,
	}, nil
}

// QuestionVisibility evaluates the only conditional-question decision shape
// organizers may create in the form builder: show targetField when a source
// answer exactly equals expected. The equality and visibility outcome live in
// reviewed Arbiter policy rather than a host-language if/else chain, while the
// builder still owns the concrete field IDs and answer value.
func (e *Engine) QuestionVisibility(value, expected, effect, targetField string) (VisibilityDecision, error) {
	match, trace, err := evaluate(e.visibility, map[string]any{
		"answer": map[string]any{
			"value":    value,
			"expected": expected,
			"effect":   effect,
		},
	})
	if err != nil {
		return VisibilityDecision{}, err
	}
	if match.Action != "FieldVisibility" {
		return VisibilityDecision{}, fmt.Errorf("unexpected visibility action %q", match.Action)
	}
	visible, ok := match.Params["visible"].(bool)
	if !ok {
		return VisibilityDecision{}, fmt.Errorf("visibility rule %q returned a non-boolean visible parameter", match.Name)
	}
	return VisibilityDecision{
		Field:   targetField,
		Visible: visible,
		Reason:  stringParam(match, "reason"),
		Rule:    match.Name,
		Trace:   trace,
	}, nil
}

// FieldVisibility retains the seeded Workshop policy's stable API for
// callers and older archive fixtures. New generic form rules should call
// QuestionVisibility directly.
func (e *Engine) FieldVisibility(format, category string) (VisibilityDecision, error) {
	return e.QuestionVisibility(format, "Workshop", "show", "workshop_needs")
}

func (e *Engine) EvaluateConflict(conflict domain.Conflict) (ConflictDecision, error) {
	match, trace, err := evaluate(e.conflicts, map[string]any{
		"conflict": map[string]any{
			"kind":                 conflict.Kind,
			"overlap_minutes":      conflict.OverlapMinutes,
			"shared_speaker_count": len(conflict.SpeakerIDs),
		},
	})
	if err != nil {
		return ConflictDecision{}, err
	}
	return ConflictDecision{
		Allowed:  match.Action != "BlockSchedule",
		Severity: stringParam(match, "severity"),
		Reason:   stringParam(match, "reason"),
		Rule:     match.Name,
		Action:   match.Action,
		Trace:    trace,
	}, nil
}

func (e *Engine) EvaluateReviewGovernance(input ReviewGovernanceInput) (ReviewGovernanceDecision, error) {
	match, trace, err := evaluate(e.review, map[string]any{
		"review": map[string]any{
			"operation":               input.Operation,
			"company_conflict":        input.CompanyConflict,
			"human_evaluations":       input.HumanEvaluations,
			"required_evaluations":    input.RequiredEvaluations,
			"chair_override":          input.ChairOverride,
			"override_reason_present": input.OverrideReasonPresent,
		},
	})
	if err != nil {
		return ReviewGovernanceDecision{}, err
	}
	allowed := match.Action == "AllowReview" || match.Action == "AllowDecision"
	return ReviewGovernanceDecision{
		Allowed: allowed,
		Reason:  stringParam(match, "reason"),
		Rule:    match.Name,
		Action:  match.Action,
		Trace:   trace,
	}, nil
}

func compile(name string) (*arbiter.Program, error) {
	source, err := Sources.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read policy %s: %w", name, err)
	}
	program, err := arbiter.Compile(source)
	if err != nil {
		return nil, fmt.Errorf("compile policy %s: %w", name, err)
	}
	return program, nil
}

func evaluate(program *arbiter.Program, facts map[string]any) (vm.MatchedRule, []string, error) {
	context := arbiter.DataFromMap(facts, program)
	matches, err := arbiter.Eval(program, context)
	if err != nil {
		return vm.MatchedRule{}, nil, err
	}
	if len(matches) == 0 {
		return vm.MatchedRule{}, nil, fmt.Errorf("no decision policy matched")
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Priority < matches[j].Priority
	})
	trace := make([]string, 0, len(matches))
	for _, match := range matches {
		trace = append(trace, fmt.Sprintf("%s → %s", match.Name, match.Action))
	}
	return matches[0], trace, nil
}

func stringParam(match vm.MatchedRule, name string) string {
	value, _ := match.Params[name].(string)
	return value
}
