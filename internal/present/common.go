package present

import (
	"fmt"
	"net/url"
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

// AllowedURL reports whether raw is safe to store and later render as an
// href or in the public /api/v1/speakers JSON (SE-4). A blank value is
// allowed -- callers use it to represent "no link set" -- but any non-blank
// value must parse as an absolute URL with scheme http or https. This
// rejects javascript:, data:, and every other scheme a browser might
// execute or render unexpectedly, before the value ever reaches storage.
func AllowedURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return (scheme == "http" || scheme == "https") && parsed.Host != ""
}

// csvFormulaPrefixes lists the leading bytes a spreadsheet application
// (Excel, Google Sheets, LibreOffice Calc) treats as the start of a formula
// when it opens a CSV file. A tab or carriage return is included because
// some spreadsheet importers still evaluate a cell that begins with either
// after trimming, and both can also be used to break a naive CSV parser's
// column alignment.
const csvFormulaPrefixes = "=+-@\t\r"

// CSVSafe neutralizes formula injection (SE-5): when cell begins with a
// byte in csvFormulaPrefixes, it prefixes cell with a single quote so the
// spreadsheet application that opens the export renders the value as
// literal text instead of evaluating it as a formula. Every CSV (and any
// future XLSX) export must route each cell through this helper before
// writing it.
func CSVSafe(cell string) string {
	if cell == "" {
		return cell
	}
	if strings.ContainsRune(csvFormulaPrefixes, rune(cell[0])) {
		return "'" + cell
	}
	return cell
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
