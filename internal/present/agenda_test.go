package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestAgendaSlotClocksRespectFirstDayEventStart(t *testing.T) {
	state, location := agendaClockTestState(t)
	day := time.Date(2026, time.October, 14, 0, 0, 0, 0, location)

	clocks := agendaSlotClocks(state, day, location)
	assertAgendaClock(t, clocks, [2]int{8, 0}, false)
	assertAgendaClock(t, clocks, [2]int{8, 30}, false)
	assertAgendaClock(t, clocks, [2]int{9, 0}, true)
	assertAgendaClock(t, clocks, [2]int{9, 17}, true)
	assertAgendaClock(t, clocks, [2]int{18, 0}, true)
}

func TestAgendaSlotClocksStopBeforeLastDayEventEnd(t *testing.T) {
	state, location := agendaClockTestState(t)
	day := time.Date(2026, time.October, 16, 0, 0, 0, 0, location)

	clocks := agendaSlotClocks(state, day, location)
	assertAgendaClock(t, clocks, [2]int{8, 0}, true)
	assertAgendaClock(t, clocks, [2]int{17, 30}, true)
	assertAgendaClock(t, clocks, [2]int{18, 0}, false)
	// Historical off-grid placements remain visible even when they predate the
	// stricter event-window rule and now sit beyond the configured event end.
	assertAgendaClock(t, clocks, [2]int{18, 7}, true)
}

func TestAgendaSlotClocksKeepFullGridOnMiddleDay(t *testing.T) {
	state, location := agendaClockTestState(t)
	day := time.Date(2026, time.October, 15, 0, 0, 0, 0, location)

	clocks := agendaSlotClocks(state, day, location)
	assertAgendaClock(t, clocks, [2]int{8, 0}, true)
	assertAgendaClock(t, clocks, [2]int{18, 0}, true)
	assertAgendaClock(t, clocks, [2]int{7, 47}, true)
	for index := 1; index < len(clocks); index++ {
		previous, current := clocks[index-1], clocks[index]
		if previous[0] > current[0] || (previous[0] == current[0] && previous[1] > current[1]) {
			t.Fatalf("agenda clocks are not chronological: %#v", clocks)
		}
	}
}

func TestAgendaBankExcludesCancelledSessions(t *testing.T) {
	state := domain.State{
		Sessions: []domain.Session{
			{ID: "waiting", Title: "Waiting", Status: "unscheduled"},
			{ID: "withdrawn", Title: "Withdrawn", Status: "cancelled"},
		},
	}

	bank := agendaBank(state)
	if len(bank) != 1 || bank[0]["id"] != "waiting" {
		t.Fatalf("agenda bank = %#v, want only the active unscheduled session", bank)
	}
}

func agendaClockTestState(t *testing.T) (domain.State, *time.Location) {
	t.Helper()
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, time.October, day, hour, minute, 0, 0, location)
	}
	return domain.State{
		Event: domain.Event{
			StartsAt: at(14, 9, 0),
			EndsAt:   at(16, 18, 0),
		},
		Sessions: []domain.Session{
			{ID: "first-off-grid", StartsAt: at(14, 9, 17), EndsAt: at(14, 9, 47)},
			{ID: "middle-off-grid", StartsAt: at(15, 7, 47), EndsAt: at(15, 8, 17)},
			{ID: "last-historical", StartsAt: at(16, 18, 7), EndsAt: at(16, 18, 37)},
		},
	}, location
}

func assertAgendaClock(t *testing.T, clocks [][2]int, target [2]int, want bool) {
	t.Helper()
	found := false
	for _, clock := range clocks {
		if clock == target {
			found = true
			break
		}
	}
	if found != want {
		t.Fatalf("clock %02d:%02d presence = %t, want %t; clocks=%#v", target[0], target[1], found, want, clocks)
	}
}
