package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestEngineUsesGenericProductionRouting(t *testing.T) {
	engine := mustEngine(t)
	decision, err := engine.Route("governance", "Talk", "Advanced")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Queue != "program-triage" || decision.Owner != "Program team" || decision.Track != "" || decision.Rule != "RouteProgramTriage" {
		t.Fatalf("unexpected routing decision: %#v", decision)
	}
}

func TestEngineUsesRoutingFallback(t *testing.T) {
	engine := mustEngine(t)
	decision, err := engine.Route("unknown", "Talk", "Introductory")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Queue != "program-triage" || decision.Track != "" || decision.Rule != "RouteProgramTriage" {
		t.Fatalf("unexpected fallback decision: %#v", decision)
	}
}

func TestEngineLoadsExampleRoutingOnlyWhenExplicitlySupplied(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "examples", "demo", "rules", "cfp-routing.arb"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewWithRoutingSource(source)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := engine.Route("governance", "Talk", "Advanced")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Queue != "governed-systems" || decision.Owner != "Theo Okafor" || decision.Track != "track-governance" || decision.Rule != "RouteGovernance" {
		t.Fatalf("unexpected example routing decision: %#v", decision)
	}
}

func TestEngineRejectsEmptyConfiguredRouting(t *testing.T) {
	if _, err := NewWithRoutingSource(nil); err == nil {
		t.Fatal("empty configured routing policy accepted")
	}
}

func TestEngineControlsConditionalFields(t *testing.T) {
	engine := mustEngine(t)
	workshop, err := engine.FieldVisibility("Workshop", "interfaces")
	if err != nil {
		t.Fatal(err)
	}
	if !workshop.Visible || workshop.Field != "workshop_needs" || workshop.Rule != "ShowMatchingAnswer" {
		t.Fatalf("unexpected workshop visibility: %#v", workshop)
	}

	talk, err := engine.FieldVisibility("Talk", "interfaces")
	if err != nil {
		t.Fatal(err)
	}
	if talk.Visible || talk.Rule != "HideNonMatchingAnswer" {
		t.Fatalf("unexpected talk visibility: %#v", talk)
	}

	custom, err := engine.QuestionVisibility("Yes", "Yes", "show", "supporting_material")
	if err != nil {
		t.Fatal(err)
	}
	if !custom.Visible || custom.Field != "supporting_material" || custom.Rule != "ShowMatchingAnswer" {
		t.Fatalf("unexpected generic visibility: %#v", custom)
	}
}

func TestEngineGovernsScheduleConflicts(t *testing.T) {
	engine := mustEngine(t)

	room, err := engine.EvaluateConflict(domain.Conflict{Kind: domain.ConflictRoom, OverlapMinutes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if room.Allowed || room.Severity != domain.SeverityHard || room.Rule != "RoomDoubleBooking" {
		t.Fatalf("unexpected room decision: %#v", room)
	}

	track, err := engine.EvaluateConflict(domain.Conflict{Kind: domain.ConflictTrack, OverlapMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !track.Allowed || track.Severity != domain.SeverityWarning || track.Rule != "TrackOverlapWarning" {
		t.Fatalf("unexpected track decision: %#v", track)
	}
}

func TestEngineGovernsReviewRecusalAndQuorum(t *testing.T) {
	engine := mustEngine(t)

	recusal, err := engine.EvaluateReviewGovernance(ReviewGovernanceInput{Operation: "score", CompanyConflict: true})
	if err != nil {
		t.Fatal(err)
	}
	if recusal.Allowed || recusal.Rule != "BlockCompanyRecusal" {
		t.Fatalf("unexpected recusal decision: %#v", recusal)
	}

	quorum, err := engine.EvaluateReviewGovernance(ReviewGovernanceInput{Operation: "decision", HumanEvaluations: 1, RequiredEvaluations: 2})
	if err != nil {
		t.Fatal(err)
	}
	if quorum.Allowed || quorum.Rule != "BlockDecisionWithoutQuorum" {
		t.Fatalf("unexpected quorum decision: %#v", quorum)
	}

	override, err := engine.EvaluateReviewGovernance(ReviewGovernanceInput{Operation: "decision", ChairOverride: true, OverrideReasonPresent: true})
	if err != nil {
		t.Fatal(err)
	}
	if !override.Allowed || override.Rule != "AllowChairOverride" {
		t.Fatalf("unexpected chair override decision: %#v", override)
	}
}

func mustEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return engine
}
