// Package airtable projects Rostrum's canonical program into Airtable. It is
// deliberately a one-way, explicit adapter: JSON, SQLite, or Postgres remain
// authoritative; Airtable receives idempotent records from the durable
// workspace outbox and never supplies state back to Rostrum.
package airtable

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

const (
	IntegrationID = "integration_airtable"

	SpeakerKind = "airtable.speaker"
	SessionKind = "airtable.session"

	RostrumIDField = "Rostrum ID"
	SchemaField    = "Rostrum Schema"

	defaultAPIBaseURL = "https://api.airtable.com/v0"
	maxBatchRecords   = 10
	baseRequestGap    = 210 * time.Millisecond
)

// Config contains only process-local connector settings. Tokens deliberately
// never enter domain.State, archive exports, the audit summary, or UI props.
type Config struct {
	Token         string
	BaseID        string
	SpeakersTable string
	SessionsTable string
	APIBaseURL    string
	HTTPClient    *http.Client
}

// FromEnv reads Airtable's Personal Access Token configuration. API keys are
// intentionally not supported: Airtable retired them in favor of PATs.
func FromEnv() Config {
	return Config{
		Token:         strings.TrimSpace(os.Getenv("AIRTABLE_PAT")),
		BaseID:        strings.TrimSpace(os.Getenv("AIRTABLE_BASE_ID")),
		SpeakersTable: configuredName("AIRTABLE_SPEAKERS_TABLE", "Speakers"),
		SessionsTable: configuredName("AIRTABLE_SESSIONS_TABLE", "Sessions"),
		APIBaseURL:    configuredName("AIRTABLE_API_BASE_URL", defaultAPIBaseURL),
	}
}

func configuredName(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// Validate gives a safe, operator-actionable error before any network call.
func (config Config) Validate() error {
	if strings.TrimSpace(config.Token) == "" {
		return fmt.Errorf("AIRTABLE_PAT is not configured")
	}
	if strings.TrimSpace(config.BaseID) == "" {
		return fmt.Errorf("AIRTABLE_BASE_ID is not configured")
	}
	if strings.TrimSpace(config.SpeakersTable) == "" || strings.TrimSpace(config.SessionsTable) == "" {
		return fmt.Errorf("Airtable speaker and session table names are required")
	}
	base := strings.TrimSpace(config.APIBaseURL)
	if base == "" {
		base = defaultAPIBaseURL
	}
	parsed, err := url.ParseRequestURI(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("AIRTABLE_API_BASE_URL must be an absolute URL")
	}
	if parsed.Scheme != "https" && !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("AIRTABLE_API_BASE_URL must use https outside a loopback test endpoint")
	}
	return nil
}

