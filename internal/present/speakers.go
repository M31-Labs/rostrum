package present

import (
	"fmt"
	"sort"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
)

func Speakers(state domain.State, query string) map[string]any {
	query = strings.ToLower(strings.TrimSpace(query))
	rows := make([]map[string]any, 0, len(state.Speakers))
	readyCount := 0
	for _, speaker := range state.Speakers {
		if query != "" && !strings.Contains(strings.ToLower(speaker.Name()+" "+speaker.Company+" "+speaker.Email), query) {
			continue
		}
		tasks := state.SpeakerTasks(speaker.ID)
		outstanding := state.OutstandingTaskCount(speaker.ID)
		complete := len(tasks) - outstanding
		readiness := Percent(complete, len(tasks))
		if readiness == 100 && len(tasks) > 0 {
			readyCount++
		}
		submissions := 0
		sessions := 0
		for _, submission := range state.Submissions {
			if containsID(submission.SpeakerIDs, speaker.ID) {
				submissions++
			}
		}
		for _, session := range state.Sessions {
			if containsID(session.SpeakerIDs, speaker.ID) {
				sessions++
			}
		}
		profileFields := []string{speaker.Role, speaker.Company, speaker.Biography, speaker.City}
		profileComplete := 0
		for _, value := range profileFields {
			if strings.TrimSpace(value) != "" {
				profileComplete++
			}
		}
		rows = append(rows, map[string]any{
			"id":              speaker.ID,
			"name":            speaker.Name(),
			"initials":        speaker.Initials(),
			"email":           speaker.Email,
			"role":            speaker.Role,
			"company":         speaker.Company,
			"city":            speaker.City,
			"readiness":       readiness,
			"readinessStyle":  fmt.Sprintf("%d%%", readiness),
			"readinessLabel":  readinessLabel(readiness, len(tasks)),
			"outstanding":     outstanding,
			"taskCount":       len(tasks),
			"profileComplete": Percent(profileComplete, len(profileFields)),
			"submissions":     submissions,
			"sessions":        sessions,
			"portalURL":       "/portal/" + speaker.ID,
			"updated":         speaker.UpdatedAt.Format("Jan 02 · 15:04"),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["name"].(string) < rows[j]["name"].(string) })

	return map[string]any{
		"section":     "speakers",
		"rows":        rows,
		"resultCount": len(rows),
		"totalCount":  len(state.Speakers),
		"readyCount":  readyCount,
		"query":       query,
	}
}

func containsID(values []string, id string) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}

func readinessLabel(percent, tasks int) string {
	if tasks == 0 {
		return "No tasks assigned"
	}
	if percent == 100 {
		return "Ready"
	}
	if percent >= 50 {
		return "In progress"
	}
	return "Needs attention"
}
