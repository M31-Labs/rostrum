// Package archive defines Rostrum's portable workspace envelope, automatic
// pre-import backups, and streaming full-archive writer. It does not own a
// store: callers validate first, write a backup, then invoke StateStore.Replace
// so JSON, SQLite, and Postgres retain the same import semantics.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/audit"
	"github.com/m31-labs/rostrum/internal/domain"
)

const (
	ExportVersion   = 1
	BackupRetention = 10
)

// Envelope is the stable, checksummed portability contract. State is not a
// string: JSON encoding keeps normal domain types intact while SHA256 binds
// the exact canonical encoding that this version exports.
type Envelope struct {
	RostrumExport int          `json:"rostrumExport"`
	SchemaVersion int          `json:"schemaVersion"`
	ExportedAt    time.Time    `json:"exportedAt"`
	SHA256        string       `json:"sha256"`
	State         domain.State `json:"state"`
}

// NewEnvelope builds an operator-facing export. Pending magic links are
// intentionally transient and excluded: exporting one must not extend a
// pending sign-in session to another instance or archive recipient.
func NewEnvelope(state domain.State) (Envelope, error) {
	return newEnvelope(state, true, time.Now().UTC())
}

// Marshal builds and encodes one export envelope.
func Marshal(state domain.State) ([]byte, error) {
	envelope, err := NewEnvelope(state)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode workspace export: %w", err)
	}
	return data, nil
}

// Decode verifies every portable import gate before returning a state. The
// caller can therefore make a backup only after this returns nil.
func Decode(data []byte) (domain.State, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return domain.State{}, fmt.Errorf("decode workspace export: %w", err)
	}
	if envelope.RostrumExport != ExportVersion {
		return domain.State{}, fmt.Errorf("unsupported rostrum export version %d; want %d", envelope.RostrumExport, ExportVersion)
	}
	if envelope.SchemaVersion != domain.CurrentSchemaVersion {
		return domain.State{}, fmt.Errorf("workspace schema version %d is not supported; want %d", envelope.SchemaVersion, domain.CurrentSchemaVersion)
	}
	encoded, err := json.Marshal(envelope.State)
	if err != nil {
		return domain.State{}, fmt.Errorf("encode imported workspace for checksum: %w", err)
	}
	actual := stateHash(encoded)
	if !constantEqual(actual, envelope.SHA256) {
		return domain.State{}, fmt.Errorf("workspace export checksum does not match its state")
	}
	if err := envelope.State.Validate(); err != nil {
		return domain.State{}, fmt.Errorf("validate imported workspace: %w", err)
	}
	return cloneState(envelope.State), nil
}

// PreserveCurrentIdentity protects the operator applying an import. A backup
// from another host may carry stale or untrusted principals; the receiving
// instance retains its own principals, passkeys, and pending links instead.
func PreserveCurrentIdentity(current, imported domain.State) domain.State {
	result := cloneState(imported)
	result.Principals = append([]domain.Principal(nil), current.Principals...)
	result.AuthPasskeys = append([]domain.AuthPasskey(nil), current.AuthPasskeys...)
	result.AuthMagicLinks = append([]domain.AuthMagicLink(nil), current.AuthMagicLinks...)
	return result
}

// RebaseUploadPaths moves imported task-completion references onto the
// receiving instance's upload directory. Archives store the bytes by the
// generated basename, not an old host's absolute path, so a validated import
// can be followed by a stopped-process asset restore on a different host.
// Existing paths are deliberately flattened: portalUpload has always stored
// uploads in one directory and a crafted archive must not introduce a nested
// or traversal path into the receiver's state.
func RebaseUploadPaths(state domain.State, uploadsDirectory string) (domain.State, error) {
	directory := filepath.Clean(strings.TrimSpace(uploadsDirectory))
	if directory == "" || directory == "." {
		return domain.State{}, fmt.Errorf("upload directory is required")
	}
	result := cloneState(state)
	for index := range result.TaskCompletions {
		storedPath := strings.TrimSpace(result.TaskCompletions[index].StoredPath)
		if storedPath == "" {
			continue
		}
		name := filepath.Base(filepath.FromSlash(storedPath))
		if name == "" || name == "." || name == string(filepath.Separator) || name == ".." {
			return domain.State{}, fmt.Errorf("task completion %s has an invalid stored upload path", result.TaskCompletions[index].ID)
		}
		result.TaskCompletions[index].StoredPath = filepath.ToSlash(filepath.Join(directory, name))
	}
	return result, nil
}

// WriteBackup atomically writes the exact pre-import state as a checksummed
// envelope, then retains the newest BackupRetention files only.
func WriteBackup(directory string, state domain.State) (string, error) {
	return writeBackupAt(directory, state, time.Now().UTC())
}

