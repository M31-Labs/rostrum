package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/session"
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

func principalForEmail(t *testing.T, email string) domain.Principal {
	t.Helper()
	workspace, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range workspace.Snapshot().Principals {
		if strings.EqualFold(principal.Email, email) {
			return principal
		}
	}
	t.Fatalf("principal %q not found", email)
	return domain.Principal{}
}

func TestParsePrincipalRolesIsStrictAndNormalizes(t *testing.T) {
	grants, err := ParsePrincipalRoles(" Owner@Example.com = organizer + chair , view@example.com=observer, former@example.com=NONE")
	if err != nil {
		t.Fatalf("ParsePrincipalRoles: %v", err)
	}
	want := []PrincipalRoleGrant{
		{Email: "owner@example.com", Roles: []string{RoleOrganizer, RoleChair}},
		{Email: "view@example.com", Roles: []string{RoleObserver}},
		{Email: "former@example.com"},
	}
	if !reflect.DeepEqual(grants, want) {
		t.Fatalf("grants = %#v, want %#v", grants, want)
	}

	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "malformed email", raw: "not-an-email=organizer"},
		{name: "display name", raw: "Owner <owner@example.com>=organizer"},
		{name: "missing equals", raw: "owner@example.com"},
		{name: "missing role", raw: "owner@example.com="},
		{name: "unknown role", raw: "owner@example.com=reviewer"},
		{name: "none mixed with role", raw: "owner@example.com=none+observer"},
		{name: "none after role", raw: "owner@example.com=organizer+none"},
		{name: "duplicate role", raw: "owner@example.com=organizer+organizer"},
		{name: "duplicate email", raw: "owner@example.com=organizer,OWNER@example.com=chair"},
		{name: "multiple rows", raw: "owner@example.com=organizer\nchair@example.com=chair"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePrincipalRoles(test.raw); err == nil {
				t.Fatalf("ParsePrincipalRoles(%q) = nil error, want rejection", test.raw)
			}
		})
	}
}

func TestAllowedGitHubHandlesNormalizesAndDeduplicates(t *testing.T) {
	t.Setenv("AUTH_GITHUB_HANDLES", " OctoCat,maintainer,octocat, ,bad_handle,maintainer-2 ")
	got := AllowedGitHubHandles()
	want := []string{"octocat", "maintainer", "maintainer-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedGitHubHandles() = %#v, want %#v", got, want)
	}
}

func TestGitHubHandleAllowlistGrantsAndPersistsOAuthOrganizer(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "")
	t.Setenv("AUTH_GITHUB_HANDLES", "octocat")

	granted, err := GrantRoles(context.Background(), auth.User{
		ID:    "github:1",
		Email: "private@example.com",
		Name:  "The Octocat",
		Meta: map[string]any{
			"provider": "github",
			"profile":  "https://github.com/OctoCat",
		},
	})
	if err != nil {
		t.Fatalf("GrantRoles(handle allowlist): %v", err)
	}
	if !hasRole(granted, RoleOrganizer) {
		t.Fatalf("GitHub handle grant roles = %v, want %s", granted.Roles, RoleOrganizer)
	}
	principal := principalForEmail(t, "private@example.com")
	if !hasRole(principalToUser(principal), RoleOrganizer) {
		t.Fatalf("GitHub handle principal roles = %v, want %s", principal.Roles, RoleOrganizer)
	}
}

func TestGitHubHandleAllowlistDoesNotTrustDisplayNamesOrOtherProviders(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "")
	t.Setenv("AUTH_GITHUB_HANDLES", "octocat")

	for name, user := range map[string]auth.User{
		"display name only": {
			Email: "display@example.com",
			Name:  "OctoCat",
			Meta:  map[string]any{"provider": "github"},
		},
		"other provider profile": {
			Email: "other@example.com",
			Meta: map[string]any{
				"provider": "google",
				"profile":  "https://github.com/octocat",
			},
		},
		"unsafe profile host": {
			Email: "unsafe@example.com",
			Meta: map[string]any{
				"provider": "github",
				"profile":  "https://github.com.evil.example/octocat",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := GrantRoles(context.Background(), user); err == nil {
				t.Fatal("unverified handle metadata was accepted")
			}
		})
	}
}

