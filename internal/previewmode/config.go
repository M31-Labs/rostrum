// Package previewmode owns the fail-closed contract for a hosted, anonymous,
// read-only Rostrum preview. Preview mode is data-agnostic: operators provide
// an identity-free workspace template and pin its exact bytes at deployment.
package previewmode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
)

const (
	ModeLive    = "live"
	ModePreview = "preview"
)

// Config contains the startup values that must be checked before a preview
// process opens its workspace. TemplatePath and DataPath are resolved by the
// caller before validation, so relative paths cannot obscure the deployment
// boundary.
type Config struct {
	Mode           string
	TemplatePath   string
	ExpectedSHA256 string
	ChecksumFile   string
	StoreDriver    string
	DataPath       string
	RostrumVersion string
}

// Enabled reports whether this process is explicitly configured as a
// read-only preview. Startup validation rejects every unknown APP_MODE.
func Enabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_MODE")), ModePreview)
}

// Validate enforces the preview deployment boundary before a workspace is
// opened. A preview uses a checksummed template, durable local storage, an
// immutable release identity, and no organizer or external credentials.
func Validate(config Config) error {
	mode := strings.ToLower(strings.TrimSpace(config.Mode))
	if mode == "" {
		mode = ModeLive
	}
	if mode != ModeLive && mode != ModePreview {
		return fmt.Errorf("APP_MODE must be live or preview (got %q)", config.Mode)
	}
	if mode != ModePreview {
		return nil
	}

	templatePath := strings.TrimSpace(config.TemplatePath)
	if templatePath == "" {
		return fmt.Errorf("APP_MODE=preview requires INITIAL_WORKSPACE_PATH")
	}
	if !filepath.IsAbs(templatePath) {
		return fmt.Errorf("APP_MODE=preview requires an absolute resolved INITIAL_WORKSPACE_PATH")
	}
	if checksumFile := strings.TrimSpace(config.ChecksumFile); checksumFile != "" && !filepath.IsAbs(checksumFile) {
		return fmt.Errorf("APP_MODE=preview requires an absolute resolved INITIAL_WORKSPACE_SHA256_FILE")
	}
	if _, err := ParseSHA256(config.ExpectedSHA256); err != nil {
		return fmt.Errorf("APP_MODE=preview requires INITIAL_WORKSPACE_SHA256: %w", err)
	}

	driver := strings.ToLower(strings.TrimSpace(config.StoreDriver))
	if driver == "" {
		driver = "json"
	}
	if driver != "json" && driver != "sqlite" {
		return fmt.Errorf("APP_MODE=preview requires STORE_DRIVER=json or sqlite, not %q", config.StoreDriver)
	}
	dataPath := strings.TrimSpace(config.DataPath)
	if dataPath == "" || dataPath == ":memory:" || !filepath.IsAbs(dataPath) {
		return fmt.Errorf("APP_MODE=preview requires an absolute durable DATA_PATH")
	}
	version := strings.TrimSpace(config.RostrumVersion)
	if version == "" || strings.EqualFold(version, "dev") {
		return fmt.Errorf("APP_MODE=preview requires an immutable ROSTRUM_VERSION")
	}
	if value := strings.TrimSpace(os.Getenv("ORGANIZER_EMAILS")); value != "" {
		return fmt.Errorf("APP_MODE=preview requires ORGANIZER_EMAILS to be empty")
	}
	if value := strings.TrimSpace(os.Getenv("RESET_SECRET")); value != "" {
		return fmt.Errorf("APP_MODE=preview must not configure RESET_SECRET")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) != "" {
		return fmt.Errorf("APP_MODE=preview must not configure DATABASE_URL")
	}
	mailDriver := strings.ToLower(strings.TrimSpace(os.Getenv("MAIL_DRIVER")))
	switch mailDriver {
	case "", "auto", "outbox", "fake":
		// Local outbox transports are safe because preview mode never starts the
		// delivery runner. Reject every explicitly networked transport.
	default:
		return fmt.Errorf("APP_MODE=preview requires an outbox mail driver, not %q", mailDriver)
	}
	for _, key := range forbiddenCredentialEnv {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return fmt.Errorf("APP_MODE=preview refuses credential or external integration %s", key)
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
	"ACCELEVENTS_API_TOKEN",
	"ACCELEVENTS_EVENT_URL",
	"ACCELEVENTS_BASE_URL",
	"AIRTABLE_PAT",
	"AIRTABLE_BASE_ID",
	"AIRTABLE_API_BASE_URL",
	"AIRTABLE_SPEAKERS_TABLE",
	"AIRTABLE_SESSIONS_TABLE",
	"AUTH_GITHUB_CLIENT_ID",
	"AUTH_GITHUB_CLIENT_SECRET",
	"AUTH_GITHUB_HANDLES",
	"AUTH_GOOGLE_CLIENT_ID",
	"AUTH_GOOGLE_CLIENT_SECRET",
	"PRINCIPAL_ROLES",
	"GOSX_STATIC_EXPORT",
}

// ValidateState prevents preview mode from serving a modified persisted
// workspace or organizer identity state. The complete canonical state must
// match the operator-supplied template that was already validated and pinned
// by its raw-byte checksum.
func ValidateState(state, template domain.State) error {
	if len(state.Principals) != 0 || len(state.AuthMagicLinks) != 0 || len(state.AuthPasskeys) != 0 {
		return fmt.Errorf("APP_MODE=preview workspace contains organizer identity state")
	}
	if hasNonReservedEmail(state) {
		return fmt.Errorf("APP_MODE=preview workspace contains a non-reserved email address; use fictional addresses under example.com, example.net, or example.org")
	}
	actual, err := StateFingerprint(state)
	if err != nil {
		return fmt.Errorf("fingerprint preview workspace: %w", err)
	}
	expected, err := StateFingerprint(template)
	if err != nil {
		return fmt.Errorf("fingerprint preview template: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("APP_MODE=preview requires the persisted workspace to match its pinned template")
	}
	return nil
}

var emailAddressPattern = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@([a-z0-9-]+\.)+[a-z]{2,63}`)

// hasNonReservedEmail keeps an accidentally imported real workspace from
// becoming an anonymous inspection surface. The check covers the complete
// encoded aggregate—including proposal values, notification recipients, and
// audit metadata—without logging the address it rejects.
func hasNonReservedEmail(state domain.State) bool {
	data, err := json.Marshal(state)
	if err != nil {
		return true
	}
	for _, address := range emailAddressPattern.FindAllString(string(data), -1) {
		at := strings.LastIndexByte(address, '@')
		if at < 0 || !reservedEmailDomain(address[at+1:]) {
			return true
		}
	}
	return false
}

func reservedEmailDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	for _, reserved := range []string{"example.com", "example.net", "example.org"} {
		if domain == reserved || strings.HasSuffix(domain, "."+reserved) {
			return true
		}
	}
	return false
}

// ParseSHA256 validates a full hexadecimal SHA-256 pin and returns its bytes.
func ParseSHA256(value string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	value = strings.TrimSpace(value)
	if value == "" {
		return digest, fmt.Errorf("checksum is empty")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return digest, fmt.Errorf("checksum must be exactly 64 hexadecimal characters")
	}
	copy(digest[:], decoded)
	return digest, nil
}

// StateFingerprint returns a stable content hash for a decoded workspace.
// It is used after persistence has normalized the original JSON document.
func StateFingerprint(state domain.State) (string, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