// Configured is suitable for UI state. It requires a complete configuration,
// not merely a token, so a partial deployment does not advertise live sync.
func (config Config) Configured() bool {
	return config.Validate() == nil
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// Projection is a canonical record ready for durable outbox serialization.
// Fields use a deliberately small, documented schema so the adapter does not
// pretend to be a general-purpose Airtable database.
type Projection struct {
	Kind     string
	EntityID string
	Fields   map[string]any
}

type payload struct {
	Fields map[string]any `json:"fields"`
}

// BuildProjections emits scheduled, non-cancelled sessions plus speakers who
// are either scheduled or accepted into the program queue. That lets an
// operations base begin collecting speaker details before every accepted talk
// has a time slot. Speaker uploads are not projected as Airtable attachments;
// Rostrum's operator-owned file store remains the blob authority.
func BuildProjections(state domain.State) []Projection {
	activeSpeakers := map[string]struct{}{}
	sessions := make([]domain.Session, 0, len(state.Sessions))
	for _, item := range state.Sessions {
		if !item.Scheduled() || item.Status == "cancelled" {
			continue
		}
		sessions = append(sessions, item)
		for _, speakerID := range item.SpeakerIDs {
			activeSpeakers[speakerID] = struct{}{}
		}
	}
	for _, submission := range state.Submissions {
		if submission.Status != domain.SubmissionAccepted && submission.Status != domain.SubmissionAcceptedQueue {
			continue
		}
		for _, speakerID := range submission.SpeakerIDs {
			activeSpeakers[speakerID] = struct{}{}
		}
	}
	sort.Slice(sessions, func(left, right int) bool { return sessions[left].ID < sessions[right].ID })

	speakers := make([]domain.Speaker, 0, len(activeSpeakers))
	for _, speaker := range state.Speakers {
		if _, active := activeSpeakers[speaker.ID]; active {
			speakers = append(speakers, speaker)
		}
	}
	sort.Slice(speakers, func(left, right int) bool { return speakers[left].ID < speakers[right].ID })

	result := make([]Projection, 0, len(speakers)+len(sessions))
	for _, speaker := range speakers {
		fields := map[string]any{
			RostrumIDField: speaker.ID,
			SchemaField:    domain.CurrentSchemaVersion,
			"Name":         speaker.Name(),
			"Email":        speaker.Email,
		}
		putString(fields, "Role", speaker.Role)
		putString(fields, "Company", speaker.Company)
		putString(fields, "Biography", speaker.Biography)
		putString(fields, "Website", speaker.WebsiteURL)
		putString(fields, "LinkedIn", speaker.LinkedInURL)
		result = append(result, Projection{Kind: SpeakerKind, EntityID: speaker.ID, Fields: fields})
	}
	for _, item := range sessions {
		room, _ := state.Room(item.RoomID)
		track, _ := state.Track(item.TrackID)
		fields := map[string]any{
			RostrumIDField: item.ID,
			SchemaField:    domain.CurrentSchemaVersion,
			"Title":        item.Title,
			"Starts At":    item.StartsAt.UTC().Format(time.RFC3339),
			"Ends At":      item.EndsAt.UTC().Format(time.RFC3339),
		}
		putString(fields, "Description", item.Description)
		putString(fields, "Room", room.Name)
		putString(fields, "Track", track.Name)
		if len(item.SpeakerIDs) > 0 {
			fields["Speaker IDs"] = strings.Join(sortedStrings(item.SpeakerIDs), ", ")
		}
		result = append(result, Projection{Kind: SessionKind, EntityID: item.ID, Fields: fields})
	}
	return result
}

func putString(fields map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		fields[key] = value
	}
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

// Enqueue stores a projection in the canonical outbox. A record is keyed by
// integration, kind, and Rostrum entity ID: unchanged delivered records are
// not re-sent, while a changed payload resets that record to pending. This
// makes re-running "sync now" cheap and makes a failed state durable.
func Enqueue(state *domain.State, projections []Projection, now time.Time) (int, error) {
	if state == nil {
		return 0, fmt.Errorf("workspace state is required")
	}
	now = now.UTC()
	addedOrChanged := 0
	for _, projection := range projections {
		if projection.Kind != SpeakerKind && projection.Kind != SessionKind {
			return addedOrChanged, fmt.Errorf("unsupported Airtable projection kind %q", projection.Kind)
		}
		if strings.TrimSpace(projection.EntityID) == "" {
			return addedOrChanged, fmt.Errorf("Airtable projection entity ID is required")
		}
		encoded, err := json.Marshal(payload{Fields: projection.Fields})
		if err != nil {
			return addedOrChanged, fmt.Errorf("encode %s projection %s: %w", projection.Kind, projection.EntityID, err)
		}
		content := string(encoded)
		key := idempotencyKey(projection.Kind, projection.EntityID, content)
		found := -1
		for index := range state.SyncOutbox {
			item := state.SyncOutbox[index]
			if item.Integration == IntegrationID && item.Kind == projection.Kind && item.EntityID == projection.EntityID {
				found = index
				break
			}
		}
		if found >= 0 {
			item := &state.SyncOutbox[found]
			if item.Payload == content {
				continue
			}
			item.Payload = content
			item.IdempotencyKey = key
			item.Attempts = 0
			item.AvailableAt = now
			item.DeliveredAt = time.Time{}
			item.LastError = ""
			addedOrChanged++
			continue
		}
		state.SyncOutbox = append(state.SyncOutbox, domain.SyncOutboxItem{
			ID:             domain.NewID("outbox"),
			Integration:    IntegrationID,
			Kind:           projection.Kind,
			EntityID:       projection.EntityID,
			Payload:        content,
			IdempotencyKey: key,
			AvailableAt:    now,
			CreatedAt:      now,
		})
		addedOrChanged++
	}
	return addedOrChanged, nil
}

func idempotencyKey(kind, entityID, body string) string {
	sum := sha256.Sum256([]byte("rostrum.airtable.v1\x1f" + kind + "\x1f" + entityID + "\x1f" + body))
	return hex.EncodeToString(sum[:])
}

// Pending returns only due, undelivered Airtable outbox items in deterministic
// order. Backoff remains persisted in AvailableAt, so a request handler never
// sleeps for a provider retry window.
func Pending(state domain.State, now time.Time) []domain.SyncOutboxItem {
	now = now.UTC()
	items := make([]domain.SyncOutboxItem, 0)
	for _, item := range state.SyncOutbox {
		if item.Integration != IntegrationID || !item.DeliveredAt.IsZero() || item.AvailableAt.After(now) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].Kind == items[right].Kind {
			return items[left].EntityID < items[right].EntityID
		}
		return items[left].Kind < items[right].Kind
	})
	return items
}