func TestGitHubHandleAllowlistDoesNotOverrideDurableRevocation(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "")
	t.Setenv("AUTH_GITHUB_HANDLES", "octocat")
	if err := ApplyPrincipalRoles("owner@example.com=organizer,private@example.com=none"); err != nil {
		t.Fatalf("ApplyPrincipalRoles: %v", err)
	}

	_, err := GrantRoles(context.Background(), auth.User{
		Email: "private@example.com",
		Meta: map[string]any{
			"provider": "github",
			"profile":  "https://github.com/octocat",
		},
	})
	if err == nil {
		t.Fatal("GitHub handle allowlist overrode durable revocation")
	}
}

func principalToUser(principal domain.Principal) auth.User {
	return auth.User{Email: principal.Email, Roles: principal.Roles}
}

func TestApplyPrincipalRolesProvisionsExactRolesAndAuditsOnce(t *testing.T) {
	newTestWorkspace(t)
	raw := "owner@example.com=organizer+chair,chair@example.com=chair,view@example.com=observer"
	if err := ApplyPrincipalRoles(raw); err != nil {
		t.Fatalf("ApplyPrincipalRoles: %v", err)
	}
	for email, want := range map[string][]string{
		"owner@example.com": {RoleOrganizer, RoleChair},
		"chair@example.com": {RoleChair},
		"view@example.com":  {RoleObserver},
	} {
		principal := principalForEmail(t, email)
		if !reflect.DeepEqual(principal.Roles, want) {
			t.Errorf("principal %s roles = %v, want %v", email, principal.Roles, want)
		}
		if principal.CreatedAt.IsZero() {
			t.Errorf("principal %s has no creation time", email)
		}
		if !principal.LastSeenAt.IsZero() {
			t.Errorf("provisioning principal %s should not record a sign-in", email)
		}
	}
	workspace, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := workspace.Snapshot()
	if len(snapshot.AuditEvents) != 1 || snapshot.AuditEvents[0].Action != "identity.principals.provisioned" {
		t.Fatalf("audit events = %+v, want one provisioning event", snapshot.AuditEvents)
	}
	if err := ApplyPrincipalRoles(raw); err != nil {
		t.Fatalf("idempotent ApplyPrincipalRoles: %v", err)
	}
	if count := len(workspace.Snapshot().AuditEvents); count != 1 {
		t.Fatalf("idempotent apply audit count = %d, want 1", count)
	}
}

func TestApplyPrincipalRolesCannotRemoveTheLastOrganizer(t *testing.T) {
	newTestWorkspace(t)
	if err := ApplyPrincipalRoles("owner@example.com=organizer,chair@example.com=chair"); err != nil {
		t.Fatalf("initial ApplyPrincipalRoles: %v", err)
	}
	workspace, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	before := workspace.Snapshot()
	if err := ApplyPrincipalRoles("owner@example.com=chair"); err == nil {
		t.Fatal("demoting the last organizer succeeded, want atomic rejection")
	}
	after := workspace.Snapshot()
	if !reflect.DeepEqual(after.Principals, before.Principals) {
		t.Fatalf("failed apply changed principals: before=%+v after=%+v", before.Principals, after.Principals)
	}
	if len(after.AuditEvents) != len(before.AuditEvents) {
		t.Fatalf("failed apply appended an audit event: before=%d after=%d", len(before.AuditEvents), len(after.AuditEvents))
	}
}

