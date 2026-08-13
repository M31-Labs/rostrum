package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestPrepareProducesValidatedPinnedWorkspaceAndUploads(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	uploads := filepath.Join(root, "uploads")
	storedUploads := filepath.Join(root, "runtime-uploads")
	workspace := filepath.Join(root, "workspace.json")
	checksum := filepath.Join(root, "workspace.sha256")
	uploadChecksums := filepath.Join(root, "uploads.sha256")
	if err := os.MkdirAll(assets, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, portrait := range portraits {
		if err := os.WriteFile(filepath.Join(assets, portrait.asset), []byte("webp:"+portrait.asset), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if err := prepare(workspace, checksum, uploadChecksums, assets, uploads, storedUploads); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	raw, err := os.ReadFile(workspace)
	if err != nil {
		t.Fatal(err)
	}
	var state domain.State
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("prepared state is invalid: %v", err)
	}
	digest := sha256.Sum256(raw)
	wantChecksum := hex.EncodeToString(digest[:]) + "\n"
	gotChecksum, err := os.ReadFile(checksum)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotChecksum) != wantChecksum {
		t.Fatalf("checksum = %q, want %q", gotChecksum, wantChecksum)
	}
	var wantUploadChecksums strings.Builder
	for _, portrait := range portraits {
		portraitBytes := []byte("webp:" + portrait.asset)
		portraitDigest := sha256.Sum256(portraitBytes)
		fmt.Fprintf(&wantUploadChecksums, "%s  %s\n", hex.EncodeToString(portraitDigest[:]), portrait.asset)
		if info, err := os.Stat(filepath.Join(uploads, portrait.asset)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("prepared upload %s: info=%v err=%v", portrait.asset, info, err)
		}
		found := false
		for _, completion := range state.TaskCompletions {
			if completion.ID != portrait.completionID {
				continue
			}
			found = true
			wantPath := filepath.ToSlash(filepath.Join(storedUploads, portrait.asset))
			if completion.StoredPath != wantPath || completion.ContentType != "image/webp" {
				t.Fatalf("completion %s = %+v, want stored path %s", completion.ID, completion, wantPath)
			}
		}
		if !found {
			t.Fatalf("missing completion %s", portrait.completionID)
		}
	}
	gotUploadChecksums, err := os.ReadFile(uploadChecksums)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotUploadChecksums) != wantUploadChecksums.String() {
		t.Fatalf("upload checksums = %q, want %q", gotUploadChecksums, wantUploadChecksums.String())
	}
}

func TestPrepareRequiresAbsolutePaths(t *testing.T) {
	if err := prepare("workspace.json", "/tmp/checksum", "/tmp/uploads.sha256", "/tmp/assets", "/tmp/uploads", ""); err == nil {
		t.Fatal("relative workspace path accepted")
	}
}

func TestCopyRegularFileRefusesToReplaceExistingUpload(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.webp")
	destination := filepath.Join(root, "destination.webp")
	if err := os.WriteFile(source, []byte("new portrait"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("existing portrait"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := copyRegularFile(source, destination); err == nil {
		t.Fatal("existing upload was replaced")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing portrait" {
		t.Fatalf("existing upload changed to %q", got)
	}
}

func TestWriteExclusiveRefusesToReplaceOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, []byte("existing workspace"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(path, []byte("replacement"), 0o640); err == nil {
		t.Fatal("existing output was replaced")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing workspace" {
		t.Fatalf("existing output changed to %q", got)
	}
}
