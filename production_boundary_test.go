package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestProductionSourceBoundary keeps the showcase as a leaf of the
// repository: production packages may be exercised by example-backed tests,
// but shipped code must never import the example or regain its fixture IDs,
// retired mode names, or legacy presentation selectors.
func TestProductionSourceBoundary(t *testing.T) {
	t.Parallel()
	forbiddenMarkers := []string{
		"spk_maya",
		"spk_theo",
		"spk_priya",
		"evt_m31_forum_2026",
		"task_headshot",
		"form_cfp_2026",
		"m31 systems forum",
		"maya chen",
		"theo okafor",
		"lina martinez",
		"priya nair",
		"agents-in-production",
		"governed-systems",
		"track-agents",
		"track-governance",
		"track-interfaces",
		"track-infra",
		"/demo/reset",
		"demo_mode",
		"readonlydemo",
		"demomode",
		"demo-headshots",
		".demo-banner",
		"demo-outbox",
		"seed=demo",
		"seed=empty",
		"from \"programma\"",
		"fictional assignment",
	}

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && productionBoundaryIgnoredDir(entry.Name(), path) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		extension := strings.ToLower(filepath.Ext(path))
		if !productionBoundarySourceFile(entry.Name(), extension) {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lowerSource := strings.ToLower(string(source))
		for _, marker := range forbiddenMarkers {
			if strings.Contains(lowerSource, marker) {
				t.Errorf("production source %s contains showcase-only marker %q", path, marker)
			}
		}
		if extension == ".go" {
			checkProductionImports(t, path, source)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk production source: %v", err)
	}
}

func productionBoundarySourceFile(name, extension string) bool {
	switch extension {
	case ".go", ".gsx", ".css", ".arb", ".html", ".js", ".json", ".yaml", ".yml", ".sh", ".env":
		return true
	}
	switch name {
	case "Dockerfile", "Makefile", ".env.example":
		return true
	default:
		return false
	}
}

func productionBoundaryIgnoredDir(name, path string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch filepath.ToSlash(path) {
	case "docs", "examples", "data", "dist", "_site", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func checkProductionImports(t *testing.T, path string, source []byte) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
	if err != nil {
		t.Errorf("parse production Go source %s: %v", path, err)
		return
	}
	const examplesPrefix = "github.com/m31-labs/rostrum/examples"
	for _, imported := range parsed.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Errorf("decode import in %s: %v", path, err)
			continue
		}
		value = strings.ToLower(value)
		if value == examplesPrefix || strings.HasPrefix(value, examplesPrefix+"/") {
			t.Errorf("production Go source %s imports showcase package %q", path, value)
		}
	}
}
