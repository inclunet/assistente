package tasklist

import (
	"encoding/json"

	"assistente/internal/database"
)

// parseWorkflowStatuses desserializa statuses do workflow
func parseWorkflowStatuses(workflow *database.TaskListWorkflow) ([]database.TaskListWorkflowStatus, error) {
	var statuses []database.TaskListWorkflowStatus
	if err := json.Unmarshal([]byte(workflow.Statuses), &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}
