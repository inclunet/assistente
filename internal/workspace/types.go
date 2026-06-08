package workspace

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// TabType define o tipo de conteúdo exibido em uma aba.
type TabType string

const (
	TabTypeChat     TabType = "chat"
	TabTypeEditor   TabType = "editor"
	TabTypeTerminal TabType = "terminal"
	TabTypeTasklist TabType = "tasklist"
)

// Tab representa uma aba dentro de um workspace.
// Toda aba tem um ConversationID dedicado para seu chat inline.
// Dados específicos do tipo (filePath, sessionId, tasklistId) ficam em State.
type Tab struct {
	ID              string         `json:"id" yaml:"id"`
	Type            TabType        `json:"type" yaml:"type"`
	ConversationID  string         `json:"conversation_id,omitempty" yaml:"conversation_id,omitempty"`
	Title           string         `json:"title" yaml:"title"`
	Position        int            `json:"position" yaml:"position"`
	ProfileOverride map[string]any `json:"profile_override,omitempty" yaml:"profile_override,omitempty"`
	State           map[string]any `json:"state,omitempty" yaml:"state,omitempty"`

	// ContentID: campo legado para migração de workspaces antigos. Ignorado após load.
	ContentID string `json:"content_id,omitempty" yaml:"content_id,omitempty"`
}

// UnmarshalYAML handles legacy workspace files where conversation_id was
// serialized as an integer (!!int). yaml.v3 refuses to unmarshal !!int into a
// string field, so we patch the node tag before decoding.
func (t *Tab) UnmarshalYAML(value *yaml.Node) error {
	// Patch conversation_id and content_id nodes: force !!str so yaml.v3
	// accepts numeric values into string fields.
	if value.Kind == yaml.MappingNode {
		for i := 0; i < len(value.Content)-1; i += 2 {
			key := value.Content[i].Value
			if key == "conversation_id" || key == "content_id" {
				val := value.Content[i+1]
				if val.Tag == "!!int" || val.Tag == "!!float" {
					val.Tag = "!!str"
				}
			}
		}
	}

	type rawTab Tab
	var raw rawTab
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*t = Tab(raw)
	return nil
}

// TabsState armazena qual aba está ativa e a lista de abas.
type TabsState struct {
	Active string `json:"active" yaml:"active"`
	Items  []Tab  `json:"items" yaml:"items"`
}

// Workspace é o container principal de abas mistas.
type Workspace struct {
	ID        string    `json:"id" yaml:"id"`
	Name      string    `json:"name" yaml:"name"`
	Profile   string    `json:"profile,omitempty" yaml:"profile,omitempty"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	LastUsed  time.Time `json:"last_used" yaml:"last_used"`
	Tabs      TabsState `json:"tabs" yaml:"tabs"`
}

// IndexEntry é um resumo de workspace no índice global.
type IndexEntry struct {
	ID       string    `json:"id" yaml:"id"`
	Name     string    `json:"name" yaml:"name"`
	Path     string    `json:"path" yaml:"path"`
	LastUsed time.Time `json:"last_used" yaml:"last_used"`
}

// Index é o índice global de todos os workspaces conhecidos.
type Index struct {
	LastOpened string       `json:"last_opened" yaml:"last_opened"`
	Workspaces []IndexEntry `json:"workspaces" yaml:"workspaces"`
}

// WorkspaceInfo é um resumo leve para listagem na UI.
type WorkspaceInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Profile  string `json:"profile"`
	TabCount int    `json:"tab_count"`
	IsActive bool   `json:"is_active"`
}

// Validate valida os campos obrigatórios de um workspace.
func (w *Workspace) Validate() error {
	if w.ID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if w.Name == "" {
		return fmt.Errorf("workspace name is required")
	}
	return nil
}

// Validate valida os campos obrigatórios de uma tab.
func (t *Tab) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("tab id is required")
	}
	if t.Type == "" {
		return fmt.Errorf("tab type is required")
	}
	validTypes := []TabType{TabTypeChat, TabTypeEditor, TabTypeTerminal, TabTypeTasklist}
	valid := false
	for _, vt := range validTypes {
		if t.Type == vt {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid tab type: %s", t.Type)
	}
	return nil
}

// FindTab encontra uma tab pelo ID dentro do workspace.
func (w *Workspace) FindTab(tabID string) *Tab {
	for i := range w.Tabs.Items {
		if w.Tabs.Items[i].ID == tabID {
			return &w.Tabs.Items[i]
		}
	}
	return nil
}

// FindTabByConversation encontra a primeira tab com o conversationId especificado.
func (w *Workspace) FindTabByConversation(conversationID string) *Tab {
	if conversationID == "" {
		return nil
	}
	for i := range w.Tabs.Items {
		if w.Tabs.Items[i].ConversationID == conversationID {
			return &w.Tabs.Items[i]
		}
	}
	return nil
}

// FindTabByState encontra a primeira tab do tipo especificado com state[key] == value.
func (w *Workspace) FindTabByState(tabType TabType, key, value string) *Tab {
	for i := range w.Tabs.Items {
		t := &w.Tabs.Items[i]
		if t.Type != tabType {
			continue
		}
		if v, ok := t.State[key].(string); ok && v == value {
			return t
		}
	}
	return nil
}

// ActiveTab retorna a tab ativa ou nil.
func (w *Workspace) ActiveTab() *Tab {
	if w.Tabs.Active == "" {
		return nil
	}
	return w.FindTab(w.Tabs.Active)
}
