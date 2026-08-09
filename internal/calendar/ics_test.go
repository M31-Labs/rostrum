package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/programma/internal/domain"
)

func TestSpeakerCalendarContainsAssignedSessions(t *testing.T) {
	state := domain.Seed(time.Now().UTC())
	data, name, err := SpeakerCalendar(state, "spk_maya")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "BEGIN:VCALENDAR\r\n") || !strings.Contains(content, "SUMMARY:Memory Without Mystery") {
		t.Fatalf("unexpected calendar:\n%s", content)
	}
	if name != "maya-chen-schedule.ics" {
		t.Fatalf("unexpected filename: %s", name)
	}
}
