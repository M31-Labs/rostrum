package archive

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestApprovedUploadZIPIsDeterministicAndIncludesOnlyApprovedFiles(t *testing.T) {
	uploads := t.TempDir()
	approvedPath := filepath.Join(uploads, "approved.pdf")
	pendingPath := filepath.Join(uploads, "pending.pdf")
	approvedBytes := []byte("%PDF-1.7 approved")
	if err := os.WriteFile(approvedPath, approvedBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingPath, []byte("%PDF-1.7 pending"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := domain.State{
		Event: domain.Event{ID: "evt_test"},
		TaskCompletions: []domain.TaskCompletion{
			{ID: "done_approved", TaskID: "task_slides", SpeakerID: "spk_a", Status: domain.TaskApproved, FileName: "final-slides.pdf", ContentType: "application/pdf", StoredPath: approvedPath},
			{ID: "done_pending", TaskID: "task_slides", SpeakerID: "spk_b", Status: domain.TaskSubmitted, FileName: "draft.pdf", ContentType: "application/pdf", StoredPath: pendingPath},
			{ID: "done_profile", TaskID: "task_profile", SpeakerID: "spk_c", Status: domain.TaskApproved},
		},
	}

	first, err := ApprovedUploadZIP(state, uploads)
	if err != nil {
		t.Fatalf("first ApprovedUploadZIP: %v", err)
	}
	second, err := ApprovedUploadZIP(state, uploads)
	if err != nil {
		t.Fatalf("second ApprovedUploadZIP: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("same approved files produced different ZIP bytes")
	}

	reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatalf("read ZIP: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("ZIP entries = %d, want manifest plus one approved file", len(reader.File))
	}
	if reader.File[0].Name != "manifest.json" || reader.File[1].Name != "files/done_approved-final-slides.pdf" {
		t.Fatalf("ZIP entries = %q, %q", reader.File[0].Name, reader.File[1].Name)
	}
	manifestFile, err := reader.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer manifestFile.Close()
	var manifest ApprovedUploadManifest
	if err := json.NewDecoder(manifestFile).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.EventID != "evt_test" || len(manifest.Files) != 1 {
		t.Fatalf("manifest = %#v, want one evt_test file", manifest)
	}
	sum := sha256.Sum256(approvedBytes)
	if manifest.Files[0].SHA256 != hex.EncodeToString(sum[:]) || manifest.Files[0].Bytes != int64(len(approvedBytes)) {
		t.Fatalf("manifest checksum = %+v, want approved file hash and bytes", manifest.Files[0])
	}
}

func TestBuildApprovedUploadBundleRejectsPathEscapeAndSymlink(t *testing.T) {
	uploads := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := domain.State{TaskCompletions: []domain.TaskCompletion{{
		ID: "done_escape", Status: domain.TaskApproved, StoredPath: outside,
	}}}
	if _, err := BuildApprovedUploadBundle(state, uploads); err == nil {
		t.Fatal("path outside upload root was accepted")
	}

	link := filepath.Join(uploads, "link.pdf")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}
	state.TaskCompletions[0].StoredPath = link
	if _, err := BuildApprovedUploadBundle(state, uploads); err == nil {
		t.Fatal("symlink upload was accepted")
	}
}