// SyncResult holds exactly the outbox transitions a caller must persist after
// network delivery. Keeping it separate makes it impossible to perform an
// external request while a StateStore mutation lock is held.
type SyncResult struct {
	Delivered []string
	Failed    map[string]string
	Requests  int
	Speakers  int
	Sessions  int
}

// Sync writes due outbox items with Airtable's PATCH + performUpsert contract.
// It batches at ten records, stays below the documented five-requests-per-
// second base limit, and stops on the first remote failure. The caller then
// persists Result via ApplyResult; any crash before that point safely retries
// an upsert against the stable "Rostrum ID" field.
func Sync(ctx context.Context, config Config, items []domain.SyncOutboxItem) (SyncResult, error) {
	if err := config.Validate(); err != nil {
		return SyncResult{}, err
	}
	groups := map[string][]domain.SyncOutboxItem{SpeakerKind: {}, SessionKind: {}}
	for _, item := range items {
		if item.Integration != IntegrationID || !item.DeliveredAt.IsZero() {
			continue
		}
		if _, known := groups[item.Kind]; !known {
			continue
		}
		groups[item.Kind] = append(groups[item.Kind], item)
	}
	client := config.httpClient()
	result := SyncResult{Failed: map[string]string{}}
	firstRequest := true
	for _, group := range []struct {
		kind  string
		table string
	}{
		{kind: SpeakerKind, table: config.SpeakersTable},
		{kind: SessionKind, table: config.SessionsTable},
	} {
		for start := 0; start < len(groups[group.kind]); start += maxBatchRecords {
			end := start + maxBatchRecords
			if end > len(groups[group.kind]) {
				end = len(groups[group.kind])
			}
			batch := groups[group.kind][start:end]
			records, err := decodeRecords(batch)
			if err != nil {
				markFailed(&result, batch, err)
				return result, err
			}
			if !firstRequest {
				if err := waitRequestGap(ctx); err != nil {
					markFailed(&result, batch, err)
					return result, err
				}
			}
			firstRequest = false
			if err := config.upsert(ctx, client, group.table, records); err != nil {
				markFailed(&result, batch, err)
				return result, err
			}
			result.Requests++
			for _, item := range batch {
				result.Delivered = append(result.Delivered, item.ID)
			}
			if group.kind == SpeakerKind {
				result.Speakers += len(batch)
			} else {
				result.Sessions += len(batch)
			}
		}
	}
	return result, nil
}

