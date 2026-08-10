package mail

import (
	"bytes"
	"io"
	"mime"
	netmail "net/mail"
	"strings"
	"testing"
)

func TestOutboxSenderRecordsMessages(t *testing.T) {
	outbox := NewOutboxSender()
	if err := outbox.Send(Message{To: "a@example.com", Subject: "one"}); err != nil {
		t.Fatalf("Send returned an error: %v", err)
	}
	if err := outbox.Send(Message{To: "b@example.com", Subject: "two"}); err != nil {
		t.Fatalf("Send returned an error: %v", err)
	}
	sent := outbox.Sent()
	if len(sent) != 2 {
		t.Fatalf("got %d recorded messages, want 2", len(sent))
	}
	if sent[0].Subject != "one" || sent[1].Subject != "two" {
		t.Fatalf("recorded messages out of order: %+v", sent)
	}
	if outbox.Name() != "demo-outbox" {
		t.Fatalf("Name() = %q, want %q", outbox.Name(), "demo-outbox")
	}
}

func TestOutboxSenderSentReturnsACopy(t *testing.T) {
	outbox := NewOutboxSender()
	_ = outbox.Send(Message{To: "a@example.com"})
	sent := outbox.Sent()
	sent[0].To = "mutated@example.com"
	if outbox.Sent()[0].To != "a@example.com" {
		t.Fatalf("Sent() leaked internal storage: mutation through the returned slice changed recorded state")
	}
}

func TestSendConfirmationComposesKeyedPortalURL(t *testing.T) {
	outbox := NewOutboxSender()
	speaker := Recipient{SpeakerID: "spk_01", Name: "Ada Lovelace", Email: "ada@example.com"}
	submission := Submission{Title: "Engines and Analysis"}

	if err := SendConfirmation(outbox, "https://cfp.example.com", speaker, submission, "tok_abc123"); err != nil {
		t.Fatalf("SendConfirmation returned an error: %v", err)
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
	wantSubject := "We received Engines and Analysis"
	if msg.Subject != wantSubject {
		t.Fatalf("Subject = %q, want %q", msg.Subject, wantSubject)
	}

	wantURL := "https://cfp.example.com/portal/spk_01?key=tok_abc123"
	if !strings.Contains(msg.TextBody, wantURL) {
		t.Fatalf("body does not contain the keyed portal URL %q; body:\n%s", wantURL, msg.TextBody)
	}
	if !strings.Contains(msg.TextBody, submission.Title) {
		t.Fatalf("body does not mention the submitted title %q; body:\n%s", submission.Title, msg.TextBody)
	}
}

func TestSendConfirmationTrimsTrailingSlashFromBase(t *testing.T) {
	outbox := NewOutboxSender()
	speaker := Recipient{SpeakerID: "spk_02", Name: "Grace Hopper", Email: "grace@example.com"}
	if err := SendConfirmation(outbox, "https://cfp.example.com/", speaker, Submission{Title: "Compilers"}, "tok_xyz"); err != nil {
		t.Fatalf("SendConfirmation returned an error: %v", err)
	}
	want := "https://cfp.example.com/portal/spk_02?key=tok_xyz"
	if !strings.Contains(outbox.Sent()[0].TextBody, want) {
		t.Fatalf("body does not contain %q after trimming the base's trailing slash", want)
	}
	if strings.Contains(outbox.Sent()[0].TextBody, "com//portal") {
		t.Fatalf("base URL was not trimmed: found a doubled slash before /portal")
	}
}

func TestSendConfirmationFallsBackWithoutTitleOrName(t *testing.T) {
	outbox := NewOutboxSender()
	speaker := Recipient{SpeakerID: "spk_03", Email: "anon@example.com"}
	if err := SendConfirmation(outbox, "https://cfp.example.com", speaker, Submission{}, "tok"); err != nil {
		t.Fatalf("SendConfirmation returned an error: %v", err)
	}
	msg := outbox.Sent()[0]
	if msg.Subject != "We received your proposal" {
		t.Fatalf("Subject = %q, want the no-title fallback", msg.Subject)
	}
	if !strings.Contains(msg.TextBody, "Hi there,") {
		t.Fatalf("body does not use the no-name fallback greeting; body:\n%s", msg.TextBody)
	}
}

func TestSendConfirmationRejectsNilSender(t *testing.T) {
	if err := SendConfirmation(nil, "https://cfp.example.com", Recipient{}, Submission{}, "tok"); err == nil {
		t.Fatalf("SendConfirmation(nil, ...) returned nil error, want an error")
	}
}

func TestPortalURL(t *testing.T) {
	cases := []struct{ base, speakerID, key, want string }{
		{"https://cfp.example.com", "spk_1", "tok", "https://cfp.example.com/portal/spk_1?key=tok"},
		{"https://cfp.example.com/", "spk_1", "tok", "https://cfp.example.com/portal/spk_1?key=tok"},
	}
	for _, c := range cases {
		if got := PortalURL(c.base, c.speakerID, c.key); got != c.want {
			t.Fatalf("PortalURL(%q, %q, %q) = %q, want %q", c.base, c.speakerID, c.key, got, c.want)
		}
	}
}

// TestFormatMessageIsValidRFC5322 proves SMTPSender composes a valid
// RFC 5322 message without dialing a relay: it feeds FormatMessage's
// output straight into the standard library's own net/mail parser and
// checks what comes back out.
func TestFormatMessageIsValidRFC5322(t *testing.T) {
	msg := Message{
		To:       "ada@example.com",
		ToName:   "Ada Lovelace",
		Subject:  "We received Engines and Analysis",
		TextBody: "Hi Ada,\n\nThanks for submitting.\n\nPortal: https://cfp.example.com/portal/spk_01?key=tok\n",
	}
	raw := FormatMessage(`"Rostrum" <noreply@example.com>`, msg)

	parsed, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("net/mail could not parse FormatMessage's output as RFC 5322: %v\n--- raw ---\n%s", err, raw)
	}

	if got := parsed.Header.Get("Subject"); got != msg.Subject {
		t.Fatalf("Subject header = %q, want %q", got, msg.Subject)
	}
	to, err := parsed.Header.AddressList("To")
	if err != nil {
		t.Fatalf("To header did not parse as an address list: %v", err)
	}
	if len(to) != 1 || to[0].Address != "ada@example.com" || to[0].Name != "Ada Lovelace" {
		t.Fatalf("To address list = %+v, want one entry for Ada Lovelace <ada@example.com>", to)
	}
	from, err := parsed.Header.AddressList("From")
	if err != nil {
		t.Fatalf("From header did not parse as an address list: %v", err)
	}
	if len(from) != 1 || from[0].Address != "noreply@example.com" {
		t.Fatalf("From address list = %+v, want noreply@example.com", from)
	}
	if got := parsed.Header.Get("Mime-Version"); got != "1.0" {
		t.Fatalf("MIME-Version header = %q, want %q", got, "1.0")
	}

	body, err := io.ReadAll(parsed.Body)
	if err != nil {
		t.Fatalf("could not read parsed body: %v", err)
	}
	if !strings.Contains(string(body), "portal/spk_01?key=tok") {
		t.Fatalf("parsed body missing the portal URL; body:\n%s", body)
	}

	if !bytes.Contains(raw, []byte("\r\n\r\n")) {
		t.Fatalf("raw message has no CRLF CRLF header/body separator")
	}
	if bytes.Contains(bytes.ReplaceAll(raw, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatalf("raw message has a bare LF outside a CRLF pair")
	}
}

func TestFormatMessageEncodesNonASCIISubject(t *testing.T) {
	raw := FormatMessage("from@example.com", Message{To: "a@example.com", Subject: "café", TextBody: "body"})
	parsed, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("net/mail could not parse the message: %v", err)
	}
	// A non-ASCII header value must not appear raw in the header block: it
	// must arrive as an RFC 2047 encoded-word. Decoding it with the
	// standard library's own decoder and recovering the original text
	// proves the encoding is well formed.
	raw2047 := parsed.Header.Get("Subject")
	if !strings.Contains(strings.ToUpper(raw2047), "=?UTF-8?") {
		t.Fatalf("Subject header = %q, want an RFC 2047 encoded-word", raw2047)
	}
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(raw2047)
	if err != nil {
		t.Fatalf("could not decode the RFC 2047 Subject header %q: %v", raw2047, err)
	}
	if decoded != "café" {
		t.Fatalf("decoded Subject = %q, want %q", decoded, "café")
	}
}

