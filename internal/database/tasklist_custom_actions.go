package database

import (
	"bytes"
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
	// Normaliza para lowercase para alinhar com a leitura case-insensitive de
	// HasSurface (EqualFold) — assim a validação não rejeita valores que a
	// renderização aceitaria.
	switch strings.ToLower(strings.TrimSpace(s)) {
	case CustomActionSurfaceCardMenu, CustomActionSurfaceCardDetail, CustomActionSurfaceBoardMenu:
		return true
	default:
		return false
	}
}

// unknownJSONField extrai o nome do campo de um erro de DisallowUnknownFields
// ("json: unknown field \"X\"") para uma mensagem mais amigável. encoding/json
// não expõe um tipo de erro dedicado, então casamos pela string.
func unknownJSONField(err error) (string, bool) {
	const prefix = "json: unknown field "
	msg := err.Error()
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return "", false
	}
	field := strings.TrimSpace(msg[idx+len(prefix):])
	field = strings.Trim(field, "\"")
	return field, field != ""
}

// ParseTaskListCustomActionsJSON interpreta o campo custom_actions e valida.
func ParseTaskListCustomActionsJSON(raw string) (*TaskListCustomActions, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return &TaskListCustomActions{}, nil
	}
	var ca TaskListCustomActions
	// DisallowUnknownFields: rejeita campos que não existem no schema (ex.: typos
	// ou aliases inventados como "emits_event"/"enabled_when") em vez de ignorá-los
	// silenciosamente — assim uma config errada falha cedo, com mensagem clara, em
	// vez de ser salva sem efeito.
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ca); err != nil {
		if field, ok := unknownJSONField(err); ok {
			return nil, fmt.Errorf("custom_actions: campo desconhecido %q — verifique o nome (campos válidos por ação: id, label, icon, surfaces, event, payload_template, link, when, confirm, danger)", field)
		}
		return nil, fmt.Errorf("custom_actions: JSON inválido: %w", err)
	}
	seen := make(map[string]bool, len(ca.Actions))
	for i, a := range ca.Actions {
		if strings.TrimSpace(a.ID) == "" {
			return nil, fmt.Errorf("custom_actions: action #%d sem id", i+1)
		}
		// id é um slug estável (referenciado em logs/payloads): mesma regra dos ids
		// de job (internal/jobs/parser.go), validada SEM trim — qualquer whitespace
		// (inclusive nas bordas, ex.: "open ") ou separador de path é rejeitado.
		id := a.ID
		if strings.ContainsAny(id, " \t\n\r\f\v/\\") {
			return nil, fmt.Errorf("custom_actions: id %q não pode conter espaços/whitespace ou separadores de path (/ \\)", id)
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
		// link só-whitespace é "presente" no JSON/UI mas renderiza para vazio no
		// trigger (nada é aberto): config confusa. Rejeita cedo. (Vazio de verdade
		// é permitido — significa "sem link", coberto pela checagem event/link acima.)
		if a.Link != "" && strings.TrimSpace(a.Link) == "" {
			return nil, fmt.Errorf("custom_actions: action %q tem link composto só por espaços em branco", id)
		}
		// event vira nome de evento publicado direto no EventBus; whitespace (ex.:
		// "tasklist.card.foo ") geraria um nome diferente e quebraria listeners/picker
		// de forma difícil de diagnosticar. PublishDomainEvent só rejeita nome vazio.
		if strings.ContainsAny(a.Event, " \t\n\r\f\v") {
			return nil, fmt.Errorf("custom_actions: action %q tem event com espaços em branco: %q", id, a.Event)
		}
		// payload_template só é aplicado quando há event (ver TriggerCustomAction):
		// permitir payload_template + apenas link seria uma config "válida" mas
		// silenciosamente sem efeito. Rejeita cedo para evitar surpresa.
		if strings.TrimSpace(a.PayloadTemplate) != "" && strings.TrimSpace(a.Event) == "" {
			return nil, fmt.Errorf("custom_actions: action %q define payload_template mas não tem event (payload_template só se aplica a event)", id)
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
