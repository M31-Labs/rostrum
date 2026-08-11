package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
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

func TestSendConfirmationWithKeyCarriesProviderIdempotencyKey(t *testing.T) {
	outbox := NewOutboxSender()
	if err := SendConfirmationWithKey(outbox, "https://cfp.example.com", Recipient{SpeakerID: "spk_1", Email: "ada@example.com"}, Submission{Title: "Compilers"}, "portal-key", "comm_123"); err != nil {
		t.Fatalf("SendConfirmationWithKey: %v", err)
	}
	if got := outbox.Sent()[0].IdempotencyKey; got != "comm_123" {
		t.Fatalf("IdempotencyKey = %q, want %q", got, "comm_123")
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

// TestFormatMessageAttachesCalendarPart proves FormatMessage composes a
// multipart/mixed body carrying both a text/plain part and a
// text/calendar; method=REQUEST part when a Message sets Calendar — the
// shape a calendar-invite send (SendInvite, or an AttachCalendar template)
// needs so Gmail and Outlook render a native accept/decline card. It
// decodes the output with the standard library's own mail and multipart
// readers rather than checking substrings, so it proves the composed
// message actually parses, not just that it contains the right text.
func TestFormatMessageAttachesCalendarPart(t *testing.T) {
	msg := Message{
		To:       "ada@example.com",
		ToName:   "Ada Lovelace",
		Subject:  "Invite: Memory Without Mystery",
		TextBody: "Hi Ada,\n\nYou're invited.\n",
		Calendar: []byte(testICS),
	}
	raw := FormatMessage(`"Rostrum" <noreply@example.com>`, msg)

	parsed, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("net/mail could not parse FormatMessage's output: %v\n--- raw ---\n%s", err, raw)
	}
	contentType := parsed.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("Content-Type header %q did not parse: %v", contentType, err)
	}
	if mediaType != "multipart/mixed" {
		t.Fatalf("media type = %q, want %q", mediaType, "multipart/mixed")
	}
	boundary := params["boundary"]
	if boundary == "" {
		t.Fatalf("Content-Type %q carries no boundary param", contentType)
	}

	body, err := io.ReadAll(parsed.Body)
	if err != nil {
		t.Fatalf("could not read parsed body: %v", err)
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var sawText, sawCalendar bool
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
		partType := part.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(partType, "text/plain"):
			sawText = true
			if !strings.Contains(string(content), "You're invited.") {
				t.Fatalf("text/plain part missing the message body: %q", content)
			}
		case strings.HasPrefix(partType, "text/calendar"):
			sawCalendar = true
			partMediaType, partParams, err := mime.ParseMediaType(partType)
			if err != nil {
				t.Fatalf("text/calendar Content-Type %q did not parse: %v", partType, err)
			}
			if partMediaType != "text/calendar" || partParams["method"] != "REQUEST" {
				t.Fatalf("calendar part Content-Type = %q, want text/calendar; method=REQUEST", partType)
			}
			if disp := part.Header.Get("Content-Disposition"); !strings.Contains(disp, `filename="invite.ics"`) {
				t.Fatalf("calendar part Content-Disposition = %q, want an invite.ics attachment", disp)
			}
			if string(content) != testICS {
				t.Fatalf("calendar part content did not round-trip the ics bytes exactly.\ngot:\n%q\nwant:\n%q", content, testICS)
			}
		}
	}
	if !sawText {
		t.Fatal("no text/plain part found in the composed message")
	}
	if !sawCalendar {
		t.Fatal("no text/calendar part found in the composed message")
	}
}

// TestFormatMessageOmitsCalendarPartWhenUnset proves a Message with no
// Calendar set still renders the plain text/plain shape every other
// message (for example SendConfirmation's) uses, so adding the Calendar
// field never changes composition for a caller that does not set it.
func TestFormatMessageOmitsCalendarPartWhenUnset(t *testing.T) {
	raw := FormatMessage("from@example.com", Message{To: "a@example.com", Subject: "plain", TextBody: "body"})
	parsed, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("net/mail could not parse the message: %v", err)
	}
	if got := parsed.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want a text/plain header", got)
	}
	if got := parsed.Header.Get("Content-Transfer-Encoding"); got != "8bit" {
		t.Fatalf("Content-Transfer-Encoding = %q, want %q", got, "8bit")
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
	t.Setenv("MAIL_DRIVER", "")
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("SMTP_HOST", "")
	sender := FromEnv()
	if _, ok := sender.(*OutboxSender); !ok {
		t.Fatalf("FromEnv() = %T, want *OutboxSender with no configured transport", sender)
	}
}

func TestFromEnvSelectsSMTPWhenHostIsSet(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "")
	t.Setenv("RESEND_API_KEY", "")
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
	t.Setenv("MAIL_DRIVER", "smtp")
	t.Setenv("RESEND_API_KEY", "")
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
	_ Sender = (*ResendSender)(nil)
	_ Named  = (*OutboxSender)(nil)
	_ Named  = (*SMTPSender)(nil)
	_ Named  = (*ResendSender)(nil)
)
