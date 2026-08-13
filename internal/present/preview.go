package present

import (
	"os"
	"strings"
)

const (
	defaultPreviewLabel   = "Read-only preview"
	defaultPreviewMessage = "Explore this workspace safely. Controls that create, move, publish, upload, or save are unavailable."
)

// ReadOnlyPreviewMode reports Rostrum's explicit hosted-preview posture.
func ReadOnlyPreviewMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_MODE")), "preview")
}

// PreviewLabel and PreviewMessage let an operator describe a hosted preview
// without embedding workspace-specific copy in the core module.
func PreviewLabel() string {
	if value := strings.TrimSpace(os.Getenv("PREVIEW_LABEL")); value != "" {
		return value
	}
	return defaultPreviewLabel
}

func PreviewMessage() string {
	if value := strings.TrimSpace(os.Getenv("PREVIEW_MESSAGE")); value != "" {
		return value
	}
	return defaultPreviewMessage
}
