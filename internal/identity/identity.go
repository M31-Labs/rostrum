// Package identity builds Rostrum's organizer identity plane: one GoSX auth
// manager, the role constants every principal carries, and the allowlist
// that decides who may act as an organizer. It owns no password and stores
// no credential; it wires the GoSX auth package (m31labs.dev/gosx/auth) over
// the application's existing session manager and the workspace JSON store.
package identity

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	stdmail "net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/session"
)

// Role constants name every principal kind Rostrum recognizes. A User.Roles
// slice may carry more than one; RequireAnyRole checks membership, not an
// exact match.
const (
	RoleOrganizer = "organizer" // full workspace control
	RoleChair     = "chair"     // decision authority: accept, reject, publish
	RoleReviewer  = "reviewer"  // scores assigned submissions
	RoleSpeaker   = "speaker"   // owns one portal only
	RoleObserver  = "observer"  // read-only organizer surface, no exports

	// roleNone is an operator-only PRINCIPAL_ROLES value. It is never copied
	// into an auth.User: the durable Principal remains as an explicit deny so
	// ORGANIZER_EMAILS cannot silently provision it again.
	roleNone = "none"

	// sessionUserKey is explicit in New and in ReconcileOrganizerSessions so
	// the canonical-role middleware never depends on an implicit framework
	// default changing underneath it.
	sessionUserKey = "gosx.user"
)

// MagicLinkFlashKey names the session flash both the magic-link flow and
// the break-glass setup flow (setup.go) use for the "check your email"
// notice, so /login renders the identical message from either path.
const MagicLinkFlashKey = "auth.magic_link"

// LoginPath is the path auth.Options.LoginPath is configured with. It is
// also the default GoSX applies, named here as a constant so
// RequireAnyRole and setup.go never drift from the manager's own default.
const LoginPath = "/login"

// New builds the session-backed auth manager Rostrum mounts every route
// behind. Call it once at startup, after the session manager exists.
func New(sessions *session.Manager) *auth.Manager {
	return auth.New(sessions, auth.Options{LoginPath: LoginPath, SessionKey: sessionUserKey})
}

// AllowedEmails parses the ORGANIZER_EMAILS environment allowlist: a
// comma-separated list of addresses, compared case-insensitively.
func AllowedEmails() []string {
	raw := strings.TrimSpace(os.Getenv("ORGANIZER_EMAILS"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		email := strings.ToLower(strings.TrimSpace(part))
		if email != "" {
			out = append(out, email)
		}
	}
	return out
}

// PrincipalRoleGrant is one deployment-owned organizer-surface identity.
// Roles contains only organizer, chair, or observer after
// ParsePrincipalRoles returns successfully. An empty Roles slice represents
// the explicit operator value "none", which revokes organizer-surface access
// while retaining a durable deny record for the address.
type PrincipalRoleGrant struct {
	Email string
	Roles []string
}

// ParsePrincipalRoles parses a single CSV row using the operator-facing form
// "email=role+role,email=role". Parsing is deliberately strict: addresses
// are normalized to lower case, duplicate addresses or roles are rejected,
// and only organizer, chair, and observer may be provisioned through this
// privileged identity plane.
func ParsePrincipalRoles(raw string) ([]PrincipalRoleGrant, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(raw))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("identity: parse PRINCIPAL_ROLES: %w", err)
	}
	if len(records) != 1 {
		return nil, fmt.Errorf("identity: PRINCIPAL_ROLES must be one CSV row")
	}
	grants := make([]PrincipalRoleGrant, 0, len(records[0]))
	seenEmails := make(map[string]struct{}, len(records[0]))
	for index, rawGrant := range records[0] {
		parts := strings.SplitN(strings.TrimSpace(rawGrant), "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("identity: PRINCIPAL_ROLES entry %d must use email=role+role", index+1)
		}
		email, err := normalizeProvisionedEmail(parts[0])
		if err != nil {
			return nil, fmt.Errorf("identity: PRINCIPAL_ROLES entry %d: %w", index+1, err)
		}
		if _, duplicate := seenEmails[email]; duplicate {
			return nil, fmt.Errorf("identity: PRINCIPAL_ROLES contains duplicate email %q", email)
		}
		seenEmails[email] = struct{}{}

		roleSpec := strings.ToLower(strings.TrimSpace(parts[1]))
		if roleSpec == "" {
			return nil, fmt.Errorf("identity: PRINCIPAL_ROLES entry %d must name at least one role or none", index+1)
		}
		if roleSpec == roleNone {
			grants = append(grants, PrincipalRoleGrant{Email: email})
			continue
		}
		rawRoles := strings.Split(roleSpec, "+")
		roles := make([]string, 0, len(rawRoles))
		seenRoles := make(map[string]struct{}, len(rawRoles))
		for _, rawRole := range rawRoles {
			role := strings.ToLower(strings.TrimSpace(rawRole))
			if role == roleNone {
				return nil, fmt.Errorf("identity: PRINCIPAL_ROLES entry %d must use none by itself", index+1)
			}
			if !provisionableRole(role) {
				return nil, fmt.Errorf("identity: PRINCIPAL_ROLES entry %d has unknown role %q", index+1, role)
			}
			if _, duplicate := seenRoles[role]; duplicate {
				return nil, fmt.Errorf("identity: PRINCIPAL_ROLES entry %d repeats role %q", index+1, role)
			}
			seenRoles[role] = struct{}{}
			roles = append(roles, role)
		}
		grants = append(grants, PrincipalRoleGrant{Email: email, Roles: roles})
	}
	return grants, nil
}

func normalizeProvisionedEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	address, err := stdmail.ParseAddress(email)
	if err != nil || address.Name != "" || !strings.EqualFold(address.Address, email) {
		return "", fmt.Errorf("invalid email address %q", strings.TrimSpace(raw))
	}
	return email, nil
}

func provisionableRole(role string) bool {
	switch role {
	case RoleOrganizer, RoleChair, RoleObserver:
		return true
	default:
		return false
	}
}

// ApplyPrincipalRoles atomically upserts the deployment-provided grants.
// Each listed principal receives exactly the listed organizer-surface roles;
// `email=none` retains the principal with no access roles as an explicit deny.
// Principals omitted from the configuration are retained, avoiding an
// accidental mass revocation from a truncated environment value. A non-empty
// configuration may not leave the durable workspace without an organizer.
// Blank input is a no-op so NewSetup can still arm the fresh-host bootstrap.
func ApplyPrincipalRoles(raw string) error {
	grants, err := ParsePrincipalRoles(raw)
	if err != nil {
		return err
	}
	if len(grants) == 0 {
		return nil
	}
	store, err := appstate.Get()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	revocations := 0
	for _, grant := range grants {
		if len(grant.Roles) == 0 {
			revocations++
		}
	}
	err = store.UpdateAudit(domain.AuditMeta{
		Actor:      "system",
		Action:     "identity.principals.provisioned",
		EntityType: "principal",
		Summary:    fmt.Sprintf("Applied deployment role configuration for %d principal(s), including %d explicit revocation(s).", len(grants), revocations),
		Origin:     "environment",
	}, func(state *domain.State) error {
		changed := false
		for _, grant := range grants {
			match := -1
			for index := range state.Principals {
				if !strings.EqualFold(strings.TrimSpace(state.Principals[index].Email), grant.Email) {
					continue
				}
				if match >= 0 {
					return fmt.Errorf("identity: workspace contains duplicate principal email %q", grant.Email)
				}
				match = index
			}
			if match < 0 {
				state.Principals = append(state.Principals, domain.Principal{
					ID:        domain.NewID("prin"),
					Email:     grant.Email,
					Roles:     append([]string(nil), grant.Roles...),
					CreatedAt: now,
				})
				changed = true
				continue
			}
			principal := &state.Principals[match]
			if principal.Email != grant.Email {
				principal.Email = grant.Email
				changed = true
			}
			if !sameRoles(principal.Roles, grant.Roles) {
				principal.Roles = append([]string(nil), grant.Roles...)
				changed = true
			}
			if principal.CreatedAt.IsZero() {
				principal.CreatedAt = now
				changed = true
			}
		}
		if !stateHasOrganizer(*state) {
			return errors.New("identity: PRINCIPAL_ROLES must retain at least one organizer principal")
		}
		if !changed {
			return errPrincipalRolesUnchanged
		}
		return nil
	})
	if errors.Is(err, errPrincipalRolesUnchanged) {
		return nil
	}
	return err
}

func stateHasOrganizer(state domain.State) bool {
	for _, principal := range state.Principals {
		for _, role := range principal.Roles {
			if role == RoleOrganizer {
				return true
			}
		}
	}
	return false
}

