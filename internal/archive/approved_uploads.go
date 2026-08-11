package archive

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

const (
	// MaxApprovedUploadBundleBytes bounds an operator download before a ZIP is
	// written. It prevents one otherwise-valid set of approved assets from
	// becoming an accidental unbounded archive job.
	MaxApprovedUploadBundleBytes int64 = 512 << 20
	MaxApprovedUploadBundleFiles       = 5_000
)

var approvedUploadBundleTime = time.Unix(0, 0).UTC()

// ApprovedUploadManifest is included in every approved-upload ZIP. It is
// intentionally timestamp-free so identical canonical state and file bytes
// produce identical archive bytes, allowing an operator to verify a download
// without trusting a server-side generation clock.
type ApprovedUploadManifest struct {
	Version int                          `json:"version"`
	EventID string                       `json:"eventId"`
	Files   []ApprovedUploadManifestFile `json:"files"`
}

type ApprovedUploadManifestFile struct {
	Path         string `json:"path"`
	CompletionID string `json:"completionId"`
	TaskID       string `json:"taskId"`
	SpeakerID    string `json:"speakerId"`
	FileName     string `json:"fileName"`
	ContentType  string `json:"contentType"`
	SHA256       string `json:"sha256"`
	Bytes        int64  `json:"bytes"`
}

// ApprovedUploadBundle is a checked snapshot of explicitly approved private
// uploads. Build it before committing an audit event or sending HTTP headers;
// Write then validates the file bytes against the recorded SHA-256 while it
// streams the deterministic ZIP.
type ApprovedUploadBundle struct {
	manifest ApprovedUploadManifest
	files    []approvedUploadFile
}

type approvedUploadFile struct {
	manifest ApprovedUploadManifestFile
	source   string
	root     string
}

// BuildApprovedUploadBundle finds only completions explicitly marked approved
// and backed by regular files under uploadsDirectory. A completion without an
// upload is naturally absent (for example an approved profile/form task);
// an existing stored path that escapes the upload directory or disappears is
// an integrity error rather than a silent partial export.
func BuildApprovedUploadBundle(state domain.State, uploadsDirectory string) (ApprovedUploadBundle, error) {
	root, err := uploadRoot(uploadsDirectory)
	if err != nil {
		return ApprovedUploadBundle{}, err
	}
	files := make([]approvedUploadFile, 0)
	var total int64
	for _, completion := range state.TaskCompletions {
		if completion.Status != domain.TaskApproved || strings.TrimSpace(completion.StoredPath) == "" {
			continue
		}
		if len(files) >= MaxApprovedUploadBundleFiles {
			return ApprovedUploadBundle{}, fmt.Errorf("approved upload bundle exceeds %d files", MaxApprovedUploadBundleFiles)
		}
		source, info, err := regularUploadPath(root, completion.StoredPath)
		if err != nil {
			return ApprovedUploadBundle{}, fmt.Errorf("approved completion %s: %w", completion.ID, err)
		}
		if info.Size() > MaxApprovedUploadBundleBytes-total {
			return ApprovedUploadBundle{}, fmt.Errorf("approved upload bundle exceeds %d bytes", MaxApprovedUploadBundleBytes)
		}
		sum, bytesRead, err := fileSHA256(source, info.Size())
		if err != nil {
			return ApprovedUploadBundle{}, fmt.Errorf("hash approved completion %s: %w", completion.ID, err)
		}
		if bytesRead != info.Size() {
			return ApprovedUploadBundle{}, fmt.Errorf("approved completion %s changed while it was read", completion.ID)
		}
		total += bytesRead
		name := archiveFileName(completion)
		files = append(files, approvedUploadFile{
			source: source,
			root:   root,
			manifest: ApprovedUploadManifestFile{
				Path:         name,
				CompletionID: completion.ID,
				TaskID:       completion.TaskID,
				SpeakerID:    completion.SpeakerID,
				FileName:     completion.FileName,
				ContentType:  completion.ContentType,
				SHA256:       sum,
				Bytes:        bytesRead,
			},
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].manifest.Path < files[j].manifest.Path
	})
	manifest := ApprovedUploadManifest{Version: 1, EventID: state.Event.ID, Files: make([]ApprovedUploadManifestFile, 0, len(files))}
	for _, file := range files {
		manifest.Files = append(manifest.Files, file.manifest)
	}
	return ApprovedUploadBundle{manifest: manifest, files: files}, nil
}

// Manifest returns a copy of the checksums and source identifiers included in
// the archive. It is useful for operator UIs and tests without exposing local
// filesystem paths.
func (bundle ApprovedUploadBundle) Manifest() ApprovedUploadManifest {
	result := bundle.manifest
	result.Files = append([]ApprovedUploadManifestFile(nil), bundle.manifest.Files...)
	return result
}

