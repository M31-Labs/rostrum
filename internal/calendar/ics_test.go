package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/domain"
)

func TestEventCalendarContainsOnlyPublishedScheduledProgram(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	data, name, err := EventCalendar(state, state.Event.Slug)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if name != "m31-systems-forum-2026-program.ics" {
		t.Fatalf("unexpected filename: %s", name)
	}
	if !strings.Contains(content, "METHOD:PUBLISH\r\n") || !strings.Contains(content, "SUMMARY:Memory Without Mystery") {
		t.Fatalf("public calendar omitted a published session:\n%s", content)
	}
	if strings.Contains(content, "Evaluation Plans That Survive Reality") {
		t.Fatalf("public calendar leaked a draft session:\n%s", content)
	}
	if got := strings.Count(content, "BEGIN:VEVENT\r\n"); got != 6 {
		t.Fatalf("public calendar emitted %d sessions, want 6", got)
	}
}

func TestEventCalendarRejectsUnknownSlug(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	if _, _, err := EventCalendar(state, "another-event"); err == nil {
		t.Fatal("EventCalendar accepted an unknown event slug")
	}
}

func TestSpeakerCalendarContainsAssignedSessions(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
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

func TestSpeakerCalendarUsesPublishMethod(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	data, _, err := SpeakerCalendar(state, "spk_maya")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "METHOD:PUBLISH\r\n") {
		t.Fatalf("feed did not carry METHOD:PUBLISH:\n%s", data)
	}
}

// TestSpeakerCalendarExcludesDraftSessions proves the SE-6 filter: a draft
// session assigned to the speaker never reaches the feed, and the feed
// never stamps a draft CONFIRMED because it never emits the event at all.
func TestSpeakerCalendarExcludesDraftSessions(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	draftFound := false
	for _, session := range state.Sessions {
		if session.Status == "draft" && contains(session.SpeakerIDs, "spk_samira") {
			draftFound = true
		}
	}
	if !draftFound {
		t.Fatal("test fixture assumption broken: seed no longer assigns spk_samira a draft session")
	}
	data, _, err := SpeakerCalendar(state, "spk_samira")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "Evaluation Plans That Survive Reality") {
		t.Fatalf("draft session leaked into the published feed:\n%s", content)
	}
	if !strings.Contains(content, "BEGIN:VCALENDAR\r\n") || !strings.Contains(content, "END:VCALENDAR\r\n") {
		t.Fatalf("feed is not a well-formed (if empty) calendar:\n%s", content)
	}
}

func TestSpeakerCalendarUnknownSpeaker(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	if _, _, err := SpeakerCalendar(state, "spk_does_not_exist"); err == nil {
		t.Fatal("SpeakerCalendar with an unknown speaker returned nil error, want an error")
	}
}

func seedInviteFixture(t *testing.T) (domain.State, domain.Session, domain.Speaker) {
	t.Helper()
	state := fixture.Seed(time.Now().UTC())
	var session domain.Session
	found := false
	for _, item := range state.Sessions {
		if item.ID == "ses_memory" {
			session = item
			found = true
		}
	}
	if !found {
		t.Fatal("test fixture assumption broken: seed no longer has ses_memory")
	}
	speaker, ok := state.Speaker("spk_maya")
	if !ok {
		t.Fatal("test fixture assumption broken: seed no longer has spk_maya")
	}
	return state, session, *speaker
}

// TestInviteIsAnAcceptDeclineRequest proves Invite's output has the shape a
// mail client renders as an accept/decline card: METHOD:REQUEST, exactly
// one ATTENDEE naming the speaker's own address, an ORGANIZER, a UID, and
// SEQUENCE:0.
func TestInviteIsAnAcceptDeclineRequest(t *testing.T) {
	state, session, speaker := seedInviteFixture(t)
	data, err := Invite(state, session, speaker, "organizer@example.com")
	if err != nil {
		t.Fatalf("Invite returned an error: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "METHOD:REQUEST\r\n") {
		t.Fatalf("invite did not carry METHOD:REQUEST:\n%s", content)
	}
	if got := strings.Count(content, "ATTENDEE;"); got != 1 {
		t.Fatalf("invite had %d ATTENDEE lines, want exactly 1:\n%s", got, content)
	}
	if !strings.Contains(content, "mailto:"+speaker.Email) {
		t.Fatalf("ATTENDEE did not carry the speaker's email %q:\n%s", speaker.Email, content)
	}
	if !strings.Contains(content, "ORGANIZER;CN=") || !strings.Contains(content, "mailto:organizer@example.com") {
		t.Fatalf("invite did not carry an ORGANIZER for organizer@example.com:\n%s", content)
	}
	if !strings.Contains(content, "UID:"+session.ID+"@rostrum.local\r\n") {
		t.Fatalf("invite did not carry the expected UID:\n%s", content)
	}
	if !strings.Contains(content, "SEQUENCE:0\r\n") {
		t.Fatalf("invite did not carry SEQUENCE:0:\n%s", content)
	}
}

func TestInviteStatusConfirmedOnlyForPublishedSessions(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		wantStatus string
	}{
		{"published session confirms", "published", "STATUS:CONFIRMED"},
		{"draft session stays tentative", "draft", "STATUS:TENTATIVE"},
		{"unknown status stays tentative", "", "STATUS:TENTATIVE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state, session, speaker := seedInviteFixture(t)
			session.Status = c.status
			data, err := Invite(state, session, speaker, "organizer@example.com")
			if err != nil {
				t.Fatalf("Invite returned an error: %v", err)
			}
			content := string(data)
			if !strings.Contains(content, c.wantStatus+"\r\n") {
				t.Fatalf("status = %q, content:\n%s", c.wantStatus, content)
			}
			if c.status != "published" && strings.Contains(content, "STATUS:CONFIRMED") {
				t.Fatalf("a %q session was stamped CONFIRMED; a draft must never be:\n%s", c.status, content)
			}
		})
	}
}

