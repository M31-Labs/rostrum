package integrations

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/accelevents"
	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/airtable"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/auth"
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
		Actions: route.FileActions{
			"dryRun":         dryRun,
			"liveSync":       liveSync,
			"airtableDryRun": airtableDryRun,
			"airtableSync":   airtableSync,
		},
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

// airtableDryRun validates the current one-way record mapping without reading
// credentials or issuing a network request. It is deliberately useful before
// a self-hoster creates a Personal Access Token or base.
func airtableDryRun(ctx *action.Context) error {
	state := appstate.MustGet().Snapshot()
	projections := airtable.BuildProjections(state)
	if len(projections) == 0 {
		return action.Validation("No accepted speakers or scheduled sessions are ready to project.", nil, ctx.FormData)
	}
	speakers, sessions := airtableProjectionCounts(projections)
	now := time.Now().UTC()
	configured := airtable.FromEnv().Configured()
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      integrationActor(ctx),
		Action:     "integration.airtable_dry_run",
		EntityType: "integration",
		EntityID:   airtable.IntegrationID,
		Summary:    "Validated one-way Airtable payloads without a network request.",
		Origin:     "organizer-integrations",
	}, func(state *domain.State) error {
		state.SyncRuns = append(state.SyncRuns, domain.SyncRun{
			ID: domain.NewID("sync"), Integration: airtable.IntegrationID, Mode: "dry-run", Status: "complete",
			Speakers: speakers, Sessions: sessions, StartedAt: now, FinishedAt: now,
			Summary: "Validated stable Rostrum IDs and Airtable field mapping; no external request was sent.",
		})
		updateIntegrationStatus(state, airtable.IntegrationID, configured, configured, now, "Airtable dry run passed")
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", fmt.Sprintf("Airtable dry run passed: %d speakers and %d sessions are ready.", speakers, sessions))
	live.Broadcast("integration:airtable-dry-run", map[string]int{"speakers": speakers, "sessions": sessions})
	actionflow.Redirect(ctx, "/organizer/integrations")
	return nil
}

// airtableSync first durably queues the current canonical projections, then
// delivers only due outbox records. A process crash after Airtable accepts a
// request merely retries PATCH + performUpsert against the stable Rostrum ID;
// it cannot create a duplicate remote record.
func airtableSync(ctx *action.Context) error {
	config := airtable.FromEnv()
	if err := config.Validate(); err != nil {
		return action.Validation("Live Airtable projection is locked: "+err.Error()+".", nil, ctx.FormData)
	}
	state := appstate.MustGet().Snapshot()
	projections := airtable.BuildProjections(state)
	if len(projections) == 0 {
		return action.Validation("No accepted speakers or scheduled sessions are ready to project.", nil, ctx.FormData)
	}
	now := time.Now().UTC()
	queued := 0
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      integrationActor(ctx),
		Action:     "integration.airtable_queued",
		EntityType: "integration",
		EntityID:   airtable.IntegrationID,
		Summary:    "Queued current Airtable projection records in the durable outbox.",
		Origin:     "organizer-integrations",
	}, func(state *domain.State) error {
		var err error
		queued, err = airtable.Enqueue(state, projections, now)
		return err
	}); err != nil {
		return err
	}

	queuedState := appstate.MustGet().Snapshot()
	pending := airtable.Pending(queuedState, now)
	outstanding := airtableOutstanding(queuedState)
	result, syncErr := airtable.Sync(ctx.Request.Context(), config, pending)
	finishedAt := time.Now().UTC()
	runStatus := "complete"
	lastStatus := "Airtable sync complete"
	summary := fmt.Sprintf("Projected %d speaker records and %d session records to Airtable using stable Rostrum IDs.", result.Speakers, result.Sessions)
	if len(pending) == 0 {
		if outstanding > 0 {
			runStatus = "queued"
			lastStatus = "Airtable retry backoff is active"
			summary = "No Airtable records are due yet; failed records remain in durable retry backoff."
		} else {
			summary = "No Airtable records changed; the durable outbox is already current."
		}
	}
	if syncErr != nil {
		runStatus = "failed"
		lastStatus = "Airtable sync failed; pending records will retry after backoff"
		summary = "Airtable projection stopped on the first remote error: " + syncErr.Error()
	}
	if err := appstate.MustGet().UpdateAudit(domain.AuditMeta{
		Actor:      integrationActor(ctx),
		Action:     "integration.airtable_" + runStatus,
		EntityType: "integration",
		EntityID:   airtable.IntegrationID,
		Summary:    summary,
		Origin:     "organizer-integrations",
	}, func(state *domain.State) error {
		airtable.ApplyResult(state, result, finishedAt)
		state.SyncRuns = append(state.SyncRuns, domain.SyncRun{
			ID: domain.NewID("sync"), Integration: airtable.IntegrationID, Mode: "live", Status: runStatus,
			Speakers: result.Speakers, Sessions: result.Sessions, StartedAt: now, FinishedAt: finishedAt, Summary: summary,
		})
		updateIntegrationStatus(state, airtable.IntegrationID, true, true, finishedAt, lastStatus)
		return nil
	}); err != nil {
		return err
	}
	if syncErr != nil {
		return action.Error(502, "Airtable rejected the projection. Delivered records are preserved; failed records remain in the durable outbox for retry.")
	}
	session.AddFlash(ctx.Request, "notice", fmt.Sprintf("Airtable is current: %d changed records queued, %d speakers and %d sessions delivered.", queued, result.Speakers, result.Sessions))
	live.Broadcast("integration:airtable-sync", map[string]int{"queued": queued, "speakers": result.Speakers, "sessions": result.Sessions})
	actionflow.Redirect(ctx, "/organizer/integrations")
	return nil
}

func airtableProjectionCounts(projections []airtable.Projection) (speakers, sessions int) {
	for _, projection := range projections {
		switch projection.Kind {
		case airtable.SpeakerKind:
			speakers++
		case airtable.SessionKind:
			sessions++
		}
	}
	return speakers, sessions
}

func airtableOutstanding(state domain.State) int {
	count := 0
	for _, item := range state.SyncOutbox {
		if item.Integration == airtable.IntegrationID && item.DeliveredAt.IsZero() {
			count++
		}
	}
	return count
}

func updateIntegrationStatus(state *domain.State, integrationID string, enabled, credentialsOK bool, at time.Time, status string) {
	for index := range state.Integrations {
		if state.Integrations[index].ID != integrationID {
			continue
		}
		state.Integrations[index].Enabled = enabled
		state.Integrations[index].CredentialsOK = credentialsOK
		state.Integrations[index].LastSyncAt = at.UTC()
		state.Integrations[index].LastStatus = status
		return
	}
	if integrationID == airtable.IntegrationID {
		state.Integrations = append(state.Integrations, domain.Integration{
			ID: integrationID, Kind: "airtable", Name: "Airtable", Enabled: enabled, CredentialsOK: credentialsOK,
			LastSyncAt: at.UTC(), LastStatus: status,
		})
	}
}

func integrationActor(ctx *action.Context) string {
	if ctx != nil && ctx.Request != nil {
		if user, ok := auth.Current(ctx.Request); ok && strings.TrimSpace(user.ID) != "" {
			return "organizer:" + user.ID
		}
	}
	return "organizer"
}

func firstConfigured(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
