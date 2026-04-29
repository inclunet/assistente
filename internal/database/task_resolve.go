package database

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ResolveTaskID resolve uma task por id numérico e/ou (lista + code).
// Regras:
//   - É obrigatório task_id (>0) ou code não vazio.
//   - Com code sem task_id, é obrigatório task_list_id e/ou task_list_slug (mesmas regras que ResolveTaskListID).
//   - Com task_id e code, a task existente deve ter exatamente esse code (após trim no argumento).
//   - Com task_id e referência de lista, a task deve pertencer à lista resolvida.
func ResolveTaskID(taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	codeTrim := strings.TrimSpace(code)
	var idVal string
	if taskID != nil {
		idVal = *taskID
	}
	hasID := idVal != ""
	hasCode := codeTrim != ""
	listPtr := taskListID
	if listPtr != nil && *listPtr == "" {
		listPtr = nil
	}
	hasListRef := listPtr != nil || strings.TrimSpace(taskListSlug) != ""

	if !hasID && !hasCode {
		return "", fmt.Errorf("informe task_id ou code")
	}
	if hasCode && !hasID && !hasListRef {
		return "", fmt.Errorf("com code é necessário task_list_id ou task_list_slug")
	}

	// Somente task_id: lista não entra na identidade (permite usar task_list_id/slug como destino em move/duplicate).
	if hasID && !hasCode {
		task, err := GetTask(idVal)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", fmt.Errorf("task_id %s não encontrado", idVal)
			}
			return "", err
		}
		return task.ID, nil
	}

	if !hasID && hasCode {
		listID, err := ResolveTaskListID(listPtr, taskListSlug)
		if err != nil {
			return "", err
		}
		task, err := FindTaskByCode(listID, codeTrim)
		if err != nil {
			return "", err
		}
		if task == nil {
			return "", fmt.Errorf("nenhuma task com code %q na lista", codeTrim)
		}
		return task.ID, nil
	}

	task, err := GetTask(idVal)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("task_id %s não encontrado", idVal)
		}
		return "", err
	}
	if task.Code != codeTrim {
		return "", fmt.Errorf("task_id %s e code %q não correspondem à mesma task", idVal, codeTrim)
	}
	if hasListRef {
		listID, err := ResolveTaskListID(listPtr, taskListSlug)
		if err != nil {
			return "", err
		}
		if task.TaskListID != listID {
			return "", fmt.Errorf("task_id %s e lista referenciada não correspondem à mesma task", idVal)
		}
	}
	return task.ID, nil
}

// ResolveTaskIDByTaskCode localiza uma task pelo campo Task.Code (ex.: ticket FSD-12345).
func ResolveTaskIDByTaskCode(taskListID *string, taskCode string) (string, error) {
	codeTrim := strings.TrimSpace(taskCode)
	if codeTrim == "" {
		return "", fmt.Errorf("task_code não pode ser vazio")
	}
	q := db.Model(&Task{}).Where("code = ?", codeTrim)
	if taskListID != nil && *taskListID != "" {
		q = q.Where("task_list_id = ?", *taskListID)
	}
	var tasks []Task
	if err := q.Find(&tasks).Error; err != nil {
		return "", err
	}
	switch len(tasks) {
	case 0:
		if taskListID != nil && *taskListID != "" {
			return "", fmt.Errorf("nenhuma task com task_code %q na lista %s", codeTrim, *taskListID)
		}
		return "", fmt.Errorf("nenhuma task com task_code %q", codeTrim)
	case 1:
		return tasks[0].ID, nil
	default:
		if taskListID != nil && *taskListID != "" {
			return "", fmt.Errorf("múltiplas tasks com task_code %q na lista %s", codeTrim, *taskListID)
		}
		return "", fmt.Errorf("várias tasks com task_code %q; informe task_list_id ou task_list_slug para restringir à lista", codeTrim)
	}
}
