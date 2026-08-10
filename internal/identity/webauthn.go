package identity

import (
	"encoding/json"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"m31labs.dev/gosx/auth"
)

// DurableWebAuthnStore persists registered passkeys in the workspace JSON
// store, so a credential survives a process restart. It implements
// auth.WebAuthnStore.
type DurableWebAuthnStore struct{}

// SaveCredential implements auth.WebAuthnStore.
func (DurableWebAuthnStore) SaveCredential(credential auth.WebAuthnCredential) error {
	store, err := appstate.Get()
	if err != nil {
		return err
	}
	userJSON, err := json.Marshal(credential.User)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	createdAt := credential.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	lastUsedAt := credential.LastUsedAt
	if lastUsedAt.IsZero() {
		lastUsedAt = now
	}
	return store.Update(func(state *domain.State) error {
		for index := range state.AuthPasskeys {
			if state.AuthPasskeys[index].ID != credential.ID {
				continue
			}
			state.AuthPasskeys[index].UserJSON = string(userJSON)
			state.AuthPasskeys[index].PublicKey = credential.PublicKey
			state.AuthPasskeys[index].Algorithm = credential.Algorithm
			state.AuthPasskeys[index].SignCount = credential.SignCount
			state.AuthPasskeys[index].Transports = credential.Transports
			state.AuthPasskeys[index].LastUsedAt = lastUsedAt
			return nil
		}
		state.AuthPasskeys = append(state.AuthPasskeys, domain.AuthPasskey{
			ID:         credential.ID,
			UserJSON:   string(userJSON),
			PublicKey:  credential.PublicKey,
			Algorithm:  credential.Algorithm,
			SignCount:  credential.SignCount,
			Transports: credential.Transports,
			CreatedAt:  createdAt,
			LastUsedAt: lastUsedAt,
		})
		return nil
	})
}

// Credential implements auth.WebAuthnStore.
func (DurableWebAuthnStore) Credential(id string) (auth.WebAuthnCredential, error) {
	store, err := appstate.Get()
	if err != nil {
		return auth.WebAuthnCredential{}, auth.ErrWebAuthnCredentialNotFound
	}
	for _, passkey := range store.Snapshot().AuthPasskeys {
		if passkey.ID != id {
			continue
		}
		return toWebAuthnCredential(passkey)
	}
	return auth.WebAuthnCredential{}, auth.ErrWebAuthnCredentialNotFound
}

// Credentials implements auth.WebAuthnStore. AuthPasskey has no indexed
// user column, so this decodes each stored UserJSON to filter by user ID —
// acceptable at the scale of one organization's registered devices.
func (DurableWebAuthnStore) Credentials(userID string) ([]auth.WebAuthnCredential, error) {
	store, err := appstate.Get()
	if err != nil {
		return nil, err
	}
	passkeys := store.Snapshot().AuthPasskeys
	out := make([]auth.WebAuthnCredential, 0, len(passkeys))
	for _, passkey := range passkeys {
		credential, err := toWebAuthnCredential(passkey)
		if err != nil {
			continue
		}
		if credential.User.ID == userID {
			out = append(out, credential)
		}
	}
	return out, nil
}

// UpdateCounter implements auth.WebAuthnStore.
func (DurableWebAuthnStore) UpdateCounter(id string, signCount uint32, usedAt time.Time) error {
	store, err := appstate.Get()
	if err != nil {
		return err
	}
	return store.Update(func(state *domain.State) error {
		for index := range state.AuthPasskeys {
			if state.AuthPasskeys[index].ID != id {
				continue
			}
			state.AuthPasskeys[index].SignCount = signCount
			state.AuthPasskeys[index].LastUsedAt = usedAt
			return nil
		}
		return auth.ErrWebAuthnCredentialNotFound
	})
}

func toWebAuthnCredential(passkey domain.AuthPasskey) (auth.WebAuthnCredential, error) {
	var user auth.User
	if err := json.Unmarshal([]byte(passkey.UserJSON), &user); err != nil {
		return auth.WebAuthnCredential{}, err
	}
	return auth.WebAuthnCredential{
		ID:         passkey.ID,
		User:       user,
		PublicKey:  passkey.PublicKey,
		Algorithm:  passkey.Algorithm,
		SignCount:  passkey.SignCount,
		Transports: passkey.Transports,
		CreatedAt:  passkey.CreatedAt,
		LastUsedAt: passkey.LastUsedAt,
	}, nil
}
