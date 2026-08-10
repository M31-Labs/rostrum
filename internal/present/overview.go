package present

import (
	"fmt"

	"github.com/m31-labs/rostrum/internal/domain"
)

func Overview(state domain.State) map[string]any {
	statusOrder := []string{
		domain.SubmissionPending,
		domain.SubmissionAcceptedQueue,
		domain.SubmissionAccepted,
		domain.SubmissionDeclineQueue,
		domain.SubmissionDeclined,
	}
	statusCounts := make(map[string]int)
	for _, submission := range state.Submissions {
		statusCounts[submission.Status]++
	}
	pipeline := make([]map[string]any, 0, len(statusOrder))
	for _, status := range statusOrder {
		count := statusCounts[status]
		pipeline = append(pipeline, map[string]any{
			"label": StatusLabel(status),
			"count": count,
			"width": fmt.Sprintf("%d%%", Percent(count, len(state.Submissions))),
			"tone":  StatusTone(status),
		})
	}

	totalAssignments := 0
	completedAssignments := 0
	for _, task := range state.Tasks {
		totalAssignments += len(task.AssignedSpeakerIDs)
		for _, speakerID := range task.AssignedSpeakerIDs {
			completion, found := completion(state, task.ID, speakerID)
			if found && (completion.Status == domain.TaskApproved || completion.Status == domain.TaskSubmitted) {
				completedAssignments++
			}
		}
	}

	completedEvaluations := len(state.Evaluations)
	expectedEvaluations := 0
	for _, plan := range state.ReviewPlans {
		expectedEvaluations += len(plan.SubmissionIDs) * plan.EvaluationsPerItem
	}

	conflicts := domain.DetectConflicts(state.Sessions)
	hardConflicts := 0
	for _, conflict := range conflicts {
		if conflict.Severity == domain.SeverityHard {
			hardConflicts++
		}
	}

	nextSessions := make([]map[string]any, 0, 4)
	for index, session := range SortedSessions(state.Sessions) {
		if index == 4 {
			break
		}
		nextSessions = append(nextSessions, map[string]any{
			"time":     TimeRange(session.StartsAt, session.EndsAt),
			"title":    session.Title,
			"room":     RoomName(state, session.RoomID),
			"track":    TrackName(state, session.TrackID),
			"speakers": SpeakerNames(state, session.SpeakerIDs),
			"status":   StatusLabel(session.Status),
			"tone":     StatusTone(session.Status),
		})
	}

	attention := []map[string]any{
		{"tone": "critical", "value": hardConflicts, "title": "hard schedule conflicts", "detail": "Speaker and room collisions must be cleared before publication.", "href": "/organizer/agenda"},
		{"tone": "accent", "value": totalAssignments - completedAssignments, "title": "outstanding speaker tasks", "detail": "Profile, headshot, agreement, AV, and slide requests are tracked live.", "href": "/organizer/portal"},
		{"tone": "neutral", "value": expectedEvaluations - completedEvaluations, "title": "reviews still due", "detail": "Human and AI-assisted scores share the same weighted rubric.", "href": "/organizer/review"},
	}

	return map[string]any{
		"section": "overview",
		"event": map[string]any{
			"name":     state.Event.Name,
			"phase":    "Program build",
			"dates":    state.Event.StartsAt.Format("Jan 02") + "–" + state.Event.EndsAt.Format("02, 2006"),
			"location": state.Event.Location,
		},
		"metrics": []map[string]any{
			{"label": "Submissions", "value": len(state.Submissions), "detail": fmt.Sprintf("%d routed automatically", len(state.Submissions)), "href": "/organizer/submissions"},
			{"label": "Review coverage", "value": fmt.Sprintf("%d%%", Percent(completedEvaluations, expectedEvaluations)), "detail": fmt.Sprintf("%d of %d evaluations", completedEvaluations, expectedEvaluations), "href": "/organizer/review"},
			{"label": "Speaker readiness", "value": fmt.Sprintf("%d%%", Percent(completedAssignments, totalAssignments)), "detail": fmt.Sprintf("%d tasks complete", completedAssignments), "href": "/organizer/portal"},
			{"label": "Schedule health", "value": len(conflicts), "detail": fmt.Sprintf("%d hard · %d warnings", hardConflicts, len(conflicts)-hardConflicts), "href": "/organizer/agenda"},
		},
		"pipeline":     pipeline,
		"attention":    attention,
		"nextSessions": nextSessions,
		"live": map[string]any{
			"endpoint": "/live",
			"updated":  state.UpdatedAt.Format("15:04:05 UTC"),
		},
	}
}

func completion(state domain.State, taskID, speakerID string) (domain.TaskCompletion, bool) {
	for _, item := range state.TaskCompletions {
		if item.TaskID == taskID && item.SpeakerID == speakerID {
			return item, true
		}
	}
	return domain.TaskCompletion{}, false
}
