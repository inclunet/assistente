package app

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/jobs"
)

// CustomActionView é a projeção de uma custom action para a UI (AEP-0067).
// Não expõe templates/condições — apenas o necessário para renderizar o item.
type CustomActionView struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Icon     string `json:"icon,omitempty"`
	Danger   bool   `json:"danger,omitempty"`
	Confirm  string `json:"confirm,omitempty"`
	HasEvent bool   `json:"hasEvent"`
	HasLink  bool   `json:"hasLink"`
}

func toCustomActionView(a database.CustomAction) CustomActionView {
	return CustomActionView{
		ID:       a.ID,
		Label:    a.Label,
		Icon:     a.Icon,
		Danger:   a.Danger,
		Confirm:  a.Confirm,
		HasEvent: a.Event != "",
		HasLink:  a.Link != "",
	}
}

// ==================== Custom Actions config (AEP-0067) ====================

func (a *App) GetTaskListCustomActions(taskListID string) (*database.TaskListCustomActions, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTaskListCustomActions(ctx, taskListID)
}

func (a *App) SetTaskListCustomActions(taskListID string, actionsJSON string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.SetTaskListCustomActions(ctx, taskListID, actionsJSON)
}

// ==================== Custom Actions render/trigger ====================

// ListCardCustomActions retorna as custom actions visíveis para um card numa
// superfície (card_menu | card_detail), avaliando o `when` server-side.
func (a *App) ListCardCustomActions(taskID string, surface string) ([]CustomActionView, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	task, err := a.taskListCtrl.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return []CustomActionView{}, nil
	}
	ca, err := a.taskListCtrl.GetTaskListCustomActions(ctx, task.TaskListID)
	if err != nil || ca == nil {
		return []CustomActionView{}, err
	}
	data := map[string]any{"task": a.customActionTaskMap(ctx, task), "now": time.Now()}
	return a.filterVisibleActions(ca.Actions, surface, data), nil
}

// ListBoardCustomActions retorna as custom actions de board (board_menu) de uma
// lista. Sem card de contexto, mas `.task` ainda carrega a lista atual.
func (a *App) ListBoardCustomActions(taskListID string) ([]CustomActionView, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	ca, err := a.taskListCtrl.GetTaskListCustomActions(ctx, taskListID)
	if err != nil || ca == nil {
		return []CustomActionView{}, err
	}
	data := map[string]any{"task": a.customActionBoardTaskMap(ctx, taskListID), "now": time.Now()}
	return a.filterVisibleActions(ca.Actions, database.CustomActionSurfaceBoardMenu, data), nil
}

func (a *App) filterVisibleActions(actions []database.CustomAction, surface string, data map[string]any) []CustomActionView {
	out := []CustomActionView{}
	for _, action := range actions {
		if !action.HasSurface(surface) {
			continue
		}
		ok, err := jobs.EvaluateConditionWithRoot(action.When, data)
		if err != nil {
			logging.Errorf(context.Background(), "app.app-tasklist-actions", "[CustomActions] when eval error (action=%q): %v", action.ID, err)
			continue
		}
		if !ok {
			continue
		}
		out = append(out, toCustomActionView(action))
	}
	return out
}

