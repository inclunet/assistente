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
	ConversationID string

	// Workspace context
	WorkspaceName    string
	WorkspaceProfile string
	ActiveTabTitle   string
	ActiveTabType    string
	Tabs             []TabInfo
	TabCount         int

	// Surface contém o contrato derivado da superfície que originou o envio.
	Surface *SurfaceInfo
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
