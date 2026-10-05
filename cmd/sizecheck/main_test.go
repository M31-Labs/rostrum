package main

import (
	"strings"
	"testing"
)

func TestReleaseProbeUsesIsolatedDevelopmentEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_MODE", "preview")
	t.Setenv("GOSX_STATIC_EXPORT", "0")
	t.Setenv("MAIL_DRIVER", "smtp")
	t.Setenv("DATA_PATH", "deployment-state")
	t.Setenv("STORE_DRIVER", "postgres")
	t.Setenv("INITIAL_WORKSPACE_PATH", "deployment-template")
	stripped := []string{
		"APP_MODE", "DATABASE_URL", "AUDIT_LOG_PATH", "BACKUP_DIR", "UPLOAD_DIR",
		"PRINCIPAL_ROLES", "ORGANIZER_EMAILS", "RESET_SECRET", "SESSION_SECRET", "TRUSTED_PROXY_CIDRS",
		"INITIAL_WORKSPACE_PATH", "INITIAL_WORKSPACE_SHA256", "INITIAL_WORKSPACE_SHA256_FILE",
		"CFP_ROUTING_POLICY_PATH", "CFP_ROUTING_POLICY_SHA256", "CFP_ROUTING_POLICY_SHA256_FILE",
	}
	for _, key := range stripped {
		t.Setenv(key, "deployment-setting")
	}
	environment := make(map[string]string)
	for _, entry := range releaseServerEnvironment(t.TempDir(), "http://127.0.0.1:1234", 1234) {
		key, value, _ := strings.Cut(entry, "=")
		if _, duplicate := environment[key]; duplicate {
			t.Fatalf("duplicate probe setting %s", key)
		}
		environment[key] = value
	}
	for key, want := range map[string]string{
		"APP_ENV": "development", "GOSX_STATIC_EXPORT": "1", "MAIL_DRIVER": "outbox",
		"STORE_DRIVER": "json", "DATA_PATH": ":memory:", "INITIAL_WORKSPACE": "fresh", "PORT": "1234",
		"PUBLIC_URL": "http://127.0.0.1:1234",
	} {
		if environment[key] != want {
			t.Errorf("probe %s = %q, want %q", key, environment[key], want)
		}
	}
	for _, key := range stripped {
		if _, found := environment[key]; found {
			t.Errorf("probe inherited deployment setting %s", key)
		}
	}
}
