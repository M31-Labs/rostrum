package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/identity"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

const authTestOrigin = "http://localhost"

type authTestClient struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
	csrf    string
}

func (c *authTestClient) request(method, path, contentType string, body []byte, csrf bool) *httptest.ResponseRecorder {
	c.t.Helper()
	r := httptest.NewRequest(method, authTestOrigin+path, bytes.NewReader(body))
	if method == http.MethodPost {
		r.Header.Set("Accept", "application/json")
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	if csrf {
		r.Header.Set("X-CSRF-Token", c.csrf)
	}
	response := httptest.NewRecorder()
	c.handler.ServeHTTP(response, r)
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "gosx_session" {
			c.cookie = cookie
		}
	}
	return response
}

func authJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (c *authTestClient) post(path string, value any, status int) *httptest.ResponseRecorder {
	c.t.Helper()
	response := c.request(http.MethodPost, path, "application/json", authJSON(c.t, value), true)
	if response.Code != status {
		c.t.Fatalf("%s status = %d, want %d: %s", path, response.Code, status, response.Body.String())
	}
	return response
}

func (c *authTestClient) current() (auth.User, bool) {
	c.t.Helper()
	response := c.request(http.MethodGet, "/current", "", nil, false)
	var result struct {
		User   auth.User
		Signed bool
		CSRF   string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		c.t.Fatal(err)
	}
	c.csrf = result.CSRF
	return result.User, result.Signed
}

func newAuthTestClient(t *testing.T, handler http.Handler) *authTestClient {
	t.Helper()
	c := &authTestClient{t: t, handler: handler}
	c.current()
	return c
}

func authTestApp(t *testing.T, sender mail.Sender) (http.Handler, *auth.Manager, *session.Manager) {
	t.Helper()
	t.Setenv("APP_MODE", "live")
	t.Setenv("ORGANIZER_EMAILS", "organizer@example.com")
	workspace, err := store.Open(":memory:", domain.EmptyState(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	appstate.Set(workspace)
	sessions := testSessionManager(t)
	manager := identity.New(sessions)
	app := server.New()
	app.Use(sessions.Middleware)
	app.Use(identity.ReconcileOrganizerSessions())
	app.Use(manager.Middleware)
	app.Use(csrfProtection(sessions))
	mountWebAuthnRoutes(app, manager, authTestOrigin)
	options := auth.MagicLinkOptions{
		BaseURL: authTestOrigin, SuccessPath: "/organizer", FailurePath: "/login",
		Resolver: auth.MagicLinkResolverFunc(identity.ResolveEmail), Store: identity.DurableMagicLinkStore{},
	}
	if sender != nil {
		options.Sender = identity.MailSender{Sender: sender}
	}
	links := manager.MagicLinks(options)
	app.Mount("POST /auth/magic-link", managedMagicLinkRequest(links))
	app.Mount("GET /auth/magic-link", links.CallbackHandler())
	app.Mount("GET /current", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Persist an anonymous session so token checks cover existing cookies.
		session.Current(r).Set("test.browser", true)
		user, signed := auth.Current(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"User": user, "Signed": signed, "CSRF": session.Token(r)})
	}))
	return app.Build(), manager, sessions
}

func TestPasskeyEndpointsRequireAuthenticationAndCSRF(t *testing.T) {
	handler, _, _ := authTestApp(t, nil)
	client := newAuthTestClient(t, handler)
	claims := map[string]any{"user": auth.User{ID: "other-user", Roles: []string{identity.RoleOrganizer}}, "id": "other-user", "roles": []string{identity.RoleOrganizer}}
	for _, path := range []string{"/auth/webauthn/register-options", "/auth/webauthn/register", "/auth/webauthn/login-options", "/auth/webauthn/login", "/auth/magic-link"} {
		response := client.request(http.MethodPost, path, "application/json", authJSON(t, claims), false)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s without CSRF = %d, want 403", path, response.Code)
		}
	}
	for _, path := range []string{"/auth/webauthn/register-options", "/auth/webauthn/register"} {
		client.post(path, claims, http.StatusUnauthorized)
		// Cold browsers use GoSX's same-origin CSRF check before a cookie exists.
		r := httptest.NewRequest(http.MethodPost, authTestOrigin+path, bytes.NewReader(authJSON(t, claims)))
		r.Header.Set("Origin", authTestOrigin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("cold anonymous registration = %d, want 401", response.Code)
		}
	}
	if _, signed := client.current(); signed || len(appstate.MustGet().Snapshot().AuthPasskeys) != 0 {
		t.Fatal("anonymous registration changed identity or credentials")
	}
}