func TestInviteRequiresAScheduledSession(t *testing.T) {
	state, session, speaker := seedInviteFixture(t)
	session.StartsAt = time.Time{}
	session.EndsAt = time.Time{}
	if _, err := Invite(state, session, speaker, "organizer@example.com"); err == nil {
		t.Fatal("Invite on an unscheduled session returned nil error, want an error")
	}
}

func TestInviteRequiresASpeakerEmail(t *testing.T) {
	state, session, speaker := seedInviteFixture(t)
	speaker.Email = ""
	if _, err := Invite(state, session, speaker, "organizer@example.com"); err == nil {
		t.Fatal("Invite with no speaker email returned nil error, want an error")
	}
}

func TestInviteRequiresAnOrganizerEmail(t *testing.T) {
	state, session, speaker := seedInviteFixture(t)
	if _, err := Invite(state, session, speaker, "  "); err == nil {
		t.Fatal("Invite with a blank organizer email returned nil error, want an error")
	}
}

// TestEscapeHandlesEveryLineBreakStyle proves L1: a lone CR, a CRLF pair,
// and a lone LF each become the literal two-character escape "\n" — none
// of them may reach the wire as a raw control character, which would open
// a stray physical line an iCalendar parser does not expect.
func TestEscapeHandlesEveryLineBreakStyle(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"CRLF", "a\r\nb", `a\nb`},
		{"lone CR", "a\rb", `a\nb`},
		{"lone LF", "a\nb", `a\nb`},
		{"semicolon", "a;b", `a\;b`},
		{"comma", "a,b", `a\,b`},
		{"backslash", `a\b`, `a\\b`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := escape(c.input); got != c.want {
				t.Fatalf("escape(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestEscapeLeavesNoRawCarriageReturn(t *testing.T) {
	if got := escape("line one\rline two\r\nline three"); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("escape left a raw CR or LF in %q", got)
	}
}

// TestInviteFoldsLongLines proves a long content line — here, a
// description well past 75 octets — is folded per RFC 5545: every
// physical line the writer emits is at most 75 octets wide, and each
// continuation line begins with a single space right after its CRLF.
func TestInviteFoldsLongLines(t *testing.T) {
	state, session, speaker := seedInviteFixture(t)
	session.Description = strings.Repeat("a very long description that must fold correctly. ", 5)
	data, err := Invite(state, session, speaker, "organizer@example.com")
	if err != nil {
		t.Fatalf("Invite returned an error: %v", err)
	}
	foldedContinuation := false
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\r\n"), "\r\n") {
		if len([]byte(line)) > 75 {
			t.Fatalf("physical line %d is %d octets, want <= 75: %q", i, len([]byte(line)), line)
		}
		if i > 0 && strings.HasPrefix(line, " ") {
			foldedContinuation = true
		}
	}
	if !foldedContinuation {
		t.Fatalf("did not find any folded continuation line for the long description:\n%s", data)
	}
}

func TestFoldLineNeverSplitsAMultiByteRune(t *testing.T) {
	// "é" is two UTF-8 bytes (0xC3 0xA9); place enough of them that a
	// naive 75-byte cut lands inside one.
	value := "SUMMARY:" + strings.Repeat("é", 60)
	folded := foldLine(value)
	for _, line := range strings.Split(folded, "\r\n ") {
		if !isValidUTF8Line(line) {
			t.Fatalf("fold produced an invalid UTF-8 continuation: %q\nfull:\n%s", line, folded)
		}
	}
}

func isValidUTF8Line(s string) bool {
	return strings.ToValidUTF8(s, "\uFFFD") == s
}
