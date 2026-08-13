package present

import (
	"fmt"
	"sort"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

func PortalOperations(state domain.State) map[string]any {
	taskRows := make([]map[string]any, 0, len(state.Tasks))
	retiredTasks := make([]map[string]any, 0)
	totalAssigned := 0
	totalComplete := 0
	totalSubmitted := 0
	totalDeclined := 0
	for _, task := range state.Tasks {
		if !task.Active() {
			retiredTasks = append(retiredTasks, map[string]any{
				"id":        task.ID,
				"title":     task.Title,
				"retiredAt": task.RetiredAt.Format("Jan 02, 2006"),
			})
			continue
		}
		approved, submitted, declined := 0, 0, 0
		assignedSpeakerIDs := make([]string, 0, len(task.AssignedSpeakerIDs))
		for _, speakerID := range task.AssignedSpeakerIDs {
			if !state.TaskAssignedToSpeaker(task, speakerID) {
				continue
			}
			assignedSpeakerIDs = append(assignedSpeakerIDs, speakerID)
			completion, found := completion(state, task.ID, speakerID)
			if !found {
				continue
			}
			switch completion.Status {
			case domain.TaskApproved:
				approved++
			case domain.TaskSubmitted:
				submitted++
			case domain.TaskDeclined:
				declined++
			}
		}
		assigned := len(assignedSpeakerIDs)
		complete := approved + submitted
		totalAssigned += assigned
		totalComplete += complete
		totalSubmitted += submitted
		totalDeclined += declined
		taskRows = append(taskRows, map[string]any{
			"id":            task.ID,
			"title":         task.Title,
			"description":   task.Description,
			"type":          StatusLabel(task.Type),
			"typeValue":     task.Type,
			"due":           task.DueAt.Format("Jan 02"),
			"dueInput":      taskInputDate(state, task.DueAt),
			"required":      task.Required,
			"acceptedOnly":  task.AcceptedOnly,
			"assignedNames": SpeakerNames(state, assignedSpeakerIDs),
			"assigned":      assigned,
			"approved":      approved,
			"submitted":     submitted,
			"declined":      declined,
			"outstanding":   assigned - complete,
			"percent":       Percent(complete, assigned),
			"percentStyle":  fmt.Sprintf("%d%%", Percent(complete, assigned)),
		})
	}

	approvals := make([]map[string]any, 0, totalSubmitted)
	for _, item := range state.TaskCompletions {
		if item.Status != domain.TaskSubmitted {
			continue
		}
		task, taskFound := state.Task(item.TaskID)
		speaker, speakerFound := state.Speaker(item.SpeakerID)
		if !taskFound || !task.Active() || !speakerFound || !state.TaskAssignedToSpeaker(*task, item.SpeakerID) {
			continue
		}
		approvals = append(approvals, map[string]any{
			"taskID":      task.ID,
			"taskTitle":   task.Title,
			"taskType":    StatusLabel(task.Type),
			"speakerID":   speaker.ID,
			"speakerName": speaker.Name(),
			"initials":    speaker.Initials(),
			"updated":     item.UpdatedAt.Format("Jan 02 · 15:04"),
			"updatedUnix": item.UpdatedAt.Unix(),
			"fileName":    item.FileName,
			"hasFile":     item.FileName != "",
			"fileURL":     "/portal-file/" + item.ID,
			"portalURL":   "/portal/" + speaker.ID,
		})
	}
	sort.Slice(approvals, func(i, j int) bool {
		return approvals[i]["updatedUnix"].(int64) > approvals[j]["updatedUnix"].(int64)
	})

	people := make([]map[string]any, 0, len(state.Speakers))
	for _, speaker := range state.Speakers {
		tasks := state.SpeakerTasks(speaker.ID)
		if len(tasks) == 0 {
			continue
		}
		outstanding := state.OutstandingTaskCount(speaker.ID)
		people = append(people, map[string]any{
			"id":          speaker.ID,
			"name":        speaker.Name(),
			"initials":    speaker.Initials(),
			"complete":    len(tasks) - outstanding,
			"total":       len(tasks),
			"outstanding": outstanding,
			"percent":     Percent(len(tasks)-outstanding, len(tasks)),
			"portalURL":   "/portal/" + speaker.ID,
		})
	}
	sort.Slice(people, func(i, j int) bool {
		if people[i]["outstanding"].(int) == people[j]["outstanding"].(int) {
			return people[i]["name"].(string) < people[j]["name"].(string)
		}
		return people[i]["outstanding"].(int) > people[j]["outstanding"].(int)
	})

	resources := make([]map[string]any, 0, len(state.Resources))
	for _, resource := range state.Resources {
		resources = append(resources, map[string]any{
			"id":      resource.ID,
			"title":   resource.Title,
			"kind":    StatusLabel(resource.Kind),
			"summary": resource.Summary,
			"order":   resource.SortOrder,
		})
	}

	speakers := make([]map[string]any, 0, len(state.Speakers))
	for _, speaker := range state.Speakers {
		speakers = append(speakers, map[string]any{
			"id":       speaker.ID,
			"name":     speaker.Name(),
			"accepted": state.SpeakerEligibleForAcceptedTasks(speaker.ID),
		})
	}
	sort.Slice(speakers, func(i, j int) bool { return speakers[i]["name"].(string) < speakers[j]["name"].(string) })
	previewPortalURL := ""
	if speakerID := FirstPortalSpeaker(state); speakerID != "" {
		previewPortalURL = "/portal/" + speakerID
	}

	return map[string]any{
		"section":   "portal",
		"workspace": WorkspaceIdentity(state),
		"metrics": []map[string]any{
			{"label": "Complete or submitted", "value": totalComplete, "detail": Percent(totalComplete, totalAssigned)},
			{"label": "Awaiting approval", "value": totalSubmitted, "detail": Percent(totalSubmitted, totalAssigned)},
			{"label": "Needs resubmission", "value": totalDeclined, "detail": Percent(totalDeclined, totalAssigned)},
			{"label": "Outstanding", "value": totalAssigned - totalComplete, "detail": Percent(totalAssigned-totalComplete, totalAssigned)},
		},
		"tasks":            taskRows,
		"hasTasks":         len(taskRows) > 0,
		"retiredTasks":     retiredTasks,
		"retiredCount":     len(retiredTasks),
		"hasRetired":       len(retiredTasks) > 0,
		"speakers":         speakers,
		"approvals":        approvals,
		"approvalCount":    len(approvals),
		"hasApprovals":     len(approvals) > 0,
		"people":           people,
		"resources":        resources,
		"previewPortalURL": previewPortalURL,
		"hasPreviewPortal": previewPortalURL != "",
	}
}

func taskInputDate(state domain.State, value time.Time) string {
	location, err := time.LoadLocation(state.Event.TimeZone)
	if err != nil {
		location = time.UTC
	}
	return value.In(location).Format("2006-01-02T15:04")
}
