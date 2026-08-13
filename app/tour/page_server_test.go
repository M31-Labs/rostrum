package tour

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/m31-labs/rostrum/examples/demo/fixture"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/store"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
)

func TestTourDataExposesSignedPersonaLinksOnlyInReadOnlyPreview(t *testing.T) {
	t.Setenv("APP_MODE", "preview")
	state := fixture.Seed(time.Now().UTC())

	live := tourData(state, false)
	livePersonas := live["personas"].([]map[string]any)
	if got := livePersonas[2]["href"].(string); got != "/organizer/review" {
		t.Fatalf("live reviewer href = %q, want organizer surface", got)
	}
	if got := livePersonas[3]["href"].(string); got != "/organizer/portal" {
		t.Fatalf("live speaker href = %q, want organizer surface", got)
	}

	preview := tourData(state, true)
	previewPersonas := preview["personas"].([]map[string]any)
	reviewerHref := previewPersonas[2]["href"].(string)
	if !strings.HasPrefix(reviewerHref, "/review/") {
		t.Fatalf("preview reviewer href = %q, want signed reviewer route", reviewerHref)
	}
	reviewerToken := strings.TrimPrefix(reviewerHref, "/review/")
	if reviewerID, ok := token.NewReviewer().VerifyReviewer(reviewerToken); !ok || reviewerID == "" {
		t.Fatalf("preview reviewer link did not verify: id=%q ok=%v", reviewerID, ok)
	}

	speakerHref := previewPersonas[3]["href"].(string)
	parts := strings.SplitN(speakerHref, "?key=", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "/portal/") {
		t.Fatalf("preview speaker href = %q, want signed portal route", speakerHref)
	}
	speakerID := strings.TrimPrefix(parts[0], "/portal/")
	if verifiedID, ok := token.New().Verify(parts[1]); !ok || verifiedID != speakerID {
		t.Fatalf("preview speaker link verified as %q/%v, want %q", verifiedID, ok, speakerID)
	}

	// These exact bearer links must not cross into a live deployment, even if
	// SESSION_SECRET and the seed IDs happen to be identical.
	t.Setenv("APP_MODE", "live")
	if reviewerID, ok := token.NewReviewer().VerifyReviewer(reviewerToken); ok || reviewerID != "" {
		t.Fatalf("live mode accepted tour reviewer token: id=%q ok=%v", reviewerID, ok)
	}
	if verifiedID, ok := token.New().Verify(parts[1]); ok || verifiedID != "" {
		t.Fatalf("live mode accepted tour speaker token: id=%q ok=%v", verifiedID, ok)
	}
}

func TestLoadTourDisablesResponseStorage(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	workspace, err := store.Open(":memory:", state)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	appstate.Set(workspace)

	request := httptest.NewRequest(http.MethodGet, "/tour", nil)
	pageState := server.NewPageStateForRequest(request)
	ctx := &route.RouteContext{Request: request, PageState: *pageState}
	if _, err := loadTour(ctx, route.FilePage{}); err != nil {
		t.Fatalf("load tour: %v", err)
	}

	headers := make(http.Header)
	server.ApplyCacheHeaders(request, headers, http.StatusOK, ctx.CacheState(), nil)
	if got := headers.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestFirstHumanReviewerPrefersActivePlan(t *testing.T) {
	state := fixture.Seed(time.Now().UTC())
	id := firstHumanReviewer(state)
	if id == "" {
		t.Fatal("expected a human reviewer in the seeded active plan")
	}
	for _, reviewer := range state.Reviewers {
		if reviewer.ID == id && reviewer.Kind == "human" && reviewer.Active() {
			return
		}
	}
	t.Fatalf("selected reviewer %q is not active and human", id)
}
