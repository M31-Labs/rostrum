package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

// icsTimestamp is the UTC DATE-TIME layout every property in this package
// uses: DTSTAMP, DTSTART, DTEND.
const icsTimestamp = "20060102T150405Z"

// statusPublished is the domain.Session.Status value that marks a session
// fit to reach a speaker's calendar, in a feed or an invite. Both
// SpeakerCalendar and Invite gate on this value (SE-6, the shared publish
// filter): a draft session never emits a calendar entry, and is never
// stamped CONFIRMED.
const statusPublished = "published"

// EventCalendar builds the public, subscribe-style calendar for the complete
// published program. It carries no speaker identity or private workspace
// state: only scheduled sessions already eligible for the public agenda are
// emitted. The stable session IDs become stable VEVENT UIDs, so importing an
// updated feed refreshes existing entries instead of duplicating them.
func EventCalendar(state domain.State, slug string) ([]byte, string, error) {
	if strings.TrimSpace(slug) == "" || state.Event.Slug != slug {
		return nil, "", fmt.Errorf("event %s not found", slug)
	}
	var builder strings.Builder
	writeLine(&builder, "BEGIN:VCALENDAR")
	writeLine(&builder, "VERSION:2.0")
	writeLine(&builder, "PRODID:-//M31 Labs//Rostrum//EN")
	writeLine(&builder, "CALSCALE:GREGORIAN")
	writeLine(&builder, "METHOD:PUBLISH")
	writeLine(&builder, "X-WR-CALNAME:"+escape(state.Event.Name))
	stamp := time.Now().UTC()
	for _, item := range state.Sessions {
		if item.Status != statusPublished || !item.Scheduled() {
			continue
		}
		room, _ := state.Room(item.RoomID)
		writeEvent(&builder, eventFields{
			uid:         item.ID + "@rostrum.local",
			stamp:       stamp,
			starts:      item.StartsAt,
			ends:        item.EndsAt,
			summary:     item.Title,
			description: item.Description,
			location:    joinLocation(room.Name, state.Event.Location),
			sequence:    0,
			status:      "CONFIRMED",
		})
	}
	writeLine(&builder, "END:VCALENDAR")
	return []byte(builder.String()), domain.Slugify(state.Event.Name) + "-program.ics", nil
}

// SpeakerCalendar builds a subscribe-style calendar feed (METHOD:PUBLISH)
// listing every published session in which speakerID appears. A draft
// session is excluded entirely — it never reaches the feed a speaker might
// subscribe to, and is never stamped CONFIRMED.
func SpeakerCalendar(state domain.State, speakerID string) ([]byte, string, error) {
	speaker, found := state.Speaker(speakerID)
	if !found {
		return nil, "", fmt.Errorf("speaker %s not found", speakerID)
	}
	var builder strings.Builder
	writeLine(&builder, "BEGIN:VCALENDAR")
	writeLine(&builder, "VERSION:2.0")
	writeLine(&builder, "PRODID:-//M31 Labs//Rostrum//EN")
	writeLine(&builder, "CALSCALE:GREGORIAN")
	writeLine(&builder, "METHOD:PUBLISH")
	writeLine(&builder, "X-WR-CALNAME:"+escape(state.Event.Name+" — "+speaker.Name()))
	stamp := time.Now().UTC()
	for _, item := range state.Sessions {
		if !contains(item.SpeakerIDs, speakerID) || item.Status != statusPublished {
			continue
		}
		room, _ := state.Room(item.RoomID)
		writeEvent(&builder, eventFields{
			uid:         item.ID + "@rostrum.local",
			stamp:       stamp,
			starts:      item.StartsAt,
			ends:        item.EndsAt,
			summary:     item.Title,
			description: item.Description,
			location:    joinLocation(room.Name, state.Event.Location),
			sequence:    0,
			status:      "CONFIRMED",
		})
	}
	writeLine(&builder, "END:VCALENDAR")
	return []byte(builder.String()), domain.Slugify(speaker.Name()) + "-schedule.ics", nil
}

// Invite builds a one-attendee calendar invite (METHOD:REQUEST) for one
// session and one speaker: the shape a mail client renders as an
// accept/decline card, distinct from SpeakerCalendar's read-only feed.
// The VEVENT carries an ORGANIZER (organizerEmail, named after the event),
// one ATTENDEE (speaker, RSVP requested), a UID stable across re-sends of
// the same session, and SEQUENCE:0.
//
// session must carry a start and end time — DTSTART/DTEND are required
// VEVENT properties — and speaker and organizerEmail must both carry an
// address; Invite returns an error otherwise. STATUS is CONFIRMED only
// when session.Status is "published" (SE-6); any other status — for
// example an invite sent ahead of publish, from the acceptance flow —
// renders TENTATIVE, so a speaker's calendar never claims a firmer
// commitment than the workspace has recorded.
//
// The caller sends the returned bytes through mail.SendInvite, which
// attaches them as the invite's text/calendar MIME part; see that
// function's doc comment for where in the application to call it.
func Invite(state domain.State, session domain.Session, speaker domain.Speaker, organizerEmail string) ([]byte, error) {
	if !session.Scheduled() {
		return nil, fmt.Errorf("calendar: session %s has no start or end time", session.ID)
	}
	if strings.TrimSpace(speaker.Email) == "" {
		return nil, fmt.Errorf("calendar: speaker %s has no email", speaker.ID)
	}
	organizerEmail = strings.TrimSpace(organizerEmail)
	if organizerEmail == "" {
		return nil, fmt.Errorf("calendar: Invite requires an organizer email")
	}
	room, _ := state.Room(session.RoomID)
	status := "TENTATIVE"
	if session.Status == statusPublished {
		status = "CONFIRMED"
	}
	var builder strings.Builder
	writeLine(&builder, "BEGIN:VCALENDAR")
	writeLine(&builder, "VERSION:2.0")
	writeLine(&builder, "PRODID:-//M31 Labs//Rostrum//EN")
	writeLine(&builder, "CALSCALE:GREGORIAN")
	writeLine(&builder, "METHOD:REQUEST")
	writeEvent(&builder, eventFields{
		uid:            session.ID + "@rostrum.local",
		stamp:          time.Now().UTC(),
		starts:         session.StartsAt,
		ends:           session.EndsAt,
		summary:        session.Title,
		description:    session.Description,
		location:       joinLocation(room.Name, state.Event.Location),
		sequence:       0,
		status:         status,
		organizerCN:    state.Event.Name,
		organizerEmail: organizerEmail,
		attendeeCN:     speaker.Name(),
		attendeeEmail:  speaker.Email,
	})
	writeLine(&builder, "END:VCALENDAR")
	return []byte(builder.String()), nil
}

