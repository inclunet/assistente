package app

import (
	"context"
	"encoding/json"
	"strings"

	"assistente/internal/contextprovider"
	"assistente/internal/database"
	"assistente/internal/tasklist"
)

// linkedTaskListsForConversation resolve as task lists vinculadas a uma conversa
// e as mapeia para o Context Provider tasklist.
// Best-effort: erros (ou ctx sem usuário) resultam em nil → contexto vazio.
func (a *App) linkedTaskListsForConversation(ctx context.Context, conversationID string) []contextprovider.LinkedTaskList {
	if a.taskListCtrl == nil || strings.TrimSpace(conversationID) == "" {
		return nil
	}
	lists, err := a.taskListCtrl.GetTaskListsByConversation(ctx, conversationID)
	if err != nil || len(lists) == 0 {
		return nil
	}
	out := make([]contextprovider.LinkedTaskList, 0, len(lists))
	for i := range lists {
		l := lists[i]
		statusMeta := map[int]database.TaskListWorkflowStatus{}
		if l.Workflow != nil && strings.TrimSpace(l.Workflow.Statuses) != "" {
			var sts []database.TaskListWorkflowStatus
			if json.Unmarshal([]byte(l.Workflow.Statuses), &sts) == nil {
				for _, s := range sts {
					statusMeta[s.ID] = s
				}
			}
		}
		tasks := make([]contextprovider.LinkedTask, 0, len(l.Tasks))
		for _, tk := range l.Tasks {
			meta := statusMeta[tk.StatusID]
			tasks = append(tasks, contextprovider.LinkedTask{
				ID:         tk.ID,
				Title:      tk.Title,
				Status:     meta.Label,
				StatusIcon: meta.Icon,
			})
		}
		out = append(out, contextprovider.LinkedTaskList{
			ID:          l.ID,
			Title:       l.Title,
			Description: l.Description,
			Tasks:       tasks,
		})
	}
	return out
}

// newTaskListService cria o TaskListService com o emitter injetado.
func (a *App) newTaskListService() *tasklist.Service {
	return tasklist.NewService(tasklist.ServiceConfig{
		Store:   tasklist.NewDBStore(),
		Emitter: a.emitter,
	})
}
