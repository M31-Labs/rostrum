package present

import (
	"fmt"
	"sort"

	"github.com/m31-labs/rostrum/internal/domain"
)

// SubmissionDetail builds the full single-submission view for
// /organizer/submissions/{id}: the title, the full abstract, every posted
// answer, the routing and rule trace, every evaluation across every review
// plan, and the session the submission became once accepted (spine SP-1).
//
// It reports an error when no submission with that ID exists, so the caller
// can render a friendly not-found state rather than a raw 500 (matching the
// loadPortal idiom in app/portal/page.server.go).
func SubmissionDetail(state domain.State, id string) (map[string]any, error) {
	submission, found := state.Submission(id)
	if !found {
		return nil, fmt.Errorf("submission %s not found", id)
	}

	speakers := make([]map[string]any, 0, len(submission.SpeakerIDs))
	for _, speakerID := range submission.SpeakerIDs {
		speaker, found := state.Speaker(speakerID)
		if !found {
			continue
		}
		speakers = append(speakers, map[string]any{
			"id":        speaker.ID,
			"name":      speaker.Name(),
			"initials":  speaker.Initials(),
			"role":      speaker.Role,
			"company":   speaker.Company,
			"email":     speaker.Email,
			"portalURL": "/portal/" + speaker.ID,
		})
	}

	statuses := make([]map[string]string, 0, len(domain.SubmissionStatuses))
	for _, item := range domain.SubmissionStatuses {
		statuses = append(statuses, map[string]string{"value": item, "label": StatusLabel(item)})
	}

	var linkedSession map[string]any
	for _, candidate := range state.Sessions {
		if candidate.SubmissionID != id {
			continue
		}
		linkedSession = map[string]any{
			"id":     candidate.ID,
			"title":  candidate.Title,
			"status": StatusLabel(candidate.Status),
			"tone":   StatusTone(candidate.Status),
			"when":   TimeRange(candidate.StartsAt, candidate.EndsAt),
		}
		break
	}

	evaluations := submissionEvaluationRows(state, id)
	answers := submissionAnswerRows(state, *submission)

	return map[string]any{
		"section":         "submissions",
		"found":           true,
		"id":              submission.ID,
		"title":           submission.Title,
		"abstract":        submission.Abstract,
		"format":          submission.Format,
		"category":        CategoryName(state, submission.CategoryID),
		"level":           submission.Level,
		"status":          StatusLabel(submission.Status),
		"statusValue":     submission.Status,
		"tone":            StatusTone(submission.Status),
		"statusOptions":   statuses,
		"canAccept":       submission.Status != domain.SubmissionAccepted,
		"queue":           submission.RoutedQueue,
		"owner":           submission.RoutedOwner,
		"ruleTrace":       submission.RuleTrace,
		"submitted":       DateTime(submission.SubmittedAt),
		"speakers":        speakers,
		"speakerCount":    len(speakers),
		"answers":         answers,
		"answerCount":     len(answers),
		"evaluations":     evaluations,
		"evaluationCount": len(evaluations),
		"session":         linkedSession,
		"hasSession":      linkedSession != nil,
	}, nil
}

// submissionAnswerRows labels each posted schema answer with its form
// field's label, when the submission's originating form still declares that
// field, and falls back to the raw key otherwise. Full field metadata lands
// with FB-1; until then this is the best available label.
func submissionAnswerRows(state domain.State, submission domain.Submission) []map[string]any {
	labels := map[string]string{}
	if form, found := state.Form(submission.FormID); found {
		for _, field := range form.Fields {
			labels[field.ID] = field.Label
		}
	}
	keys := make([]string, 0, len(submission.Answers))
	for key := range submission.Answers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		label := labels[key]
		if label == "" {
			label = StatusLabel(key)
		}
		rows = append(rows, map[string]any{"label": label, "value": submission.Answers[key]})
	}
	return rows
}

// submissionEvaluationRows lists every evaluation recorded for the
// submission, across every review plan, each labeled with its plan name,
// reviewer, per-criterion scores, and recommendation. It is a full audit
// trail of every human review, unfiltered by review round or coverage.
func submissionEvaluationRows(state domain.State, submissionID string) []map[string]any {
	rows := make([]map[string]any, 0)
	for _, evaluation := range state.Evaluations {
		if evaluation.SubmissionID != submissionID {
			continue
		}
		plan, planFound := state.ReviewPlan(evaluation.PlanID)
		planName := evaluation.PlanID
		scores := make([]map[string]any, 0, len(evaluation.Scores))
		if planFound {
			planName = plan.Name
			for _, criterion := range plan.Criteria {
				value, ok := evaluation.Scores[criterion.ID]
				if !ok {
					continue
				}
				scores = append(scores, map[string]any{
					"name":  criterion.Name,
					"value": Score(value),
					"max":   fmt.Sprintf("%.0f", criterion.MaxScore),
				})
			}
		}
		reviewerName := "Unknown reviewer"
		for _, reviewer := range state.Reviewers {
			if reviewer.ID == evaluation.ReviewerID {
				reviewerName = reviewer.Name
				break
			}
		}
		rows = append(rows, map[string]any{
			"id":             evaluation.ID,
			"planName":       planName,
			"reviewer":       reviewerName,
			"scores":         scores,
			"comments":       evaluation.Comments,
			"recommendation": StatusLabel(evaluation.Recommendation),
			"updated":        DateTime(evaluation.UpdatedAt),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		left, right := rows[i]["planName"].(string), rows[j]["planName"].(string)
		if left == right {
			return rows[i]["reviewer"].(string) < rows[j]["reviewer"].(string)
		}
		return left < right
	})
	return rows
}
