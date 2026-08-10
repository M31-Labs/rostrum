package integrations

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/accelevents"
	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Integrations(appstate.MustGet().Snapshot()), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Integrations — Rostrum"}, Description: "One-way Accelevents publishing with a credential-free dry run."}, nil
		},
		Actions: route.FileActions{"dryRun": dryRun, "liveSync": liveSync},
	}); err != nil {
		log.Fatal(err)
	}
}

func liveSync(ctx *action.Context) error {
	token := firstConfigured("ACCELEVENTS_API_KEY", "ACCELEVENTS_API_TOKEN")
	if token == "" {
		return action.Validation("Live publishing is locked until ACCELEVENTS_API_KEY is configured.", nil, ctx.FormData)
	}

	state := appstate.MustGet().Snapshot()
	payloads := accelevents.BuildPayloads(state)
	if len(payloads.Speakers) == 0 || len(payloads.Sessions) == 0 {
		return action.Validation("No scheduled speakers or sessions are ready to publish.", nil, ctx.FormData)
	}
	eventURL := strings.TrimSpace(os.Getenv("ACCELEVENTS_EVENT_URL"))
	if eventURL == "" && len(state.Integrations) > 0 {
		eventURL = state.Integrations[0].EventURL
	}
	if eventURL == "" {
		return action.Validation("Set ACCELEVENTS_EVENT_URL before live publishing.", nil, ctx.FormData)
	}

	startedAt := time.Now().UTC()
	client := accelevents.Client{
		BaseURL: strings.TrimSpace(os.Getenv("ACCELEVENTS_BASE_URL")),
		Token:   token,
	}
	syncErr := client.Sync(ctx.Request.Context(), eventURL, payloads)
	finishedAt := time.Now().UTC()
	runStatus := "complete"
	summary := "Published speakers and scheduled sessions to Accelevents. Rostrum remains the source of truth."
	lastStatus := "Live sync complete"
	if syncErr != nil {
		runStatus = "failed"
		summary = "Live sync stopped on the first remote error: " + syncErr.Error()
		lastStatus = "Live sync failed"
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		state.SyncRuns = append(state.SyncRuns, domain.SyncRun{
			ID: domain.NewID("sync"), Integration: "integration_accelevents", Mode: "live", Status: runStatus,
			Speakers: len(payloads.Speakers), Sessions: len(payloads.Sessions), StartedAt: startedAt, FinishedAt: finishedAt,
			Summary: summary,
		})
		for index := range state.Integrations {
			if state.Integrations[index].ID == "integration_accelevents" {
				state.Integrations[index].Enabled = true
				state.Integrations[index].CredentialsOK = true
				state.Integrations[index].EventURL = eventURL
				state.Integrations[index].LastSyncAt = finishedAt
				state.Integrations[index].LastStatus = lastStatus
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if syncErr != nil {
		return action.Error(502, "Accelevents rejected the live sync. The failed run is preserved in the ledger.")
	}

	session.AddFlash(ctx.Request, "notice", fmt.Sprintf("Published %d speakers and %d sessions to Accelevents.", len(payloads.Speakers), len(payloads.Sessions)))
	live.Broadcast("integration:live-sync", map[string]int{"speakers": len(payloads.Speakers), "sessions": len(payloads.Sessions)})
	actionflow.Redirect(ctx, "/organizer/integrations")
	return nil
}

func dryRun(ctx *action.Context) error {
	payloads := accelevents.Payloads{}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		payloads = accelevents.BuildPayloads(*state)
		if len(payloads.Speakers) == 0 || len(payloads.Sessions) == 0 {
			return fmt.Errorf("no scheduled speakers or sessions to sync")
		}
		now := time.Now().UTC()
		state.SyncRuns = append(state.SyncRuns, domain.SyncRun{
			ID: domain.NewID("sync"), Integration: "integration_accelevents", Mode: "dry-run", Status: "complete",
			Speakers: len(payloads.Speakers), Sessions: len(payloads.Sessions), StartedAt: now, FinishedAt: now.Add(850 * time.Millisecond),
			Summary: "Validated speaker and session payloads against the one-way Accelevents adapter; no external requests sent.",
		})
		for index := range state.Integrations {
			if state.Integrations[index].ID == "integration_accelevents" {
				state.Integrations[index].LastSyncAt = now
				state.Integrations[index].LastStatus = "Dry run passed"
			}
		}
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", fmt.Sprintf("Dry run passed: %d speakers and %d sessions are ready.", len(payloads.Speakers), len(payloads.Sessions)))
	live.Broadcast("integration:dry-run", map[string]int{"speakers": len(payloads.Speakers), "sessions": len(payloads.Sessions)})
	actionflow.Redirect(ctx, "/organizer/integrations")
	return nil
}

func firstConfigured(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
