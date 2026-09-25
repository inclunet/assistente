package app

import (
	"assistente/internal/commandcatalog"
	"assistente/internal/workspace"
)

type workspaceEditorModeChangedEvent struct {
	Workspace *workspace.Workspace `json:"workspace"`
	TabID     string               `json:"tabId"`
	Mode      string               `json:"mode"`
}

func editorModeForCommand(commandID string) (string, bool) {
	switch commandID {
	case commandEditorModeMarkdownID:
		return "markdown", true
	case commandEditorModeRichID:
		return "rich", true
	case commandEditorModeViewID:
		return "view", true
	default:
		return "", false
	}
}

func commandEditorModeRegistration(commandID string) (commandcatalog.Definition, commandcatalog.HandlerContract) {
	mode, ok := editorModeForCommand(commandID)
	if !ok {
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	pt := map[string]string{"markdown": "Modo Markdown", "rich": "Modo rico", "view": "Modo de visualização"}[mode]
	en := map[string]string{"markdown": "Markdown mode", "rich": "Rich mode", "view": "View mode"}[mode]
	es := map[string]string{"markdown": "Modo Markdown", "rich": "Modo enriquecido", "view": "Modo de visualización"}[mode]
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Write, HasMutableTarget: true, Route: "contextual/workspace/editor/mode/" + mode, Classification: commandcatalog.HandlerBackend}
	return commandcatalog.Definition{
		ID: commandID, Effect: commandcatalog.Write, Decision: commandcatalog.NoDecision, HasMutableTarget: true,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context:        commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		Presentation: &commandcatalog.Presentation{Version: "editor-mode-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: pt, Description: "Define o modo de exibição da aba de editor", Category: "Editor"},
			"en":    {Name: en, Description: "Sets the display mode of the editor tab", Category: "Editor"},
			"es":    {Name: es, Description: "Define el modo de visualización de la pestaña del editor", Category: "Editor"},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
	}, contract
}
