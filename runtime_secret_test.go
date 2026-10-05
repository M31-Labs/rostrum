package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"os/exec"
	"testing"

	"github.com/m31-labs/rostrum/internal/token"
)

func TestRuntimeSessionSecretPreservesIssuedLinks(t *testing.T) {
	const configured = " \tcompatibility-secret-at-least-32-characters\r\n"
	if os.Getenv("ROSTRUM_TEST_SECRET_COMPATIBILITY") != "1" {
		// Signers are process-wide singletons. Use a fresh process so other
		// tests cannot determine the secret before this startup regression.
		t.Setenv("ROSTRUM_TEST_SECRET_COMPATIBILITY", "1")
		command := exec.Command(os.Args[0], "-test.run=^TestRuntimeSessionSecretPreservesIssuedLinks$")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("signer compatibility: %v\n%s", err, output)
		}
		return
	}
	// Reproduce links issued with the previous purpose strings and original
	// configured bytes, before runtimeSessionSecret resolves the startup key.
	legacyLink := func(purpose, body string) string {
		key := sha256.Sum256([]byte(purpose + configured))
		mac := hmac.New(sha256.New, key[:])
		_, _ = mac.Write([]byte(body))
		encode := base64.RawURLEncoding.EncodeToString
		return encode([]byte(body)) + "." + encode(mac.Sum(nil))
	}
	speakerLink := legacyLink("rostrum.portal.token.v1:", `{"sid":"legacy-speaker","exp":4102444800}`)
	reviewerLink := legacyLink("rostrum.reviewer.token.v1:", `{"rvid":"legacy-reviewer","exp":4102444800}`)
	resolved, err := runtimeSessionSecret("https://rostrum.example", "production", configured)
	if err != nil || resolved != configured {
		t.Fatal("startup changed the configured secret bytes")
	}
	t.Setenv("APP_MODE", "live")
	t.Setenv("SESSION_SECRET", resolved)
	if id, ok := token.New().Verify(speakerLink); !ok || id != "legacy-speaker" {
		t.Fatal("startup invalidated a previously issued speaker link")
	}
	if id, ok := token.NewReviewer().VerifyReviewer(reviewerLink); !ok || id != "legacy-reviewer" {
		t.Fatal("startup invalidated a previously issued reviewer link")
	}
}
