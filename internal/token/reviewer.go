// This file adds RV-2's reviewer token to the token package PT-2 built for
// the speaker portal. A reviewer token signs "who this reviewer is", the
// same way the portal Token signs "who this speaker is" — but it is a
// distinct type with its own derived key, so the two kinds can never
// cross-authenticate: a speaker link opened as a reviewer link, or a
// reviewer link opened as a speaker link, both fail Verify.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// reviewerKeyPurpose derives the reviewer-token signing key from the same
// SESSION_SECRET the portal Token reads, through a purpose string distinct
// from keyPurpose. Because HMAC-SHA256 is keyed, a reviewer token signed
// under this key never verifies against the portal Token's key (and a
// portal token never verifies against this one), even when both processes
// read the identical SESSION_SECRET value.
const (
	reviewerKeyPurpose        = "rostrum.reviewer.token.v1:"
	reviewerPreviewKeyPurpose = "rostrum.reviewer.preview-token.v1:"
)

// ReviewerToken signs and verifies reviewer identity tokens for one
// process. It mirrors Token's HMAC-SHA256, URL-safe base64, body.sig shape
// exactly, but is a separate type over a separate key so a caller can never
// pass one where the other is expected — the compiler enforces that a
// *Token and a *ReviewerToken are different things, and the derived key
// enforces it again at the byte level.
type ReviewerToken struct {
	key        []byte
	previewKey []byte
	ttl        time.Duration
}

var (
	reviewerOnce     sync.Once
	reviewerInstance *ReviewerToken
)

// NewReviewer returns the process-wide reviewer-token signer. It derives its
// HMAC-SHA256 key from SESSION_SECRET the first time a caller asks for it,
// exactly as New() does for the portal Token, so no main.go wiring is
// required to use this package for reviewer links either.
func NewReviewer() *ReviewerToken {
	reviewerOnce.Do(func() {
		reviewerInstance = newReviewerToken(os.Getenv("SESSION_SECRET"))
	})
	return reviewerInstance
}

func newReviewerToken(secret string) *ReviewerToken {
	if strings.TrimSpace(secret) == "" {
		secret = developmentSecret
	}
	sum := sha256.Sum256([]byte(reviewerKeyPurpose + secret))
	previewSum := sha256.Sum256([]byte(reviewerPreviewKeyPurpose + secret))
	return &ReviewerToken{key: sum[:], previewKey: previewSum[:], ttl: TTL}
}

// reviewerClaims is the signed payload for a reviewer token. Its field name
// ("rvid") differs from claims' speaker field ("sid") on purpose: even a
// body decoded under the wrong claims type carries no speaker or reviewer
// ID by accident. This is defense in depth — the derived key already makes
// a cross-kind signature impossible to forge — not the primary boundary.
type reviewerClaims struct {
	ReviewerID string `json:"rvid"`
	ExpiresAt  int64  `json:"exp"`
	Audience   string `json:"aud,omitempty"`
}

// SignReviewer returns a signed, expiring token that binds reviewerID to
// whoever holds it. An organizer places the result in a reviewer's link as
// /review/<token>.
func (t *ReviewerToken) SignReviewer(reviewerID string) string {
	body, err := json.Marshal(reviewerClaims{
		ReviewerID: strings.TrimSpace(reviewerID),
		ExpiresAt:  time.Now().Add(t.ttl).Unix(),
	})
	if err != nil {
		// reviewerClaims contains only primitive fields and never fails to marshal.
		return ""
	}
	return encodeSegment(body) + "." + encodeSegment(t.sign(body))
}

// SignReviewerPreview returns a preview-only reviewer token for a persona
// link on /tour. A live process rejects it even when it has the same
// SESSION_SECRET and reviewer records as the preview process.
func (t *ReviewerToken) SignReviewerPreview(reviewerID string) string {
	body, err := json.Marshal(reviewerClaims{
		ReviewerID: strings.TrimSpace(reviewerID),
		ExpiresAt:  time.Now().Add(t.ttl).Unix(),
		Audience:   previewAudience,
	})
	if err != nil {
		return ""
	}
	return encodeSegment(body) + "." + encodeSegment(t.signPreview(body))
}

// VerifyReviewer reports the reviewer ID bound to tok when tok carries a
// valid signature and has not expired. Any tampering, truncation, malformed
// encoding, expiry, or a token signed by the unrelated speaker/portal Token
// reports ok=false with an empty reviewerID; VerifyReviewer never trusts a
// value this ReviewerToken did not sign itself.
func (t *ReviewerToken) VerifyReviewer(tok string) (reviewerID string, ok bool) {
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
	// Authenticate before parsing the body. Preview signatures are considered
	// only by a process explicitly running in preview mode.
	wantAudience := ""
	switch {
	case hmac.Equal(sig, t.sign(body)):
	case previewModeEnabled() && hmac.Equal(sig, t.signPreview(body)):
		wantAudience = previewAudience
	default:
		return "", false
	}
	var c reviewerClaims
	if err := json.Unmarshal(body, &c); err != nil {
		return "", false
	}
	if c.ReviewerID == "" || c.Audience != wantAudience {
		return "", false
	}
	if time.Now().Unix() > c.ExpiresAt {
		return "", false
	}
	return c.ReviewerID, true
}

func (t *ReviewerToken) sign(body []byte) []byte {
	return signWithKey(t.key, body)
}

func (t *ReviewerToken) signPreview(body []byte) []byte {
	return signWithKey(t.previewKey, body)
}
