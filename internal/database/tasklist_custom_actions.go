package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Superfícies válidas onde uma custom action pode aparecer (AEP-0067).
const (
	CustomActionSurfaceCardMenu   = "card_menu"
	CustomActionSurfaceCardDetail = "card_detail"
	CustomActionSurfaceBoardMenu  = "board_menu"
)

// TaskListCustomActions é o conteúdo JSON da coluna custom_actions (AEP-0067).
type TaskListCustomActions struct {
	Actions []CustomAction `json:"actions,omitempty"`
}

// CustomAction é uma ação customizável de uma lista. Pode publicar um evento no
// EventBus de jobs e/ou abrir um link (deeplink interno ou URL externa).
type CustomAction struct {
	ID              string   `json:"id"`                         // estável (slug)
	Label           string   `json:"label"`                      // texto do item/botão
	Icon            string   `json:"icon,omitempty"`             // emoji/ícone opcional
	Surfaces        []string `json:"surfaces,omitempty"`         // card_menu (default) | card_detail | board_menu
	Event           string   `json:"event,omitempty"`            // evento a publicar (opcional)
	PayloadTemplate string   `json:"payload_template,omitempty"` // Go template -> objeto JSON
	Link            string   `json:"link,omitempty"`             // Go template -> deeplink interno OU URL externa (opcional)
	When            string   `json:"when,omitempty"`             // Go template de visibilidade (truthy)
	Danger          bool     `json:"danger,omitempty"`
	Confirm         string   `json:"confirm,omitempty"` // texto de confirmação opcional
}

// HasSurface indica se a action deve aparecer numa superfície. Sem surfaces
// declaradas, o default é card_menu.
func (a CustomAction) HasSurface(surface string) bool {
	if len(a.Surfaces) == 0 {
		return surface == CustomActionSurfaceCardMenu
	}
	for _, s := range a.Surfaces {
		if strings.EqualFold(strings.TrimSpace(s), surface) {
			return true
		}
	}
	return false
}

func isValidCustomActionSurface(s string) bool {
	switch strings.TrimSpace(s) {
	case CustomActionSurfaceCardMenu, CustomActionSurfaceCardDetail, CustomActionSurfaceBoardMenu:
		return true
	default:
		return false
	}
}

// ParseTaskListCustomActionsJSON interpreta o campo custom_actions e valida.
func ParseTaskListCustomActionsJSON(raw string) (*TaskListCustomActions, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return &TaskListCustomActions{}, nil
	}
	var ca TaskListCustomActions
	if err := json.Unmarshal([]byte(s), &ca); err != nil {
		return nil, fmt.Errorf("custom_actions: JSON inválido: %w", err)
	}
	seen := make(map[string]bool, len(ca.Actions))
	for i, a := range ca.Actions {
		id := strings.TrimSpace(a.ID)
		if id == "" {
			return nil, fmt.Errorf("custom_actions: action #%d sem id", i+1)
		}
		if seen[id] {
			return nil, fmt.Errorf("custom_actions: id duplicado %q", id)
		}
		seen[id] = true
		if strings.TrimSpace(a.Label) == "" {
			return nil, fmt.Errorf("custom_actions: action %q sem label", id)
		}
		if strings.TrimSpace(a.Event) == "" && strings.TrimSpace(a.Link) == "" {
			return nil, fmt.Errorf("custom_actions: action %q precisa de event e/ou link", id)
		}
		for _, surf := range a.Surfaces {
			if !isValidCustomActionSurface(surf) {
				return nil, fmt.Errorf("custom_actions: action %q tem surface inválida %q", id, surf)
			}
		}
	}
	return &ca, nil
}

// LoadTaskListCustomActionsWithContext carrega as custom actions da lista do
// usuário do contexto.
func LoadTaskListCustomActionsWithContext(ctx context.Context, taskListID string) (*TaskListCustomActions, error) {
	var tl TaskList
	if err := ScopeByUser(ctx, db.WithContext(ctx).Select("custom_actions"), "user_id").First(&tl, "id = ?", taskListID).Error; err != nil {
		return nil, err
	}
	return ParseTaskListCustomActionsJSON(tl.CustomActions)
}

// SetTaskListCustomActionsWithContext persiste o JSON das custom actions da
// tasklist do usuário do contexto (string vazia = sem ações).
func SetTaskListCustomActionsWithContext(ctx context.Context, taskListID string, actionsJSON string) error {
	s := strings.TrimSpace(actionsJSON)
	if s != "" {
		if _, err := ParseTaskListCustomActionsJSON(s); err != nil {
			return err
		}
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&TaskList{}), "user_id").Where("id = ?", taskListID).Update("custom_actions", s).Error
}
