package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"m31labs.dev/gosx/auth"
)

// githubEnv and googleEnv name the environment variables that configure each
// OAuth provider. A provider is built only when both variables in its pair
// are set; the login page shows only what Providers built.
const (
	envGitHubClientID     = "AUTH_GITHUB_CLIENT_ID"
	envGitHubClientSecret = "AUTH_GITHUB_CLIENT_SECRET"
	envGoogleClientID     = "AUTH_GOOGLE_CLIENT_ID"
	envGoogleClientSecret = "AUTH_GOOGLE_CLIENT_SECRET"
)

// Providers builds every OAuth provider whose pair of environment variables
// is set, wrapped so a successful callback only ever signs in an
// allowlisted (or previously provisioned) organizer. publicBase is the
// absolute origin (PUBLIC_URL) each provider's redirect URL is built from:
// "{publicBase}/auth/oauth/{name}/callback".
func Providers(publicBase string) []auth.OAuthProvider {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	var providers []auth.OAuthProvider
	if id, secret, ok := githubCredentials(); ok {
		provider := auth.GitHubProvider(id, secret, base+"/auth/oauth/github/callback")
		provider.Resolver = allowlistResolver(provider.Resolver)
		providers = append(providers, provider)
	}
	if id, secret, ok := googleCredentials(); ok {
		provider := auth.GoogleProvider(id, secret, base+"/auth/oauth/google/callback")
		provider.Resolver = allowlistResolver(googleUserInfoResolver())
		providers = append(providers, provider)
	}
	return providers
}

// ConfiguredProviderNames reports which providers Providers would build, in
// mount order, without building the full provider (and its resolver
// closures). The login page uses this to show only real sign-in options.
func ConfiguredProviderNames() []string {
	var names []string
	if _, _, ok := githubCredentials(); ok {
		names = append(names, "github")
	}
	if _, _, ok := googleCredentials(); ok {
		names = append(names, "google")
	}
	return names
}

func githubCredentials() (id, secret string, ok bool) {
	id = strings.TrimSpace(os.Getenv(envGitHubClientID))
	secret = strings.TrimSpace(os.Getenv(envGitHubClientSecret))
	return id, secret, id != "" && secret != ""
}

func googleCredentials() (id, secret string, ok bool) {
	id = strings.TrimSpace(os.Getenv(envGoogleClientID))
	secret = strings.TrimSpace(os.Getenv(envGoogleClientSecret))
	return id, secret, id != "" && secret != ""
}

// allowlistResolver wraps a provider's own resolver so a verified identity
// only ever becomes a session when GrantRoles approves it. inner is never
// nil here: GitHub always ships a built-in resolver, and Providers gives
// Google googleUserInfoResolver in its place.
func allowlistResolver(inner auth.OAuthUserResolver) auth.OAuthUserResolver {
	return auth.OAuthUserResolverFunc(func(ctx context.Context, provider auth.OAuthProvider, client *http.Client, token auth.OAuthToken) (auth.User, error) {
		user, err := inner.ResolveOAuthUser(ctx, provider, client, token)
		if err != nil {
			return auth.User{}, err
		}
		if strings.EqualFold(strings.TrimSpace(provider.Name), "github") {
			if err := requireGitHubVerifiedEmail(ctx, client, token.AccessToken, user.Email); err != nil {
				return auth.User{}, err
			}
		}
		return GrantRoles(ctx, user)
	})
}

func requireGitHubVerifiedEmail(ctx context.Context, client *http.Client, accessToken, email string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("github email verification failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("github email verification failed with status %d", res.StatusCode)
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64*1024)).Decode(&emails); err != nil {
		return fmt.Errorf("github email verification response: %w", err)
	}
	for _, candidate := range emails {
		if candidate.Primary && candidate.Verified && strings.EqualFold(strings.TrimSpace(candidate.Email), strings.TrimSpace(email)) {
			return nil
		}
	}
	return fmt.Errorf("github account has no verified primary email")
}

// googleUserInfoResolver fetches the verified identity from Google's
// OpenID Connect userinfo endpoint. GoogleProvider ships no resolver of its
// own (auth.OAuthProvider.Resolver is nil by default for Google), so this
// is the "default path" the design calls for, written locally because the
// GoSX package's own default-path fetch is unexported.
func googleUserInfoResolver() auth.OAuthUserResolver {
	return auth.OAuthUserResolverFunc(func(ctx context.Context, provider auth.OAuthProvider, client *http.Client, token auth.OAuthToken) (auth.User, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.UserInfoURL, nil)
		if err != nil {
			return auth.User{}, err
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		req.Header.Set("Accept", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return auth.User{}, err
		}
		defer res.Body.Close()
		if res.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			return auth.User{}, fmt.Errorf("google userinfo failed: %s", strings.TrimSpace(string(body)))
		}
		var payload struct {
			Sub           string `json:"sub"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
			Picture       string `json:"picture"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			return auth.User{}, err
		}
		// Defense in depth (review m1): never trust an unverified Google email
		// for an allowlist match. A resolver error aborts the OAuth callback
		// before any sign-in, so an unverified identity never gets a session.
		if !payload.EmailVerified {
			return auth.User{}, fmt.Errorf("google account email is not verified")
		}
		meta := map[string]any{"provider": "google"}
		if payload.Picture != "" {
			meta["avatar"] = payload.Picture
		}
		return auth.User{
			ID:    payload.Sub,
			Email: strings.ToLower(strings.TrimSpace(payload.Email)),
			Name:  strings.TrimSpace(payload.Name),
			Meta:  meta,
		}, nil
	})
}
