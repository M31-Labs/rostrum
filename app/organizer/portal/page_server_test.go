package portal

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

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

func TestApproveHeadshotRequiresDurableValidatedImage(t *testing.T) {
	for _, test := range []struct {
		name      string
		prepare   func(t *testing.T, uploads, outside string) string
		wantError bool
	}{
		{
			name: "missing file",
			prepare: func(t *testing.T, uploads, outside string) string {
				return filepath.Join(uploads, "missing.jpg")
			},
			wantError: true,
		},
		{
			name: "outside upload root",
			prepare: func(t *testing.T, uploads, outside string) string {
				path := filepath.Join(outside, "portrait.jpg")
				writePortalTestFile(t, path, portalTestJPEG)
				return path
			},
			wantError: true,
		},
		{
			name: "non image bytes",
			prepare: func(t *testing.T, uploads, outside string) string {
				path := filepath.Join(uploads, "portrait.jpg")
				writePortalTestFile(t, path, []byte("this is not an image"))
				return path
			},
			wantError: true,
		},
		{
			name: "valid durable upload",
			prepare: func(t *testing.T, uploads, outside string) string {
				path := filepath.Join(uploads, "portrait.jpg")
				writePortalTestFile(t, path, portalTestJPEG)
				return path
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			uploads := filepath.Join(root, "uploads")
			outside := filepath.Join(root, "outside")
			if err := os.MkdirAll(uploads, 0o750); err != nil {
				t.Fatalf("create uploads: %v", err)
			}
			if err := os.MkdirAll(outside, 0o750); err != nil {
				t.Fatalf("create outside directory: %v", err)
			}
			storedPath := test.prepare(t, uploads, outside)
			workspace := openPortalApprovalWorkspace(t, "headshot", storedPath)
			ConfigureHeadshotApprovalValidator(portalTestHeadshotValidator(uploads))
			t.Cleanup(func() { ConfigureHeadshotApprovalValidator(nil) })

			err := approveTask(portalApprovalContext())
			if test.wantError && err == nil {
				t.Fatal("approveTask succeeded, want durable-media validation failure")
			}
			if !test.wantError && err != nil {
				t.Fatalf("approveTask: %v", err)
			}

			snapshot := workspace.Snapshot()
			completion, found := snapshot.Completion("task_portrait", "spk_alex")
			if !found {
				t.Fatal("completion disappeared")
			}
			speaker, found := snapshot.Speaker("spk_alex")
			if !found {
				t.Fatal("speaker disappeared")
			}
			if test.wantError {
				if completion.Status != domain.TaskSubmitted || speaker.HeadshotURL != "" || len(snapshot.AuditEvents) != 0 {
					t.Fatalf("rejected approval mutated state: completion=%#v speaker=%#v audits=%#v", completion, speaker, snapshot.AuditEvents)
				}
				return
			}
			if completion.Status != domain.TaskApproved {
				t.Fatalf("completion status = %q, want %q", completion.Status, domain.TaskApproved)
			}
			if speaker.HeadshotURL != "/portal-file/done_portrait" {
				t.Fatalf("speaker headshot URL = %q", speaker.HeadshotURL)
			}
			if len(snapshot.AuditEvents) != 1 || snapshot.AuditEvents[0].Action != "portal.task_approved" {
				t.Fatalf("approval audits = %#v", snapshot.AuditEvents)
			}
		})
	}
}

func TestApproveHeadshotFailsClosedWithoutConfiguredValidator(t *testing.T) {
	ConfigureHeadshotApprovalValidator(nil)
	workspace := openPortalApprovalWorkspace(t, "headshot", "/durable/uploads/portrait.jpg")

	if err := approveTask(portalApprovalContext()); err == nil {
		t.Fatal("approveTask succeeded without a configured durable-media validator")
	}
	snapshot := workspace.Snapshot()
	completion, _ := snapshot.Completion("task_portrait", "spk_alex")
	if completion.Status != domain.TaskSubmitted {
		t.Fatalf("completion status = %q, want submitted", completion.Status)
	}
}

func TestApproveNonHeadshotDoesNotRequireMediaValidator(t *testing.T) {
	ConfigureHeadshotApprovalValidator(nil)
	workspace := openPortalApprovalWorkspace(t, "file", "")

	if err := approveTask(portalApprovalContext()); err != nil {
		t.Fatalf("approveTask non-headshot: %v", err)
	}
	snapshot := workspace.Snapshot()
	completion, _ := snapshot.Completion("task_portrait", "spk_alex")
	if completion.Status != domain.TaskApproved {
		t.Fatalf("completion status = %q, want approved", completion.Status)
	}
}

var portalTestJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}

func writePortalTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func portalTestHeadshotValidator(uploads string) HeadshotApprovalValidator {
	return func(completion domain.TaskCompletion) error {
		root, err := filepath.Abs(uploads)
		if err != nil {
			return err
		}
		candidate, err := filepath.Abs(filepath.FromSlash(strings.TrimSpace(completion.StoredPath)))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, candidate)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return errors.New("outside uploads root")
		}
		file, err := os.Open(candidate)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("not a regular file")
		}
		prefix := make([]byte, 512)
		count, err := file.Read(prefix)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		switch http.DetectContentType(prefix[:count]) {
		case "image/jpeg", "image/png", "image/webp":
			return nil
		default:
			return errors.New("not a supported image")
		}
	}
}

func openPortalApprovalWorkspace(t *testing.T, taskType, storedPath string) *store.JSONStore {
	t.Helper()
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	state := domain.EmptyState(now)
	state.Speakers = []domain.Speaker{{
		ID: "spk_alex", FirstName: "Alex", LastName: "Rivera", Email: "alex@example.com", CreatedAt: now, UpdatedAt: now,
	}}
	state.Tasks = []domain.Task{{
		ID: "task_portrait", Title: "Speaker portrait", Type: taskType, DueAt: now.Add(24 * time.Hour),
		AssignedSpeakerIDs: []string{"spk_alex"}, CreatedAt: now, UpdatedAt: now,
	}}
	state.TaskCompletions = []domain.TaskCompletion{{
		ID: "done_portrait", TaskID: "task_portrait", SpeakerID: "spk_alex", Status: domain.TaskSubmitted,
		FileName: "portrait.jpg", ContentType: "image/jpeg", StoredPath: filepath.ToSlash(storedPath), CompletedAt: now, UpdatedAt: now,
	}}
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)
	t.Cleanup(func() { _ = workspace.Close() })
	return workspace
}

func portalApprovalContext() *action.Context {
	return &action.Context{
		Request:  httptest.NewRequest(http.MethodPost, "/organizer/portal/__actions/approveTask", nil),
		FormData: map[string]string{"task_id": "task_portrait", "speaker_id": "spk_alex"},
	}
}
