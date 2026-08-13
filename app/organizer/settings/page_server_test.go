package settings

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

func TestProgramShapeActionsCreateTrackRoomAndCategory(t *testing.T) {
	state := fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)

	for _, testCase := range []struct {
		name string
		run  func(*action.Context) error
		data map[string]string
	}{
		{
			name: "track",
			run:  addTrack,
			data: map[string]string{"name": "Operations", "color": "teal", "description": "Live program operations."},
		},
		{
			name: "room",
			run:  addRoom,
			data: map[string]string{"name": "Luna Hall", "capacity": "240"},
		},
		{
			name: "category",
			run:  addCategory,
			data: map[string]string{
				"name": "Operations", "owner_name": "Alex Kim", "owner_email": "alex@example.com", "track_id": "track-operations",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.run(&action.Context{
				Request:  httptest.NewRequest(http.MethodPost, "/organizer/settings", nil),
				FormData: testCase.data,
			}); err != nil {
				t.Fatalf("add %s: %v", testCase.name, err)
			}
		})
	}

	snapshot := workspace.Snapshot()
	if track, found := snapshot.Track("track-operations"); !found || track.Color != "teal" {
		t.Fatalf("track = %#v, want persisted Operations track", track)
	}
	if room, found := snapshot.Room("room-luna-hall"); !found || room.Capacity != 240 {
		t.Fatalf("room = %#v, want persisted Luna Hall", room)
	}
	category, found := snapshot.Category("operations")
	if !found || category.TrackID != "track-operations" || category.OwnerEmail != "alex@example.com" {
		t.Fatalf("category = %#v, want persisted category with owner and track", category)
	}
	if len(snapshot.AuditEvents) != 3 {
		t.Fatalf("audit events = %#v, want one event per program-shape mutation", snapshot.AuditEvents)
	}
}

func TestSaveEventRebasesScheduledSessionsByCalendarDaysAndTimezone(t *testing.T) {
	state := fixture.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	before, found := state.Session("ses_memory")
	if !found {
		t.Fatal("seeded scheduled session is missing")
	}
	oldLocation, err := time.LoadLocation(state.Event.TimeZone)
	if err != nil {
		t.Fatal(err)
	}
	newLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	newStart := state.Event.StartsAt.In(oldLocation).AddDate(0, 0, 2)
	newEnd := state.Event.EndsAt.In(oldLocation).AddDate(0, 0, 2)
	if err := saveEvent(&action.Context{
		Request: httptest.NewRequest(http.MethodPost, "/organizer/settings/__actions/saveEvent", nil),
		FormData: map[string]string{
			"name":        state.Event.Name,
			"slug":        state.Event.Slug,
			"type":        state.Event.Type,
			"website":     state.Event.WebsiteURL,
			"location":    state.Event.Location,
			"timezone":    newLocation.String(),
			"starts_at":   newStart.Format("2006-01-02T15:04"),
			"ends_at":     newEnd.Format("2006-01-02T15:04"),
			"theme":       state.Event.Theme,
			"description": state.Event.Description,
		},
	}); err != nil {
		t.Fatalf("saveEvent: %v", err)
	}

	result := workspace.Snapshot()
	after, found := result.Session("ses_memory")
	if !found || !after.Scheduled() {
		t.Fatalf("rebased session = %#v, want scheduled session", after)
	}
	wantStart := before.StartsAt.In(oldLocation).AddDate(0, 0, 2)
	gotStart := after.StartsAt.In(newLocation)
	if gotStart.Year() != wantStart.Year() || gotStart.Month() != wantStart.Month() || gotStart.Day() != wantStart.Day() || gotStart.Hour() != wantStart.Hour() || gotStart.Minute() != wantStart.Minute() {
		t.Fatalf("rebased start = %s, want local wall time %s on date shift", gotStart, wantStart)
	}
	if after.EndsAt.Sub(after.StartsAt) != before.EndsAt.Sub(before.StartsAt) {
		t.Fatalf("rebased duration = %s, want %s", after.EndsAt.Sub(after.StartsAt), before.EndsAt.Sub(before.StartsAt))
	}
	if event := result.Event; event.TimeZone != newLocation.String() || !event.StartsAt.Equal(time.Date(newStart.Year(), newStart.Month(), newStart.Day(), newStart.Hour(), newStart.Minute(), 0, 0, newLocation)) {
		t.Fatalf("saved event = %#v, want new timezone and dates", event)
	}
}