func authData(counter uint32) []byte {
	hash := sha256.Sum256([]byte("localhost"))
	data := make([]byte, 37)
	copy(data, hash[:])
	data[32] = 0x05 // User presence and verification.
	binary.BigEndian.PutUint32(data[33:], counter)
	return data
}

func TestPasskeyEnrollmentPreservesIdentityAndDiscoverableLoginWorks(t *testing.T) {
	handler, manager, sessions := authTestApp(t, nil)
	client := newAuthTestClient(t, handler)
	user, err := identity.ResolveEmail(t.Context(), "organizer@example.com")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the existing authenticated setup/OAuth handoff.
	signIn := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !manager.SignIn(r, user) {
			t.Fatal("sign in failed")
		}
	}))
	client.handler = signIn
	client.request(http.MethodGet, "/", "", nil, false)
	client.handler = handler
	client.current()
	begin := client.post("/auth/webauthn/register-options", map[string]any{
		"user": auth.User{ID: "other-user", Roles: []string{identity.RoleOrganizer}}, "next": "/organizer",
	}, http.StatusOK)
	var creation struct{ Options auth.WebAuthnCreationOptions }
	if err := json.Unmarshal(begin.Body.Bytes(), &creation); err != nil {
		t.Fatal(err)
	}
	if creation.Options.User.ID != base64.RawURLEncoding.EncodeToString([]byte(user.ID)) {
		t.Fatal("registration used request claims instead of authenticated identity")
	}
	// Revoke organizer privileges while enrollment is in progress.
	workspace := appstate.MustGet()
	if err := workspace.Update(func(state *domain.State) error {
		state.Principals[0].Roles = []string{identity.RoleObserver}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	credentialID := encode([]byte("test-credential"))
	var registration auth.WebAuthnRegistrationResponse
	registration.ID, registration.RawID, registration.Type = credentialID, credentialID, "public-key"
	registration.Response.ClientDataJSON = encode(authJSON(t, map[string]string{"type": "webauthn.create", "challenge": creation.Options.Challenge, "origin": authTestOrigin}))
	registration.Response.AuthenticatorData = encode(authData(0))
	registration.Response.PublicKey, registration.Response.PublicKeyAlgorithm = encode(publicKey), -7
	client.post("/auth/webauthn/register", registration, http.StatusOK)
	expectedRoles := []string{identity.RoleObserver}
	current, signed := client.current()
	if !signed || current.ID != user.ID || !reflect.DeepEqual(current.Roles, expectedRoles) {
		t.Fatalf("enrollment changed session identity or restored privileges: %#v", current)
	}
	credential, err := (identity.DurableWebAuthnStore{}).Credential(credentialID)
	if err != nil || credential.User.ID != user.ID || !reflect.DeepEqual(credential.User.Roles, expectedRoles) {
		t.Fatal("credential did not preserve canonical identity and current roles")
	}
	// A separate anonymous browser signs in with the enrolled passkey.
	anonymous := newAuthTestClient(t, handler)
	begin = anonymous.post("/auth/webauthn/login-options", map[string]string{"next": "/organizer"}, http.StatusOK)
	var request struct{ Options auth.WebAuthnRequestOptions }
	if err := json.Unmarshal(begin.Body.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Options.AllowCredentials) != 0 || strings.Contains(begin.Body.String(), credentialID) {
		t.Fatal("anonymous options disclosed account credential IDs")
	}
	clientData := authJSON(t, map[string]string{"type": "webauthn.get", "challenge": request.Options.Challenge, "origin": authTestOrigin})
	data := authData(1)
	clientHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte(nil), data...), clientHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	var assertion auth.WebAuthnAuthenticationResponse
	assertion.ID, assertion.RawID, assertion.Type = credentialID, credentialID, "public-key"
	assertion.Response.ClientDataJSON, assertion.Response.AuthenticatorData, assertion.Response.Signature = encode(clientData), encode(data), encode(signature)
	anonymous.post("/auth/webauthn/login", assertion, http.StatusOK)
	current, signed = anonymous.current()
	if !signed || current.ID != user.ID || !reflect.DeepEqual(current.Roles, expectedRoles) {
		t.Fatal("passkey login did not restore the canonical identity")
	}
}

