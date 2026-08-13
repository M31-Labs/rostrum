package token

import (
	"encoding/json"
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

func TestPreviewSpeakerTokenRequiresPreviewMode(t *testing.T) {
	// Model preview and live deployments with the same secret and IDs.
	// The process mode, not deployment hygiene, must enforce the boundary.
	previewSigner := newToken("shared-preview-and-live-secret")
	liveVerifier := newToken("shared-preview-and-live-secret")
	t.Setenv("APP_MODE", "preview")
	signed := previewSigner.SignPreview("spk_maya")
	if signed == "" {
		t.Fatal("SignPreview returned an empty token")
	}

	if id, ok := previewSigner.Verify(signed); !ok || id != "spk_maya" {
		t.Fatalf("preview process rejected preview token: id=%q ok=%v", id, ok)
	}

	t.Setenv("APP_MODE", "live")
	if id, ok := liveVerifier.Verify(signed); ok || id != "" {
		t.Fatalf("live process accepted preview token: id=%q ok=%v", id, ok)
	}
}

func TestNormalSpeakerTokenRemainsValidAcrossModes(t *testing.T) {
	tok := newToken("shared-preview-and-live-secret")
	signed := tok.Sign("spk_maya")

	// omitempty preserves the original sid+exp wire shape for ordinary links.
	bodySegment, _, ok := strings.Cut(signed, ".")
	if !ok {
		t.Fatalf("normal token has unexpected shape: %q", signed)
	}
	body, err := decodeSegment(bodySegment)
	if err != nil {
		t.Fatalf("decode normal token: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode normal claims: %v", err)
	}
	if _, present := payload["aud"]; present {
		t.Fatalf("normal token unexpectedly changed wire shape: %s", body)
	}

	for _, mode := range []string{"live", "preview"} {
		t.Setenv("APP_MODE", mode)
		if id, valid := tok.Verify(signed); !valid || id != "spk_maya" {
			t.Fatalf("normal token rejected in %s mode: id=%q ok=%v", mode, id, valid)
		}
	}
}
