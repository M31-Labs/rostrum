package mail

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultResendAPIBaseURL is intentionally configurable only through
// RESEND_API_BASE_URL. Tests can point at an httptest server without a
// network dependency while deployments retain the documented Resend endpoint.
const defaultResendAPIBaseURL = "https://api.resend.com"

// ResendSender delivers transactional messages through Resend's POST
// /emails API. It implements Sender directly so application flows stay
// provider-neutral; a Postmark, SES, or other transactional sender needs to
// implement only Send and, optionally, Named.
type ResendSender struct {
	APIKey  string
	BaseURL string
	From    string
	Client  *http.Client
}

// Name reports the stable transport identity used in Communication records.
func (sender *ResendSender) Name() string { return "resend" }

// Send sends one plain-text message and carries a calendar invite, when
// present, as a base64 invite.ics attachment. It intentionally does not
// include provider response text in errors: callers may log errors but must
// never leak upstream payloads, recipient detail, or configuration into the
// organizer surface.
func (sender *ResendSender) Send(message Message) error {
	if strings.TrimSpace(sender.APIKey) == "" {
		return fmt.Errorf("mail: Resend sender has no API key configured")
	}
	if strings.TrimSpace(sender.From) == "" {
		return fmt.Errorf("mail: Resend sender has no From address configured")
	}
	if strings.TrimSpace(message.To) == "" {
		return fmt.Errorf("mail: message has no recipient")
	}
	endpoint, err := sender.endpoint()
	if err != nil {
		return err
	}
	payload := resendEmail{
		From:    sender.From,
		To:      []string{message.To},
		Subject: message.Subject,
		Text:    message.TextBody,
	}
	if len(message.Calendar) > 0 {
		payload.Attachments = []resendAttachment{{
			Filename: "invite.ics",
			Content:  base64.StdEncoding.EncodeToString(message.Calendar),
		}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("mail: encode Resend message: %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mail: prepare Resend request")
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sender.APIKey))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", resendIdempotencyKey(message))

	response, err := sender.client().Do(request)
	if err != nil {
		return fmt.Errorf("mail: Resend delivery request failed")
	}
	defer response.Body.Close()
	// Drain a bounded amount for connection reuse, but do not expose a remote
	// response body in a log or state mutation.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("mail: Resend rejected delivery with status %d", response.StatusCode)
	}
	return nil
}

func (sender *ResendSender) endpoint() (string, error) {
	base := strings.TrimSpace(sender.BaseURL)
	if base == "" {
		base = defaultResendAPIBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", fmt.Errorf("mail: Resend API base URL is invalid")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/emails"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (sender *ResendSender) client() *http.Client {
	if sender.Client != nil {
		return sender.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

type resendEmail struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	Text        string             `json:"text"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
}

type resendAttachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// resendIdempotencyKey gives a caller-supplied Communication ID priority. A
// deterministic fallback protects retries from duplicate provider delivery
// even in older application paths that have not yet threaded that ID through.
func resendIdempotencyKey(message Message) string {
	if candidate := strings.TrimSpace(message.IdempotencyKey); candidate != "" && len(candidate) <= 256 && !strings.ContainsAny(candidate, "\r\n") {
		return candidate
	}
	hash := sha256.New()
	for _, part := range [][]byte{
		[]byte(message.To), []byte{0}, []byte(message.Subject), []byte{0}, []byte(message.TextBody), []byte{0}, message.Calendar,
	} {
		_, _ = hash.Write(part)
	}
	return "rostrum-" + hex.EncodeToString(hash.Sum(nil))
}
