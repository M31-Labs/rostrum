package agenda

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

func TestCreateSessionAddsManualSessionToBankAndPreservesDuration(t *testing.T) {
	state := fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
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

func TestMoveSessionRejectsPlacementsOutsideTheEventWindow(t *testing.T) {
	for _, test := range []struct {
		name     string
		startsAt string
	}{
		{name: "before event start", startsAt: "2026-10-14T08:30"},
		{name: "session ends after event end", startsAt: "2026-10-16T17:30"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := openAgendaTestWorkspace(t, fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)))
			beforeSnapshot := workspace.Snapshot()
			before, found := beforeSnapshot.Session("ses_rules")
			if !found {
				t.Fatal("seed session ses_rules not found")
			}
			beforeValue := *before
			beforeAudits := len(beforeSnapshot.AuditEvents)

			err := moveSession(&action.Context{
				Request: httptest.NewRequest(http.MethodPost, "/organizer/agenda/__actions/moveSession", nil),
				FormData: map[string]string{
					"session_id": "ses_rules",
					"room_id":    "room-atrium",
					"track_id":   "track-agents",
					"starts_at":  test.startsAt,
				},
			})
			requireAgendaValidation(t, err)

			afterSnapshot := workspace.Snapshot()
			after, found := afterSnapshot.Session("ses_rules")
			if !found {
				t.Fatal("ses_rules disappeared after rejected move")
			}
			if !after.StartsAt.Equal(beforeValue.StartsAt) || !after.EndsAt.Equal(beforeValue.EndsAt) ||
				after.RoomID != beforeValue.RoomID || after.TrackID != beforeValue.TrackID || after.Status != beforeValue.Status {
				t.Fatalf("rejected move changed session: before=%#v after=%#v", beforeValue, *after)
			}
			if len(afterSnapshot.AuditEvents) != beforeAudits {
				t.Fatalf("rejected move appended an audit event: before=%d after=%d", beforeAudits, len(afterSnapshot.AuditEvents))
			}
		})
	}
}

func TestCancelledSessionCannotMoveOrReturnToBank(t *testing.T) {
	for _, operation := range []string{"move", "unschedule"} {
		t.Run(operation, func(t *testing.T) {
			state := fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
			cancelled, found := state.Session("ses_rules")
			if !found {
				t.Fatal("seed session ses_rules not found")
			}
			cancelled.Status = "cancelled"
			workspace := openAgendaTestWorkspace(t, state)
			beforeSnapshot := workspace.Snapshot()
			before, _ := beforeSnapshot.Session("ses_rules")
			beforeValue := *before
			beforeAudits := len(beforeSnapshot.AuditEvents)

			ctx := &action.Context{
				Request: httptest.NewRequest(http.MethodPost, "/organizer/agenda/__actions/"+operation, nil),
				FormData: map[string]string{
					"session_id": "ses_rules",
					"room_id":    "room-atrium",
					"track_id":   "track-agents",
					"starts_at":  "2026-10-14T09:00",
					"day":        "2026-10-14",
				},
			}
			var err error
			if operation == "move" {
				err = moveSession(ctx)
			} else {
				err = unscheduleSession(ctx)
			}
			requireAgendaValidation(t, err)

			afterSnapshot := workspace.Snapshot()
			after, found := afterSnapshot.Session("ses_rules")
			if !found {
				t.Fatal("ses_rules disappeared after rejected lifecycle change")
			}
			if after.Status != "cancelled" || !after.StartsAt.Equal(beforeValue.StartsAt) || !after.EndsAt.Equal(beforeValue.EndsAt) ||
				after.RoomID != beforeValue.RoomID || after.TrackID != beforeValue.TrackID {
				t.Fatalf("rejected %s changed cancelled session: before=%#v after=%#v", operation, beforeValue, *after)
			}
			if len(afterSnapshot.AuditEvents) != beforeAudits {
				t.Fatalf("rejected %s appended an audit event: before=%d after=%d", operation, beforeAudits, len(afterSnapshot.AuditEvents))
			}
		})
	}
}

func TestPublishAgendaLeavesCancelledSessionsCancelled(t *testing.T) {
	state := fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	for index := range state.Sessions {
		state.Sessions[index].Status = "cancelled"
	}
	// Keep one ordinary, conflict-free session to prove publishing still moves
	// eligible work forward while every cancelled peer remains terminal.
	state.Sessions[0].Status = "draft"
	state.Sessions[0].SpeakerIDs = nil
	state.Communications = nil
	workspace := openAgendaTestWorkspace(t, state)

	err := publishAgenda(&action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/organizer/agenda/__actions/publishAgenda", nil),
		FormData: map[string]string{},
	})
	if err != nil {
		t.Fatalf("publishAgenda: %v", err)
	}

	snapshot := workspace.Snapshot()
	if snapshot.Sessions[0].Status != "published" {
		t.Fatalf("eligible session status = %q, want published", snapshot.Sessions[0].Status)
	}
	for _, item := range snapshot.Sessions[1:] {
		if item.Status != "cancelled" {
			t.Errorf("cancelled session %s status = %q after publish", item.ID, item.Status)
		}
	}
	if len(snapshot.AuditEvents) != 1 || snapshot.AuditEvents[0].Action != "agenda.published" {
		t.Fatalf("publish audit events = %#v, want one agenda.published event", snapshot.AuditEvents)
	}
}

func TestPublishAgendaRollsBackArbiterBlockedConflicts(t *testing.T) {
	workspace := openAgendaTestWorkspace(t, fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)))
	before := workspace.Snapshot()
	statuses := make(map[string]string, len(before.Sessions))
	for _, item := range before.Sessions {
		statuses[item.ID] = item.Status
	}

	err := publishAgenda(&action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/organizer/agenda/__actions/publishAgenda", nil),
		FormData: map[string]string{},
	})
	requireAgendaValidation(t, err)

	after := workspace.Snapshot()
	for _, item := range after.Sessions {
		if item.Status != statuses[item.ID] {
			t.Errorf("blocked publish changed %s from %q to %q", item.ID, statuses[item.ID], item.Status)
		}
	}
	if len(after.AuditEvents) != len(before.AuditEvents) {
		t.Fatalf("blocked publish appended an audit event: before=%d after=%d", len(before.AuditEvents), len(after.AuditEvents))
	}
}

func openAgendaTestWorkspace(t *testing.T, state domain.State) *store.JSONStore {
	t.Helper()
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.Close() })
	appstate.Set(workspace)
	return workspace
}

func requireAgendaValidation(t *testing.T, err error) {
	t.Helper()
	result, ok := err.(*action.ResultError)
	if !ok || result.StatusCode() != http.StatusUnprocessableEntity {
		t.Fatalf("action error = %#v, want 422 validation", err)
	}
}