// TriggerCustomAction renderiza payload_template/link server-side, publica o
// evento (proveniência user, pois é iniciada por humano) e devolve o link
// renderizado para o frontend abrir via openTaskLink.
func (a *App) TriggerCustomAction(taskListID string, taskID string, actionID string) (string, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return "", err
	}

	// Resolve o card (opcional) e o taskListID efetivo.
	var task *database.Task
	if taskID != "" {
		task, err = a.taskListCtrl.GetTask(ctx, taskID)
		if err != nil {
			return "", err
		}
		if task == nil {
			return "", fmt.Errorf("task %q not found", taskID)
		}
		switch {
		case taskListID == "":
			taskListID = task.TaskListID
		case taskListID != task.TaskListID:
			// Card e lista informados não pertencem um ao outro: recusa cedo
			// para não disparar a ação/configuração da lista errada.
			return "", fmt.Errorf("task %q does not belong to task list %q", taskID, taskListID)
		}
	}
	if taskListID == "" {
		return "", fmt.Errorf("task list id is required")
	}

	ca, err := a.taskListCtrl.GetTaskListCustomActions(ctx, taskListID)
	if err != nil {
		return "", err
	}
	var action *database.CustomAction
	if ca != nil {
		for i := range ca.Actions {
			if ca.Actions[i].ID == actionID {
				action = &ca.Actions[i]
				break
			}
		}
	}
	if action == nil {
		return "", fmt.Errorf("custom action %q not found", actionID)
	}

	// Revalida o surface server-side: uma action só de board_menu não pode ser
	// disparada com um card (taskID), e uma action de card não pode ser disparada
	// como board action (sem taskID). A listagem já filtra por surface, mas um
	// caller direto (devtools) poderia burlar a regra de visibilidade do servidor.
	if taskID != "" {
		if !action.HasSurface(database.CustomActionSurfaceCardMenu) && !action.HasSurface(database.CustomActionSurfaceCardDetail) {
			return "", fmt.Errorf("custom action %q is not available for cards", actionID)
		}
	} else if !action.HasSurface(database.CustomActionSurfaceBoardMenu) {
		return "", fmt.Errorf("custom action %q is not available as a board action", actionID)
	}

	var taskMap map[string]any
	if task != nil {
		taskMap = a.customActionTaskMap(ctx, task)
	} else {
		taskMap = a.customActionBoardTaskMap(ctx, taskListID)
	}
	data := map[string]any{"task": taskMap, "now": time.Now()}

	// Reavalia o `when` server-side no momento do trigger. A listagem já filtra
	// por visibilidade, mas um caller direto (ex.: devtools) poderia disparar uma
	// ação que deveria estar escondida/inválida para este card — então revalida.
	if action.When != "" {
		ok, werr := jobs.EvaluateConditionWithRoot(action.When, data)
		if werr != nil {
			return "", fmt.Errorf("evaluate when: %w", werr)
		}
		if !ok {
			return "", fmt.Errorf("custom action %q is not available in the current context", actionID)
		}
	}

	// Link renderizado (devolvido ao frontend).
	var renderedLink string
	if action.Link != "" {
		renderedLink, err = jobs.RenderWithRoot(action.Link, data)
		if err != nil {
			return "", fmt.Errorf("render link: %w", err)
		}
		// Campo ausente no template renderiza "<no value>": normaliza para vazio
		// (RenderWithRoot já fez TrimSpace).
		if renderedLink == "<no value>" {
			renderedLink = ""
		}
		// Se sobrou algo, precisa ser um esquema que o frontend (openTaskLink) sabe
		// abrir; do contrário a UI ignoraria silenciosamente e o usuário não veria
		// erro nenhum. Falha explícito para a UI exibir o toast.
		if renderedLink != "" && !isSupportedActionLink(renderedLink) {
			return "", fmt.Errorf("custom action %q rendered an unsupported link %q (expected assistente://, http:// or https://)", action.ID, renderedLink)
		}
	}

	// Evento publicado no EventBus (se configurado).
	if action.Event != "" {
		if a.jobMgr == nil {
			return "", fmt.Errorf("custom action %q requires jobs manager to publish event %q", action.ID, action.Event)
		}
		payload := map[string]any{
			"action_id":      action.ID,
			"task_list_id":   taskListID,
			"task_list_slug": taskMap["task_list_slug"],
		}
		if task != nil {
			payload["task_id"] = task.ID
			payload["code"] = task.Code
			payload["title"] = task.Title
		}
		if action.PayloadTemplate != "" {
			rendered, rerr := jobs.RenderWithRoot(action.PayloadTemplate, data)
			if rerr != nil {
				return "", fmt.Errorf("render payload_template: %w", rerr)
			}
			if rendered != "" {
				var extra map[string]any
				if jerr := json.Unmarshal([]byte(rendered), &extra); jerr != nil {
					return "", fmt.Errorf("payload_template did not render to a JSON object: %w", jerr)
				}
				for k, v := range extra {
					if _, exists := payload[k]; exists {
						continue
					}
					payload[k] = v
				}
			}
		}
		if perr := a.jobMgr.PublishDomainEvent(ctx, action.Event, payload); perr != nil {
			return "", fmt.Errorf("publish event: %w", perr)
		}
	}

	return renderedLink, nil
}

// isSupportedActionLink informa se o link renderizado usa um esquema que o
// frontend (lib/deepLinks.openTaskLink) sabe abrir: deep link interno
// (assistente://) ou URL externa (http/https). Qualquer outro seria ignorado
// silenciosamente pela UI.
func isSupportedActionLink(link string) bool {
	return strings.HasPrefix(link, "assistente://") ||
		strings.HasPrefix(link, "http://") ||
		strings.HasPrefix(link, "https://")
}

// customActionTaskMap normaliza os campos de um card para a raiz `.task` dos templates.
func (a *App) customActionTaskMap(ctx context.Context, task *database.Task) map[string]any {
	m := emptyTaskMap()
	if task == nil {
		return m
	}
	m["task_id"] = task.ID
	m["id"] = task.ID
	m["task_list_id"] = task.TaskListID
	m["code"] = task.Code
	m["title"] = task.Title
	m["description"] = task.Description
	m["status_id"] = task.StatusID
	m["assignee_id"] = task.AssigneeID
	m["assignee_name"] = task.AssigneeName
	m["creator_id"] = task.CreatorID
	m["link"] = task.Link
	conversationID := ""
	if task.ConversationID != nil {
		conversationID = *task.ConversationID
	}
	m["conversation_id"] = conversationID
	if task.ParentID != nil {
		m["parent_id"] = *task.ParentID
	}
	if tl, err := a.taskListCtrl.GetTaskList(ctx, task.TaskListID); err == nil && tl != nil {
		m["task_list_slug"] = tl.Slug
	}
	return m
}

func (a *App) customActionBoardTaskMap(ctx context.Context, taskListID string) map[string]any {
	m := emptyTaskMap()
	m["task_list_id"] = taskListID
	if tl, err := a.taskListCtrl.GetTaskList(ctx, taskListID); err == nil && tl != nil {
		m["task_list_slug"] = tl.Slug
	}
	return m
}

func emptyTaskMap() map[string]any {
	return map[string]any{
		"task_id":         "",
		"id":              "",
		"task_list_id":    "",
		"task_list_slug":  "",
		"code":            "",
		"title":           "",
		"description":     "",
		"status_id":       0,
		"parent_id":       "",
		"assignee_id":     "",
		"assignee_name":   "",
		"creator_id":      "",
		"link":            "",
		"conversation_id": "",
	}
}

// customActionEventNames coleta os nomes de eventos das custom actions de todas
// as listas do usuário (para o picker do JobBuilder).
func (a *App) customActionEventNames(ctx context.Context) []string {
	// GetAllTaskLists já traz a coluna custom_actions de todas as listas do
	// usuário numa única consulta; parseamos em memória para evitar um N+1
	// (uma query de custom_actions por lista).
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
