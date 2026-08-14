package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DomainEventPublisher é a superfície mínima que TriggerCustomAction precisa
// do jobs.Manager (PublishDomainEvent).
type DomainEventPublisher interface {
	PublishDomainEvent(ctx context.Context, name string, payload map[string]any) error
}

// TasklistActions é o bind Wails do domínio tasklist_actions / custom actions
// (AEP-0088). Auth só via WithUser — sem chamar o helper de auth do App no call site.
type TasklistActions struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.TaskListController
	jobMgr  DomainEventPublisher
}

// NewTasklistActions cria o bind vazio; AttachTasklistActions preenche deps no startup.
func NewTasklistActions() *TasklistActions {
	return &TasklistActions{}
}

// AttachTasklistActions associa Session, controller e publisher após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
// jobMgr pode ser nil; TriggerCustomAction falha se a action exigir evento.
func AttachTasklistActions(api *TasklistActions, session Session, ctrl *controllers.TaskListController, jobMgr DomainEventPublisher) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
	api.jobMgr = jobMgr
}

func (api *TasklistActions) deps() (Session, *controllers.TaskListController, DomainEventPublisher, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, nil, ErrTasklistActionsNotWired
	}
	return api.session, api.ctrl, api.jobMgr, nil
}

// GetTaskListCustomActions retorna as custom actions configuradas na lista.
func (api *TasklistActions) GetTaskListCustomActions(taskListID string) (*database.TaskListCustomActions, error) {
	session, ctrl, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.TaskListCustomActions, error) {
		return ctrl.GetTaskListCustomActions(ctx, taskListID)
	})
}

// SetTaskListCustomActions persiste as custom actions (JSON) da lista.
func (api *TasklistActions) SetTaskListCustomActions(taskListID string, actionsJSON string) error {
	session, ctrl, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetTaskListCustomActions(ctx, taskListID, actionsJSON)
	})
	return err
}

// ListCardCustomActions retorna as custom actions visíveis para um card numa
// superfície (card_menu | card_detail), avaliando o `when` server-side.
func (api *TasklistActions) ListCardCustomActions(taskID string, surface string) ([]apidto.CustomActionView, error) {
	session, ctrl, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.CustomActionView, error) {
		task, err := ctrl.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if task == nil {
			return []apidto.CustomActionView{}, nil
		}
		ca, err := ctrl.GetTaskListCustomActions(ctx, task.TaskListID)
		if err != nil || ca == nil {
			return []apidto.CustomActionView{}, err
		}
		data := map[string]any{"task": customActionTaskMap(ctx, ctrl, task), "now": time.Now()}
		return filterVisibleActions(ca.Actions, surface, data), nil
	})
}

// ListBoardCustomActions retorna as custom actions de board (board_menu) de uma
// lista. Sem card de contexto, mas `.task` ainda carrega a lista atual.
func (api *TasklistActions) ListBoardCustomActions(taskListID string) ([]apidto.CustomActionView, error) {
	session, ctrl, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.CustomActionView, error) {
		ca, err := ctrl.GetTaskListCustomActions(ctx, taskListID)
		if err != nil || ca == nil {
			return []apidto.CustomActionView{}, err
		}
		data := map[string]any{"task": customActionBoardTaskMap(ctx, ctrl, taskListID), "now": time.Now()}
		return filterVisibleActions(ca.Actions, database.CustomActionSurfaceBoardMenu, data), nil
	})
}

// TriggerCustomAction renderiza payload_template/link server-side, publica o
// evento (proveniência user, pois é iniciada por humano) e devolve o link
// renderizado para o frontend abrir via openTaskLink.
func (api *TasklistActions) TriggerCustomAction(taskListID string, taskID string, actionID string) (string, error) {
	session, ctrl, jobMgr, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return triggerCustomAction(ctx, ctrl, jobMgr, taskListID, taskID, actionID)
	})
}

func triggerCustomAction(
	ctx context.Context,
	ctrl *controllers.TaskListController,
	jobMgr DomainEventPublisher,
	taskListID, taskID, actionID string,
) (string, error) {
	var task *database.Task
	var err error
	if taskID != "" {
		task, err = ctrl.GetTask(ctx, taskID)
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
			return "", fmt.Errorf("task %q does not belong to task list %q", taskID, taskListID)
		}
	}
	if taskListID == "" {
		return "", fmt.Errorf("task list id is required")
	}

	ca, err := ctrl.GetTaskListCustomActions(ctx, taskListID)
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

	if taskID != "" {
		if !action.HasSurface(database.CustomActionSurfaceCardMenu) && !action.HasSurface(database.CustomActionSurfaceCardDetail) {
			return "", fmt.Errorf("custom action %q is not available for cards", actionID)
		}
	} else if !action.HasSurface(database.CustomActionSurfaceBoardMenu) {
		return "", fmt.Errorf("custom action %q is not available as a board action", actionID)
	}

	var taskMap map[string]any
	if task != nil {
		taskMap = customActionTaskMap(ctx, ctrl, task)
	} else {
		taskMap = customActionBoardTaskMap(ctx, ctrl, taskListID)
	}
	data := map[string]any{"task": taskMap, "now": time.Now()}

	if action.When != "" {
		ok, werr := jobs.EvaluateConditionWithRoot(action.When, data)
		if werr != nil {
			return "", fmt.Errorf("evaluate when: %w", werr)
		}
		if !ok {
			return "", fmt.Errorf("custom action %q is not available in the current context", actionID)
		}
	}

	var renderedLink string
	if action.Link != "" {
		renderedLink, err = jobs.RenderWithRoot(action.Link, data)
		if err != nil {
			return "", fmt.Errorf("render link: %w", err)
		}
		if renderedLink == "<no value>" {
			renderedLink = ""
		}
		if renderedLink != "" && !isSupportedActionLink(renderedLink) {
			return "", fmt.Errorf("custom action %q rendered an unsupported link %q (expected assistente://, http:// or https://)", action.ID, renderedLink)
		}
	}

	if action.Event != "" {
		if jobMgr == nil {
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
		if perr := jobMgr.PublishDomainEvent(ctx, action.Event, payload); perr != nil {
			return "", fmt.Errorf("publish event: %w", perr)
		}
	}

	return renderedLink, nil
}

func filterVisibleActions(actions []database.CustomAction, surface string, data map[string]any) []apidto.CustomActionView {
	out := []apidto.CustomActionView{}
	for _, action := range actions {
		if !action.HasSurface(surface) {
			continue
		}
		ok, err := jobs.EvaluateConditionWithRoot(action.When, data)
		if err != nil {
			logging.Errorf(context.Background(), "wailsapi.tasklist-actions", "[CustomActions] when eval error (action=%q): %v", action.ID, err)
			continue
		}
		if !ok {
			continue
		}
		out = append(out, toCustomActionView(action))
	}
	return out
}
