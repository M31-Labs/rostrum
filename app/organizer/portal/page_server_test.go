package portal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestStagedHeadshotCommitsOnlyWhenAsked(t *testing.T) {
	directory := t.TempDir()
	previous := resolvePublicHeadshotDir
	resolvePublicHeadshotDir = func() string { return directory }
	t.Cleanup(func() { resolvePublicHeadshotDir = previous })

	sourcePath := filepath.Join(t.TempDir(), "source.jpg")
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	stage, err := stageHeadshotCopy("spk_test", sourcePath, "portrait.jpg")
	if err != nil {
		t.Fatalf("stageHeadshotCopy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "spk_test.jpg")); !os.IsNotExist(err) {
		t.Fatalf("staged copy became public before Commit, stat err = %v", err)
	}
	if err := stage.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	published, err := os.ReadFile(filepath.Join(directory, "spk_test.jpg"))
	if err != nil {
		t.Fatalf("read published headshot: %v", err)
	}
	if !bytes.Equal(published, payload) {
		t.Fatalf("published bytes = %x, want %x", published, payload)
	}

	stage.Discard(true)
	if _, err := os.Stat(filepath.Join(directory, "spk_test.jpg")); !os.IsNotExist(err) {
		t.Fatalf("Discard(true) did not remove public image, stat err = %v", err)
	}
}

func TestOrganizerTaskInputRejectsInvalidTypeAndDueDate(t *testing.T) {
	_, fieldErrors := parseTaskInput(map[string]string{
		"title":  "Slides",
		"type":   "shell-command",
		"due_at": "not-a-date",
	}, domain.Event{TimeZone: "UTC"})
	if fieldErrors["type"] == "" || fieldErrors["due_at"] == "" {
		t.Fatalf("field errors = %#v, want type and due date failures", fieldErrors)
	}
}
