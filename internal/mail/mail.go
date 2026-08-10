// Package mail sends transactional email for Rostrum. Today that means
// one message: the confirmation a speaker receives right after submitting a
// proposal, carrying a link straight into their portal.
//
// The package defines one Sender interface with two implementations:
//
//   - OutboxSender, the zero-credential demo default. It records every
//     message instead of dialing a network relay, so a judge or a local run
//     with no SMTP configuration still gets an observable "sent" record.
//   - SMTPSender, a real relay over the standard library's net/smtp.
//
// FromEnv chooses between them from environment configuration, so the rest
// of the application never branches on which one is active.
//
// This package is self-contained: it does not import internal/store or any
// other Rostrum package. A caller sends outside any store lock and
// records the outcome (sent or failed) as a separate step; see
// SendConfirmation's doc comment for the exact call to make and where.
package mail

import (
	"os"
	"strings"
	"sync"
)

// Message is one email, independent of how it gets delivered.
type Message struct {
	To       string // recipient address, for example "ada@example.com"
	ToName   string // recipient display name, for example "Ada Lovelace"
	Subject  string
	TextBody string

	// Calendar carries an optional RFC 5545 calendar invite (for example
	// calendar.Invite's return value) to attach alongside TextBody. Leave
	// it nil for a plain-text message. When set, FormatMessage composes a
	// multipart/mixed body carrying TextBody as its text/plain part and
	// Calendar as a "text/calendar; method=REQUEST" part named
	// invite.ics — the shape Gmail and Outlook render as a native
	// accept/decline card. A Sender that only records messages (for
	// example OutboxSender) keeps Calendar as a separate field rather than
	// folding it into TextBody, so a test or an outbox viewer can inspect
	// the invite bytes directly instead of parsing MIME out of the body.
	Calendar []byte
}

// Sender delivers one Message. Send returns a non-nil error only when the
// message did not reach its destination. A caller records the outcome (for
// example in a Communication row); it must never store or render the raw
// error text on a page — log the detail, keep a generic category instead.
type Sender interface {
	Send(msg Message) error
}

// Named is an optional capability a Sender may implement to report a
// stable identity, for example for a Communication record's Provider
// field. Both OutboxSender and SMTPSender implement it.
type Named interface {
	Name() string
}

// OutboxSender is the demo-safe default. It records every message it
// receives instead of dialing a network relay, so submitting a proposal
// with no SMTP configuration still produces an observable "sent" row.
// OutboxSender never performs network I/O and never fails.
type OutboxSender struct {
	mu   sync.Mutex
	sent []Message
}

// NewOutboxSender returns an empty OutboxSender ready to record messages.
func NewOutboxSender() *OutboxSender {
	return &OutboxSender{}
}

// Send records msg and always returns nil. The outbox has no failure mode
// because it never touches the network.
func (o *OutboxSender) Send(msg Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, msg)
	return nil
}

// Sent returns a copy of every message recorded so far, oldest first. A
// test, or an outbox page presenter, uses this to observe what "sent"
// without a real relay.
func (o *OutboxSender) Sent() []Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Message, len(o.sent))
	copy(out, o.sent)
	return out
}

// Name reports the sender identity for a Communication record's Provider
// field. It matches the "demo-outbox" value the application already
// records today.
func (o *OutboxSender) Name() string {
	return "demo-outbox"
}

// FromEnv builds the Sender the process should use, chosen once from
// environment configuration: SMTPSender when SMTP_HOST is set, otherwise
// OutboxSender. Call it once at startup and reuse the result. FromEnv
// performs no network I/O itself, so choosing SMTP never blocks or fails
// here even when the relay turns out to be unreachable; that failure
// surfaces on the first Send instead.
//
// Recognized environment variables:
//
//   - SMTP_HOST — relay hostname; SMTP is selected when this is set.
//   - SMTP_PORT — relay port; defaults to "587" (STARTTLS submission).
//   - SMTP_USER — username for PLAIN auth; omit for an open relay.
//   - SMTP_PASSWORD — password for PLAIN auth.
//   - MAIL_FROM — the From address, for example "Rostrum <noreply@example.com>".
func FromEnv() Sender {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		return NewOutboxSender()
	}
	return &SMTPSender{
		Host:     host,
		Port:     envOrDefault("SMTP_PORT", "587"),
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("MAIL_FROM"),
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