func TestSMTPSenderSendRequiresHost(t *testing.T) {
	s := &SMTPSender{From: "a@example.com"}
	if err := s.Send(Message{To: "b@example.com"}); err == nil {
		t.Fatalf("Send with no Host configured returned nil error, want an error")
	}
}

func TestSMTPSenderSendRequiresRecipient(t *testing.T) {
	s := &SMTPSender{Host: "localhost", From: "a@example.com"}
	if err := s.Send(Message{}); err == nil {
		t.Fatalf("Send with no recipient returned nil error, want an error")
	}
}

func TestSMTPSenderName(t *testing.T) {
	if (&SMTPSender{}).Name() != "smtp" {
		t.Fatalf("SMTPSender.Name() = %q, want %q", (&SMTPSender{}).Name(), "smtp")
	}
}

func TestFromEnvSelectsOutboxByDefault(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	sender := FromEnv()
	if _, ok := sender.(*OutboxSender); !ok {
		t.Fatalf("FromEnv() = %T, want *OutboxSender when SMTP_HOST is unset", sender)
	}
}

func TestFromEnvSelectsSMTPWhenHostIsSet(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USER", "rostrum")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("MAIL_FROM", "Rostrum <noreply@example.com>")

	sender := FromEnv()
	smtpSender, ok := sender.(*SMTPSender)
	if !ok {
		t.Fatalf("FromEnv() = %T, want *SMTPSender when SMTP_HOST is set", sender)
	}
	if smtpSender.Host != "smtp.example.com" || smtpSender.Port != "2525" ||
		smtpSender.Username != "rostrum" || smtpSender.Password != "secret" ||
		smtpSender.From != "Rostrum <noreply@example.com>" {
		t.Fatalf("FromEnv() built %+v from environment, fields did not round-trip", smtpSender)
	}
}

func TestFromEnvDefaultsSMTPPort(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "")
	sender := FromEnv().(*SMTPSender)
	if sender.Port != "587" {
		t.Fatalf("default SMTP_PORT = %q, want %q", sender.Port, "587")
	}
}

// Compile-time assertions that both senders satisfy Sender and Named.
var (
	_ Sender = (*OutboxSender)(nil)
	_ Sender = (*SMTPSender)(nil)
	_ Named  = (*OutboxSender)(nil)
	_ Named  = (*SMTPSender)(nil)
)
