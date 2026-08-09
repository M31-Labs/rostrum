package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Conflict struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Severity       string   `json:"severity"`
	SessionIDs     []string `json:"sessionIds"`
	SpeakerIDs     []string `json:"speakerIds"`
	RoomID         string   `json:"roomId"`
	TrackID        string   `json:"trackId"`
	OverlapMinutes int      `json:"overlapMinutes"`
	Message        string   `json:"message"`
	Rule           string   `json:"rule"`
}

const (
	ConflictSpeaker = "speaker"
	ConflictRoom    = "room"
	ConflictTrack   = "track"
	SeverityHard    = "hard"
	SeverityWarning = "warning"
)

func DetectConflicts(sessions []Session) []Conflict {
	conflicts := make([]Conflict, 0)
	for leftIndex := 0; leftIndex < len(sessions); leftIndex++ {
		left := sessions[leftIndex]
		if !left.Scheduled() || left.Status == "cancelled" {
			continue
		}
		for rightIndex := leftIndex + 1; rightIndex < len(sessions); rightIndex++ {
			right := sessions[rightIndex]
			if !right.Scheduled() || right.Status == "cancelled" || !overlaps(left, right) {
				continue
			}
			overlapMinutes := overlapDuration(left, right)
			sharedSpeakers := intersection(left.SpeakerIDs, right.SpeakerIDs)
			if len(sharedSpeakers) > 0 {
				conflicts = append(conflicts, Conflict{
					ID:             conflictID(ConflictSpeaker, left.ID, right.ID),
					Kind:           ConflictSpeaker,
					Severity:       SeverityHard,
					SessionIDs:     []string{left.ID, right.ID},
					SpeakerIDs:     sharedSpeakers,
					OverlapMinutes: overlapMinutes,
					Message:        fmt.Sprintf("A speaker is scheduled in two sessions for %d overlapping minutes.", overlapMinutes),
					Rule:           "SpeakerDoubleBooking",
				})
			}
			if left.RoomID != "" && left.RoomID == right.RoomID {
				conflicts = append(conflicts, Conflict{
					ID:             conflictID(ConflictRoom, left.ID, right.ID),
					Kind:           ConflictRoom,
					Severity:       SeverityHard,
					SessionIDs:     []string{left.ID, right.ID},
					RoomID:         left.RoomID,
					OverlapMinutes: overlapMinutes,
					Message:        fmt.Sprintf("Two sessions occupy the same room for %d overlapping minutes.", overlapMinutes),
					Rule:           "RoomDoubleBooking",
				})
			}
			if left.TrackID != "" && left.TrackID == right.TrackID {
				conflicts = append(conflicts, Conflict{
					ID:             conflictID(ConflictTrack, left.ID, right.ID),
					Kind:           ConflictTrack,
					Severity:       SeverityWarning,
					SessionIDs:     []string{left.ID, right.ID},
					TrackID:        left.TrackID,
					OverlapMinutes: overlapMinutes,
					Message:        fmt.Sprintf("Two sessions in the same track overlap for %d minutes.", overlapMinutes),
					Rule:           "TrackOverlapWarning",
				})
			}
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Severity != conflicts[j].Severity {
			return conflicts[i].Severity < conflicts[j].Severity
		}
		return conflicts[i].ID < conflicts[j].ID
	})
	return conflicts
}

func ConflictsForSession(sessions []Session, sessionID string) []Conflict {
	all := DetectConflicts(sessions)
	filtered := make([]Conflict, 0)
	for _, conflict := range all {
		if contains(conflict.SessionIDs, sessionID) {
			filtered = append(filtered, conflict)
		}
	}
	return filtered
}

func overlaps(left, right Session) bool {
	return left.StartsAt.Before(right.EndsAt) && right.StartsAt.Before(left.EndsAt)
}

func overlapDuration(left, right Session) int {
	start := left.StartsAt
	if right.StartsAt.After(start) {
		start = right.StartsAt
	}
	end := left.EndsAt
	if right.EndsAt.Before(end) {
		end = right.EndsAt
	}
	minutes := int(end.Sub(start).Round(time.Minute) / time.Minute)
	if minutes < 0 {
		return 0
	}
	return minutes
}

func intersection(left, right []string) []string {
	set := make(map[string]struct{}, len(left))
	for _, value := range left {
		set[value] = struct{}{}
	}
	shared := make([]string, 0)
	for _, value := range right {
		if _, ok := set[value]; ok {
			shared = append(shared, value)
		}
	}
	sort.Strings(shared)
	return shared
}

func conflictID(kind, left, right string) string {
	ids := []string{left, right}
	sort.Strings(ids)
	return strings.Join([]string{kind, ids[0], ids[1]}, ":")
}
