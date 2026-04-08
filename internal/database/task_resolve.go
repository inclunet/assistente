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

// ResolveTaskIDByTaskCode localiza uma task pelo campo Task.Code (ex.: ticket FSD-12345).
// Se taskListID != nil, a busca é restrita a essa lista (útil para desambiguar quando o mesmo code existe em várias listas).
// Erros: code vazio; nenhuma task; ou múltiplas tasks com o mesmo code quando a busca é global.
func ResolveTaskIDByTaskCode(taskListID *uint, taskCode string) (uint, error) {
	codeTrim := strings.TrimSpace(taskCode)
	if codeTrim == "" {
		return 0, fmt.Errorf("task_code não pode ser vazio")
	}
	q := db.Model(&Task{}).Where("code = ?", codeTrim)
	if taskListID != nil && *taskListID > 0 {
		q = q.Where("task_list_id = ?", *taskListID)
	}
	var tasks []Task
	if err := q.Find(&tasks).Error; err != nil {
		return 0, err
	}
	switch len(tasks) {
	case 0:
		if taskListID != nil && *taskListID > 0 {
			return 0, fmt.Errorf("nenhuma task com task_code %q na lista %d", codeTrim, *taskListID)
		}
		return 0, fmt.Errorf("nenhuma task com task_code %q", codeTrim)
	case 1:
		return tasks[0].ID, nil
	default:
		if taskListID != nil && *taskListID > 0 {
			return 0, fmt.Errorf("múltiplas tasks com task_code %q na lista %d", codeTrim, *taskListID)
		}
		return 0, fmt.Errorf("várias tasks com task_code %q; informe task_list_id ou task_list_slug para restringir à lista", codeTrim)
	}
}