// eventFields carries one VEVENT's content; writeEvent renders it.
// organizerEmail and attendeeEmail are optional: SpeakerCalendar's feed
// leaves both empty (a subscribe feed names no attendee); Invite always
// sets both.
type eventFields struct {
	uid            string
	stamp          time.Time
	starts         time.Time
	ends           time.Time
	summary        string
	description    string
	location       string
	sequence       int
	status         string
	organizerCN    string
	organizerEmail string
	attendeeCN     string
	attendeeEmail  string
}

func writeEvent(builder *strings.Builder, f eventFields) {
	writeLine(builder, "BEGIN:VEVENT")
	writeLine(builder, "UID:"+escape(f.uid))
	writeLine(builder, "DTSTAMP:"+f.stamp.UTC().Format(icsTimestamp))
	writeLine(builder, "DTSTART:"+f.starts.UTC().Format(icsTimestamp))
	writeLine(builder, "DTEND:"+f.ends.UTC().Format(icsTimestamp))
	writeLine(builder, "SUMMARY:"+escape(f.summary))
	writeLine(builder, "DESCRIPTION:"+escape(f.description))
	writeLine(builder, "LOCATION:"+escape(f.location))
	if f.organizerEmail != "" {
		writeLine(builder, "ORGANIZER;CN="+quotedParam(f.organizerCN)+":mailto:"+f.organizerEmail)
	}
	if f.attendeeEmail != "" {
		writeLine(builder, "ATTENDEE;CN="+quotedParam(f.attendeeCN)+";RSVP=TRUE:mailto:"+f.attendeeEmail)
	}
	writeLine(builder, fmt.Sprintf("SEQUENCE:%d", f.sequence))
	writeLine(builder, "STATUS:"+f.status)
	writeLine(builder, "END:VEVENT")
}

func joinLocation(room, eventLocation string) string {
	room = strings.TrimSpace(room)
	eventLocation = strings.TrimSpace(eventLocation)
	switch {
	case room == "":
		return eventLocation
	case eventLocation == "":
		return room
	default:
		return room + ", " + eventLocation
	}
}

// writeLine folds value at 75 octets (foldLine) and appends it to builder
// as one physical-then-continuation content line, CRLF terminated.
func writeLine(builder *strings.Builder, value string) {
	builder.WriteString(foldLine(value))
	builder.WriteString("\r\n")
}

// foldLine folds one unfolded iCalendar content line at 75 octets per
// RFC 5545 section 3.1: each continuation begins with a CRLF followed by
// a single space, and a fold never splits a multi-byte UTF-8 rune.
func foldLine(value string) string {
	const limit = 75
	if len(value) <= limit {
		return value
	}
	data := []byte(value)
	var out strings.Builder
	pos := 0
	for pos < len(data) {
		width := limit
		if pos > 0 {
			width = limit - 1 // the leading fold space counts toward the 75
		}
		end := pos + width
		if end > len(data) {
			end = len(data)
		}
		if end < len(data) {
			for end > pos && isUTF8Continuation(data[end]) {
				end--
			}
		}
		if pos > 0 {
			out.WriteString("\r\n ")
		}
		out.Write(data[pos:end])
		pos = end
	}
	return out.String()
}

func isUTF8Continuation(b byte) bool {
	return b&0xC0 == 0x80
}

// escape prepares a TEXT value for one iCalendar content line per
// RFC 5545 section 3.3.11: a backslash, semicolon, or comma gets a
// backslash escape, and any line break — a CRLF pair, a lone CR, or a
// lone LF — becomes a literal "\n" so the value never introduces a stray
// physical line of its own.
func escape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, ";", "\\;")
	value = strings.ReplaceAll(value, ",", "\\,")
	value = strings.ReplaceAll(value, "\r\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

// quotedParam renders value as an iCalendar quoted param-value
// (RFC 5545 section 3.2): wrapped in double quotes, with an embedded
// double quote or line break — characters QSAFE-CHAR excludes — replaced
// so the parameter never breaks the surrounding content line.
func quotedParam(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, `"`, "'")
	return `"` + value + `"`
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