func writeBackupAt(directory string, state domain.State, at time.Time) (string, error) {
	directory = filepath.Clean(strings.TrimSpace(directory))
	if directory == "" || directory == "." {
		return "", fmt.Errorf("backup directory is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	envelope, err := newEnvelope(state, false, at)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode workspace backup: %w", err)
	}
	name := "rostrum-" + at.UTC().Format("20060102T150405.000000000Z") + ".json"
	path := filepath.Join(directory, name)
	temporary, err := os.CreateTemp(directory, ".rostrum-backup-*")
	if err != nil {
		return "", fmt.Errorf("create workspace backup: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("secure workspace backup: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write workspace backup: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync workspace backup: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close workspace backup: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", fmt.Errorf("publish workspace backup: %w", err)
	}
	if err := retainBackups(directory); err != nil {
		return "", err
	}
	return path, nil
}

// WriteTarGZ streams a complete archive with the portable workspace export,
// all regular upload files, and all independent audit ledger segments. It
// follows no symlinks and never builds a temporary archive on disk.
func WriteTarGZ(destination io.Writer, state domain.State, uploadsDirectory, auditLogPath string) error {
	if destination == nil {
		return fmt.Errorf("archive destination is required")
	}
	workspace, err := Marshal(state)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(destination)
	tarWriter := tar.NewWriter(gzipWriter)
	close := func() error {
		if err := tarWriter.Close(); err != nil {
			_ = gzipWriter.Close()
			return err
		}
		return gzipWriter.Close()
	}
	if err := writeBytes(tarWriter, "workspace.json", workspace, 0o600, time.Now().UTC()); err != nil {
		_ = close()
		return err
	}
	if err := writeDirectory(tarWriter, uploadsDirectory, "uploads"); err != nil {
		_ = close()
		return err
	}
	segments, err := audit.SegmentFiles(auditLogPath)
	if err != nil {
		_ = close()
		return err
	}
	for _, path := range segments {
		if err := writeFile(tarWriter, path, filepath.ToSlash(filepath.Join("audit", filepath.Base(path)))); err != nil {
			_ = close()
			return err
		}
	}
	if err := close(); err != nil {
		return fmt.Errorf("finish workspace archive: %w", err)
	}
	return nil
}

func newEnvelope(state domain.State, stripMagicLinks bool, at time.Time) (Envelope, error) {
	state = cloneState(state)
	if stripMagicLinks {
		state.AuthMagicLinks = nil
	}
	if err := state.Validate(); err != nil {
		return Envelope{}, fmt.Errorf("validate workspace export: %w", err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return Envelope{}, fmt.Errorf("encode workspace state: %w", err)
	}
	return Envelope{
		RostrumExport: ExportVersion,
		SchemaVersion: state.SchemaVersion,
		ExportedAt:    at.UTC(),
		SHA256:        stateHash(encoded),
		State:         state,
	}, nil
}

func retainBackups(directory string) error {
	paths, err := filepath.Glob(filepath.Join(directory, "rostrum-*.json"))
	if err != nil {
		return fmt.Errorf("find workspace backups: %w", err)
	}
	sort.Strings(paths)
	for len(paths) > BackupRetention {
		if err := os.Remove(paths[0]); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove expired workspace backup: %w", err)
		}
		paths = paths[1:]
	}
	return nil
}

func writeDirectory(writer *tar.Writer, directory, prefix string) error {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil
	}
	info, err := os.Stat(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat archive directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("archive path %s is not a directory", directory)
	}
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		return writeFile(writer, path, filepath.ToSlash(filepath.Join(prefix, relative)))
	})
}

func writeFile(writer *tar.Writer, path, archivePath string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat archive file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("archive file %s is not regular", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open archive file %s: %w", path, err)
	}
	defer file.Close()
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("make archive header %s: %w", path, err)
	}
	header.Name = filepath.ToSlash(archivePath)
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("write archive header %s: %w", path, err)
	}
	// The active audit segment can receive an append while an archive is
	// streaming. The tar header commits the size observed above, so copy only
	// that snapshot-sized prefix rather than letting a concurrent append exceed
	// the entry and corrupt the rest of the archive.
	written, err := io.Copy(writer, io.LimitReader(file, info.Size()))
	if err != nil {
		return fmt.Errorf("write archive file %s: %w", path, err)
	}
	if written != info.Size() {
		return fmt.Errorf("archive file %s changed while it was read", path)
	}
	return nil
}

func writeBytes(writer *tar.Writer, path string, data []byte, mode int64, at time.Time) error {
	if err := writer.WriteHeader(&tar.Header{Name: path, Mode: mode, Size: int64(len(data)), ModTime: at.UTC()}); err != nil {
		return fmt.Errorf("write archive header %s: %w", path, err)
	}
	if _, err := io.Copy(writer, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("write archive file %s: %w", path, err)
	}
	return nil
}

func stateHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func constantEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	value := byte(0)
	for index := range left {
		value |= left[index] ^ right[index]
	}
	return value == 0
}

func cloneState(state domain.State) domain.State {
	data, _ := json.Marshal(state)
	var result domain.State
	_ = json.Unmarshal(data, &result)
	return result
}
