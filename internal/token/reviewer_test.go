package token

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSignVerifyReviewerRoundTrip(t *testing.T) {
	tok := newReviewerToken("test-secret-value-not-for-prod")
	signed := tok.SignReviewer("rev_ada")
	if signed == "" {
		t.Fatal("SignReviewer returned an empty token")
	}
	id, ok := tok.VerifyReviewer(signed)
	if !ok {
		t.Fatal("VerifyReviewer rejected a token it just signed")
	}
	if id != "rev_ada" {
		t.Fatalf("VerifyReviewer returned reviewer %q, want rev_ada", id)
	}
}

func TestVerifyReviewerRejectsWrongKey(t *testing.T) {
	signer := newReviewerToken("secret-a")
	verifier := newReviewerToken("secret-b")
	signed := signer.SignReviewer("rev_ada")
	if _, ok := verifier.VerifyReviewer(signed); ok {
		t.Fatal("VerifyReviewer accepted a token signed with a different key")
	}
}

func TestVerifyReviewerRejectsTamperedReviewerID(t *testing.T) {
	tok := newReviewerToken("test-secret-value-not-for-prod")
	signed := tok.SignReviewer("rev_ada")
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected token shape: %q", signed)
	}
	forged := encodeSegment([]byte(`{"rvid":"rev_other","exp":9999999999}`)) + "." + parts[1]
	if _, ok := tok.VerifyReviewer(forged); ok {
		t.Fatal("VerifyReviewer accepted a body edited without re-signing")
	}
}

func TestVerifyReviewerRejectsExpiredToken(t *testing.T) {
	tok := newReviewerToken("test-secret-value-not-for-prod")
	tok.ttl = -1 * time.Hour
	signed := tok.SignReviewer("rev_ada")
	if _, ok := tok.VerifyReviewer(signed); ok {
		t.Fatal("VerifyReviewer accepted an expired token")
	}
}

func TestVerifyReviewerRejectsGarbage(t *testing.T) {
	tok := newReviewerToken("test-secret-value-not-for-prod")
	cases := []string{"", "no-dot-here", ".", "abc.", ".xyz", "not base64 at all.also not base64"}
	for _, c := range cases {
		if _, ok := tok.VerifyReviewer(c); ok {
			t.Fatalf("VerifyReviewer accepted garbage input %q", c)
		}
	}
}

func TestNewReviewerDerivesFromEnvironment(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	reviewerOnce = sync.Once{}
	first := NewReviewer()
	signed := first.SignReviewer("rev_ada")
	if _, ok := first.VerifyReviewer(signed); !ok {
		t.Fatal("process-wide reviewer signer could not verify its own token")
	}
}

func TestDemoReviewerTokenRequiresDemoMode(t *testing.T) {
	// Model a demo and live deployment with the same secret and seeded IDs.
	demoSigner := newReviewerToken("shared-demo-and-live-secret")
	liveVerifier := newReviewerToken("shared-demo-and-live-secret")
	t.Setenv("APP_MODE", "demo")
	signed := demoSigner.SignReviewerDemo("rev_ada")
	if signed == "" {
		t.Fatal("SignReviewerDemo returned an empty token")
	}

	if id, ok := demoSigner.VerifyReviewer(signed); !ok || id != "rev_ada" {
		t.Fatalf("demo process rejected demo reviewer token: id=%q ok=%v", id, ok)
	}

	t.Setenv("APP_MODE", "live")
	if id, ok := liveVerifier.VerifyReviewer(signed); ok || id != "" {
		t.Fatalf("live process accepted demo reviewer token: id=%q ok=%v", id, ok)
	}
}

func TestNormalReviewerTokenRemainsValidAcrossModes(t *testing.T) {
	tok := newReviewerToken("shared-demo-and-live-secret")
	signed := tok.SignReviewer("rev_ada")
	for _, mode := range []string{"live", "demo"} {
		t.Setenv("APP_MODE", mode)
		if id, valid := tok.VerifyReviewer(signed); !valid || id != "rev_ada" {
			t.Fatalf("normal reviewer token rejected in %s mode: id=%q ok=%v", mode, id, valid)
		}
	}
}

// TestSpeakerAndReviewerTokensNeverCrossAuthenticate is the RV-2 isolation
// guarantee this file exists to prove: a speaker (portal) token can never
// verify as a reviewer token, and a reviewer token can never verify as a
// speaker token, even when both are signed under the identical
// SESSION_SECRET. The two kinds derive different HMAC keys, so a forged or
// misrouted token is rejected before its claims are even inspected.
func TestSpeakerAndReviewerTokensNeverCrossAuthenticate(t *testing.T) {
	secret := "shared-secret-both-kinds-read"
	speaker := newToken(secret)
	reviewer := newReviewerToken(secret)

	speakerToken := speaker.Sign("spk_maya")
	if _, ok := reviewer.VerifyReviewer(speakerToken); ok {
		t.Fatal("VerifyReviewer accepted a speaker-signed portal token")
	}

	reviewerToken := reviewer.SignReviewer("rev_ada")
	if _, ok := speaker.Verify(reviewerToken); ok {
		t.Fatal("Verify accepted a reviewer-signed token as a portal token")
	}

	t.Setenv("APP_MODE", "demo")
	demoSpeakerToken := speaker.SignDemo("spk_maya")
	if _, ok := reviewer.VerifyReviewer(demoSpeakerToken); ok {
		t.Fatal("VerifyReviewer accepted a demo speaker token")
	}
	demoReviewerToken := reviewer.SignReviewerDemo("rev_ada")
	if _, ok := speaker.Verify(demoReviewerToken); ok {
		t.Fatal("Verify accepted a demo reviewer token as a portal token")
	}
}
