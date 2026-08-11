package present

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/m31-labs/rostrum/internal/accelevents"
	"github.com/m31-labs/rostrum/internal/airtable"
	"github.com/m31-labs/rostrum/internal/domain"
)

func Integrations(state domain.State) map[string]any {
	// A fresh workspace configures no integration yet; fall back to a zero
	// value so the page renders its empty state instead of panicking.
	integration := integrationByID(state, "integration_accelevents", domain.Integration{ID: "integration_accelevents", Kind: "accelevents", Name: "Accelevents"})
	airtableIntegration := integrationByID(state, airtable.IntegrationID, domain.Integration{ID: airtable.IntegrationID, Kind: "airtable", Name: "Airtable", LastStatus: "Configure a Personal Access Token and base ID"})
	payloads := accelevents.BuildPayloads(state)
	airtableProjections := airtable.BuildProjections(state)
	airtableSpeakers, airtableSessions := projectionCounts(airtableProjections)
	sample := map[string]any{}
	if len(payloads.Sessions) > 0 && len(payloads.Speakers) > 0 {
		sample = map[string]any{"speaker": payloads.Speakers[0], "session": payloads.Sessions[0]}
	}
	sampleJSON, _ := json.MarshalIndent(sample, "", "  ")
	airtableSample := map[string]any{}
	if len(airtableProjections) > 0 {
		airtableSample = map[string]any{"record": airtableProjections[0].Fields}
	}
	airtableSampleJSON, _ := json.MarshalIndent(airtableSample, "", "  ")
	runs := make([]map[string]any, 0, len(state.SyncRuns))
	for index := len(state.SyncRuns) - 1; index >= 0; index-- {
		run := state.SyncRuns[index]
		runs = append(runs, map[string]any{
			"id":          run.ID,
			"integration": integrationName(state, run.Integration),
			"mode":        StatusLabel(run.Mode),
			"status":      StatusLabel(run.Status),
			"tone":        StatusTone(run.Status),
			"speakers":    run.Speakers,
			"sessions":    run.Sessions,
			"when":        DateTime(run.FinishedAt),
			"summary":     run.Summary,
		})
	}
	configured := strings.TrimSpace(os.Getenv("ACCELEVENTS_API_KEY")) != "" || strings.TrimSpace(os.Getenv("ACCELEVENTS_API_TOKEN")) != ""
	credentialLabel := "Dry-run only"
	credentialTone := "status-accent"
	if configured {
		credentialLabel = "Credentials ready"
		credentialTone = "status-positive"
	}
	airtableConfig := airtable.FromEnv()
	airtableConfigured := airtableConfig.Configured()
	airtableCredentialLabel := "Dry-run only"
	airtableCredentialTone := "status-accent"
	if airtableConfigured {
		airtableCredentialLabel = "Credentials ready"
		airtableCredentialTone = "status-positive"
	}
	pending, failed := outboxCounts(state)
	return map[string]any{
		"section":   "integrations",
		"demoMode":  DemoMode(),
		"workspace": WorkspaceIdentity(state),
		"integration": map[string]any{
			"id":              integration.ID,
			"name":            integration.Name,
			"kind":            integration.Kind,
			"eventURL":        integration.EventURL,
			"enabled":         integration.Enabled,
			"configured":      configured,
			"credentialLabel": credentialLabel,
			"credentialTone":  credentialTone,
			"status":          integration.LastStatus,
			"lastSync":        DateTime(integration.LastSyncAt),
			"speakerCount":    len(payloads.Speakers),
			"sessionCount":    len(payloads.Sessions),
			"direction":       "Rostrum → Accelevents",
			"speakerEndpoint": "/rest/host/event/{eventUrl}/speaker",
			"sessionEndpoint": "/rest/host/event/{eventUrl}/session",
		},
		"airtable": map[string]any{
			"id":              airtableIntegration.ID,
			"name":            airtableIntegration.Name,
			"enabled":         airtableIntegration.Enabled,
			"configured":      airtableConfigured,
			"credentialLabel": airtableCredentialLabel,
			"credentialTone":  airtableCredentialTone,
			"status":          airtableIntegration.LastStatus,
			"lastSync":        DateTime(airtableIntegration.LastSyncAt),
			"speakerCount":    airtableSpeakers,
			"sessionCount":    airtableSessions,
			"pending":         pending,
			"failed":          failed,
			"direction":       "Rostrum → Airtable",
			"speakerTable":    airtableConfig.SpeakersTable,
			"sessionTable":    airtableConfig.SessionsTable,
		},
		"sampleJSON":         string(sampleJSON),
		"airtableSampleJSON": string(airtableSampleJSON),
		"runs":               runs,
		"runCount":           len(runs),
	}
}

func integrationByID(state domain.State, id string, fallback domain.Integration) domain.Integration {
	for _, integration := range state.Integrations {
		if integration.ID == id {
			return integration
		}
	}
	return fallback
}

func integrationName(state domain.State, id string) string {
	return integrationByID(state, id, domain.Integration{Name: "External integration"}).Name
}

func projectionCounts(projections []airtable.Projection) (speakers, sessions int) {
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

func outboxCounts(state domain.State) (pending, failed int) {
	for _, item := range state.SyncOutbox {
		if item.Integration != airtable.IntegrationID || !item.DeliveredAt.IsZero() {
			continue
		}
		pending++
		if strings.TrimSpace(item.LastError) != "" {
			failed++
		}
	}
	return pending, failed
}
