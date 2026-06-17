package chat

import "assistente/internal/profiles"

// TemplateData contém as variáveis disponíveis para templates de skills.
// Definido aqui (não em internal/prompt) para que ambos os pacotes possam referenciá-lo
// sem criar um import circular: prompt já importa chat.
type TemplateData struct {
	Profile            *profiles.Profile
	ProfileSlug        string
	ToolCallingEnabled bool
	EnabledTools       []string
	EnabledToolCount   int
	ConversationID     string

	// Task list context used by auto-loaded tasklist skills. When no task list is
	// linked to the conversation, HasTaskLists remains false and templates render empty.
	HasTaskLists bool
	TaskLists    []TemplateTaskList

	// Workspace context
	WorkspaceID      string
	ProjectID        string
	WorkspaceName    string
	WorkspaceProfile string
	ActiveTabTitle   string
	ActiveTabType    string
	Tabs             []TabInfo
	TabCount         int

	// Surface contém o contrato derivado da superfície que originou o envio.
	Surface *SurfaceInfo
}

type TemplateTaskList struct {
	ID          string
	Title       string
	Description string
	Tasks       []TemplateTask
}

type TemplateTask struct {
	ID         string
	Title      string
	Status     string
	StatusIcon string
}

// TabInfo é uma visão simplificada de uma aba do workspace para templates de skills.
type TabInfo struct {
	Title     string
	Type      string
	ContentID string
	IsActive  bool
}

// SurfaceInfo contém o contexto estruturado da superfície ativa para skills/templates.
type SurfaceInfo struct {
	Type    string
	Title   string
	State   map[string]any
	Context map[string]any
}
