package identity

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"m31labs.dev/gosx/auth"
	"m31labs.dev/gosx/session"
)

func TestFirstOrganizerSetupAndSignIn(t *testing.T) {
	for _, mailConfigured := range []bool{false, true} {
		t.Run(map[bool]string{false: "without-mail", true: "with-mail"}[mailConfigured], func(t *testing.T) {
			newTestWorkspace(t)
			t.Setenv("ORGANIZER_EMAILS", "")
			sessions := session.MustNew("setup-test-secret-at-least-32-characters", session.Options{AllowInsecure: true})
			manager := New(sessions)
			var delivered string
			links := manager.MagicLinks(auth.MagicLinkOptions{
				BaseURL: "http://localhost", SuccessPath: "/organizer", Store: DurableMagicLinkStore{},
				Resolver: auth.MagicLinkResolverFunc(ResolveEmail),
				Sender: auth.MagicLinkSenderFunc(func(_ context.Context, delivery auth.MagicLinkDelivery) error {
					delivered = delivery.URL
					return nil
				}),
			})
			digest := sha256.Sum256([]byte("one-time-setup"))
			setup := &Setup{manager: manager, magicLinks: links, mailConfigured: mailConfigured, tokenHash: digest[:]}
			finish := sessions.Middleware(manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, err := setup.Complete(r, "one-time-setup", "organizer@example.com", "")
				if err != nil {
					t.Fatal(err)
				}
				want := "/organizer"
				if mailConfigured {
					want = LoginPath
				}
				if path != want {
					t.Fatalf("setup target = %q, want %q", path, want)
				}
			})))
			response := httptest.NewRecorder()
			finish.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/setup", nil))
			if setup.Verify("one-time-setup") {
				t.Fatal("setup token remained usable")
			}
			cookies := response.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("setup did not establish session state")
			}
			if mailConfigured {
				link, err := url.Parse(delivered)
				if err != nil || delivered == "" {
					t.Fatal("setup did not send a sign-in link")
				}
				callback := httptest.NewRequest(http.MethodGet, link.RequestURI(), nil)
				callback.AddCookie(cookies[0])
				response = httptest.NewRecorder()
				sessions.Middleware(manager.Middleware(links.CallbackHandler())).ServeHTTP(response, callback)
				if response.Code != http.StatusSeeOther {
					t.Fatal("setup callback failed")
				}
				cookies = response.Result().Cookies()
			}
			check := httptest.NewRequest(http.MethodGet, "/organizer", nil)
			check.AddCookie(cookies[0])
			sessions.Middleware(manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, signed := auth.Current(r)
				if !signed || user.ID != "organizer@example.com" || !hasRole(user, RoleOrganizer) {
					t.Fatal("setup did not authenticate the first organizer")
				}
			}))).ServeHTTP(httptest.NewRecorder(), check)
		})
	}
}
