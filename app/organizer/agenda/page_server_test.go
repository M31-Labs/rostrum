package agenda

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

func TestCreateSessionAddsManualSessionToBankAndPreservesDuration(t *testing.T) {
	state := domain.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)

	create := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/agenda/__actions/createSession", nil),
		FormData: map[string]string{
			"title":            "Manual program briefing",
			"description":      "An organizer-authored briefing session.",
			"format":           "Talk",
			"track_id":         "track-agents",
			"duration_minutes": "60",
			"speaker_spk_maya": "on",
			"day":              "2026-10-14",
		},
	}
	if err := createSession(create); err != nil {
		t.Fatalf("createSession: %v", err)
	}

	snapshot := workspace.Snapshot()
	var created domain.Session
	for _, item := range snapshot.Sessions {
		if item.Title == "Manual program briefing" {
			created = item
			break
		}
	}
	if created.ID == "" || created.Status != "unscheduled" || created.Scheduled() || created.DurationMinutes != 60 {
		t.Fatalf("created session = %#v, want unscheduled 60-minute session", created)
	}
	if len(created.SpeakerIDs) != 1 || created.SpeakerIDs[0] != "spk_maya" {
		t.Fatalf("created speakers = %#v, want Maya", created.SpeakerIDs)
	}
	if len(snapshot.AuditEvents) != 1 || snapshot.AuditEvents[0].Action != "agenda.session_created" {
		t.Fatalf("audit events = %#v, want manual-session provenance", snapshot.AuditEvents)
	}

	agendaData, err := present.Agenda(snapshot, "day", "2026-10-14")
	if err != nil {
		t.Fatalf("present agenda: %v", err)
	}
	bank, ok := agendaData["bank"].([]map[string]any)
	if !ok {
		t.Fatalf("bank = %#v, want agenda cards", agendaData["bank"])
	}
	found := false
	for _, card := range bank {
		if card["id"] == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("manual session %s is absent from agenda bank %#v", created.ID, bank)
	}

	move := &action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/agenda/__actions/moveSession", nil),
		FormData: map[string]string{
			"session_id": created.ID,
			"room_id":    "room-atrium",
			"track_id":   "track-agents",
			"starts_at":  "2026-10-14T09:00",
		},
	}
	if err := moveSession(move); err != nil {
		t.Fatalf("moveSession: %v", err)
	}
	finalSnapshot := workspace.Snapshot()
	scheduled, found := finalSnapshot.Session(created.ID)
	if !found || !scheduled.Scheduled() || scheduled.EndsAt.Sub(scheduled.StartsAt) != time.Hour {
		t.Fatalf("scheduled manual session = %#v, want a placed 60-minute session", scheduled)
	}
}
