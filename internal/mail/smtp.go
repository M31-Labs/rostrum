package mail

import (
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPSender delivers mail through a real relay using the standard
// library's net/smtp. Build one from environment configuration through
// FromEnv, or construct it directly for a specific relay.
type SMTPSender struct {
	Host     string
	Port     string // defaults to "587" when empty
	Username string // omit for an open relay that needs no auth
	Password string
	From     string // envelope and header From address, for example "Rostrum <noreply@example.com>"
}

// Name reports the sender identity ("smtp") for a Communication record's
// Provider field.
func (s *SMTPSender) Name() string {
	return "smtp"
}

// Send formats msg as an RFC 5322 message with FormatMessage and delivers
// it over SMTP, authenticating with PLAIN auth when Username is set. It
// dials the relay named by Host and Port, so it performs network I/O; a
// test that wants to check composition without a network call should call
// FormatMessage directly instead.
func (s *SMTPSender) Send(msg Message) error {
	if strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("mail: SMTPSender has no host configured")
	}
	if strings.TrimSpace(msg.To) == "" {
		return fmt.Errorf("mail: message has no recipient")
	}
	addr := net.JoinHostPort(s.Host, s.port())
	body := FormatMessage(s.From, msg)
	var auth smtp.Auth
	if strings.TrimSpace(s.Username) != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	return smtp.SendMail(addr, auth, AddressOnly(s.From), []string{msg.To}, body)
}

func (s *SMTPSender) port() string {
	if strings.TrimSpace(s.Port) == "" {
		return "587"
	}
	return s.Port
}

// FormatMessage renders msg as an RFC 5322 message ready to hand to
// smtp.SendMail: a header block (From, To, Subject, Date, MIME-Version,
// Content-Type, and, for a plain-text message, Content-Transfer-Encoding),
// a blank line, then the body with every line ending in CRLF as RFC 5322
// requires. It performs no network I/O, so a test can call it directly to
// check composition without dialing a relay.
//
// When msg.Calendar is set, the body is instead a multipart/mixed entity
// (InviteMIME) carrying msg.TextBody as its text/plain part and
// msg.Calendar as a "text/calendar; method=REQUEST" part — the shape
// Gmail and Outlook render as a native accept/decline card, and any other
// client offers as an invite.ics attachment.
func FormatMessage(from string, msg Message) []byte {
	var b strings.Builder
	headers := []string{
		"From: " + from,
		"To: " + formatAddress(msg.ToName, msg.To),
		"Subject: " + encodeSubject(msg.Subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
	}
	body := toCRLF(msg.TextBody)
	if len(msg.Calendar) > 0 {
		multipartBody, err := InviteMIME(inviteBoundary, msg.TextBody, msg.Calendar)
		if err == nil {
			// A multipart entity declares its own per-part transfer
			// encoding (InviteMIME sets 8bit on both parts), so the
			// top-level header omits Content-Transfer-Encoding here.
			headers = append(headers, "Content-Type: "+InviteContentType(inviteBoundary))
			body = multipartBody
		} else {
			// inviteBoundary is a fixed, valid boundary, so this branch is
			// unreachable in practice; fall back to a plain-text message
			// rather than silently drop the calendar invite from view.
			headers = append(headers, `Content-Type: text/plain; charset="UTF-8"`, "Content-Transfer-Encoding: 8bit")
		}
	} else {
		headers = append(headers, `Content-Type: text/plain; charset="UTF-8"`, "Content-Transfer-Encoding: 8bit")
	}
	for _, header := range headers {
		b.WriteString(header)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// formatAddress renders a display-name address per RFC 5322 3.4 (a quoted
// name plus an angle-bracketed address), or the bare address when name is
// empty.
func formatAddress(name, address string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return address
	}
	return fmt.Sprintf("%q <%s>", name, address)
}

// AddressOnly strips a "Name <addr>" header value down to the bare
// address smtp.SendMail expects for its envelope-sender argument. It
// returns from unchanged when no angle brackets are present. Exported so a
// caller that needs a bare address from a "Name <addr>" value configured
// elsewhere (for example calendar.Invite's organizerEmail argument, sourced
// from MAIL_FROM) can reuse this instead of duplicating the parsing.
func AddressOnly(from string) string {
	open := strings.IndexByte(from, '<')
	shut := strings.IndexByte(from, '>')
	if open >= 0 && shut > open {
		return strings.TrimSpace(from[open+1 : shut])
	}
	return strings.TrimSpace(from)
}

// encodeSubject leaves an ASCII-only subject unencoded — the common case
// for this application — and MIME-encodes anything else (RFC 2047) so a
// non-ASCII subject never corrupts the header block.
func encodeSubject(subject string) string {
	for _, r := range subject {
		if r > 127 {
			return mime.QEncoding.Encode("UTF-8", subject)
		}
	}
	return subject
}

// toCRLF normalizes body to CRLF line endings, first collapsing any
// existing CRLF to LF so a mixed-ending input never doubles up.
func toCRLF(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	return strings.ReplaceAll(body, "\n", "\r\n")
}
