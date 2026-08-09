package rules

import (
	"testing"

	"github.com/odvcencio/programma/internal/domain"
)

func TestEngineRoutesGovernanceSubmissions(t *testing.T) {
	engine := mustEngine(t)
	decision, err := engine.Route("governance", "Talk", "Advanced")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Queue != "governed-systems" || decision.Owner != "Theo Okafor" || decision.Rule != "RouteGovernance" {
		t.Fatalf("unexpected routing decision: %#v", decision)
	}
}

func TestEngineUsesRoutingFallback(t *testing.T) {
	engine := mustEngine(t)
	decision, err := engine.Route("unknown", "Talk", "Introductory")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Queue != "program-triage" || decision.Rule != "RouteFallback" {
		t.Fatalf("unexpected fallback decision: %#v", decision)
	}
}

func TestEngineControlsConditionalFields(t *testing.T) {
	engine := mustEngine(t)
	workshop, err := engine.FieldVisibility("Workshop", "interfaces")
	if err != nil {
		t.Fatal(err)
	}
	if !workshop.Visible || workshop.Field != "workshop_needs" || workshop.Rule != "ShowWorkshopNeeds" {
		t.Fatalf("unexpected workshop visibility: %#v", workshop)
	}

	talk, err := engine.FieldVisibility("Talk", "interfaces")
	if err != nil {
		t.Fatal(err)
	}
	if talk.Visible || talk.Rule != "HideWorkshopNeeds" {
		t.Fatalf("unexpected talk visibility: %#v", talk)
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

func mustEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return engine
}