func TestApplyPrincipalRolesCanDemoteOneOfMultipleOrganizersWithoutRemovingOmittedPrincipals(t *testing.T) {
	newTestWorkspace(t)
	if err := ApplyPrincipalRoles("first@example.com=organizer,second@example.com=organizer,chair@example.com=chair"); err != nil {
		t.Fatalf("initial ApplyPrincipalRoles: %v", err)
	}
	if err := ApplyPrincipalRoles("first@example.com=observer"); err != nil {
		t.Fatalf("demote with another organizer present: %v", err)
	}
	if got := principalForEmail(t, "first@example.com").Roles; !reflect.DeepEqual(got, []string{RoleObserver}) {
		t.Fatalf("demoted roles = %v, want [%s]", got, RoleObserver)
	}
	if got := principalForEmail(t, "second@example.com").Roles; !reflect.DeepEqual(got, []string{RoleOrganizer}) {
		t.Fatalf("omitted organizer roles = %v, want retained [%s]", got, RoleOrganizer)
	}
	if got := principalForEmail(t, "chair@example.com").Roles; !reflect.DeepEqual(got, []string{RoleChair}) {
		t.Fatalf("omitted chair roles = %v, want retained [%s]", got, RoleChair)
	}
}

func TestApplyPrincipalRolesExplicitlyRevokesWithoutAllowlistReelevation(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "former@example.com")
	if err := ApplyPrincipalRoles("owner@example.com=organizer,former@example.com=organizer+chair"); err != nil {
		t.Fatalf("initial ApplyPrincipalRoles: %v", err)
	}
	if err := ApplyPrincipalRoles("former@example.com=none"); err != nil {
		t.Fatalf("explicit revocation: %v", err)
	}
	former := principalForEmail(t, "former@example.com")
	if len(former.Roles) != 0 {
		t.Fatalf("revoked principal roles = %v, want none", former.Roles)
	}

	resolved, err := ResolveEmail(context.Background(), "FORMER@example.com")
	if err != nil {
		t.Fatalf("ResolveEmail(revoked): %v", err)
	}
	if len(resolved.Roles) != 0 {
		t.Fatalf("allowlisted revoked magic-link roles = %v, want none", resolved.Roles)
	}
	if _, err := GrantRoles(context.Background(), auth.User{Email: "former@example.com"}); err == nil {
		t.Fatal("allowlisted revoked OAuth identity was re-elevated")
	}
	if roles := principalForEmail(t, "former@example.com").Roles; len(roles) != 0 {
		t.Fatalf("sign-in attempt changed revoked durable roles to %v", roles)
	}

	workspace, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	audits := workspace.Snapshot().AuditEvents
	if len(audits) != 2 || !strings.Contains(audits[1].Summary, "1 explicit revocation") {
		t.Fatalf("revocation audit events = %+v, want one atomic explicit-revocation record", audits)
	}
}

func TestApplyPrincipalRolesCannotExplicitlyRevokeLastOrganizer(t *testing.T) {
	newTestWorkspace(t)
	if err := ApplyPrincipalRoles("owner@example.com=organizer"); err != nil {
		t.Fatalf("initial ApplyPrincipalRoles: %v", err)
	}
	workspace, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	before := workspace.Snapshot()
	if err := ApplyPrincipalRoles("owner@example.com=none"); err == nil {
		t.Fatal("explicitly revoking the last organizer succeeded")
	}
	after := workspace.Snapshot()
	if !reflect.DeepEqual(after.Principals, before.Principals) || len(after.AuditEvents) != len(before.AuditEvents) {
		t.Fatalf("failed revocation was not atomic: before=%+v after=%+v", before, after)
	}
}

