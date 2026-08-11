package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/audit"
	"github.com/m31-labs/rostrum/internal/domain"
)

func TestEnvelopeRejectsTamperingAndPreservesCurrentIdentity(t *testing.T) {
	state := domain.Seed(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC))
	state.AuthMagicLinks = []domain.AuthMagicLink{{Token: "hashed", Email: "owner@example.com", ExpiresAt: time.Now().Add(time.Hour)}}
	data, err := Marshal(state)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	imported, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(imported.AuthMagicLinks) != 0 {
		t.Fatalf("exported magic links = %#v, want transient links stripped", imported.AuthMagicLinks)
	}

	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.State.Event.Name = "tampered"
	tampered, _ := json.Marshal(envelope)
	if _, err := Decode(tampered); err == nil {
		t.Fatal("Decode(tampered) = nil error, want checksum failure")
	}

	current := domain.Seed(time.Now().UTC())
	current.Principals = []domain.Principal{{ID: "principal_current", Email: "current@example.com"}}
	current.AuthPasskeys = []domain.AuthPasskey{{ID: "passkey_current"}}
	kept := PreserveCurrentIdentity(current, imported)
	if len(kept.Principals) != 1 || kept.Principals[0].ID != "principal_current" || len(kept.AuthPasskeys) != 1 {
		t.Fatalf("identity after import preparation = %#v, want current identity", kept)
	}
}

func TestWriteBackupRetainsNewestTen(t *testing.T) {
	directory := t.TempDir()
	state := domain.Seed(time.Now().UTC())
	for index := 0; index < BackupRetention+2; index++ {
		if _, err := writeBackupAt(directory, state, time.Date(2026, time.August, 1, 0, 0, index, 0, time.UTC)); err != nil {
			t.Fatalf("backup %d: %v", index, err)
		}
	}
	paths, err := filepath.Glob(filepath.Join(directory, "rostrum-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != BackupRetention {
		t.Fatalf("backups = %d, want %d", len(paths), BackupRetention)
	}
	if filepath.Base(paths[0]) != "rostrum-20260801T000002.000000000Z.json" {
		t.Fatalf("oldest retained backup = %s, want third backup", filepath.Base(paths[0]))
	}
}

func TestWriteTarGZIncludesWorkspaceUploadsAndAudit(t *testing.T) {
	root := t.TempDir()
	uploads := filepath.Join(root, "uploads")
	if err := os.MkdirAll(uploads, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "slide.pdf"), []byte("slides"), 0o600); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(root, "audit.log")
	ledger, err := audit.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Append(audit.Event{Kind: "decision.accepted", Subject: "sub_1"}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := WriteTarGZ(&output, domain.Seed(time.Now().UTC()), uploads, auditPath); err != nil {
		t.Fatalf("WriteTarGZ: %v", err)
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	files := map[string]string{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		contents := new(bytes.Buffer)
		if _, err := contents.ReadFrom(tarReader); err != nil {
			t.Fatal(err)
		}
		files[header.Name] = contents.String()
	}
	if files["workspace.json"] == "" || files["uploads/slide.pdf"] != "slides" || files["audit/audit.log"] == "" {
		t.Fatalf("archive files = %#v, want workspace, upload, and audit ledger", files)
	}
}
