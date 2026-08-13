package identity

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"m31labs.dev/gosx/auth"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestGitHubAllowlistRequiresVerifiedPrimaryEmail(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		wantErr bool
	}{
		{
			name:    "verified primary",
			payload: `[{"email":"owner@example.com","primary":true,"verified":true}]`,
		},
		{
			name:    "unverified primary",
			payload: `[{"email":"owner@example.com","primary":true,"verified":false}]`,
			wantErr: true,
		},
		{
			name:    "verified secondary only",
			payload: `[{"email":"owner@example.com","primary":false,"verified":true}]`,
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != "https://api.github.com/user/emails" {
					t.Fatalf("request URL = %s", request.URL)
				}
				if got := request.Header.Get("Authorization"); got != "Bearer oauth-token" {
					t.Fatalf("authorization = %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(test.payload)),
					Header:     make(http.Header),
					Request:    request,
				}, nil
			})}
			err := requireGitHubVerifiedEmail(context.Background(), client, "oauth-token", "OWNER@example.com")
			if (err != nil) != test.wantErr {
				t.Fatalf("requireGitHubVerifiedEmail() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestGitHubAllowlistResolverVerifiesBeforeGranting(t *testing.T) {
	newTestWorkspace(t)
	t.Setenv("AUTH_GITHUB_HANDLES", "octocat")
	resolver := allowlistResolver(auth.OAuthUserResolverFunc(func(context.Context, auth.OAuthProvider, *http.Client, auth.OAuthToken) (auth.User, error) {
		return auth.User{
			ID:    "github:1",
			Email: "owner@example.com",
			Meta: map[string]any{
				"provider": "github",
				"profile":  "https://github.com/octocat",
			},
		}, nil
	}))
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`[{"email":"owner@example.com","primary":true,"verified":true}]`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	user, err := resolver.ResolveOAuthUser(context.Background(), auth.OAuthProvider{Name: "github"}, client, auth.OAuthToken{AccessToken: "oauth-token"})
	if err != nil {
		t.Fatalf("ResolveOAuthUser: %v", err)
	}
	if !hasRole(user, RoleOrganizer) {
		t.Fatalf("resolved roles = %v, want %s", user.Roles, RoleOrganizer)
	}
}
