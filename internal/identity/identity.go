// Package identity builds Rostrum's organizer identity plane: one GoSX auth
// manager, the role constants every principal carries, and the allowlist
// that decides who may act as an organizer. It owns no password and stores
// no credential; it wires the GoSX auth package (m31labs.dev/gosx/auth) over
// the application's existing session manager and the workspace JSON store.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	return auth.New(sessions, auth.Options{LoginPath: LoginPath})
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

// isAllowed reports whether email is on the ORGANIZER_EMAILS allowlist or
// matches a stored Principal that already carries RoleOrganizer.
func isAllowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, allowed := range AllowedEmails() {
		if allowed == email {
			return true
		}
	}
	store, err := appstate.Get()
	if err != nil {
		return false
	}
	for _, principal := range store.Snapshot().Principals {
		if !strings.EqualFold(principal.Email, email) {
			continue
		}
		for _, role := range principal.Roles {
			if role == RoleOrganizer {
				return true
			}
		}
	}
	return false
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
// before it issues a token. It never returns an error: an allowlisted or
// previously provisioned email is granted RoleOrganizer and its Principal
// is upserted; every other address still resolves to a plain signed-in
// User with no roles. Either way the caller (main.go's mounted
// RequestHandler) sends the identical "check your email" response, so this
// endpoint is never an allowlist oracle — the only difference an attacker
// could probe for is invisible to them.
func ResolveEmail(_ context.Context, rawEmail string) (auth.User, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	user := auth.User{ID: email, Email: email}
	if !isAllowed(email) {
		return user, nil
	}
	if err := upsertOrganizerPrincipal(email, ""); err != nil {
		// A bookkeeping write failure must not block a legitimate sign-in:
		// the next magic-link request re-derives the same allowlist grant.
		return user, nil
	}
	user.Roles = []string{RoleOrganizer}
	return user, nil
}

// GrantRoles resolves organizer access for an OAuth-verified identity and
// upserts the matching Principal on success. Unlike ResolveEmail, it
// returns an error when the email is neither on the allowlist nor already
// a stored organizer Principal; the OAuth resolver wrapper (oauth.go)
// treats that error as a hard stop, so a non-allowlisted verified identity
// never receives a session. This is safe to make stricter than the
// magic-link path because OAuth already proved the caller controls the
// address — there is no anonymous prober to keep an oracle from, only the
// account holder learning their own address is not on the list.
func GrantRoles(_ context.Context, user auth.User) (auth.User, error) {
	email := strings.ToLower(strings.TrimSpace(user.Email))
	if email == "" {
		return auth.User{}, errNoEmail
	}
	if !isAllowed(email) {
		return auth.User{}, errNotAllowed
	}
	if err := upsertOrganizerPrincipal(email, user.Name); err != nil {
		return auth.User{}, err
	}
	user.Email = email
	user.Roles = addRole(user.Roles, RoleOrganizer)
	return user, nil
}

var (
	errNoEmail    = errors.New("identity: oauth user has no email")
	errNotAllowed = errors.New("identity: email is not on the organizer allowlist")
)

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