func sameRoles(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// hasStoredOrganizer reports whether the workspace already holds a
// Principal carrying RoleOrganizer. setup.go uses this to decide whether
// the break-glass bootstrap needs to arm at all.
func hasStoredOrganizer() bool {
	store, err := appstate.Get()
	if err != nil {
		return false
	}
	for _, principal := range store.Snapshot().Principals {
		for _, role := range principal.Roles {
			if role == RoleOrganizer {
				return true
			}
		}
	}
	return false
}

// ResolveEmail is the auth.MagicLinkResolver the magic-link flow calls
// before it issues a token. Stored organizer, chair, and observer roles are
// copied into the token exactly; the legacy ORGANIZER_EMAILS allowlist still
// provisions RoleOrganizer for an address with no stored access grant.
// Unknown addresses resolve to users with no roles and are never persisted.
// The resolver deliberately returns no error for either case so the mounted
// magic-link request endpoint cannot become an identity-provisioning oracle.
func ResolveEmail(_ context.Context, rawEmail string) (auth.User, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	user := auth.User{ID: email, Email: email}
	roles, allowed, err := resolveAccessRoles(email, "")
	if err != nil || !allowed {
		return user, nil
	}
	user.Roles = roles
	return user, nil
}

// GrantRoles resolves organizer-surface access for an OAuth-verified
// identity. Durable principal roles replace (rather than extend) any roles
// supplied by the provider, so provider claims can never self-elevate an
// unknown address. Unlike ResolveEmail, this returns an error for an
// unprovisioned address so the OAuth callback aborts before SignIn.
func GrantRoles(_ context.Context, user auth.User) (auth.User, error) {
	email := strings.ToLower(strings.TrimSpace(user.Email))
	if email == "" {
		return auth.User{}, errNoEmail
	}
	roles, allowed, err := resolveAccessRoles(email, user.Name)
	if err != nil {
		return auth.User{}, err
	}
	if !allowed {
		return auth.User{}, errNotAllowed
	}
	user.Email = email
	user.Roles = roles
	return user, nil
}

var (
	errNoEmail                 = errors.New("identity: oauth user has no email")
	errNotAllowed              = errors.New("identity: email is not provisioned for organizer access")
	errPrincipalRolesUnchanged = errors.New("identity: principal roles unchanged")
)

// resolveAccessRoles atomically looks up and touches a known principal. An
// explicit durable principal wins over ORGANIZER_EMAILS, preserving a chair,
// observer, or explicit no-access grant even when that same address remains
// on the legacy organizer allowlist. The allowlist is consulted only when no
// Principal record exists for the address.
func resolveAccessRoles(email, name string) ([]string, bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, false, nil
	}
	store, err := appstate.Get()
	if err != nil {
		return nil, false, err
	}
	allowlisted := organizerEmailAllowed(email)
	name = strings.TrimSpace(name)
	now := time.Now().UTC()
	var resolved []string
	err = store.Update(func(state *domain.State) error {
		match := -1
		for index := range state.Principals {
			if !strings.EqualFold(strings.TrimSpace(state.Principals[index].Email), email) {
				continue
			}
			if match >= 0 {
				return fmt.Errorf("identity: workspace contains duplicate principal email %q", email)
			}
			match = index
		}
		if match >= 0 {
			principal := &state.Principals[match]
			resolved = principalAccessRoles(principal.Roles)
			if len(resolved) == 0 {
				return errNotAllowed
			}
			principal.Email = email
			principal.LastSeenAt = now
			if name != "" && principal.Name == "" {
				principal.Name = name
			}
			return nil
		}
		if !allowlisted {
			return errNotAllowed
		}
		resolved = []string{RoleOrganizer}
		state.Principals = append(state.Principals, domain.Principal{
			ID:         domain.NewID("prin"),
			Email:      email,
			Name:       name,
			Roles:      append([]string(nil), resolved...),
			CreatedAt:  now,
			LastSeenAt: now,
		})
		return nil
	})
	if errors.Is(err, errNotAllowed) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return append([]string(nil), resolved...), true, nil
}

func organizerEmailAllowed(email string) bool {
	for _, allowed := range AllowedEmails() {
		if strings.EqualFold(allowed, email) {
			return true
		}
	}
	return false
}

func principalAccessRoles(roles []string) []string {
	resolved := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		if !provisionableRole(role) {
			continue
		}
		if _, duplicate := seen[role]; duplicate {
			continue
		}
		seen[role] = struct{}{}
		resolved = append(resolved, role)
	}
	return resolved
}