func (config Config) httpClient() *http.Client {
	if config.HTTPClient != nil {
		return config.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func waitRequestGap(ctx context.Context) error {
	timer := time.NewTimer(baseRequestGap)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type apiRecord struct {
	Fields map[string]any `json:"fields"`
}

func decodeRecords(items []domain.SyncOutboxItem) ([]apiRecord, error) {
	records := make([]apiRecord, 0, len(items))
	for _, item := range items {
		var decoded payload
		if err := json.Unmarshal([]byte(item.Payload), &decoded); err != nil {
			return nil, fmt.Errorf("decode Airtable outbox item %s: %w", item.ID, err)
		}
		if len(decoded.Fields) == 0 || strings.TrimSpace(fmt.Sprint(decoded.Fields[RostrumIDField])) == "" {
			return nil, fmt.Errorf("Airtable outbox item %s is missing %q", item.ID, RostrumIDField)
		}
		records = append(records, apiRecord{Fields: decoded.Fields})
	}
	return records, nil
}

func (config Config) upsert(ctx context.Context, client *http.Client, table string, records []apiRecord) error {
	body, err := json.Marshal(struct {
		Records       []apiRecord `json:"records"`
		PerformUpsert struct {
			FieldsToMergeOn []string `json:"fieldsToMergeOn"`
		} `json:"performUpsert"`
		Typecast bool `json:"typecast"`
	}{
		Records: records,
		PerformUpsert: struct {
			FieldsToMergeOn []string `json:"fieldsToMergeOn"`
		}{FieldsToMergeOn: []string{RostrumIDField}},
		Typecast: true,
	})
	if err != nil {
		return fmt.Errorf("encode Airtable upsert: %w", err)
	}
	base := strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	if base == "" {
		base = defaultAPIBaseURL
	}
	endpoint := base + "/" + url.PathEscape(strings.TrimSpace(config.BaseID)) + "/" + url.PathEscape(strings.TrimSpace(table))
	request, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Airtable upsert request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(config.Token))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Rostrum/1 Airtable projection")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Airtable request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		if response.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("Airtable rate limited the base (429); queued records will retry after backoff: %s", strings.TrimSpace(string(message)))
		}
		return fmt.Errorf("Airtable %s returned %s: %s", endpoint, response.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func markFailed(result *SyncResult, items []domain.SyncOutboxItem, err error) {
	if result.Failed == nil {
		result.Failed = map[string]string{}
	}
	for _, item := range items {
		result.Failed[item.ID] = strings.TrimSpace(err.Error())
	}
}

// ApplyResult updates only durable outbox state. It does no I/O and should be
// called from an audited StateStore mutation after Sync returns.
func ApplyResult(state *domain.State, result SyncResult, now time.Time) {
	if state == nil {
		return
	}
	now = now.UTC()
	delivered := map[string]struct{}{}
	for _, id := range result.Delivered {
		delivered[id] = struct{}{}
	}
	for index := range state.SyncOutbox {
		item := &state.SyncOutbox[index]
		if _, ok := delivered[item.ID]; ok {
			item.Attempts++
			item.DeliveredAt = now
			item.AvailableAt = now
			item.LastError = ""
			continue
		}
		if message, failed := result.Failed[item.ID]; failed {
			item.Attempts++
			item.LastError = truncate(message, 500)
			item.AvailableAt = retryAt(now, item.Attempts)
		}
	}
}

func retryAt(now time.Time, attempts int) time.Time {
	if attempts < 1 {
		attempts = 1
	}
	delay := 30 * time.Second
	for index := 1; index < attempts && delay < 15*time.Minute; index++ {
		delay *= 2
	}
	if delay > 15*time.Minute {
		delay = 15 * time.Minute
	}
	return now.UTC().Add(delay)
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
