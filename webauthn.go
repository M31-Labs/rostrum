package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/m31-labs/rostrum/internal/identity"
	"m31labs.dev/gosx/auth"
)

// legacyWebAuthnLoginOptions restores credential selection for non-discoverable
// passkeys enrolled before the upgrade. Only the returned browser options use
// the email hint: GoSX owns the anonymous challenge and verifies the assertion
// against the stored credential before signing in. Registration never uses it.
func legacyWebAuthnLoginOptions(webAuthn *auth.WebAuthn) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Login string `json:"login"`
			Email string `json:"email"`
			Next  string `json:"next"`
			User  struct {
				Email string `json:"email"`
			} `json:"user"`
		}
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "Invalid passkey request.", http.StatusBadRequest)
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid passkey request.", http.StatusBadRequest)
				return
			}
			payload.Login, payload.Email, payload.Next = r.Form.Get("login"), r.Form.Get("email"), r.Form.Get("next")
		}
		options, err := webAuthn.BeginAuthentication(r, "", payload.Next)
		if err != nil {
			http.Error(w, "Unable to begin passkey sign-in.", http.StatusBadRequest)
			return
		}
		hint := strings.TrimSpace(payload.Login)
		if hint == "" {
			hint = strings.TrimSpace(payload.Email)
		}
		if hint == "" {
			// GoSX's declarative email selector sends user.email.
			hint = strings.TrimSpace(payload.User.Email)
		}
		if _, signedIn := auth.Current(r); !signedIn && hint != "" {
			credentials, err := (identity.DurableWebAuthnStore{}).CredentialsForEmail(hint)
			if err != nil {
				http.Error(w, "Unable to begin passkey sign-in.", http.StatusInternalServerError)
				return
			}
			for _, credential := range credentials {
				options.AllowCredentials = append(options.AllowCredentials, auth.WebAuthnCredentialDescriptor{
					Type: "public-key", ID: credential.ID, Transports: credential.Transports,
				})
			}
			if len(options.AllowCredentials) == 0 {
				// Unknown addresses and accounts without passkeys receive the
				// same successful options shape, with a random unusable ID.
				var decoy [32]byte
				if _, err := rand.Read(decoy[:]); err != nil {
					http.Error(w, "Unable to begin passkey sign-in.", http.StatusInternalServerError)
					return
				}
				options.AllowCredentials = []auth.WebAuthnCredentialDescriptor{{
					Type: "public-key", ID: base64.RawURLEncoding.EncodeToString(decoy[:]),
				}}
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "options": options})
	})
}