// ReconcileOrganizerSessions canonicalizes organizer-surface authority from
// State.Principals before auth.Manager resolves the request user. Signed
// cookies, pending magic links, and passkey records can all carry an older
// auth.User payload; after any one of those payloads is signed in, this
// middleware replaces organizer/chair/observer roles on every request with
// the current durable grant. Missing, duplicate, malformed, or explicitly
// revoked principals fail closed by losing every organizer-surface role.
// Speaker and reviewer roles are preserved and sessions unrelated to the
// organizer identity plane are left untouched.
//
// Mount this after session.Manager.Middleware and before auth.Manager.Middleware.
func ReconcileOrganizerSessions() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			store := session.Current(r)
			if store == nil {
				next.ServeHTTP(w, r)
				return
			}
			var user auth.User
			if !store.Decode(sessionUserKey, &user) || strings.TrimSpace(user.ID) == "" {
				next.ServeHTTP(w, r)
				return
			}
			roles, reconcile := canonicalOrganizerRoles(user)
			if reconcile && !sameRoles(user.Roles, roles) {
				user.Roles = roles
				store.Set(sessionUserKey, user)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// canonicalOrganizerRoles returns the complete canonical role slice and
// whether the request belongs to the organizer identity plane. Durable roles
// replace only organizer-facing roles; a mixed speaker/organizer session keeps
// its speaker role after organizer access is revoked.
func canonicalOrganizerRoles(user auth.User) ([]string, bool) {
	email := strings.ToLower(strings.TrimSpace(user.Email))
	hadAccess := hasOrganizerAccessRole(user.Roles)
	if email == "" {
		if hadAccess {
			return replaceOrganizerAccessRoles(user.Roles, nil), true
		}
		return user.Roles, false
	}
	workspace, err := appstate.Get()
	if err != nil {
		if hadAccess {
			return replaceOrganizerAccessRoles(user.Roles, nil), true
		}
		return user.Roles, false
	}

	var (
		matched       bool
		duplicate     bool
		durableAccess []string
	)
	for _, principal := range workspace.Snapshot().Principals {
		if !strings.EqualFold(strings.TrimSpace(principal.Email), email) {
			continue
		}
		if matched {
			duplicate = true
			break
		}
		matched = true
		durableAccess = principalAccessRoles(principal.Roles)
	}
	if !matched && !hadAccess {
		return user.Roles, false
	}
	if duplicate || !matched {
		durableAccess = nil
	}
	return replaceOrganizerAccessRoles(user.Roles, durableAccess), true
}

func hasOrganizerAccessRole(roles []string) bool {
	for _, role := range roles {
		if provisionableRole(role) {
			return true
		}
	}
	return false
}

func replaceOrganizerAccessRoles(current, durable []string) []string {
	result := make([]string, 0, len(current)+len(durable))
	seen := make(map[string]struct{}, len(current)+len(durable))
	for _, role := range current {
		if provisionableRole(role) {
			continue
		}
		if _, duplicate := seen[role]; duplicate {
			continue
		}
		seen[role] = struct{}{}
		result = append(result, role)
	}
	for _, role := range durable {
		if !provisionableRole(role) {
			continue
		}
		if _, duplicate := seen[role]; duplicate {
			continue
		}
		seen[role] = struct{}{}
		result = append(result, role)
	}
	return result
}

// upsertOrganizerPrincipal records or refreshes the Principal for email:
// LastSeenAt bumps on every call, RoleOrganizer is added if missing, and
// name fills in Name only when the stored record has none yet.
func upsertOrganizerPrincipal(email, name string) error {
	store, err := appstate.Get()
	if err != nil {
		return err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	now := time.Now().UTC()
	return store.Update(func(state *domain.State) error {
		for index := range state.Principals {
			principal := &state.Principals[index]
			if !strings.EqualFold(principal.Email, email) {
				continue
			}
			principal.Roles = addRole(principal.Roles, RoleOrganizer)
			principal.LastSeenAt = now
			if name != "" && principal.Name == "" {
				principal.Name = name
			}
			return nil
		}
		state.Principals = append(state.Principals, domain.Principal{
			ID:         domain.NewID("prin"),
			Email:      email,
			Name:       name,
			Roles:      []string{RoleOrganizer},
			CreatedAt:  now,
			LastSeenAt: now,
		})
		return nil
	})
}

func addRole(roles []string, role string) []string {
	for _, existing := range roles {
		if existing == role {
			return roles
		}
	}
	return append(append([]string(nil), roles...), role)
}

// RequireAnyRole builds a middleware that lets a request through when the
// resolved user carries at least one of roles. auth.Manager.RequireRole
// checks exactly one role per wrap; this is the composed form the
// organizer surface needs, since organizer, chair, and observer sessions
// all reach /organizer. An anonymous or under-privileged visitor gets the
// same response shape Manager.RequireRole gives for one role: a JSON
// request gets 401, everything else redirects to LoginPath with the
// original path preserved.
func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.Current(r)
			if ok && hasAnyRole(user, roles) {
				next.ServeHTTP(w, r)
				return
			}
			denyUnauthorized(w, r)
		})
	}
}

func hasAnyRole(user auth.User, roles []string) bool {
	for _, role := range roles {
		for _, candidate := range user.Roles {
			if candidate == role {
				return true
			}
		}
	}
	return false
}

func denyUnauthorized(w http.ResponseWriter, r *http.Request) {
	if requestWantsJSON(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "authentication required"})
		return
	}
	target := LoginPath
	if r != nil && r.URL != nil {
		values := url.Values{}
		values.Set("next", r.URL.RequestURI())
		target += "?" + values.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func requestWantsJSON(r *http.Request) bool {
	if r == nil {
		return false
	}
	accept := r.Header.Get("Accept")
	contentType := r.Header.Get("Content-Type")
	return strings.Contains(accept, "application/json") || strings.HasPrefix(contentType, "application/json")
}
