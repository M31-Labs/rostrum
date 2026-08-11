package store

import (
	"errors"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestReadOnlyRejectsEveryMutation(t *testing.T) {
	seed := domain.Seed(time.Now().UTC())
	base, err := Open(":memory:", seed)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer base.Close()
	readOnly := ReadOnly(base)

	meta := domain.AuditMeta{Actor: "test", Action: "test.mutation", EntityType: "workspace", Summary: "test"}
	checks := []struct {
		name string
		call func() error
	}{
		{name: "update", call: func() error { return readOnly.Update(func(*domain.State) error { return nil }) }},
		{name: "update audit", call: func() error { return readOnly.UpdateAudit(meta, func(*domain.State) error { return nil }) }},
		{name: "reset", call: func() error { return readOnly.Reset() }},
		{name: "replace", call: func() error { return readOnly.Replace(seed, meta) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, ErrReadOnlyDemo) {
				t.Fatalf("error = %v, want ErrReadOnlyDemo", err)
			}
		})
	}

	if got := readOnly.Snapshot(); got.Event.ID != seed.Event.ID || len(got.AuditEvents) != 0 {
		t.Fatalf("read-only snapshot changed after rejected mutations: event=%q audit=%d", got.Event.ID, len(got.AuditEvents))
	}
	if got := readOnly.Path(); got != base.Path() {
		t.Fatalf("Path() = %q, want delegated path %q", got, base.Path())
	}
}
