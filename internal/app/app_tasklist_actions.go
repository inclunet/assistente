package app

import (
	"assistente/internal/database"
	"context"
)

// customActionEventNames coleta os nomes de eventos das custom actions de todas
// as listas do usuário (para o picker do JobBuilder / ListKnownEvents).
// Injetado no bind wailsapi.Jobs via AttachJobs (AEP-0088).
func (a *App) customActionEventNames(ctx context.Context) []string {
	// GetAllTaskLists já traz a coluna custom_actions de todas as listas do
	// usuário numa única consulta; parseamos em memória para evitar um N+1
	// (uma query de custom_actions por lista).
	if a.taskListCtrl == nil {
		return nil
	}
	lists, err := a.taskListCtrl.GetAllTaskLists(ctx)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	for i := range lists {
		ca, err := database.ParseTaskListCustomActionsJSON(lists[i].CustomActions)
		if err != nil || ca == nil {
			continue
		}
		for _, action := range ca.Actions {
			if action.Event != "" && !seen[action.Event] {
				seen[action.Event] = true
				names = append(names, action.Event)
			}
		}
	}
	return names
}