func TestLegacyNonDiscoverablePasskeyLoginWithEmailHint(t *testing.T) {
	for _, hint := range []string{"login", "email", "user.email", "form"} {
		t.Run(hint, func(t *testing.T) {
			handler, _, _ := authTestApp(t, nil)
			owner, err := identity.ResolveEmail(t.Context(), "organizer@example.com")
			if err != nil {
				t.Fatal(err)
			}
			owner.ID = "legacy-account"
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			publicKey, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
			if err != nil {
				t.Fatal(err)
			}
			encode := base64.RawURLEncoding.EncodeToString
			credentialID := encode([]byte("legacy-non-discoverable"))
			credentials := identity.DurableWebAuthnStore{}
			if err := credentials.SaveCredential(auth.WebAuthnCredential{
				ID: credentialID, User: owner, PublicKey: publicKey, Algorithm: -7, SignCount: 7,
			}); err != nil {
				t.Fatal(err)
			}
			client := newAuthTestClient(t, handler)
			before := appstate.MustGet().Snapshot()
			payload := map[string]any{"next": "/organizer", hint: "  ORGANIZER@example.com  "}
			if hint == "user.email" {
				payload = map[string]any{"user": auth.User{ID: "forged-account", Email: owner.Email, Roles: []string{identity.RoleChair}}}
			}
			var begin *httptest.ResponseRecorder
			if hint == "form" {
				begin = client.request(http.MethodPost, "/auth/webauthn/login-options", "application/x-www-form-urlencoded", []byte(url.Values{"login": {owner.Email}, "next": {"/organizer"}}.Encode()), true)
				if begin.Code != http.StatusOK {
					t.Fatalf("form options = %d: %s", begin.Code, begin.Body.String())
				}
			} else {
				begin = client.post("/auth/webauthn/login-options", payload, http.StatusOK)
			}
			var request struct{ Options auth.WebAuthnRequestOptions }
			if err := json.Unmarshal(begin.Body.Bytes(), &request); err != nil {
				t.Fatal(err)
			}
			if len(request.Options.AllowCredentials) != 1 || request.Options.AllowCredentials[0].ID != credentialID {
				t.Fatal("email hint did not offer the legacy credential")
			}
			if _, signed := client.current(); signed || !reflect.DeepEqual(before, appstate.MustGet().Snapshot()) {
				t.Fatal("email hint authenticated or mutated the workspace")
			}
			// Knowing the email and offered ID does not replace the private key.
			attacker := newAuthTestClient(t, handler)
			attackBegin := attacker.post("/auth/webauthn/login-options", map[string]string{"email": owner.Email}, http.StatusOK)
			var attackRequest struct{ Options auth.WebAuthnRequestOptions }
			if err := json.Unmarshal(attackBegin.Body.Bytes(), &attackRequest); err != nil {
				t.Fatal(err)
			}
			var forged auth.WebAuthnAuthenticationResponse
			forged.ID, forged.RawID, forged.Type = credentialID, credentialID, "public-key"
			forged.Response.ClientDataJSON = encode(authJSON(t, map[string]string{"type": "webauthn.get", "challenge": attackRequest.Options.Challenge, "origin": authTestOrigin}))
			forged.Response.AuthenticatorData, forged.Response.Signature = encode(authData(8)), encode([]byte("invalid-signature"))
			attacker.post("/auth/webauthn/login", forged, http.StatusUnauthorized)
			if _, signed := attacker.current(); signed || !reflect.DeepEqual(before, appstate.MustGet().Snapshot()) {
				t.Fatal("email hint bypassed signature verification")
			}
			// The legacy device requires allowCredentials and returns no userHandle.
			clientData := authJSON(t, map[string]string{"type": "webauthn.get", "challenge": request.Options.Challenge, "origin": authTestOrigin})
			data := authData(8)
			clientHash := sha256.Sum256(clientData)
			digest := sha256.Sum256(append(append([]byte(nil), data...), clientHash[:]...))
			signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
			if err != nil {
				t.Fatal(err)
			}
			var assertion auth.WebAuthnAuthenticationResponse
			assertion.ID, assertion.RawID, assertion.Type = credentialID, credentialID, "public-key"
			assertion.Response.ClientDataJSON, assertion.Response.AuthenticatorData, assertion.Response.Signature = encode(clientData), encode(data), encode(signature)
			client.post("/auth/webauthn/login", assertion, http.StatusOK)
			current, signed := client.current()
			if !signed || current.ID != owner.ID || !reflect.DeepEqual(current.Roles, owner.Roles) {
				t.Fatal("legacy login trusted hint claims instead of the credential owner")
			}
			stored, err := credentials.Credential(credentialID)
			if err != nil || stored.SignCount != 8 {
				t.Fatal("legacy login did not advance the signature counter")
			}
			client.post("/auth/webauthn/login", assertion, http.StatusUnauthorized)
		})
	}
}

