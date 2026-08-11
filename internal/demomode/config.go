// Package demomode owns the fail-closed contract for Rostrum's hosted,
// read-only demonstration deployment. A normal self-host remains live and
// interactive; APP_MODE=demo is an explicit deployment choice that may only
// serve the fictional seed through a mutation-proof store.
package demomode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
)

const (
	ModeLive = "live"
	ModeDemo = "demo"
)

// Config contains the startup values that must be checked before a demo
// process opens its workspace. Paths are passed in after the main program has
// applied its normal defaults, so a relative or in-memory path cannot hide
// behind an omitted environment variable.
type Config struct {
	Mode           string
	Seed           string
	LegacyDemoMode string
	StoreDriver    string
	DataPath       string
	RostrumVersion string
}

// Enabled reports whether this process is explicitly configured as the
// hosted, read-only demo. An omitted or unknown mode is not silently treated
// as demo; startup validation rejects unknown values instead.
func Enabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_MODE")), ModeDemo)
}

// Validate enforces the demo deployment boundary before any workspace is
// opened. The demo must use the fictional seed, a durable local store, an
// immutable release identity, and no integration or identity credentials.
func Validate(config Config) error {
	mode := strings.ToLower(strings.TrimSpace(config.Mode))
	if mode == "" {
		mode = ModeLive
	}
	if mode != ModeLive && mode != ModeDemo {
		return fmt.Errorf("APP_MODE must be live or demo (got %q)", config.Mode)
	}
	if mode != ModeDemo {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(config.Seed), "demo") {
		return fmt.Errorf("APP_MODE=demo requires SEED=demo")
	}
	if strings.EqualFold(strings.TrimSpace(config.LegacyDemoMode), "memory") {
		return fmt.Errorf("APP_MODE=demo requires durable storage; DEMO_MODE=memory is not allowed")
	}
	driver := strings.ToLower(strings.TrimSpace(config.StoreDriver))
	if driver == "" {
		driver = "json"
	}
	if driver != "json" && driver != "sqlite" {
		return fmt.Errorf("APP_MODE=demo requires STORE_DRIVER=json or sqlite, not %q", config.StoreDriver)
	}
	dataPath := strings.TrimSpace(config.DataPath)
	if dataPath == "" || dataPath == ":memory:" || !filepath.IsAbs(dataPath) {
		return fmt.Errorf("APP_MODE=demo requires an absolute durable DATA_PATH")
	}
	version := strings.TrimSpace(config.RostrumVersion)
	if version == "" || strings.EqualFold(version, "dev") {
		return fmt.Errorf("APP_MODE=demo requires an immutable ROSTRUM_VERSION")
	}
	if value := strings.TrimSpace(os.Getenv("ORGANIZER_EMAILS")); value != "" {
		return fmt.Errorf("APP_MODE=demo requires ORGANIZER_EMAILS to be empty")
	}
	if value := strings.TrimSpace(os.Getenv("RESET_SECRET")); value != "" {
		return fmt.Errorf("APP_MODE=demo must not configure RESET_SECRET")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) != "" {
		return fmt.Errorf("APP_MODE=demo must not configure DATABASE_URL")
	}
	mailDriver := strings.ToLower(strings.TrimSpace(os.Getenv("MAIL_DRIVER")))
	switch mailDriver {
	case "", "auto", "outbox", "demo-outbox", "fake":
		// The demo may retain the normal outbox defaults, but never select a
		// network transport even if a deployment accidentally supplies one.
	default:
		return fmt.Errorf("APP_MODE=demo requires an outbox mail driver, not %q", mailDriver)
	}
	for _, key := range forbiddenCredentialEnv {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return fmt.Errorf("APP_MODE=demo refuses credential or external integration %s", key)
		}
	}
	return nil
}

var forbiddenCredentialEnv = []string{
	"RESEND_API_KEY",
	"SMTP_HOST",
	"SMTP_USER",
	"SMTP_PASSWORD",
	"ACCELEVENTS_API_KEY",
	"AIRTABLE_PAT",
	"AUTH_GITHUB_CLIENT_ID",
	"AUTH_GITHUB_CLIENT_SECRET",
	"AUTH_GOOGLE_CLIENT_ID",
	"AUTH_GOOGLE_CLIENT_SECRET",
	"GOSX_STATIC_EXPORT",
}

// ValidateState prevents APP_MODE=demo from being pointed at a real workspace
// that happens to use the same storage driver. The event identity is the
// seeded demo's stable fingerprint; organizer principals, magic links, and
// passkeys must also be absent so no real identity can cross the boundary.
func ValidateState(state, seed domain.State) error {
	if state.Event.ID != seed.Event.ID || state.Event.Slug != seed.Event.Slug || state.Event.Name != seed.Event.Name {
		return fmt.Errorf("APP_MODE=demo requires the fictional seeded workspace")
	}
	if len(state.Principals) != 0 || len(state.AuthMagicLinks) != 0 || len(state.AuthPasskeys) != 0 {
		return fmt.Errorf("APP_MODE=demo workspace contains organizer identity state")
	}
	return nil
}
