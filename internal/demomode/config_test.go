package demomode

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func validDemoConfig(t *testing.T) Config {
	t.Helper()
	for _, key := range append([]string{"APP_MODE", "SEED", "DEMO_MODE", "STORE_DRIVER", "DATA_PATH", "ROSTRUM_VERSION", "DATABASE_URL", "ORGANIZER_EMAILS", "RESET_SECRET", "MAIL_DRIVER"}, forbiddenCredentialEnv...) {
		t.Setenv(key, "")
	}
	t.Setenv("MAIL_DRIVER", "outbox")
	return Config{
		Mode:           ModeDemo,
		Seed:           "demo",
		LegacyDemoMode: "false",
		StoreDriver:    "sqlite",
		DataPath:       filepath.Join(t.TempDir(), "demo.sqlite"),
		RostrumVersion: "2026.08.11-cc32b9b",
	}
}

func TestValidateDemoRequiresExplicitSafeDeployment(t *testing.T) {
	if err := Validate(validDemoConfig(t)); err != nil {
		t.Fatalf("valid demo config rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "seed", mutate: func(config *Config) { config.Seed = "fresh" }, want: "SEED=demo"},
		{name: "memory legacy mode", mutate: func(config *Config) { config.LegacyDemoMode = "memory" }, want: "durable"},
		{name: "unsupported driver", mutate: func(config *Config) { config.StoreDriver = "postgres" }, want: "json or sqlite"},
		{name: "relative path", mutate: func(config *Config) { config.DataPath = "data/demo.sqlite" }, want: "absolute"},
		{name: "memory path", mutate: func(config *Config) { config.DataPath = ":memory:" }, want: "absolute"},
		{name: "development version", mutate: func(config *Config) { config.RostrumVersion = "dev" }, want: "immutable"},
		{name: "database url", mutate: func(config *Config) { t.Setenv("DATABASE_URL", "postgres://example") }, want: "DATABASE_URL"},
		{name: "organizer allowlist", mutate: func(config *Config) { t.Setenv("ORGANIZER_EMAILS", "owner@example.com") }, want: "ORGANIZER_EMAILS"},
		{name: "reset secret", mutate: func(config *Config) { t.Setenv("RESET_SECRET", "secret") }, want: "RESET_SECRET"},
		{name: "network mail driver", mutate: func(config *Config) { t.Setenv("MAIL_DRIVER", "smtp") }, want: "outbox"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			config := validDemoConfig(t)
			test.mutate(&config)
			err := Validate(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsDemoCredentials(t *testing.T) {
	for _, key := range forbiddenCredentialEnv {
		t.Run(key, func(t *testing.T) {
			config := validDemoConfig(t)
			t.Setenv(key, "configured")
			if err := Validate(config); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Validate() = %v, want %s refusal", err, key)
			}
		})
	}
}

func TestValidateStateRequiresFictionalIdentityFreeSeed(t *testing.T) {
	seed := domain.Seed(time.Now().UTC())
	if err := ValidateState(seed, seed); err != nil {
		t.Fatalf("seed rejected: %v", err)
	}

	wrongEvent := seed
	wrongEvent.Event.Name = "Real program"
	if err := ValidateState(wrongEvent, seed); err == nil {
		t.Fatal("real event identity accepted as demo state")
	}

	withPrincipal := seed
	withPrincipal.Principals = []domain.Principal{{ID: "principal_real", Email: "owner@example.com"}}
	if err := ValidateState(withPrincipal, seed); err == nil {
		t.Fatal("organizer identity accepted as demo state")
	}
}
