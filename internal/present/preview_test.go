package present

import "testing"

func TestPreviewPresentationModeAndCopy(t *testing.T) {
	t.Setenv("APP_MODE", "live")
	t.Setenv("PREVIEW_LABEL", "")
	t.Setenv("PREVIEW_MESSAGE", "")
	if ReadOnlyPreviewMode() {
		t.Fatal("live APP_MODE reported as read-only preview")
	}
	if PreviewLabel() != defaultPreviewLabel || PreviewMessage() != defaultPreviewMessage {
		t.Fatal("empty preview copy did not use calm defaults")
	}

	t.Setenv("APP_MODE", "preview")
	t.Setenv("PREVIEW_LABEL", "Evaluation workspace")
	t.Setenv("PREVIEW_MESSAGE", "Inspect every workflow without saving changes.")
	if !ReadOnlyPreviewMode() {
		t.Fatal("explicit preview mode was not reported")
	}
	if PreviewLabel() != "Evaluation workspace" || PreviewMessage() != "Inspect every workflow without saving changes." {
		t.Fatal("operator preview copy was not preserved")
	}
}
