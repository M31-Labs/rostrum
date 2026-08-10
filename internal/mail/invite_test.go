package mail

import (
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

// A minimal but well-formed calendar invite, standing in for the bytes
// calendar.Invite would return — this package never imports
// internal/calendar (see mail.go's package doc comment), so a caller
// supplies the invite bytes; a test does the same with a fixture.
const testICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"METHOD:REQUEST\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:ses_memory@rostrum.local\r\n" +
	"ORGANIZER;CN=\"M31 Systems Forum 2026\":mailto:organizer@example.com\r\n" +
	"ATTENDEE;CN=\"Ada Lovelace\";RSVP=TRUE:mailto:ada@example.com\r\n" +
	"SEQUENCE:0\r\n" +
	"STATUS:CONFIRMED\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// invitePart is one decoded MIME part: its header and its content.
type invitePart struct {
	header  textproto.MIMEHeader
	content []byte
}

// parseInviteParts decodes a body InviteMIME returned, using the standard
// library's own multipart reader, and returns its parts — proving the
// composition round-trips through a real MIME parser rather than merely
// containing the right substrings.
func parseInviteParts(t *testing.T, boundary, body string) []invitePart {
	t.Helper()
	reader := multipart.NewReader(strings.NewReader(body), boundary)
	var parts []invitePart
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("could not read a MIME part: %v", err)
		}
		content, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("could not read a MIME part's content: %v", err)
		}
		parts = append(parts, invitePart{header: part.Header, content: content})
	}
	return parts
}

func TestInviteMIMEComposesTextAndCalendarParts(t *testing.T) {
	body, err := InviteMIME(inviteBoundary, "Hi Ada,\n\nSee the attached invite.\n", []byte(testICS))
	if err != nil {
		t.Fatalf("InviteMIME returned an error: %v", err)
	}

	parts := parseInviteParts(t, inviteBoundary, body)
	if len(parts) != 2 {
		t.Fatalf("got %d MIME parts, want 2 (text/plain, text/calendar)", len(parts))
	}

	textHeader := parts[0].header
	if got := textHeader["Content-Type"]; len(got) != 1 || !strings.HasPrefix(got[0], "text/plain") {
		t.Fatalf("part 0 Content-Type = %v, want a text/plain header", got)
	}
	if !strings.Contains(string(parts[0].content), "See the attached invite.") {
		t.Fatalf("part 0 content missing the human-readable body: %q", parts[0].content)
	}

	icsHeader := parts[1].header
	contentType := ""
	if got := icsHeader["Content-Type"]; len(got) == 1 {
		contentType = got[0]
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("part 1 Content-Type %q did not parse: %v", contentType, err)
	}
	if mediaType != "text/calendar" {
		t.Fatalf("part 1 media type = %q, want %q", mediaType, "text/calendar")
	}
	if params["method"] != "REQUEST" {
		t.Fatalf("part 1 Content-Type method param = %q, want %q", params["method"], "REQUEST")
	}
	if disp := icsHeader["Content-Disposition"]; len(disp) != 1 || !strings.Contains(disp[0], `filename="invite.ics"`) {
		t.Fatalf("part 1 Content-Disposition = %v, want an invite.ics attachment", disp)
	}
	if string(parts[1].content) != testICS {
		t.Fatalf("part 1 content did not round-trip the ics bytes exactly.\ngot:\n%q\nwant:\n%q", parts[1].content, testICS)
	}
}

