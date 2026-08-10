package present

import (
	"sort"
	"strings"

	"github.com/m31-labs/rostrum/internal/domain"
)

func Submissions(state domain.State, query, status, category string) map[string]any {
	query = strings.ToLower(strings.TrimSpace(query))
	rows := make([]map[string]any, 0, len(state.Submissions))
	for _, submission := range state.Submissions {
		if status != "" && status != "all" && submission.Status != status {
			continue
		}
		if category != "" && category != "all" && submission.CategoryID != category {
			continue
		}
		speakers := SpeakerNames(state, submission.SpeakerIDs)
		if query != "" && !strings.Contains(strings.ToLower(submission.Title+" "+speakers), query) {
			continue
		}
		rows = append(rows, map[string]any{
			"id":          submission.ID,
			"title":       submission.Title,
			"speaker":     speakers,
			"initials":    Initials(state, submission.SpeakerIDs[0]),
			"category":    CategoryName(state, submission.CategoryID),
			"format":      submission.Format,
			"level":       submission.Level,
			"status":      StatusLabel(submission.Status),
			"statusValue": submission.Status,
			"tone":        StatusTone(submission.Status),
			"queue":       submission.RoutedQueue,
			"owner":       submission.RoutedOwner,
			"submitted":   submission.SubmittedAt.Format("Jan 02"),
			"trace":       strings.Join(submission.RuleTrace, " → "),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["title"].(string) < rows[j]["title"].(string) })

	statuses := make([]map[string]string, 0, len(domain.SubmissionStatuses)+1)
	statuses = append(statuses, map[string]string{"value": "all", "label": "All statuses"})
	for _, item := range domain.SubmissionStatuses {
		statuses = append(statuses, map[string]string{"value": item, "label": StatusLabel(item)})
	}
	categories := []map[string]string{{"value": "all", "label": "All categories"}}
	for _, item := range state.Event.Categories {
		categories = append(categories, map[string]string{"value": item.ID, "label": item.Name})
	}

	return map[string]any{
		"section":     "submissions",
		"rows":        rows,
		"resultCount": len(rows),
		"totalCount":  len(state.Submissions),
		"filters": map[string]any{
			"query":      query,
			"status":     status,
			"category":   category,
			"statuses":   statuses,
			"categories": categories,
		},
		"statusOptions": statuses[1:],
	}
}
