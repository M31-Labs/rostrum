package present

import (
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestPortalOperationsDerivesPreviewPortalFromAssignedSpeaker(t *testing.T) {
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	state := domain.EmptyState(now)
	state.Speakers = []domain.Speaker{{
		ID: "spk_alex", FirstName: "Alex", LastName: "Rivera", Email: "alex@example.com", CreatedAt: now, UpdatedAt: now,
	}}
	state.Tasks = []domain.Task{{
		ID: "task_profile", Title: "Confirm profile", Type: "profile", DueAt: now.Add(24 * time.Hour),
		AssignedSpeakerIDs: []string{"spk_alex"}, CreatedAt: now, UpdatedAt: now,
	}}

	view := PortalOperations(state)
	if got := view["previewPortalURL"]; got != "/portal/spk_alex" {
		t.Fatalf("preview portal URL = %q, want assigned speaker route", got)
	}
	if has, ok := view["hasPreviewPortal"].(bool); !ok || !has {
		t.Fatalf("hasPreviewPortal = %#v, want true", view["hasPreviewPortal"])
	}
}

func TestPortalOperationsOmitsPreviewPortalWithoutAssignments(t *testing.T) {
	view := PortalOperations(domain.EmptyState(time.Now().UTC()))
	if got := view["previewPortalURL"]; got != "" {
		t.Fatalf("preview portal URL = %q, want empty", got)
	}
	if has, ok := view["hasPreviewPortal"].(bool); !ok || has {
		t.Fatalf("hasPreviewPortal = %#v, want false", view["hasPreviewPortal"])
	}
}
