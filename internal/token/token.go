// Package token issues and verifies signed, expiring identity tokens that
// bind a portal session to one speaker. A token travels as a URL query
// parameter (?key=<token>) inside a speaker's invitation link. The route
// that receives it verifies the token once, then stores the bound speaker
// ID in the GoSX session, so later requests need no query key.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// developmentSecret backs token signing when SESSION_SECRET is unset, so a
// bare local run still issues working portal links. main.go already refuses
// to start in production without a unique SESSION_SECRET, so this fallback
// never weakens a production deployment.
const developmentSecret = "rostrum-development-secret-change-me"

// TTL is how long a signed token stays valid after Sign issues it. A
// speaker invitation is emailed once and may sit unopened for weeks, so the
// window is generous rather than tied to the session lifetime.
const TTL = 90 * 24 * time.Hour

// keyPurpose separates the derived signing key from any other key HMAC-SHA256
// might derive from the same SESSION_SECRET elsewhere in the process.
const keyPurpose = "rostrum.portal.token.v1:"

// Token signs and verifies portal identity tokens for one process.
type Token struct {
	key []byte
	ttl time.Duration
}

var (
	once     sync.Once
	instance *Token
)

// New returns the process-wide token signer. It derives its HMAC-SHA256 key
// from the SESSION_SECRET environment variable the first time a caller asks
// for it, so no main.go wiring is required to use this package.
func New() *Token {
	once.Do(func() {
		instance = newToken(os.Getenv("SESSION_SECRET"))
	})
	return instance
}

func newToken(secret string) *Token {
	if strings.TrimSpace(secret) == "" {
		secret = developmentSecret
	}
	sum := sha256.Sum256([]byte(keyPurpose + secret))
	return &Token{key: sum[:], ttl: TTL}
}

// claims is the signed payload. Both fields are exported to json so the MAC
// covers a stable, unambiguous encoding.
type claims struct {
	SpeakerID string `json:"sid"`
	ExpiresAt int64  `json:"exp"`
}

// Sign returns a signed, expiring token that binds speakerID to whoever
// holds it. Callers place the result in a portal link as ?key=<token>.
func (t *Token) Sign(speakerID string) string {
	body, err := json.Marshal(claims{
		SpeakerID: strings.TrimSpace(speakerID),
		ExpiresAt: time.Now().Add(t.ttl).Unix(),
	})
	if err != nil {
		// claims has two primitive fields and never fails to marshal.
		return ""
	}
	return encodeSegment(body) + "." + encodeSegment(t.sign(body))
}

// Verify reports the speaker ID bound to tok when tok carries a valid
// signature and has not expired. Any tampering, truncation, malformed
// encoding, or expiry reports ok=false with an empty speakerID; Verify never
// trusts a value it did not sign itself.
func (t *Token) Verify(tok string) (speakerID string, ok bool) {
	tok = strings.TrimSpace(tok)
	dot := strings.IndexByte(tok, '.')
	if dot <= 0 || dot == len(tok)-1 {
		return "", false
	}
	body, err := decodeSegment(tok[:dot])
	if err != nil {
		return "", false
	}
	sig, err := decodeSegment(tok[dot+1:])
	if err != nil {
		return "", false
	}
	// hmac.Equal compares MACs in constant time so a forged or truncated
	// signature never leaks timing information about the correct value.
	if !hmac.Equal(sig, t.sign(body)) {
		return "", false
	}
	var c claims
	if err := json.Unmarshal(body, &c); err != nil {
		return "", false
	}
	if c.SpeakerID == "" {
		return "", false
	}
	if time.Now().Unix() > c.ExpiresAt {
		return "", false
	}
	return c.SpeakerID, true
}

func (t *Token) sign(body []byte) []byte {
	mac := hmac.New(sha256.New, t.key)
	mac.Write(body)
	return mac.Sum(nil)
}

func encodeSegment(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeSegment(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
