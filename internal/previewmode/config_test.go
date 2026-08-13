package previewmode

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func validPreviewConfig(t *testing.T) Config {
	t.Helper()
	for _, key := range append([]string{"APP_MODE", "STORE_DRIVER", "DATA_PATH", "ROSTRUM_VERSION", "DATABASE_URL", "ORGANIZER_EMAILS", "RESET_SECRET", "MAIL_DRIVER"}, forbiddenCredentialEnv...) {
		t.Setenv(key, "")
	}
	t.Setenv("MAIL_DRIVER", "outbox")
	return Config{
		Mode:           ModePreview,
		TemplatePath:   filepath.Join(t.TempDir(), "workspace.json"),
		ExpectedSHA256: strings.Repeat("a", 64),
		StoreDriver:    "sqlite",
		DataPath:       filepath.Join(t.TempDir(), "preview.sqlite"),
		RostrumVersion: "2026.08.12-5f83a1d",
	}
}

func TestValidatePreviewRequiresExplicitSafeDeployment(t *testing.T) {
	if err := Validate(validPreviewConfig(t)); err != nil {
		t.Fatalf("valid preview config rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "unknown mode", mutate: func(config *Config) { config.Mode = "demo" }, want: "live or preview"},
		{name: "template path", mutate: func(config *Config) { config.TemplatePath = "" }, want: "INITIAL_WORKSPACE_PATH"},
		{name: "relative template path", mutate: func(config *Config) { config.TemplatePath = "workspace.json" }, want: "absolute resolved"},
		{name: "template checksum", mutate: func(config *Config) { config.ExpectedSHA256 = "abc" }, want: "64 hexadecimal"},
		{name: "relative checksum file", mutate: func(config *Config) { config.ChecksumFile = "workspace.sha256" }, want: "SHA256_FILE"},
		{name: "unsupported driver", mutate: func(config *Config) { config.StoreDriver = "postgres" }, want: "json or sqlite"},
		{name: "relative data path", mutate: func(config *Config) { config.DataPath = "data/preview.sqlite" }, want: "absolute"},
		{name: "memory data path", mutate: func(config *Config) { config.DataPath = ":memory:" }, want: "absolute"},
		{name: "development version", mutate: func(config *Config) { config.RostrumVersion = "dev" }, want: "immutable"},
		{name: "database url", mutate: func(config *Config) { t.Setenv("DATABASE_URL", "postgres://example") }, want: "DATABASE_URL"},
		{name: "organizer allowlist", mutate: func(config *Config) { t.Setenv("ORGANIZER_EMAILS", "owner@example.com") }, want: "ORGANIZER_EMAILS"},
		{name: "reset secret", mutate: func(config *Config) { t.Setenv("RESET_SECRET", "secret") }, want: "RESET_SECRET"},
		{name: "network mail driver", mutate: func(config *Config) { t.Setenv("MAIL_DRIVER", "smtp") }, want: "outbox"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			config := validPreviewConfig(t)
			test.mutate(&config)
			err := Validate(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsPreviewCredentials(t *testing.T) {
	for _, key := range forbiddenCredentialEnv {
		t.Run(key, func(t *testing.T) {
			config := validPreviewConfig(t)
			t.Setenv(key, "configured")
			if err := Validate(config); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Validate() = %v, want %s refusal", err, key)
			}
		})
	}
}

func TestValidateStateRequiresIdentityFreePinnedTemplate(t *testing.T) {
	template := domain.FreshState(time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC))
	if err := ValidateState(template, template); err != nil {
		t.Fatalf("template rejected: %v", err)
	}

	withPrincipal := template
	withPrincipal.Principals = []domain.Principal{{ID: "principal_real", Email: "owner@example.com"}}
	if err := ValidateState(withPrincipal, withPrincipal); err == nil {
		t.Fatal("organizer identity accepted as preview state")
	}

	modified := template
	modified.Event.Name = "Modified after deployment"
	if err := ValidateState(modified, template); err == nil {
		t.Fatal("modified persisted workspace accepted")
	}

	withRealAddress := template
	withRealAddress.Speakers = []domain.Speaker{{ID: "spk_owner", FirstName: "Real", LastName: "Person", Email: "owner@company.com"}}
	if err := ValidateState(withRealAddress, withRealAddress); err == nil || !strings.Contains(err.Error(), "non-reserved email") {
		t.Fatalf("real email preview error = %v", err)
	}

	withFictionalAddress := template
	withFictionalAddress.Speakers = []domain.Speaker{{ID: "spk_example", FirstName: "Example", LastName: "Person", Email: "person@subdomain.example.org"}}
	if err := ValidateState(withFictionalAddress, withFictionalAddress); err != nil {
		t.Fatalf("reserved example email rejected: %v", err)
	}
}

func TestParseSHA256(t *testing.T) {
	valid := strings.Repeat("A0", 32)
	if _, err := ParseSHA256(valid); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	for _, value := range []string{"", "abc", strings.Repeat("g", 64), strings.Repeat("a", 66)} {
		if _, err := ParseSHA256(value); err == nil {
			t.Fatalf("invalid checksum %q accepted", value)
		}
	}
}
