package identity

import (
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/auth"
)

func TestDurableWebAuthnStoreRejectsDuplicateIDsAtomically(t *testing.T) {
	for _, driver := range []string{"json", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			workspace, err := store.OpenConfigured(driver, filepath.Join(t.TempDir(), "workspace"), "", domain.EmptyState(time.Now()))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workspace.Close() })
			appstate.Set(workspace)
			durable := DurableWebAuthnStore{}
			results := make(chan error, 8)
			var workers sync.WaitGroup
			for i := range 8 {
				workers.Go(func() {
					results <- durable.SaveCredential(auth.WebAuthnCredential{
						ID: "shared-id", User: auth.User{ID: string(rune('a' + i))},
						PublicKey: []byte{byte(i)}, Algorithm: -7,
					})
				})
			}
			workers.Wait()
			close(results)
			saved := 0
			for err := range results {
				if err == nil {
					saved++
				} else if !errors.Is(err, auth.ErrWebAuthnCredentialExists) {
					t.Fatalf("SaveCredential: %v", err)
				}
			}
			if saved != 1 || len(workspace.Snapshot().AuthPasskeys) != 1 {
				t.Fatalf("successful saves = %d, want exactly one", saved)
			}
			before, err := durable.Credential("shared-id")
			if err != nil {
				t.Fatal(err)
			}
			if err := durable.SaveCredential(auth.WebAuthnCredential{ID: before.ID, User: auth.User{ID: "replacement"}, PublicKey: []byte("replacement")}); !errors.Is(err, auth.ErrWebAuthnCredentialExists) {
				t.Fatalf("duplicate save = %v", err)
			}
			after, err := durable.Credential(before.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("duplicate save changed the original credential")
			}
			usedAt := time.Now().UTC()
			if err := durable.UpdateCounter(before.ID, 3, usedAt); err != nil {
				t.Fatal(err)
			}
			after, err = durable.Credential(before.ID)
			if err != nil || after.SignCount != 3 || !after.LastUsedAt.Equal(usedAt) || !reflect.DeepEqual(after.User, before.User) {
				t.Fatal("counter update did not preserve credential ownership")
			}
		})
	}
}
