package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/mail"
	"m31labs.dev/gosx/auth"
)

// MailSender adapts internal/mail.Sender to auth.MagicLinkSender, so the
// magic-link flow sends through the same transport (local outbox or SMTP)
// every other Rostrum email already uses.
type MailSender struct {
	Sender mail.Sender
}

// SendMagicLink implements auth.MagicLinkSender.
func (s MailSender) SendMagicLink(_ context.Context, delivery auth.MagicLinkDelivery) error {
	if s.Sender == nil {
		return fmt.Errorf("identity: magic link sender requires a mail.Sender")
	}
	return s.Sender.Send(mail.Message{
		To:       delivery.Email,
		ToName:   delivery.User.Name,
		Subject:  "Your Rostrum sign-in link",
		TextBody: signInBody(delivery.URL, delivery.ExpiresAt),
	})
}

func signInBody(url string, expiresAt time.Time) string {
	var body strings.Builder
	body.WriteString("Use this link to sign in to Rostrum:\n\n")
	body.WriteString(url)
	body.WriteString("\n\n")
	fmt.Fprintf(&body, "The link expires at %s and works once.\n\n", expiresAt.UTC().Format(time.RFC1123))
	body.WriteString("If you did not request this link, ignore this message.\n")
	return body.String()
}

// DurableMagicLinkStore persists issued magic-link tokens in the workspace
// JSON store, so a token survives a process restart. Save and Consume never
// store the raw token, only its SHA-256 hex digest: a leaked backup or log
// line never hands out a working sign-in link, the same guarantee setup.go
// gives the break-glass token.
type DurableMagicLinkStore struct{}

// Save implements auth.MagicLinkStore.
func (DurableMagicLinkStore) Save(token auth.MagicLinkToken) error {
	store, err := appstate.Get()
	if err != nil {
		return err
	}
	userJSON, err := json.Marshal(token.User)
	if err != nil {
		return err
	}
	digest := hashToken(token.Token)
	now := time.Now().UTC()
	return store.Update(func(state *domain.State) error {
		state.AuthMagicLinks = pruneExpiredLinks(state.AuthMagicLinks, now)
		state.AuthMagicLinks = append(state.AuthMagicLinks, domain.AuthMagicLink{
			Token:     digest,
			Email:     token.Email,
			UserJSON:  string(userJSON),
			Next:      token.Next,
			ExpiresAt: token.ExpiresAt,
		})
		return nil
	})
}

// Consume implements auth.MagicLinkStore. It deletes the matching entry
// whether or not it has expired, so a token never verifies twice — one-time
// use holds even for an expired link a caller retries.
func (DurableMagicLinkStore) Consume(rawToken string, now time.Time) (auth.MagicLinkToken, error) {
	store, err := appstate.Get()
	if err != nil {
		return auth.MagicLinkToken{}, auth.ErrMagicLinkInvalid
	}
	digest := hashToken(rawToken)
	var found *domain.AuthMagicLink
	updateErr := store.Update(func(state *domain.State) error {
		kept := make([]domain.AuthMagicLink, 0, len(state.AuthMagicLinks))
		for _, link := range state.AuthMagicLinks {
			if link.Token == digest && found == nil {
				captured := link
				found = &captured
				continue
			}
			if link.ExpiresAt.Before(now) {
				continue
			}
			kept = append(kept, link)
		}
		state.AuthMagicLinks = kept
		return nil
	})
	if updateErr != nil {
		return auth.MagicLinkToken{}, updateErr
	}
	if found == nil {
		return auth.MagicLinkToken{}, auth.ErrMagicLinkInvalid
	}
	if !found.ExpiresAt.IsZero() && now.After(found.ExpiresAt) {
		return auth.MagicLinkToken{}, auth.ErrMagicLinkExpired
	}
	var user auth.User
	if err := json.Unmarshal([]byte(found.UserJSON), &user); err != nil {
		return auth.MagicLinkToken{}, auth.ErrMagicLinkInvalid
	}
	return auth.MagicLinkToken{
		Token:     rawToken,
		Email:     found.Email,
		User:      user,
		Next:      found.Next,
		ExpiresAt: found.ExpiresAt,
	}, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func pruneExpiredLinks(links []domain.AuthMagicLink, now time.Time) []domain.AuthMagicLink {
	kept := make([]domain.AuthMagicLink, 0, len(links))
	for _, link := range links {
		if link.ExpiresAt.IsZero() || link.ExpiresAt.After(now) {
			kept = append(kept, link)
		}
	}
	return kept
}
