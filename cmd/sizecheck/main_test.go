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
	t.Setenv("INITIAL_WORKSPACE_PATH", "deployment-template")
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
		"DATA_PATH": ":memory:", "INITIAL_WORKSPACE": "fresh", "PORT": "1234",
		"PUBLIC_URL": "http://127.0.0.1:1234",
	} {
		if environment[key] != want {
			t.Errorf("probe %s = %q, want %q", key, environment[key], want)
		}
	}
	for _, key := range []string{"APP_MODE", "INITIAL_WORKSPACE_PATH"} {
		if _, found := environment[key]; found {
			t.Errorf("probe inherited deployment setting %s", key)
		}
	}
}