func TestPasskeyEmailHintsReturnUniformOptionsWithoutIdentityChanges(t *testing.T) {
	handler, _, _ := authTestApp(t, nil)
	// The allowlisted address also has no passkeys. Looking it up must not
	// provision it, authenticate it, or return a distinct account error.
	for _, hint := range []string{"organizer@example.com", "unknown@example.com"} {
		client := newAuthTestClient(t, handler)
		before := appstate.MustGet().Snapshot()
		begin := client.post("/auth/webauthn/login-options", map[string]string{"email": hint}, http.StatusOK)
		var response struct {
			OK      bool `json:"ok"`
			Options auth.WebAuthnRequestOptions
		}
		if err := json.Unmarshal(begin.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if !response.OK || response.Options.Challenge == "" || len(response.Options.AllowCredentials) != 1 || response.Options.AllowCredentials[0].Type != "public-key" {
			t.Fatal("unknown hint did not return normal credential options")
		}
		if strings.Contains(begin.Body.String(), hint) || strings.Contains(begin.Body.String(), "roles") {
			t.Fatal("hint options exposed account claims")
		}
		client.post("/auth/webauthn/register-options", map[string]any{"user": auth.User{ID: hint, Email: hint, Roles: []string{identity.RoleOrganizer}}}, http.StatusUnauthorized)
		client.post("/auth/webauthn/login", auth.WebAuthnAuthenticationResponse{ID: response.Options.AllowCredentials[0].ID}, http.StatusUnauthorized)
		if _, signed := client.current(); signed || !reflect.DeepEqual(before, appstate.MustGet().Snapshot()) {
			t.Fatal("unknown hint changed identity or credentials")
		}
	}
}

func TestMagicLinkResponsesNeverExposeDeliveredLinkAndCallbackSignsIn(t *testing.T) {
	for _, contentType := range []string{"application/json", "application/x-www-form-urlencoded", "native-form"} {
		t.Run(contentType, func(t *testing.T) {
			sender := mail.NewOutboxSender()
			handler, _, _ := authTestApp(t, sender)
			client := newAuthTestClient(t, handler)
			body := authJSON(t, map[string]string{"email": "organizer@example.com", "next": "/organizer"})
			if contentType != "application/json" {
				body = []byte(url.Values{"email": {"organizer@example.com"}, "next": {"/organizer"}}.Encode())
			}
			expected := http.StatusOK
			if contentType == "native-form" {
				contentType = "application/x-www-form-urlencoded"
				expected = http.StatusSeeOther
				client.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.Header.Del("Accept")
					handler.ServeHTTP(w, r)
				})
			}
			response := client.request(http.MethodPost, "/auth/magic-link", contentType, body, true)
			if response.Code != expected || len(sender.Sent()) != 1 {
				t.Fatalf("request status = %d, deliveries = %d", response.Code, len(sender.Sent()))
			}
			var link string
			for _, line := range strings.Split(sender.Sent()[0].TextBody, "\n") {
				if strings.HasPrefix(line, authTestOrigin+"/auth/magic-link?") {
					link = line
				}
			}
			parsed, err := url.Parse(link)
			if err != nil || link == "" || parsed.Query().Get("token") == "" {
				t.Fatal("mail adapter did not deliver a callback link")
			}
			wire := response.Body.String() + response.Header().Get("Location")
			if strings.Contains(wire, link) || strings.Contains(wire, parsed.Query().Get("token")) {
				t.Fatal("request response exposed the delivered sign-in link")
			}
			client.handler = handler
			if _, signed := client.current(); signed {
				t.Fatal("requesting a link signed the browser in")
			}
			callback := client.request(http.MethodGet, parsed.RequestURI(), "", nil, false)
			if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/organizer" {
				t.Fatal("delivered callback did not complete sign-in")
			}
			current, signed := client.current()
			if !signed || current.ID != "organizer@example.com" || !reflect.DeepEqual(current.Roles, []string{identity.RoleOrganizer}) {
				t.Fatal("magic-link sign-in did not establish organizer identity")
			}
			other := newAuthTestClient(t, handler)
			other.request(http.MethodGet, parsed.RequestURI(), "", nil, false)
			if _, signed := other.current(); signed {
				t.Fatal("magic link was reusable")
			}
		})
	}
}

