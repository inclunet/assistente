package tasklist

import (
	"encoding/json"

	"assistente/internal/database"
)

// simpleWorkflowTemplate gera um workflow simples (A Fazer → Concluído)
func simpleWorkflowTemplate() *database.TaskListWorkflow {
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "A Fazer", Color: "var(--color-warning)", Icon: "⌛"},
		{ID: 2, Order: 1, Label: "Concluído", Color: "var(--color-success)", Icon: "✅"},
	}
	statusesJSON, _ := json.Marshal(statuses)

	transitions := database.TaskListWorkflowTransitions{
		1: {2},
		2: {1},
	}
	transitionsJSON, _ := json.Marshal(transitions)

	return &database.TaskListWorkflow{
		Statuses:           string(statusesJSON),
		AllowedTransitions: string(transitionsJSON),
		InitialStatusID:    1,
	}
}

// parseWorkflowStatuses desserializa statuses do workflow
func parseWorkflowStatuses(workflow *database.TaskListWorkflow) ([]database.TaskListWorkflowStatus, error) {
	var statuses []database.TaskListWorkflowStatus
	if err := json.Unmarshal([]byte(workflow.Statuses), &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}
