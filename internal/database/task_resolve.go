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
func ResolveTaskID(taskListID *uint, taskListSlug string, taskID *uint, code string) (uint, error) {
	codeTrim := strings.TrimSpace(code)
	var idVal uint
	if taskID != nil {
		idVal = *taskID
	}
	hasID := idVal > 0
	hasCode := codeTrim != ""
	listPtr := taskListID
	if listPtr != nil && *listPtr == 0 {
		listPtr = nil
	}
	hasListRef := listPtr != nil || strings.TrimSpace(taskListSlug) != ""

	if !hasID && !hasCode {
		return 0, fmt.Errorf("informe task_id ou code")
	}
	if hasCode && !hasID && !hasListRef {
		return 0, fmt.Errorf("com code é necessário task_list_id ou task_list_slug")
	}

	// Somente task_id: lista não entra na identidade (permite usar task_list_id/slug como destino em move/duplicate).
	if hasID && !hasCode {
		task, err := GetTask(idVal)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, fmt.Errorf("task_id %d não encontrado", idVal)
			}
			return 0, err
		}
		return task.ID, nil
	}

	if !hasID && hasCode {
		listID, err := ResolveTaskListID(listPtr, taskListSlug)
		if err != nil {
			return 0, err
		}
		task, err := FindTaskByCode(listID, codeTrim)
		if err != nil {
			return 0, err
		}
		if task == nil {
			return 0, fmt.Errorf("nenhuma task com code %q na lista", codeTrim)
		}
		return task.ID, nil
	}

	task, err := GetTask(idVal)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("task_id %d não encontrado", idVal)
		}
		return 0, err
	}
	if task.Code != codeTrim {
		return 0, fmt.Errorf("task_id %d e code %q não correspondem à mesma task", idVal, codeTrim)
	}
	if hasListRef {
		listID, err := ResolveTaskListID(listPtr, taskListSlug)
		if err != nil {
			return 0, err
		}
		if task.TaskListID != listID {
			return 0, fmt.Errorf("task_id %d e lista referenciada não correspondem à mesma task", idVal)
		}
	}
	return task.ID, nil
}