func TestApplyPrincipalRolesRequiresOrganizerOnFreshWorkspace(t *testing.T) {
	newTestWorkspace(t)
	if err := ApplyPrincipalRoles("chair@example.com=chair,view@example.com=observer"); err == nil {
		t.Fatal("chair/observer-only configuration succeeded without an organizer")
	}
	workspace, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got := workspace.Snapshot(); len(got.Principals) != 0 || len(got.AuditEvents) != 0 {
		t.Fatalf("failed provisioning was not atomic: principals=%+v audit=%+v", got.Principals, got.AuditEvents)
	}
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

// TestConfiguredRolesSurviveMagicLinkAndOAuthLogin proves that the
// deployment mapping, rather than an OAuth provider role claim or the legacy
// organizer allowlist, is authoritative once a Principal exists.
func TestConfiguredRolesSurviveMagicLinkAndOAuthLogin(t *testing.T) {
	newTestWorkspace(t)
	// Including the chair and observer in the legacy allowlist is intentional:
	// their explicit stored grants must not be flattened to organizer.
	t.Setenv("ORGANIZER_EMAILS", "chair@example.com,view@example.com")
	if err := ApplyPrincipalRoles("owner@example.com=organizer,chair@example.com=chair,view@example.com=observer"); err != nil {
		t.Fatalf("ApplyPrincipalRoles: %v", err)
	}

	chair, err := ResolveEmail(context.Background(), "CHAIR@example.com")
	if err != nil {
		t.Fatalf("ResolveEmail(chair): %v", err)
	}
	if !reflect.DeepEqual(chair.Roles, []string{RoleChair}) {
		t.Fatalf("magic-link chair roles = %v, want [%s]", chair.Roles, RoleChair)
	}
	store := DurableMagicLinkStore{}
	expires := time.Now().UTC().Add(10 * time.Minute)
	if err := store.Save(auth.MagicLinkToken{Token: "chair-token", Email: chair.Email, User: chair, ExpiresAt: expires}); err != nil {
		t.Fatalf("Save magic link: %v", err)
	}
	consumed, err := store.Consume("chair-token", time.Now().UTC())
	if err != nil {
		t.Fatalf("Consume magic link: %v", err)
	}
	if !reflect.DeepEqual(consumed.User.Roles, []string{RoleChair}) {
		t.Fatalf("consumed magic-link roles = %v, want [%s]", consumed.User.Roles, RoleChair)
	}

	observer, err := GrantRoles(context.Background(), auth.User{
		ID:    "oauth-subject",
		Email: "VIEW@example.com",
		Name:  "Observer",
		Roles: []string{RoleOrganizer}, // must never be trusted from a provider
	})
	if err != nil {
		t.Fatalf("GrantRoles(observer): %v", err)
	}
	if !reflect.DeepEqual(observer.Roles, []string{RoleObserver}) {
		t.Fatalf("OAuth observer roles = %v, want [%s]", observer.Roles, RoleObserver)
	}
	if observer.ID != "oauth-subject" {
		t.Fatalf("OAuth provider subject = %q, want preserved", observer.ID)
	}

	principal := principalForEmail(t, "view@example.com")
	if !reflect.DeepEqual(principal.Roles, []string{RoleObserver}) {
		t.Fatalf("stored observer roles after login = %v, want [%s]", principal.Roles, RoleObserver)
	}
	if principal.Name != "Observer" || principal.LastSeenAt.IsZero() {
		t.Fatalf("stored observer was not refreshed on login: %+v", principal)
	}
}

func TestUnknownOAuthClaimsAndMagicLinkAddressesNeverElevate(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("ORGANIZER_EMAILS", "")
	if err := ApplyPrincipalRoles("owner@example.com=organizer"); err != nil {
		t.Fatalf("ApplyPrincipalRoles: %v", err)
	}

	magicUser, err := ResolveEmail(context.Background(), "unknown@example.com")
	if err != nil {
		t.Fatalf("ResolveEmail(unknown): %v", err)
	}
	if len(magicUser.Roles) != 0 {
		t.Fatalf("unknown magic-link roles = %v, want none", magicUser.Roles)
	}
	if _, err := GrantRoles(context.Background(), auth.User{
		Email: "unknown@example.com",
		Roles: []string{RoleOrganizer, RoleChair},
	}); err == nil {
		t.Fatal("unknown OAuth identity with provider role claims was accepted")
	}
	workspace, err := appstate.Get()
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range workspace.Snapshot().Principals {
		if strings.EqualFold(principal.Email, "unknown@example.com") {
			t.Fatal("unknown sign-in address was persisted as a Principal")
		}
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

func TestReconcileOrganizerSessionsUsesDurableRolesForEveryRequest(t *testing.T) {
	newTestWorkspace(t)
	if err := ApplyPrincipalRoles("owner@example.com=organizer+chair,backup@example.com=organizer"); err != nil {
		t.Fatalf("initial ApplyPrincipalRoles: %v", err)
	}
	sessions, err := session.New("identity-reconciliation-test-secret-over-32-bytes", session.Options{AllowInsecure: true})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	manager := New(sessions)
	staleCookie := signInTestUser(t, sessions, manager, auth.User{
		ID:    "oauth-subject",
		Email: "owner@example.com",
		Roles: []string{RoleReviewer, RoleOrganizer, RoleChair},
	})

	if err := ApplyPrincipalRoles("owner@example.com=observer"); err != nil {
		t.Fatalf("demote active principal: %v", err)
	}
	demoted, demotionCookies := reconciledTestUser(t, sessions, manager, staleCookie)
	if want := []string{RoleReviewer, RoleObserver}; !reflect.DeepEqual(demoted.Roles, want) {
		t.Fatalf("demoted session roles = %v, want %v", demoted.Roles, want)
	}
	if len(demotionCookies) == 0 {
		t.Fatal("demoted session was not re-signed with canonical roles")
	}

	if err := ApplyPrincipalRoles("owner@example.com=none"); err != nil {
		t.Fatalf("revoke active principal: %v", err)
	}
	// Reuse the original organizer cookie to model a stale cookie, passkey
	// UserJSON, or pending magic-link UserJSON that predates revocation.
	revoked, revocationCookies := reconciledTestUser(t, sessions, manager, staleCookie)
	if want := []string{RoleReviewer}; !reflect.DeepEqual(revoked.Roles, want) {
		t.Fatalf("revoked session roles = %v, want only non-organizer roles %v", revoked.Roles, want)
	}
	if len(revocationCookies) == 0 {
		t.Fatal("revoked session was not re-signed without organizer authority")
	}
}

func TestReconcileOrganizerSessionsFailsClosedWithoutDurablePrincipal(t *testing.T) {
	newTestWorkspace(t)
	sessions, err := session.New("identity-missing-principal-test-secret-over-32", session.Options{AllowInsecure: true})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	manager := New(sessions)
	staleCookie := signInTestUser(t, sessions, manager, auth.User{
		ID:    "forged-or-restored-subject",
		Email: "missing@example.com",
		Roles: []string{RoleOrganizer},
	})
	user, _ := reconciledTestUser(t, sessions, manager, staleCookie)
	if len(user.Roles) != 0 {
		t.Fatalf("missing-principal session roles = %v, want none", user.Roles)
	}
}

func signInTestUser(t *testing.T, sessions *session.Manager, manager *auth.Manager, user auth.User) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !manager.SignIn(r, user) {
			t.Fatal("SignIn returned false")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sign-in", nil))
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "gosx_session" {
			return cookie
		}
	}
	t.Fatal("sign-in response did not set a session cookie")
	return nil
}

func reconciledTestUser(t *testing.T, sessions *session.Manager, manager *auth.Manager, cookie *http.Cookie) (auth.User, []*http.Cookie) {
	t.Helper()
	var got auth.User
	recorder := httptest.NewRecorder()
	handler := sessions.Middleware(ReconcileOrganizerSessions()(manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = auth.Current(r)
		if !ok {
			t.Error("reconciled request has no authenticated user")
		}
		w.WriteHeader(http.StatusNoContent)
	}))))
	request := httptest.NewRequest(http.MethodGet, "/organizer", nil)
	request.AddCookie(cookie)
	handler.ServeHTTP(recorder, request)
	return got, recorder.Result().Cookies()
}
