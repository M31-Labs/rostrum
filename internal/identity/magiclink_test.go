package identity

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"m31labs.dev/gosx/auth"
)

// TestDurableMagicLinkStoreSaveConsumeRoundTrip covers the durable store's
// Save/Consume contract: Save persists only the token's digest (never the
// raw token), Consume returns the exact user, next target, and email Save
// received, and a second Consume of the same token fails -- one-time use is
// structural, not a convention the caller must remember to enforce.
func TestDurableMagicLinkStoreSaveConsumeRoundTrip(t *testing.T) {
	newTestWorkspace(t)

	durable := DurableMagicLinkStore{}
	now := time.Now().UTC()
	token := auth.MagicLinkToken{
		Token: "test-token-value",
		Email: "owner@example.com",
		User: auth.User{
			ID:    "owner@example.com",
			Email: "owner@example.com",
			Roles: []string{RoleOrganizer},
		},
		Next:      "/organizer",
		ExpiresAt: now.Add(20 * time.Minute),
	}
	if err := durable.Save(token); err != nil {
		t.Fatalf("Save: %v", err)
	}

	current, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range current.Snapshot().AuthMagicLinks {
		if link.Token == token.Token {
			t.Fatal("Save stored the raw token instead of its SHA-256 digest")
		}
	}

	consumed, err := durable.Consume(token.Token, now)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if consumed.Email != token.Email {
		t.Fatalf("Consume email = %q, want %q", consumed.Email, token.Email)
	}
	if consumed.Next != token.Next {
		t.Fatalf("Consume next = %q, want %q", consumed.Next, token.Next)
	}
	if !hasRole(consumed.User, RoleOrganizer) {
		t.Fatalf("Consume user roles = %v, want %s", consumed.User.Roles, RoleOrganizer)
	}

	if _, err := durable.Consume(token.Token, now); err != auth.ErrMagicLinkInvalid {
		t.Fatalf("second Consume error = %v, want ErrMagicLinkInvalid", err)
	}

	if remaining := len(current.Snapshot().AuthMagicLinks); remaining != 0 {
		t.Fatalf("AuthMagicLinks after consume = %d, want 0", remaining)
	}
}

// TestDurableMagicLinkStoreConsumeExpired covers Consume's expiry check: a
// token past its ExpiresAt fails with ErrMagicLinkExpired, and (mirroring
// the round-trip test) is removed from the store either way.
func TestDurableMagicLinkStoreConsumeExpired(t *testing.T) {
	newTestWorkspace(t)

	durable := DurableMagicLinkStore{}
	now := time.Now().UTC()
	token := auth.MagicLinkToken{
		Token:     "expired-token",
		Email:     "owner@example.com",
		User:      auth.User{ID: "owner@example.com", Email: "owner@example.com"},
		ExpiresAt: now.Add(-1 * time.Minute),
	}
	if err := durable.Save(token); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := durable.Consume(token.Token, now); err != auth.ErrMagicLinkExpired {
		t.Fatalf("Consume(expired) error = %v, want ErrMagicLinkExpired", err)
	}
}

// TestDurableMagicLinkStoreConsumeUnknownToken covers the never-issued
// case: Consume must fail closed, not zero-value open.
func TestDurableMagicLinkStoreConsumeUnknownToken(t *testing.T) {
	newTestWorkspace(t)

	durable := DurableMagicLinkStore{}
	if _, err := durable.Consume("never-issued", time.Now().UTC()); err != auth.ErrMagicLinkInvalid {
		t.Fatalf("Consume(unknown) error = %v, want ErrMagicLinkInvalid", err)
	}
}
