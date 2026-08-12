package accelevents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

type SpeakerPayload struct {
	ExternalID  string `json:"externalId"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Email       string `json:"email"`
	JobTitle    string `json:"jobTitle,omitempty"`
	Company     string `json:"company,omitempty"`
	Biography   string `json:"biography,omitempty"`
	Website     string `json:"website,omitempty"`
	LinkedInURL string `json:"linkedinUrl,omitempty"`
}

type SessionPayload struct {
	ExternalID  string   `json:"externalId"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	StartTime   string   `json:"startTime"`
	EndTime     string   `json:"endTime"`
	Location    string   `json:"location,omitempty"`
	Track       string   `json:"track,omitempty"`
	SpeakerIDs  []string `json:"speakerExternalIds,omitempty"`
}

type Payloads struct {
	Speakers []SpeakerPayload `json:"speakers"`
	Sessions []SessionPayload `json:"sessions"`
}

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func BuildPayloads(state domain.State) Payloads {
	activeSpeakers := make(map[string]struct{})
	for _, item := range state.Sessions {
		if item.Status != "published" || !item.Scheduled() {
			continue
		}
		for _, speakerID := range item.SpeakerIDs {
			activeSpeakers[speakerID] = struct{}{}
		}
	}
	speakers := make([]SpeakerPayload, 0, len(activeSpeakers))
	for _, speaker := range state.Speakers {
		if _, found := activeSpeakers[speaker.ID]; !found {
			continue
		}
		speakers = append(speakers, SpeakerPayload{
			ExternalID: speaker.ID, FirstName: speaker.FirstName, LastName: speaker.LastName, Email: speaker.Email,
			JobTitle: speaker.Role, Company: speaker.Company, Biography: speaker.Biography, Website: speaker.WebsiteURL, LinkedInURL: speaker.LinkedInURL,
		})
	}
	sessions := make([]SessionPayload, 0, len(state.Sessions))
	for _, item := range state.Sessions {
		if !item.Scheduled() || item.Status != "published" {
			continue
		}
		room, _ := state.Room(item.RoomID)
		track, _ := state.Track(item.TrackID)
		sessions = append(sessions, SessionPayload{
			ExternalID: item.ID, Title: item.Title, Description: item.Description,
			StartTime: item.StartsAt.UTC().Format(time.RFC3339), EndTime: item.EndsAt.UTC().Format(time.RFC3339),
			Location: room.Name, Track: track.Name, SpeakerIDs: append([]string(nil), item.SpeakerIDs...),
		})
	}
	return Payloads{Speakers: speakers, Sessions: sessions}
}

func (client Client) Sync(ctx context.Context, eventURL string, payloads Payloads) error {
	if strings.TrimSpace(client.Token) == "" {
		return fmt.Errorf("Accelevents API token is not configured")
	}
	base := strings.TrimRight(client.BaseURL, "/")
	if base == "" {
		base = "https://api.accelevents.com"
	}
	if _, err := url.ParseRequestURI(base); err != nil {
		return fmt.Errorf("invalid Accelevents base URL: %w", err)
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	for _, speaker := range payloads.Speakers {
		if err := client.post(ctx, httpClient, base+"/rest/host/event/"+url.PathEscape(eventURL)+"/speaker", speaker); err != nil {
			return err
		}
	}
	for _, item := range payloads.Sessions {
		if err := client.post(ctx, httpClient, base+"/rest/host/event/"+url.PathEscape(eventURL)+"/session", item); err != nil {
			return err
		}
	}
	return nil
}

func (client Client) post(ctx context.Context, httpClient *http.Client, endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+client.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return fmt.Errorf("Accelevents %s returned %s: %s", endpoint, response.Status, strings.TrimSpace(string(message)))
	}
	return nil
}
