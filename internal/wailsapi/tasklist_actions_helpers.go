package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/database"
	"context"
	"strings"
)

func toCustomActionView(a database.CustomAction) apidto.CustomActionView {
	return apidto.CustomActionView{
		ID:       a.ID,
		Label:    a.Label,
		Icon:     a.Icon,
		Danger:   a.Danger,
		Confirm:  a.Confirm,
		HasEvent: a.Event != "",
		HasLink:  a.Link != "",
	}
}

// isSupportedActionLink informa se o link renderizado usa um esquema que o
// frontend (lib/deepLinks.openTaskLink) sabe abrir: deep link interno
// (assistente://) ou URL externa (http/https).
func isSupportedActionLink(link string) bool {
	return strings.HasPrefix(link, "assistente://") ||
		strings.HasPrefix(link, "http://") ||
		strings.HasPrefix(link, "https://")
}

// customActionTaskMap normaliza os campos de um card para a raiz `.task` dos templates.
func customActionTaskMap(ctx context.Context, ctrl *controllers.TaskListController, task *database.Task) map[string]any {
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
	if ctrl != nil {
		if tl, err := ctrl.GetTaskList(ctx, task.TaskListID); err == nil && tl != nil {
			m["task_list_slug"] = tl.Slug
		}
	}
	return m
}

func customActionBoardTaskMap(ctx context.Context, ctrl *controllers.TaskListController, taskListID string) map[string]any {
	m := emptyTaskMap()
	m["task_list_id"] = taskListID
	if ctrl != nil {
		if tl, err := ctrl.GetTaskList(ctx, taskListID); err == nil && tl != nil {
			m["task_list_slug"] = tl.Slug
		}
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