// Write produces a byte-stable ZIP: manifest first, files in a stable path
// order, Store compression, fixed metadata, and no generated timestamp. Each
// file's SHA-256 is rechecked while streaming so a concurrent replacement
// fails closed rather than returning a manifest for different bytes.
func (bundle ApprovedUploadBundle) Write(destination io.Writer) error {
	if destination == nil {
		return fmt.Errorf("approved upload bundle destination is required")
	}
	manifest, err := json.Marshal(bundle.manifest)
	if err != nil {
		return fmt.Errorf("encode approved upload manifest: %w", err)
	}
	writer := zip.NewWriter(destination)
	if err := writeZIPBytes(writer, "manifest.json", manifest); err != nil {
		_ = writer.Close()
		return err
	}
	for _, file := range bundle.files {
		if err := writeZIPFile(writer, file); err != nil {
			_ = writer.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish approved upload bundle: %w", err)
	}
	return nil
}

// ApprovedUploadZIP is a convenience for callers that need bytes rather than
// a streaming response. HTTP handlers should build once, record their audit
// event, then call bundle.Write directly.
func ApprovedUploadZIP(state domain.State, uploadsDirectory string) ([]byte, error) {
	bundle, err := BuildApprovedUploadBundle(state, uploadsDirectory)
	if err != nil {
		return nil, err
	}
	var result bytes.Buffer
	if err := bundle.Write(&result); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func uploadRoot(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", fmt.Errorf("uploads directory is required")
	}
	root, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return "", fmt.Errorf("resolve uploads directory: %w", err)
	}
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		// A workspace with no stored approved files still has a useful,
		// deterministic manifest-only ZIP. If state references a missing file,
		// regularUploadPath below fails closed when it is actually inspected.
		return root, nil
	}
	if err != nil {
		return "", fmt.Errorf("stat uploads directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("uploads directory is not a directory")
	}
	return root, nil
}

func regularUploadPath(root, storedPath string) (string, os.FileInfo, error) {
	path := filepath.FromSlash(strings.TrimSpace(storedPath))
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", nil, fmt.Errorf("resolve stored upload path: %w", err)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("stored upload path is outside the uploads directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, fmt.Errorf("stat stored upload: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("stored upload is not a regular file")
	}
	return path, info, nil
}

func fileSHA256(path string, expectedSize int64) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, expectedSize+1))
	if err != nil {
		return "", written, err
	}
	if written != expectedSize {
		return "", written, fmt.Errorf("file size changed from %d to %d", expectedSize, written)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func archiveFileName(completion domain.TaskCompletion) string {
	name := filepath.Base(filepath.FromSlash(strings.TrimSpace(completion.FileName)))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "upload"
	}
	return "files/" + safeArchiveComponent(completion.ID) + "-" + safeArchiveComponent(name)
}

func safeArchiveComponent(value string) string {
	var result strings.Builder
	for _, runeValue := range value {
		switch {
		case runeValue >= 'a' && runeValue <= 'z', runeValue >= 'A' && runeValue <= 'Z', runeValue >= '0' && runeValue <= '9', runeValue == '.', runeValue == '-', runeValue == '_':
			result.WriteRune(runeValue)
		default:
			result.WriteByte('_')
		}
	}
	if result.Len() == 0 {
		return "file"
	}
	return result.String()
}

func writeZIPBytes(writer *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetModTime(approvedUploadBundleTime)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create ZIP entry %s: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("write ZIP entry %s: %w", name, err)
	}
	return nil
}

func writeZIPFile(writer *zip.Writer, item approvedUploadFile) error {
	path, info, err := regularUploadPath(item.root, item.source)
	if err != nil {
		return fmt.Errorf("recheck ZIP source %s: %w", item.manifest.CompletionID, err)
	}
	if info.Size() != item.manifest.Bytes {
		return fmt.Errorf("approved completion %s changed size before archive", item.manifest.CompletionID)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open ZIP source %s: %w", item.manifest.CompletionID, err)
	}
	defer file.Close()
	header := &zip.FileHeader{Name: item.manifest.Path, Method: zip.Store}
	header.SetModTime(approvedUploadBundleTime)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create ZIP entry %s: %w", item.manifest.Path, err)
	}
	hash := sha256.New()
	written, err := io.Copy(entry, io.TeeReader(io.LimitReader(file, item.manifest.Bytes+1), hash))
	if err != nil {
		return fmt.Errorf("write ZIP source %s: %w", item.manifest.CompletionID, err)
	}
	if written != item.manifest.Bytes {
		return fmt.Errorf("approved completion %s changed while archive was written", item.manifest.CompletionID)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != item.manifest.SHA256 {
		return fmt.Errorf("approved completion %s changed while archive was written", item.manifest.CompletionID)
	}
	return nil
}
