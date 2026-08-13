package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/session"
)

// Setup is the one-time break-glass bootstrap for a fresh self-host with no
// ORGANIZER_EMAILS configured and no stored organizer Principal. It holds
// only the SHA-256 hash of a single-use setup token in memory — never the
// token itself — and arms exactly once, at construction.
//
// This differs from a shared or logged credential in every way that
// matters: the token is single-use and process-wide, it dies at first use,
// only its hash is ever held, and completing it creates one attributable
// Principal instead of an anonymous scope.
type Setup struct {
	manager        *auth.Manager
	magicLinks     *auth.MagicLinks
	mailConfigured bool

	mu        sync.Mutex
	tokenHash []byte // nil once consumed, or when break-glass never armed
}

// NewSetup arms the break-glass bootstrap when ORGANIZER_EMAILS is empty and
// no stored Principal already carries RoleOrganizer. It logs the one-time
// setup URL exactly once; a restart with an existing organizer never
// re-arms it. publicBase is the absolute origin (PUBLIC_URL) the logged URL
// is built from. mailConfigured tells Complete whether to email the first
// organizer a magic link (true) or sign the browser in directly (false) —
// pass whether a real mail transport (not the local outbox) is configured.
func NewSetup(manager *auth.Manager, magicLinks *auth.MagicLinks, mailConfigured bool, publicBase string) *Setup {
	setup := &Setup{manager: manager, magicLinks: magicLinks, mailConfigured: mailConfigured}
	if len(AllowedEmails()) > 0 || hasStoredOrganizer() {
		return setup
	}
	token, err := randomSetupToken()
	if err != nil {
		log.Printf("identity: could not arm break-glass setup: %v", err)
		return setup
	}
	sum := sha256.Sum256([]byte(token))
	setup.tokenHash = sum[:]
	log.Printf("Rostrum has no organizer yet. Finish setup at: %s/setup?token=%s", strings.TrimRight(publicBase, "/"), token)
	return setup
}

func randomSetupToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Verify reports whether token matches the armed setup token, without
// consuming it. app/setup's loader calls this on every GET so a reload
// keeps working until the token is actually spent.
func (s *Setup) Verify(token string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verifyLocked(token)
}

func (s *Setup) verifyLocked(token string) bool {
	if s.tokenHash == nil || strings.TrimSpace(token) == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(sum[:], s.tokenHash) == 1
}

// consume verifies and single-use-invalidates token in one step. A second
// call with the same token, from any request, reports false: the hash is
// gone the instant the first call matches, so /setup returns 404 forever
// after.
func (s *Setup) consume(token string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.verifyLocked(token) {
		return false
	}
	s.tokenHash = nil
	return true
}

// Complete finishes the break-glass bootstrap: it consumes token, records
// the first Principal with RoleOrganizer, and either emails that address a
// magic link (when mail is configured) or signs the browser in directly
// (when it is not — the in-memory outbox has no operator delivery channel).
// It returns the path the caller should redirect to on success.
func (s *Setup) Complete(r *http.Request, token, email, name string) (redirectPath string, err error) {
	if s == nil || s.manager == nil {
		return "", fmt.Errorf("setup is not available")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if email == "" || !strings.Contains(email, "@") {
		return "", fmt.Errorf("enter a valid email address")
	}
	if !s.consume(token) {
		return "", fmt.Errorf("this setup link has already been used or is invalid")
	}
	if err := upsertOrganizerPrincipal(email, name); err != nil {
		return "", fmt.Errorf("could not save the first organizer: %w", err)
	}
	if s.mailConfigured && s.magicLinks != nil {
		_, sendErr := s.magicLinks.Send(r, email, "/organizer")
		if sendErr == nil {
			session.AddFlash(r, MagicLinkFlashKey, map[string]any{"status": "sent", "email": email})
			return LoginPath, nil
		}
		log.Printf("identity: setup could not email the first organizer, signing the browser in directly: %v", sendErr)
	}
	user := auth.User{ID: email, Email: email, Name: name, Roles: []string{RoleOrganizer}}
	if !s.manager.SignIn(r, user) {
		return "", fmt.Errorf("could not start a session; sign in from %s instead", LoginPath)
	}
	session.AddFlash(r, "notice", "You are the first organizer. Welcome to Rostrum.")
	return "/organizer", nil
}

var (
	setupMu        sync.RWMutex
	setupSingleton *Setup
)

// SetSetup stores the process-wide Setup instance main.go builds at
// startup. app/setup's page module calls CurrentSetup to reach it, because
// its init() runs before main() builds the manager.
func SetSetup(setup *Setup) {
	setupMu.Lock()
	defer setupMu.Unlock()
	setupSingleton = setup
}

// CurrentSetup returns the process-wide Setup instance, or nil before
// SetSetup runs.
func CurrentSetup() *Setup {
	setupMu.RLock()
	defer setupMu.RUnlock()
	return setupSingleton
}
