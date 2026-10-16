package app

import "assistente/internal/commandcatalog"

const commandWorkspaceChatOpenID = "workspace.chat.open"

func commandWorkspaceChatOpenRegistration() (commandcatalog.Definition, commandcatalog.HandlerContract) {
	contract := commandcatalog.HandlerContract{
		Effect:           commandcatalog.Write,
		HasMutableTarget: true,
		Route:            "contextual/workspace/chat/open",
		Classification:   commandcatalog.HandlerBackend,
	}
	return commandcatalog.Definition{
		ID:               commandWorkspaceChatOpenID,
		Effect:           commandcatalog.Write,
		Decision:         commandcatalog.NoDecision,
		HasMutableTarget: true,
		AllowedSources:   []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{
			Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion,
		}}},
		Presentation: &commandcatalog.Presentation{Version: "workspace-context-chat-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Abrir chat contextual", Description: "Abre o chat contextual da aba ativa", Category: "Workspace", Aliases: []string{"abrir chat", "chat contextual"}},
			"en":    {Name: "Open contextual chat", Description: "Opens the contextual chat for the active tab", Category: "Workspace", Aliases: []string{"open chat", "contextual chat"}},
			"es":    {Name: "Abrir chat contextual", Description: "Abre el chat contextual de la pestaña activa", Category: "Workspace", Aliases: []string{"abrir chat", "chat contextual"}},
		}},
		ArgumentsSchema:       &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema:          &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk:                  commandcatalog.RiskLow,
		Persistence:           commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes:                []commandcatalog.Scope{commandcatalog.ScopeWorkspace},
		Availability:          commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute:          contract.Route,
		HandlerClassification: contract.Classification,
	}, contract
}
