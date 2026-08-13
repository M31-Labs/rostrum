// Package mail sends transactional email for Rostrum. Today that means
// one message: the confirmation a speaker receives right after submitting a
// proposal, carrying a link straight into their portal.
//
// The package defines one Sender interface with three implementations:
//
//   - OutboxSender, the zero-credential local default. It records every
//     message instead of dialing a network relay, so an offline installation
//     still gets an observable delivery record without claiming real receipt.
//   - SMTPSender, a real relay over the standard library's net/smtp. It
//     keeps self-hosted and standards-based deployments fully viable.
//   - ResendSender, a small HTTP API adapter for Resend-style transactional
//     delivery. Other providers can implement Sender without changing the
//     submission, acceptance, or reminder workflows.
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
	"fmt"
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

	// IdempotencyKey is an optional stable key for providers that support
	// de-duplication. ResendSender derives a deterministic key when it is
	// empty, but callers that retry a known Communication should pass that
	// row's ID here.
	IdempotencyKey string
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

// OutboxSender is the network-free default. It records every message it
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
// field.
func (o *OutboxSender) Name() string {
	return "outbox"
}

// configurationErrorSender fails closed for an invalid explicit mail
// configuration. Treating an unknown MAIL_DRIVER as the local outbox would
// make a production deployment look as if it sent real mail when it did not.
type configurationErrorSender struct {
	err error
}

func (sender configurationErrorSender) Send(Message) error { return sender.err }
func (sender configurationErrorSender) Name() string       { return "mail-config" }

// FromEnv builds the Sender the process should use. MAIL_DRIVER can select
// outbox, smtp, or resend explicitly. When it is absent, Resend wins when a
// RESEND_API_KEY exists, followed by SMTP when SMTP_HOST exists, followed by
// the zero-credential local outbox. That preserves older SMTP-only setups
// while making a Resend deployment require no application code fork.
//
// FromEnv performs no network I/O itself, so choosing a network provider
// never blocks or fails here; transport failures surface on the first Send.
//
// Recognized environment variables:
//
//   - MAIL_DRIVER — optional explicit outbox, smtp, or resend selection.
//   - RESEND_API_KEY — API key for the Resend email API.
//   - RESEND_API_BASE_URL — override for a Resend-compatible test endpoint;
//     defaults to https://api.resend.com.
//   - SMTP_HOST — relay hostname; SMTP is selected when this is set.
//   - SMTP_PORT — relay port; defaults to "587" (STARTTLS submission).
//   - SMTP_USER — username for PLAIN auth; omit for an open relay.
//   - SMTP_PASSWORD — password for PLAIN auth.
//   - MAIL_FROM — the From address, for example "Rostrum <noreply@example.com>".
func FromEnv() Sender {
	driver, err := driverFromEnv()
	if err != nil {
		return configurationErrorSender{err: err}
	}
	switch driver {
	case "outbox":
		return NewOutboxSender()
	case "resend":
		return &ResendSender{
			APIKey:  os.Getenv("RESEND_API_KEY"),
			BaseURL: envOrDefault("RESEND_API_BASE_URL", defaultResendAPIBaseURL),
			From:    os.Getenv("MAIL_FROM"),
		}
	case "smtp":
		return &SMTPSender{
			Host:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
			Port:     envOrDefault("SMTP_PORT", "587"),
			Username: os.Getenv("SMTP_USER"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("MAIL_FROM"),
		}
	default:
		return configurationErrorSender{err: fmt.Errorf("mail: unsupported driver %q", driver)}
	}
}

// TransportConfigured reports whether FromEnv resolves to a complete real
// transport. Setup uses it to decide if sending a magic link is possible;
// a selected but incomplete SMTP or Resend configuration is intentionally
// not treated as ready.
func TransportConfigured() bool {
	driver, err := driverFromEnv()
	if err != nil {
		return false
	}
	switch driver {
	case "resend":
		return strings.TrimSpace(os.Getenv("RESEND_API_KEY")) != "" && strings.TrimSpace(os.Getenv("MAIL_FROM")) != ""
	case "smtp":
		return strings.TrimSpace(os.Getenv("SMTP_HOST")) != "" && strings.TrimSpace(os.Getenv("MAIL_FROM")) != ""
	default:
		return false
	}
}

func driverFromEnv() (string, error) {
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("MAIL_DRIVER")))
	switch driver {
	case "", "auto":
		if strings.TrimSpace(os.Getenv("RESEND_API_KEY")) != "" {
			return "resend", nil
		}
		if strings.TrimSpace(os.Getenv("SMTP_HOST")) != "" {
			return "smtp", nil
		}
		return "outbox", nil
	case "outbox", "fake":
		return "outbox", nil
	case "smtp", "resend":
		return driver, nil
	default:
		return "", fmt.Errorf("mail: MAIL_DRIVER must be outbox, smtp, or resend (got %q)", driver)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
