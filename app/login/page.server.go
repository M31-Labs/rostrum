// Package login renders /login: the single sign-in surface for every
// organizer-facing identity method Rostrum ships. It is a zero-bespoke-
// JavaScript GoSX page: a magic-link email form, one anchor per configured
// OAuth provider, and a passkey button bound by the framework's own
// auth.WebAuthnScript() runtime. See specs/identity-plane.md.
package login

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/identity"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: loadLogin,
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			// The WebAuthn runtime is an inline <script> GoSX owns (like
			// server.NavigationScript()), added here so it ships only on the
			// page that needs it. main.go hashes the identical render for the
			// CSP script-src allowlist.
			ctx.AddHead(auth.WebAuthnScript())
			return server.Metadata{Title: server.Title{Default: "Sign in — Rostrum"}}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}
}

func loadLogin(ctx *route.RouteContext, page route.FilePage) (any, error) {
	next := sanitizeNext(ctx.Query("next"))
	providers := oauthProviderLinks(next)
	return map[string]any{
		"workspace":       present.WorkspaceIdentity(appstate.MustGet().Snapshot()),
		"readOnlyDemo":    present.ReadOnlyDemoMode(),
		"next":            next,
		"providers":       providers,
		"hasProviders":    len(providers) > 0,
		"notice":          loginNotice(ctx.Request),
		"webauthnPayload": webauthnPayloadJSON(ctx.Request),
	}, nil
}

// loginNotice reads the magic-link and OAuth flashes directly (rather than
// through the template's ambient `flash` binding) so the template only ever
// handles plain, identifier-safe map keys — neither flash key
// ("auth.magic_link", "auth.oauth") is itself a valid dotted field path.
func loginNotice(r *http.Request) map[string]any {
	store := session.Current(r)
	if store == nil {
		return map[string]any{"show": false}
	}
	if fields, ok := firstFlashFields(store.Flashes(identity.MagicLinkFlashKey)); ok {
		switch fields["status"] {
		case "sent":
			return map[string]any{"show": true, "kind": "ok", "message": "Check your email for a sign-in link. It works once and expires soon."}
		case "signed_in":
			return map[string]any{"show": true, "kind": "ok", "message": "Signed in."}
		case "error":
			return map[string]any{"show": true, "kind": "error", "message": "That sign-in link is invalid or has expired. Request a new one below."}
		}
	}
	if fields, ok := firstFlashFields(store.Flashes("auth.oauth")); ok {
		if fields["status"] == "error" {
			return map[string]any{"show": true, "kind": "error", "message": "That sign-in attempt did not complete. Try again."}
		}
	}
	return map[string]any{"show": false}
}

func firstFlashFields(entries []any) (map[string]any, bool) {
	if len(entries) == 0 {
		return nil, false
	}
	fields, ok := entries[0].(map[string]any)
	return fields, ok
}

// oauthProviderLinks lists only the OAuth providers main.go actually
// mounted (identity.ConfiguredProviderNames mirrors identity.Providers), so
// the page never advertises a sign-in method that would 404.
func oauthProviderLinks(next string) []map[string]string {
	names := identity.ConfiguredProviderNames()
	links := make([]map[string]string, 0, len(names))
	for _, name := range names {
		links = append(links, map[string]string{
			"name":  name,
			"label": "Continue with " + providerLabel(name),
			"href":  oauthHref(name, next),
		})
	}
	return links
}

func providerLabel(name string) string {
	switch name {
	case "github":
		return "GitHub"
	case "google":
		return "Google"
	default:
		return name
	}
}

func oauthHref(name, next string) string {
	target := "/auth/oauth/" + name
	if next != "" {
		target += "?" + url.Values{"next": {next}}.Encode()
	}
	return target
}

// sanitizeNext keeps only a same-site path, so a forged `next` never turns
// the sign-in form or an OAuth anchor into an open redirect. auth.MagicLinks
// and auth.OAuth both re-validate their own `next` value before using it;
// this is the page's own defense so it never even links to an unsafe target.
func sanitizeNext(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return ""
	}
	return value
}

// webauthnPayloadJSON builds the JSON auth.WebAuthnScript()'s declarative
// binding posts as the request body prefix: the session's CSRF token, which
// session.Manager.Protect requires on the passkey option and finish POSTs
// the same way it requires csrf_token on every other unsafe request.
func webauthnPayloadJSON(r *http.Request) string {
	payload, err := json.Marshal(map[string]string{"csrfToken": session.Token(r)})
	if err != nil {
		return "{}"
	}
	return string(payload)
}
