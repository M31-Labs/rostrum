package present

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/m31-labs/rostrum/internal/accelevents"
	"github.com/m31-labs/rostrum/internal/domain"
)

func Integrations(state domain.State) map[string]any {
	// A fresh workspace configures no integration yet; fall back to a zero
	// value so the page renders its empty state instead of panicking.
	var integration domain.Integration
	if len(state.Integrations) > 0 {
		integration = state.Integrations[0]
	}
	payloads := accelevents.BuildPayloads(state)
	sample := map[string]any{}
	if len(payloads.Sessions) > 0 && len(payloads.Speakers) > 0 {
		sample = map[string]any{"speaker": payloads.Speakers[0], "session": payloads.Sessions[0]}
	}
	sampleJSON, _ := json.MarshalIndent(sample, "", "  ")
	runs := make([]map[string]any, 0, len(state.SyncRuns))
	for index := len(state.SyncRuns) - 1; index >= 0; index-- {
		run := state.SyncRuns[index]
		runs = append(runs, map[string]any{
			"id":       run.ID,
			"mode":     StatusLabel(run.Mode),
			"status":   StatusLabel(run.Status),
			"tone":     StatusTone(run.Status),
			"speakers": run.Speakers,
			"sessions": run.Sessions,
			"when":     DateTime(run.FinishedAt),
			"summary":  run.Summary,
		})
	}
	configured := strings.TrimSpace(os.Getenv("ACCELEVENTS_API_KEY")) != "" || strings.TrimSpace(os.Getenv("ACCELEVENTS_API_TOKEN")) != ""
	credentialLabel := "Dry-run only"
	credentialTone := "status-accent"
	if configured {
		credentialLabel = "Credentials ready"
		credentialTone = "status-positive"
	}
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
		"sampleJSON": string(sampleJSON),
		"runs":       runs,
		"runCount":   len(runs),
	}
}