func TestInviteMIMEIsDeterministic(t *testing.T) {
	first, err := InviteMIME(inviteBoundary, "same input", []byte(testICS))
	if err != nil {
		t.Fatalf("InviteMIME returned an error: %v", err)
	}
	second, err := InviteMIME(inviteBoundary, "same input", []byte(testICS))
	if err != nil {
		t.Fatalf("InviteMIME returned an error: %v", err)
	}
	if first != second {
		t.Fatalf("InviteMIME with identical inputs produced different output:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestInviteMIMERejectsAnInvalidBoundary(t *testing.T) {
	if _, err := InviteMIME("", "text", []byte(testICS)); err == nil {
		t.Fatal("InviteMIME with an empty boundary returned nil error, want an error")
	}
}

func TestInviteContentTypeParsesAsMultipartMixed(t *testing.T) {
	mediaType, params, err := mime.ParseMediaType(InviteContentType(inviteBoundary))
	if err != nil {
		t.Fatalf("InviteContentType did not parse as a media type: %v", err)
	}
	if mediaType != "multipart/mixed" {
		t.Fatalf("media type = %q, want %q", mediaType, "multipart/mixed")
	}
	if params["boundary"] != inviteBoundary {
		t.Fatalf("boundary param = %q, want %q", params["boundary"], inviteBoundary)
	}
}

func TestSendInviteSendsThroughTheGivenSender(t *testing.T) {
	outbox := NewOutboxSender()
	speaker := Recipient{SpeakerID: "spk_maya", Name: "Ada Lovelace", Email: "ada@example.com"}
	session := InviteSession{
		Title:    "Memory Without Mystery",
		StartsAt: time.Date(2026, time.October, 15, 9, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, time.October, 15, 9, 45, 0, 0, time.UTC),
		Location: "Meridian Room, San Francisco, California",
	}

	if err := SendInvite(outbox, speaker, session, []byte(testICS)); err != nil {
		t.Fatalf("SendInvite returned an error: %v", err)
	}

	sent := outbox.Sent()
	if len(sent) != 1 {
		t.Fatalf("got %d recorded messages, want 1", len(sent))
	}
	msg := sent[0]

	if msg.To != speaker.Email {
		t.Fatalf("To = %q, want %q", msg.To, speaker.Email)
	}
	if msg.ToName != speaker.Name {
		t.Fatalf("ToName = %q, want %q", msg.ToName, speaker.Name)
	}
	wantSubject := "Invite: Memory Without Mystery"
	if msg.Subject != wantSubject {
		t.Fatalf("Subject = %q, want %q", msg.Subject, wantSubject)
	}
	if !strings.Contains(msg.TextBody, "text/calendar; method=REQUEST") {
		t.Fatalf("composed body missing the text/calendar; method=REQUEST part header:\n%s", msg.TextBody)
	}
	if !strings.Contains(msg.TextBody, testICS) {
		t.Fatalf("composed body did not carry the ics bytes verbatim:\n%s", msg.TextBody)
	}
	if !strings.Contains(msg.TextBody, "Meridian Room, San Francisco, California") {
		t.Fatalf("composed body missing the session location:\n%s", msg.TextBody)
	}
}

func TestSendInviteRejectsNilSender(t *testing.T) {
	err := SendInvite(nil, Recipient{Email: "ada@example.com"}, InviteSession{}, []byte(testICS))
	if err == nil {
		t.Fatal("SendInvite(nil, ...) returned nil error, want an error")
	}
}

func TestSendInviteRejectsMissingSpeakerEmail(t *testing.T) {
	outbox := NewOutboxSender()
	err := SendInvite(outbox, Recipient{Name: "Ada Lovelace"}, InviteSession{}, []byte(testICS))
	if err == nil {
		t.Fatal("SendInvite with no speaker email returned nil error, want an error")
	}
}

func TestSendInviteRejectsEmptyICS(t *testing.T) {
	outbox := NewOutboxSender()
	err := SendInvite(outbox, Recipient{Email: "ada@example.com"}, InviteSession{}, nil)
	if err == nil {
		t.Fatal("SendInvite with no ics bytes returned nil error, want an error")
	}
}

func TestInviteSubjectFallsBackWithoutATitle(t *testing.T) {
	outbox := NewOutboxSender()
	if err := SendInvite(outbox, Recipient{Email: "ada@example.com"}, InviteSession{}, []byte(testICS)); err != nil {
		t.Fatalf("SendInvite returned an error: %v", err)
	}
	if got := outbox.Sent()[0].Subject; got != "Your session invite" {
		t.Fatalf("Subject = %q, want the no-title fallback", got)
	}
}

func TestInviteTextOmitsLocationWhenBlank(t *testing.T) {
	outbox := NewOutboxSender()
	session := InviteSession{Title: "Untitled Session"}
	if err := SendInvite(outbox, Recipient{Email: "ada@example.com"}, session, []byte(testICS)); err != nil {
		t.Fatalf("SendInvite returned an error: %v", err)
	}
	if strings.Contains(outbox.Sent()[0].TextBody, "Location:") {
		t.Fatalf("body carries a Location: line despite an empty session.Location:\n%s", outbox.Sent()[0].TextBody)
	}
}
