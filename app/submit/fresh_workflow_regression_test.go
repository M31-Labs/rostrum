package submit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/ratelimit"
	"github.com/m31-labs/rostrum/internal/store"
	"m31labs.dev/gosx/action"
)

func TestFreshWorkspaceAcceptsAValidPublicProposal(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "outbox")
	submissionLimiter = ratelimit.NewCounter(5)
	submissionIPLimiter = ratelimit.NewTokenBucket(10, time.Hour)
	draftCreationLimiter = ratelimit.NewCounter(draftCreationSessionLimit)
	draftCreationIPLimiter = ratelimit.NewTokenBucket(draftCreationIPLimit, time.Hour)

	now := time.Now().UTC()
	workspace, err := store.Open(":memory:", domain.FreshState(now))
	if err != nil {
		t.Fatalf("open fresh workspace: %v", err)
	}
	appstate.Set(workspace)
	request := httptest.NewRequest(http.MethodPost, "/submit/call-for-proposals/__actions/submitProposal", nil)
	request.RemoteAddr = "198.51.100.24:443"
	if err := submitProposal(&action.Context{
		Request: request,
		FormData: map[string]string{
			"form_id":    "form_cfp",
			"title":      "Operating dependable community platforms",
			"abstract":   "A practical account of turning a fragile community service into a dependable platform with clear ownership, safe changes, and useful feedback loops.",
			"format":     "Talk",
			"category":   "general",
			"level":      "Intermediate",
			"first_name": "Jordan",
			"last_name":  "Lee",
			"email":      "jordan@example.com",
			"role":       "Platform engineer",
			"company":    "Community Systems",
			"biography":  "Jordan helps community teams build reliable software and humane operating practices.",
		},
	}); err != nil {
		t.Fatalf("submitProposal from FreshState: %v", err)
	}

	snapshot := workspace.Snapshot()
	if len(snapshot.Submissions) != 1 || len(snapshot.Speakers) != 1 {
		t.Fatalf("fresh intake counts = submissions:%d speakers:%d", len(snapshot.Submissions), len(snapshot.Speakers))
	}
	proposal := snapshot.Submissions[0]
	if proposal.Status != domain.SubmissionPending || proposal.FormID != "form_cfp" || proposal.CategoryID != "general" {
		t.Fatalf("stored proposal = %#v", proposal)
	}
	if proposal.RoutedQueue != "program-triage" || proposal.RoutedOwner != "Program team" || proposal.TrackID != "" {
		t.Fatalf("generic routing result = queue:%q owner:%q track:%q", proposal.RoutedQueue, proposal.RoutedOwner, proposal.TrackID)
	}
	if len(proposal.SpeakerIDs) != 1 || proposal.SpeakerIDs[0] != snapshot.Speakers[0].ID {
		t.Fatalf("proposal speaker binding = %#v / %#v", proposal.SpeakerIDs, snapshot.Speakers)
	}
	task, found := snapshot.Task("task_profile")
	if !found || len(task.AssignedSpeakerIDs) != 1 || task.AssignedSpeakerIDs[0] != snapshot.Speakers[0].ID {
		t.Fatalf("fresh profile task assignment = %#v", task)
	}
	if len(snapshot.Communications) != 1 || snapshot.Communications[0].TemplateID != "tpl_submission_confirmation" || snapshot.Communications[0].Status != domain.CommunicationSent {
		t.Fatalf("fresh confirmation record = %#v", snapshot.Communications)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("fresh workspace after public submission: %v", err)
	}
}
