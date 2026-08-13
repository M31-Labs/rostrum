// Command prepare materializes the fictional example workspace and its
// approved uploads. It belongs to the example, not the Rostrum server: the
// production binary only sees a validated, checksummed workspace document.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
)

type portrait struct {
	completionID string
	asset        string
	fileName     string
}

var portraits = []portrait{
	{completionID: "done_maya_headshot", asset: "spk_maya.webp", fileName: "maya-chen-headshot.webp"},
	{completionID: "done_theo_headshot", asset: "spk_theo.webp", fileName: "theo-okafor-headshot.webp"},
	{completionID: "done_priya_headshot", asset: "spk_priya.webp", fileName: "priya-nair-headshot.webp"},
}

func main() {
	workspacePath := flag.String("workspace", "", "absolute output path for the raw workspace JSON")
	checksumPath := flag.String("checksum", "", "absolute output path for the workspace SHA-256")
	uploadChecksumPath := flag.String("upload-checksums", "", "absolute output path for the sha256sum-compatible upload manifest")
	assetDirectory := flag.String("assets", "", "absolute directory containing the example portraits")
	uploadDirectory := flag.String("uploads", "", "absolute directory that receives the portrait bytes")
	storedUploadDirectory := flag.String("stored-uploads", "", "absolute runtime upload directory recorded in the workspace; defaults to -uploads")
	flag.Parse()

	if err := prepare(*workspacePath, *checksumPath, *uploadChecksumPath, *assetDirectory, *uploadDirectory, *storedUploadDirectory); err != nil {
		fmt.Fprintln(os.Stderr, "prepare demo example:", err)
		os.Exit(1)
	}
}

func prepare(workspacePath, checksumPath, uploadChecksumPath, assetDirectory, uploadDirectory, storedUploadDirectory string) error {
	paths := map[string]*string{
		"-workspace":        &workspacePath,
		"-checksum":         &checksumPath,
		"-upload-checksums": &uploadChecksumPath,
		"-assets":           &assetDirectory,
		"-uploads":          &uploadDirectory,
	}
	for name, value := range paths {
		resolved, err := requiredAbsolute(name, *value)
		if err != nil {
			return err
		}
		*value = resolved
	}
	if strings.TrimSpace(storedUploadDirectory) == "" {
		storedUploadDirectory = uploadDirectory
	}
	var err error
	storedUploadDirectory, err = requiredAbsolute("-stored-uploads", storedUploadDirectory)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(uploadDirectory, 0o750); err != nil {
		return fmt.Errorf("create upload directory: %w", err)
	}

	state := fixture.Seed(fixture.SeedTime())
	var uploadChecksums strings.Builder
	for _, item := range portraits {
		if item.asset != filepath.Base(item.asset) || strings.ContainsAny(item.asset, "\\/\r\n\t ") {
			return fmt.Errorf("portrait asset name %q is not a safe manifest entry", item.asset)
		}
		source := filepath.Join(assetDirectory, item.asset)
		destination := filepath.Join(uploadDirectory, item.asset)
		digest, err := copyRegularFile(source, destination)
		if err != nil {
			return err
		}
		fmt.Fprintf(&uploadChecksums, "%s  %s\n", digest, item.asset)
		found := false
		for index := range state.TaskCompletions {
			completion := &state.TaskCompletions[index]
			if completion.ID != item.completionID {
				continue
			}
			completion.FileName = item.fileName
			completion.ContentType = "image/webp"
			completion.StoredPath = filepath.ToSlash(filepath.Join(storedUploadDirectory, item.asset))
			found = true
			break
		}
		if !found {
			return fmt.Errorf("fixture is missing completion %s", item.completionID)
		}
	}
	if err := state.Validate(); err != nil {
		return fmt.Errorf("validate prepared workspace: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspace: %w", err)
	}
	raw = append(raw, '\n')
	if err := writeExclusive(workspacePath, raw, 0o640); err != nil {
		return fmt.Errorf("write workspace: %w", err)
	}
	digest := sha256.Sum256(raw)
	checksum := []byte(hex.EncodeToString(digest[:]) + "\n")
	if err := writeExclusive(checksumPath, checksum, 0o640); err != nil {
		return fmt.Errorf("write checksum: %w", err)
	}
	if err := writeExclusive(uploadChecksumPath, []byte(uploadChecksums.String()), 0o640); err != nil {
		return fmt.Errorf("write upload checksums: %w", err)
	}
	return nil
}

func requiredAbsolute(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be absolute", name)
	}
	return filepath.Clean(value), nil
}

func copyRegularFile(source, destination string) (string, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", fmt.Errorf("inspect portrait %s: %w", filepath.Base(source), err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("portrait %s is not a regular file", filepath.Base(source))
	}
	input, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open portrait %s: %w", filepath.Base(source), err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return "", fmt.Errorf("create upload %s: %w", filepath.Base(destination), err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = output.Close()
			_ = os.Remove(destination)
		}
	}()
	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(output, digest), input); err != nil {
		return "", fmt.Errorf("copy portrait %s: %w", filepath.Base(source), err)
	}
	if err := output.Sync(); err != nil {
		return "", fmt.Errorf("sync upload %s: %w", filepath.Base(destination), err)
	}
	if err := output.Close(); err != nil {
		return "", fmt.Errorf("close upload %s: %w", filepath.Base(destination), err)
	}
	complete = true
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = output.Close()
			_ = os.Remove(path)
		}
	}()
	if _, err := output.Write(data); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