func TestMagicLinkWithoutSenderFailsClosed(t *testing.T) {
	handler, _, _ := authTestApp(t, nil)
	client := newAuthTestClient(t, handler)
	response := client.post("/auth/magic-link", map[string]string{"email": "organizer@example.com"}, http.StatusBadRequest)
	if strings.Contains(response.Body.String(), "token") || len(appstate.MustGet().Snapshot().AuthMagicLinks) != 0 {
		t.Fatal("missing sender issued or exposed a magic link")
	}
}

func TestColdBrowserMagicLinkRequestUsesSameOriginCSRF(t *testing.T) {
	sender := mail.NewOutboxSender()
	handler, _, _ := authTestApp(t, sender)
	request := httptest.NewRequest(http.MethodPost, authTestOrigin+"/auth/magic-link", bytes.NewReader(authJSON(t, map[string]string{"email": "organizer@example.com"})))
	request.Header.Set("Origin", authTestOrigin)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(sender.Sent()) != 1 || strings.Contains(response.Body.String(), "token") {
		t.Fatal("cold browser could not request a delivered link safely")
	}
}

func TestRuntimeSessionSecretsAreRandomLocallyAndRequiredElsewhere(t *testing.T) {
	for _, placeholder := range []string{"", " \t\r\n", developmentSessionSecret, "change-me-in-production", "gosx-app-session-secret", "REPLACE_ME", " \tREPLACE_ME\r\n"} {
		first, err := runtimeSessionSecret(authTestOrigin, "development", placeholder)
		if err != nil || len(first) < 32 {
			t.Fatal("local development did not generate a strong secret")
		}
		second, err := runtimeSessionSecret(authTestOrigin, "development", placeholder)
		if err != nil || first == second {
			t.Fatal("development reused a fixed session secret")
		}
		for _, posture := range []struct{ origin, env string }{{authTestOrigin, "production"}, {"https://rostrum.example", "development"}} {
			if _, err := runtimeSessionSecret(posture.origin, posture.env, placeholder); err == nil {
				t.Fatal("production/public runtime accepted a placeholder secret")
			}
		}
	}
	secret := "explicit-session-secret-at-least-32-characters"
	if resolved, err := runtimeSessionSecret(authTestOrigin, "development", secret); err != nil || resolved != secret {
		t.Fatal("explicit secret was not preserved")
	}
}
