package token

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	tok := newToken("test-secret-value-not-for-prod")
	signed := tok.Sign("spk_maya")
	if signed == "" {
		t.Fatal("Sign returned an empty token")
	}
	id, ok := tok.Verify(signed)
	if !ok {
		t.Fatal("Verify rejected a token it just signed")
	}
	if id != "spk_maya" {
		t.Fatalf("Verify returned speaker %q, want spk_maya", id)
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	signer := newToken("secret-a")
	verifier := newToken("secret-b")
	signed := signer.Sign("spk_maya")
	if _, ok := verifier.Verify(signed); ok {
		t.Fatal("Verify accepted a token signed with a different key")
	}
}

func TestVerifyRejectsTamperedSpeakerID(t *testing.T) {
	tok := newToken("test-secret-value-not-for-prod")
	signed := tok.Sign("spk_maya")
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected token shape: %q", signed)
	}
	forged := encodeSegment([]byte(`{"sid":"spk_other","exp":9999999999}`)) + "." + parts[1]
	if _, ok := tok.Verify(forged); ok {
		t.Fatal("Verify accepted a body edited without re-signing")
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	tok := newToken("test-secret-value-not-for-prod")
	tok.ttl = -1 * time.Hour
	signed := tok.Sign("spk_maya")
	if _, ok := tok.Verify(signed); ok {
		t.Fatal("Verify accepted an expired token")
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	tok := newToken("test-secret-value-not-for-prod")
	cases := []string{"", "no-dot-here", ".", "abc.", ".xyz", "not base64 at all.also not base64"}
	for _, c := range cases {
		if _, ok := tok.Verify(c); ok {
			t.Fatalf("Verify accepted garbage input %q", c)
		}
	}
}

func TestNewDerivesFromEnvironment(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	once = sync.Once{}
	first := New()
	signed := first.Sign("spk_maya")
	if _, ok := first.Verify(signed); !ok {
		t.Fatal("process-wide signer could not verify its own token")
	}
}
