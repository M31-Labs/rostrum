package identity

import (
	"context"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/auth"
)

func newTestWorkspace(t *testing.T) {
	t.Helper()
	workspace, err := store.Open(":memory:", domain.EmptyState(time.Now().UTC()))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
}

func hasRole(user auth.User, role string) bool {
	for _, candidate := range user.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// TestResolveEmailGrantsRoleOnlyForAllowlistedAddress covers the magic-link
// resolver: an allowlisted email (compared case-insensitively) is granted
// RoleOrganizer and upserted as a Principal; an unknown email still
// resolves without an error -- ResolveEmail must never error, or a
// different status code from /auth/magic-link would make the endpoint an
// allowlist oracle -- and it is never recorded as a Principal.
func TestResolveEmailGrantsRoleOnlyForAllowlistedAddress(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "owner@example.com, Ops@Example.com")

	allowed, err := ResolveEmail(context.Background(), "OWNER@example.com")
	if err != nil {
		t.Fatalf("ResolveEmail(allowed): %v", err)
	}
	if !hasRole(allowed, RoleOrganizer) {
		t.Fatalf("allowed user roles = %v, want %s", allowed.Roles, RoleOrganizer)
	}

	denied, err := ResolveEmail(context.Background(), "stranger@example.com")
	if err != nil {
		t.Fatalf("ResolveEmail(denied) returned an error, want nil: %v", err)
	}
	if hasRole(denied, RoleOrganizer) {
		t.Fatalf("denied user roles = %v, want no %s", denied.Roles, RoleOrganizer)
	}
	if denied.Email != "stranger@example.com" {
		t.Fatalf("denied user email = %q, want stranger@example.com", denied.Email)
	}

	current, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	foundAllowed := false
	for _, principal := range current.Snapshot().Principals {
		if principal.Email == "owner@example.com" {
			foundAllowed = true
		}
		if principal.Email == "stranger@example.com" {
			t.Fatal("a denied email must never be recorded as a Principal")
		}
	}
	if !foundAllowed {
		t.Fatal("an allowed email must be upserted as a Principal")
	}
}

// TestGrantRolesRejectsNonAllowlistedOAuthIdentity covers the stricter OAuth
// path: unlike ResolveEmail, GrantRoles errors for a non-allowlisted email,
// so the wrapping resolver (oauth.go) aborts the callback before SignIn.
func TestGrantRolesRejectsNonAllowlistedOAuthIdentity(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "owner@example.com")

	if _, err := GrantRoles(context.Background(), auth.User{Email: "stranger@example.com"}); err == nil {
		t.Fatal("GrantRoles(denied) = nil error, want an error that aborts the OAuth callback")
	}

	granted, err := GrantRoles(context.Background(), auth.User{Email: "owner@example.com", Name: "Owner"})
	if err != nil {
		t.Fatalf("GrantRoles(allowed): %v", err)
	}
	if !hasRole(granted, RoleOrganizer) {
		t.Fatalf("granted user roles = %v, want %s", granted.Roles, RoleOrganizer)
	}
}

// TestResolveEmailHonorsStoredPrincipal covers the second allowlist path: a
// Principal already carrying RoleOrganizer (for example, one setup.go
// created) grants access even when ORGANIZER_EMAILS is empty.
func TestResolveEmailHonorsStoredPrincipal(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "")

	if err := upsertOrganizerPrincipal("first@example.com", "First Organizer"); err != nil {
		t.Fatalf("upsertOrganizerPrincipal: %v", err)
	}

	granted, err := ResolveEmail(context.Background(), "first@example.com")
	if err != nil {
		t.Fatalf("ResolveEmail: %v", err)
	}
	if !hasRole(granted, RoleOrganizer) {
		t.Fatalf("granted user roles = %v, want %s", granted.Roles, RoleOrganizer)
	}
}
