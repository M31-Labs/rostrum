package present

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func SpeakerName(state domain.State, id string) string {
	for _, speaker := range state.Speakers {
		if speaker.ID == id {
			return speaker.Name()
		}
	}
	return "Unassigned"
}

func SpeakerNames(state domain.State, ids []string) string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, SpeakerName(state, id))
	}
	return strings.Join(names, ", ")
}

func Initials(state domain.State, id string) string {
	for _, speaker := range state.Speakers {
		if speaker.ID == id {
			return speaker.Initials()
		}
	}
	return "—"
}

func TrackName(state domain.State, id string) string {
	if track, ok := state.Track(id); ok {
		return track.Name
	}
	return "Unassigned"
}

func RoomName(state domain.State, id string) string {
	if room, ok := state.Room(id); ok {
		return room.Name
	}
	return "Unassigned"
}

func CategoryName(state domain.State, id string) string {
	if category, ok := state.Category(id); ok {
		return category.Name
	}
	return "Uncategorized"
}

func StatusLabel(status string) string {
	return strings.NewReplacer("_", " ", "-", " ").Replace(strings.Title(status))
}

func StatusTone(status string) string {
	switch status {
	case "accepted", "approved", "sent", "published", "complete", "open":
		return "positive"
	case "declined", "failed", "cancelled":
		return "critical"
	case "accepted_queue", "submitted", "scheduled", "queued", "dry-run":
		return "accent"
	default:
		return "neutral"
	}
}

func DateTime(value time.Time) string {
	if value.IsZero() {
		return "Not scheduled"
	}
	return value.Format("Mon, Jan 02 · 15:04")
}

func TimeRange(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return "Unscheduled"
	}
	return start.Format("15:04") + "–" + end.Format("15:04")
}

func Percent(part, total int) int {
	if total == 0 {
		return 0
	}
	return int(float64(part)/float64(total)*100 + 0.5)
}

func Score(value float64) string {
	return fmt.Sprintf("%.1f", value)
}

func SortedSessions(sessions []domain.Session) []domain.Session {
	result := append([]domain.Session(nil), sessions...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartsAt.Equal(result[j].StartsAt) {
			return result[i].Title < result[j].Title
		}
		return result[i].StartsAt.Before(result[j].StartsAt)
	})
	return result
}
