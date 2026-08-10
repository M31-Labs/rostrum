package mail

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"
)

// InviteSession is the session a calendar-invite email describes. It
// carries only what this package needs, so mail never depends on
// internal/domain or internal/calendar; a caller maps a domain.Session
// into one at the call site.
type InviteSession struct {
	Title    string
	StartsAt time.Time
	EndsAt   time.Time
	Location string // for example "Meridian Room, San Francisco, California"
}

// inviteBoundary is the fixed MIME boundary SendInvite composes with.
// Fixed rather than random so the composed body — and every test that
// checks its shape — is deterministic; a boundary only needs to be a
// string the two attached parts never contain verbatim, which this one,
// scoped to this package and this purpose, satisfies.
const inviteBoundary = "rostrum-invite-4f8f9d2b"

// SendInvite renders and sends a calendar-invite email for one speaker and
// one session: a short human-readable body plus the session's iCal
// invite, composed as a multipart/mixed body (InviteMIME) carrying a
// text/calendar; method=REQUEST part — the shape Gmail and Outlook render
// as a native accept/decline card, and any other client offers as an
// invite.ics attachment.
//
// Pass ics as the []byte calendar.Invite(state, session, speaker,
// organizerEmail) returns; this package stays self-contained and never
// imports internal/calendar or internal/domain (see the package doc
// comment in mail.go), so the caller builds the invite bytes and maps the
// domain types into Recipient/InviteSession at the call site — the same
// pattern SendConfirmation uses for Recipient/Submission.
//
// Call this after the durable mutation that makes the invite valid has
// committed, and outside any store lock — Send is a network operation,
// the same rule SendConfirmation's doc comment states. This unit
// (calendar invites) delivers the capability; wiring the call sites is a
// separate unit. Per the calendar-invite spec, wire SendInvite from three
// places:
//
//   - publishAgenda (app/organizer/agenda/page.server.go), once per
//     speaker, right after a session's Status flips to "published";
//   - queueMessage (app/organizer/communications/page.server.go), when
//     the selected template has AttachCalendar set;
//   - the submission-to-session acceptance flow, when a speaker accepts
//     an assigned session.
//
// Record the outcome as a Communication row exactly as SendConfirmation's
// doc comment describes: "sent" on a nil error, "failed" with a
// sanitized category otherwise — never the raw error text (M8).
func SendInvite(sender Sender, speaker Recipient, session InviteSession, ics []byte) error {
	if sender == nil {
		return fmt.Errorf("mail: SendInvite requires a Sender")
	}
	if strings.TrimSpace(speaker.Email) == "" {
		return fmt.Errorf("mail: SendInvite requires a speaker email")
	}
	if len(ics) == 0 {
		return fmt.Errorf("mail: SendInvite requires a non-empty calendar invite")
	}
	body, err := InviteMIME(inviteBoundary, inviteText(speaker, session), ics)
	if err != nil {
		return fmt.Errorf("mail: could not compose the invite MIME body: %w", err)
	}
	msg := Message{
		To:       speaker.Email,
		ToName:   speaker.Name,
		Subject:  inviteSubject(session),
		TextBody: body,
	}
	return sender.Send(msg)
}

// InviteMIME composes a multipart/mixed body (RFC 2046) carrying a
// human-readable text/plain part plus ics as a
// "text/calendar; method=REQUEST" part with an invite.ics filename — the
// two parts a calendar-invite email needs. It performs no I/O and, for a
// fixed boundary, is deterministic, so a test can check its shape without
// a relay.
//
// The returned bytes are the MIME entity's body only; the top-level
// message also needs a Content-Type header carrying this same boundary —
// see InviteContentType — which is why SendInvite documents the follow-up
// this leaves for whoever wires actual SMTP delivery: today's
// Message/FormatMessage pipeline (internal/mail/mail.go, smtp.go) has no
// field for a caller to override the outer Content-Type header the way a
// multipart message needs. Extending it is a small, separate change.
func InviteMIME(boundary, textBody string, ics []byte) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.SetBoundary(boundary); err != nil {
		return "", fmt.Errorf("mail: invalid MIME boundary %q: %w", boundary, err)
	}

	textPart, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {`text/plain; charset="UTF-8"`},
		"Content-Transfer-Encoding": {"8bit"},
	})
	if err != nil {
		return "", err
	}
	if _, err := textPart.Write([]byte(toCRLF(textBody))); err != nil {
		return "", err
	}

	icsPart, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {`text/calendar; method=REQUEST; charset="UTF-8"`},
		"Content-Transfer-Encoding": {"8bit"},
		"Content-Disposition":       {`attachment; filename="invite.ics"`},
	})
	if err != nil {
		return "", err
	}
	if _, err := icsPart.Write(ics); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// InviteContentType returns the top-level Content-Type header value the
// body InviteMIME(boundary, ...) returns needs on the enclosing message:
// multipart/mixed with the matching boundary, quoted per RFC 2045.
func InviteContentType(boundary string) string {
	return `multipart/mixed; boundary="` + boundary + `"`
}

func inviteSubject(session InviteSession) string {
	title := strings.TrimSpace(session.Title)
	if title == "" {
		return "Your session invite"
	}
	return "Invite: " + title
}

func inviteText(speaker Recipient, session InviteSession) string {
	name := strings.TrimSpace(speaker.Name)
	if name == "" {
		name = "there"
	}
	title := strings.TrimSpace(session.Title)
	if title == "" {
		title = "your session"
	}
	var body strings.Builder
	fmt.Fprintf(&body, "Hi %s,\n\n", name)
	fmt.Fprintf(&body, "You're invited to %q", title)
	if !session.StartsAt.IsZero() && !session.EndsAt.IsZero() {
		fmt.Fprintf(&body, " on %s", session.StartsAt.Format("Monday, January 2, 2006 at 15:04 MST"))
	}
	body.WriteString(".\n\n")
	if location := strings.TrimSpace(session.Location); location != "" {
		fmt.Fprintf(&body, "Location: %s\n\n", location)
	}
	body.WriteString("Open the attached invite to add it to your calendar and accept or decline.\n\n")
	body.WriteString("See you there,\nThe program team\n")
	return body.String()
}
