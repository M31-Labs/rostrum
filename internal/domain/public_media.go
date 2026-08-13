package domain

import "strings"

// ApprovedHeadshot returns the newest approved completion from any active
// headshot task assigned to speakerID. A workspace can run more than one
// portrait collection over its lifetime; public projections should not depend
// on task insertion order or keep retired collection work visible.
func (state State) ApprovedHeadshot(speakerID string) (TaskCompletion, bool) {
	speakerID = strings.TrimSpace(speakerID)
	if speakerID == "" {
		return TaskCompletion{}, false
	}
	activeTasks := make(map[string]struct{})
	for _, task := range state.Tasks {
		if task.Active() && task.Type == "headshot" && state.TaskAssignedToSpeaker(task, speakerID) {
			activeTasks[task.ID] = struct{}{}
		}
	}
	var selected TaskCompletion
	found := false
	for _, completion := range state.TaskCompletions {
		if completion.SpeakerID != speakerID || completion.Status != TaskApproved ||
			strings.TrimSpace(completion.FileName) == "" || strings.TrimSpace(completion.StoredPath) == "" {
			continue
		}
		if _, active := activeTasks[completion.TaskID]; !active {
			continue
		}
		if !found || completion.UpdatedAt.After(selected.UpdatedAt) ||
			(completion.UpdatedAt.Equal(selected.UpdatedAt) && completion.ID > selected.ID) {
			selected = completion
			found = true
		}
	}
	return selected, found
}
