package mail

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendSenderPostsMailAndCalendarAttachment(t *testing.T) {
	var got resendEmail
	var gotIdempotency string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/emails" {
			t.Fatalf("request = %s %s, want POST /emails", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer re_test_key" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		gotIdempotency = request.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Fatalf("decode Resend payload: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()

	sender := &ResendSender{
		APIKey:  "re_test_key",
		BaseURL: server.URL,
		From:    "Rostrum <noreply@example.com>",
		Client:  server.Client(),
	}
	calendar := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	if err := sender.Send(Message{
		To: "ada@example.com", Subject: "Invitation", TextBody: "Hi Ada", Calendar: calendar,
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.From != "Rostrum <noreply@example.com>" || len(got.To) != 1 || got.To[0] != "ada@example.com" || got.Subject != "Invitation" || got.Text != "Hi Ada" {
		t.Fatalf("Resend payload = %+v, want mapped email fields", got)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "invite.ics" {
		t.Fatalf("attachments = %+v, want one invite.ics attachment", got.Attachments)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.Attachments[0].Content)
	if err != nil || string(decoded) != string(calendar) {
		t.Fatalf("calendar attachment = %q, decode err = %v, want %q", got.Attachments[0].Content, err, calendar)
	}
	if !strings.HasPrefix(gotIdempotency, "rostrum-") || len(gotIdempotency) > 256 {
		t.Fatalf("Idempotency-Key = %q, want a bounded Rostrum key", gotIdempotency)
	}
}

func TestResendSenderHonorsExplicitIdempotencyKey(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotKey = request.Header.Get("Idempotency-Key")
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	sender := &ResendSender{APIKey: "re_test", BaseURL: server.URL, From: "noreply@example.com", Client: server.Client()}
	if err := sender.Send(Message{To: "ada@example.com", IdempotencyKey: "comm_123"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotKey != "comm_123" {
		t.Fatalf("Idempotency-Key = %q, want caller-supplied key", gotKey)
	}
}

func TestResendSenderSanitizesRemoteFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"message":"the secret provider response must not leak"}`))
	}))
	defer server.Close()
	sender := &ResendSender{APIKey: "re_do_not_leak", BaseURL: server.URL, From: "noreply@example.com", Client: server.Client()}
	err := sender.Send(Message{To: "ada@example.com"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Send error = %v, want a safe status category", err)
	}
	if strings.Contains(err.Error(), "secret provider response") || strings.Contains(err.Error(), "re_do_not_leak") {
		t.Fatalf("Send error leaked remote response or API key: %v", err)
	}
}

func TestResendSenderRejectsIncompleteConfiguration(t *testing.T) {
	for _, sender := range []*ResendSender{
		{From: "noreply@example.com"},
		{APIKey: "re_test"},
	} {
		if err := sender.Send(Message{To: "ada@example.com"}); err == nil {
			t.Fatalf("Send(%+v) returned nil, want config error", sender)
		}
	}
}

func TestFromEnvSelectsResendAndChecksTransportReadiness(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "resend")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("RESEND_API_BASE_URL", "https://resend.example.test")
	t.Setenv("MAIL_FROM", "Rostrum <noreply@example.com>")
	sender, ok := FromEnv().(*ResendSender)
	if !ok {
		t.Fatalf("FromEnv() = %T, want *ResendSender", FromEnv())
	}
	if sender.APIKey != "re_test" || sender.BaseURL != "https://resend.example.test" || sender.From != "Rostrum <noreply@example.com>" {
		t.Fatalf("ResendSender = %+v, want environment values", sender)
	}
	if !TransportConfigured() {
		t.Fatal("TransportConfigured() = false, want a complete Resend configuration")
	}
}

func TestFromEnvDoesNotTreatIncompleteOrUnknownTransportAsReady(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "resend")
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("MAIL_FROM", "")
	if TransportConfigured() {
		t.Fatal("TransportConfigured() = true for an incomplete Resend configuration")
	}
	t.Setenv("MAIL_DRIVER", "not-a-provider")
	if _, ok := FromEnv().(configurationErrorSender); !ok {
		t.Fatalf("FromEnv() = %T, want configurationErrorSender for unknown driver", FromEnv())
	}
	if TransportConfigured() {
		t.Fatal("TransportConfigured() = true for an unknown driver")
	}
}
