package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/domain"
)

func SpeakerCalendar(state domain.State, speakerID string) ([]byte, string, error) {
	speaker, found := state.Speaker(speakerID)
	if !found {
		return nil, "", fmt.Errorf("speaker %s not found", speakerID)
	}
	var builder strings.Builder
	writeLine(&builder, "BEGIN:VCALENDAR")
	writeLine(&builder, "VERSION:2.0")
	writeLine(&builder, "PRODID:-//M31 Labs//Programma//EN")
	writeLine(&builder, "CALSCALE:GREGORIAN")
	writeLine(&builder, "METHOD:PUBLISH")
	writeLine(&builder, "X-WR-CALNAME:"+escape(state.Event.Name+" — "+speaker.Name()))
	for _, item := range state.Sessions {
		if !contains(item.SpeakerIDs, speakerID) || !item.Scheduled() {
			continue
		}
		room, _ := state.Room(item.RoomID)
		writeLine(&builder, "BEGIN:VEVENT")
		writeLine(&builder, "UID:"+escape(item.ID+"@programma.local"))
		writeLine(&builder, "DTSTAMP:"+time.Now().UTC().Format("20060102T150405Z"))
		writeLine(&builder, "DTSTART:"+item.StartsAt.UTC().Format("20060102T150405Z"))
		writeLine(&builder, "DTEND:"+item.EndsAt.UTC().Format("20060102T150405Z"))
		writeLine(&builder, "SUMMARY:"+escape(item.Title))
		writeLine(&builder, "DESCRIPTION:"+escape(item.Description))
		writeLine(&builder, "LOCATION:"+escape(room.Name+", "+state.Event.Location))
		writeLine(&builder, "STATUS:CONFIRMED")
		writeLine(&builder, "END:VEVENT")
	}
	writeLine(&builder, "END:VCALENDAR")
	return []byte(builder.String()), domain.Slugify(speaker.Name()) + "-schedule.ics", nil
}

func writeLine(builder *strings.Builder, value string) {
	builder.WriteString(value)
	builder.WriteString("\r\n")
}

func escape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, ";", "\\;")
	value = strings.ReplaceAll(value, ",", "\\,")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
