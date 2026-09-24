package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// ErrTaskListConfigConflict indica que a configuração (workflow ou custom
// actions) mudou no banco desde que o editor a leu. A mensagem começa com um
// código estável porque o frontend só recebe o texto do erro pelo Wails.
var ErrTaskListConfigConflict = errors.New("TASKLIST_CONFIG_CONFLICT: a configuração da lista foi alterada em outro lugar")

// TaskListWorkflowSnapshot é o estado do workflow que o editor leu e sobre o
// qual calculou a alteração.
type TaskListWorkflowSnapshot struct {
	Statuses        []TaskListWorkflowStatus `json:"statuses"`
	Transitions     map[int][]int            `json:"transitions"`
	InitialStatusID int                      `json:"initialStatusId"`
}

// canonicalWorkflow serializa o workflow de forma que diferenças sem efeito
// (ordem do array de statuses, ordem dos destinos, origem sem destinos) não
// contem como alteração.
func canonicalWorkflow(statuses []TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int) string {
	sortedStatuses := slices.Clone(statuses)
	sort.SliceStable(sortedStatuses, func(i, j int) bool {
		if sortedStatuses[i].Order != sortedStatuses[j].Order {
			return sortedStatuses[i].Order < sortedStatuses[j].Order
		}
		return sortedStatuses[i].ID < sortedStatuses[j].ID
	})
	normalized := make(map[int][]int, len(transitions))
	for from, to := range transitions {
		if len(to) == 0 {
			continue
		}
		targets := slices.Clone(to)
		slices.Sort(targets)
		normalized[from] = slices.Compact(targets)
	}
	out, _ := json.Marshal(struct {
		Statuses    []TaskListWorkflowStatus `json:"s"`
		Transitions map[int][]int            `json:"t"`
		Initial     int                      `json:"i"`
	}{sortedStatuses, normalized, initialStatusID})
	return string(out)
}

// ensureWorkflowUnchanged compara, dentro da transação, o workflow gravado com
// o snapshot esperado.
func ensureWorkflowUnchanged(ctx context.Context, tx *gorm.DB, taskListID string, expected TaskListWorkflowSnapshot) error {
	var current TaskListWorkflow
	if err := taskListWorkflowQuery(ctx, tx.Model(&TaskListWorkflow{})).
		Where("task_list_workflows.task_list_id = ?", taskListID).
		First(&current).Error; err != nil {
		return err
	}
	var statuses []TaskListWorkflowStatus
	if strings.TrimSpace(current.Statuses) != "" {
		if err := json.Unmarshal([]byte(current.Statuses), &statuses); err != nil {
			return fmt.Errorf("workflow gravado com statuses inválidos: %w", err)
		}
	}
	var transitions map[int][]int
	if strings.TrimSpace(current.AllowedTransitions) != "" {
		if err := json.Unmarshal([]byte(current.AllowedTransitions), &transitions); err != nil {
			return fmt.Errorf("workflow gravado com transições inválidas: %w", err)
		}
	}
	if canonicalWorkflow(statuses, transitions, current.InitialStatusID) !=
		canonicalWorkflow(expected.Statuses, expected.Transitions, expected.InitialStatusID) {
		return ErrTaskListConfigConflict
	}
	return nil
}

func canonicalCustomActions(raw string) (string, error) {
	parsed, err := ParseTaskListCustomActionsJSON(raw)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(parsed)
	return string(out), nil
}

// SetTaskListCustomActionsCheckedWithContext grava as custom actions somente se
// o conteúdo atual ainda for equivalente a expectedJSON; caso contrário devolve
// ErrTaskListConfigConflict sem gravar.
func SetTaskListCustomActionsCheckedWithContext(ctx context.Context, taskListID, expectedJSON, actionsJSON string) error {
	s := strings.TrimSpace(actionsJSON)
	if s != "" {
		if _, err := ParseTaskListCustomActionsJSON(s); err != nil {
			return err
		}
	}
	expected, err := canonicalCustomActions(expectedJSON)
	if err != nil {
		return fmt.Errorf("estado esperado inválido: %w", err)
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tl TaskList
		if err := ScopeByUser(ctx, tx.Select("custom_actions"), "user_id").First(&tl, "id = ?", taskListID).Error; err != nil {
			return err
		}
		current, err := canonicalCustomActions(tl.CustomActions)
		if err != nil {
			return err
		}
		if current != expected {
			return ErrTaskListConfigConflict
		}
		return ScopeByUser(ctx, tx.Model(&TaskList{}), "user_id").Where("id = ?", taskListID).Update("custom_actions", s).Error
	})
}
