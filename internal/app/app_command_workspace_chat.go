package app

import (
	"assistente/internal/commandcatalog"
	"assistente/internal/workspace"
)

type commandExecutionClass uint8

const (
	commandExecutionUnknown commandExecutionClass = iota
	commandExecutionDurable
	commandExecutionAuditedUI
	commandExecutionLocalUI
)

const (
	commandChatModelOpenID                = "chat.model.open"
	commandChatHistoryOpenID              = "chat.history.open"
	commandChatProfileOpenID              = "chat.profile.open"
	commandChatPinnedOpenID               = "chat.pinned.open"
	commandChatTokensOpenID               = "chat.tokens.open"
	commandEditorMenuFileOpenID           = "editor.menu.file.open"
	commandEditorMenuInsertOpenID         = "editor.menu.insert.open"
	commandEditorMenuFormatOpenID         = "editor.menu.format.open"
	commandEditorMenuModeOpenID           = "editor.menu.mode.open"
	commandEditorSlidesOpenID             = "editor.slides.open"
	commandEditorPresentationFullscreenID = "editor.presentation.fullscreen"
	commandEditorModeMarkdownID           = "editor.mode.markdown"
	commandEditorModeRichID               = "editor.mode.rich"
	commandEditorModeViewID               = "editor.mode.view"

	commandWorkspaceTabChatCreateID     = "workspace.tab.chat.create"
	commandWorkspaceTabEditorCreateID   = "workspace.tab.editor.create"
	commandWorkspaceTabTerminalCreateID = "workspace.tab.terminal.create"
	commandWorkspaceTabTasklistCreateID = "workspace.tab.tasklist.create"
	commandWorkspaceTabCloseID          = "workspace.tab.close"
	commandWorkspaceTabNextID           = "workspace.tab.next"
	commandWorkspaceTabPreviousID       = "workspace.tab.previous"
	commandWorkspaceTabFirstID          = "workspace.tab.first"
	commandWorkspaceTabSecondID         = "workspace.tab.second"
	commandWorkspaceTabThirdID          = "workspace.tab.third"
	commandWorkspaceTabFourthID         = "workspace.tab.fourth"
	commandWorkspaceTabFifthID          = "workspace.tab.fifth"
	commandWorkspaceTabSixthID          = "workspace.tab.sixth"
	commandWorkspaceTabSeventhID        = "workspace.tab.seventh"
	commandWorkspaceTabEighthID         = "workspace.tab.eighth"
	commandWorkspaceTabNinthID          = "workspace.tab.ninth"
	commandWorkspaceTabGoToID           = "workspace.tab.go_to"
)

var commandWorkspaceTabNavigationIDs = []string{
	commandWorkspaceTabNextID, commandWorkspaceTabPreviousID,
	commandWorkspaceTabFirstID, commandWorkspaceTabSecondID, commandWorkspaceTabThirdID,
	commandWorkspaceTabFourthID, commandWorkspaceTabFifthID, commandWorkspaceTabSixthID,
	commandWorkspaceTabSeventhID, commandWorkspaceTabEighthID, commandWorkspaceTabNinthID, commandWorkspaceTabGoToID,
}

func workspaceTabTypeForCommand(commandID string) (workspace.TabType, bool) {
	// O alvo é deliberadamente fechado e imutável: a UI nunca fornece nem
	// transforma um tipo arbitrário em uma escrita de workspace.
	switch commandID {
	case commandWorkspaceTabChatCreateID:
		return workspace.TabTypeChat, true
	case commandWorkspaceTabEditorCreateID:
		return workspace.TabTypeEditor, true
	case commandWorkspaceTabTerminalCreateID:
		return workspace.TabTypeTerminal, true
	case commandWorkspaceTabTasklistCreateID:
		return workspace.TabTypeTasklist, true
	default:
		return "", false
	}
}

func isWorkspaceTabCreateCommand(commandID string) bool {
	_, ok := workspaceTabTypeForCommand(commandID)
	return ok
}

func isWorkspaceTabMutationCommand(commandID string) bool {
	return isWorkspaceTabCreateCommand(commandID) || commandID == commandWorkspaceTabCloseID
}

// Inclui mutações de workspace que não alteram uma aba. A criação conserva
// o snapshot de origem para impedir uma confirmação em outro contexto.
func isWorkspaceMutationCommand(commandID string) bool {
	_, editorMode := editorModeForCommand(commandID)
	return isPageMutationCommand(commandID) || isWorkspaceTabMutationCommand(commandID) || commandID == commandWorkspaceCreateID || commandID == commandWorkspaceChatOpenID || commandID == commandConversationClearID || commandID == commandTerminalInterruptID || commandID == commandTerminalSessionCreateID || commandID == commandTerminalSessionCloseID || isChatActionCommand(commandID) || isChatMessageBackendCommand(commandID) || editorMode || isEditorFileCommand(commandID)
}

// commandExecutionClassForID é a política de produto fechada. Read não implica
// execução local: somente os IDs abaixo podem escapar do executor auditado.
func commandExecutionClassForID(commandID string) commandExecutionClass {
	for _, item := range commandProductPagePresentation {
		if commandID == item.id {
			return commandExecutionLocalUI
		}
	}
	if commandID == commandProductShortcutsShowID || commandID == commandWorkspacePanelFocusID {
		return commandExecutionLocalUI
	}
	for _, id := range commandWorkspaceTabNavigationIDs {
		if commandID == id {
			return commandExecutionLocalUI
		}
	}
	for _, navigation := range commandProductUINavigation {
		if commandID == navigation.id {
			return commandExecutionLocalUI
		}
	}
	for _, picker := range commandProductChatPickers {
		if commandID == picker.id {
			return commandExecutionLocalUI
		}
	}
	for _, editorCommand := range commandProductEditorMenus {
		if commandID == editorCommand.id {
			return commandExecutionLocalUI
		}
	}
	return commandExecutionUnknown
}

func commandExecutionClassForDefinition(definition commandcatalog.Definition) commandExecutionClass {
	if local := commandExecutionClassForID(definition.ID); local != commandExecutionUnknown {
		if definition.Effect == commandcatalog.Read && definition.HandlerClassification == commandcatalog.HandlerUI &&
			!definition.HasMutableTarget && !definition.MutatesEffectiveCapability && definition.Decision == commandcatalog.NoDecision {
			return local
		}
		return commandExecutionUnknown
	}
	switch definition.HandlerClassification {
	case commandcatalog.HandlerUI:
		return commandExecutionAuditedUI
	case commandcatalog.HandlerBackend, commandcatalog.HandlerInternal, commandcatalog.HandlerTool, commandcatalog.HandlerJob:
		return commandExecutionDurable
	default:
		return commandExecutionUnknown
	}
}

func isLocalUICommand(commandID string) bool {
	return commandExecutionClassForID(commandID) == commandExecutionLocalUI
}

func workspaceTabNavigationForCommand(commandID string) (direction, position int, ok bool) {
	if commandID == commandWorkspaceTabGoToID {
		return 0, 0, true
	}
	switch commandID {
	case commandWorkspaceTabNextID:
		return 1, 0, true
	case commandWorkspaceTabPreviousID:
		return -1, 0, true
	case commandWorkspaceTabFirstID:
		return 0, 1, true
	case commandWorkspaceTabSecondID:
		return 0, 2, true
	case commandWorkspaceTabThirdID:
		return 0, 3, true
	case commandWorkspaceTabFourthID:
		return 0, 4, true
	case commandWorkspaceTabFifthID:
		return 0, 5, true
	case commandWorkspaceTabSixthID:
		return 0, 6, true
	case commandWorkspaceTabSeventhID:
		return 0, 7, true
	case commandWorkspaceTabEighthID:
		return 0, 8, true
	case commandWorkspaceTabNinthID:
		return 0, 9, true
	default:
		return 0, 0, false
	}
}

func commandWorkspaceTabNavigationRegistration(commandID string) (commandcatalog.Definition, commandcatalog.HandlerContract) {
	if commandID == commandWorkspaceTabGoToID {
		contract := commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "ui/workspace/tab/navigate", Classification: commandcatalog.HandlerUI}
		locales := map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Ir para aba", Description: "Vai para uma aba do workspace vinculado", Category: "Workspace", Aliases: []string{"aba", "destino"}},
			"en":    {Name: "Go to tab", Description: "Goes to a tab in the linked workspace", Category: "Workspace", Aliases: []string{"tab", "target"}},
			"es":    {Name: "Ir a pestaña", Description: "Va a una pestaña del espacio de trabajo vinculado", Category: "Workspace", Aliases: []string{"pestaña", "destino"}},
		}
		minimum := float64(1)
		return commandcatalog.Definition{
			ID: commandID, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
			AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
			Context:        commandcatalog.ContextPolicy{None: true},
			Presentation:   &commandcatalog.Presentation{Version: "workspace-tab-go-to-v1", Locales: locales},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
				"workspace_id": {Type: commandcatalog.SchemaString},
				"target_mode":  {Type: commandcatalog.SchemaString, Enum: []any{"position", "specific"}},
				"position":     {Type: commandcatalog.SchemaInteger, Optional: true, Minimum: &minimum},
				"tab_id":       {Type: commandcatalog.SchemaString, Optional: true},
			}, Required: []string{"workspace_id", "target_mode"}},
			ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			Risk:         commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceNever},
			Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
		}, contract
	}
	direction, position, ok := workspaceTabNavigationForCommand(commandID)
	if !ok || (direction == 0) == (position == 0) {
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	pt, en, es := "", "", ""
	switch commandID {
	case commandWorkspaceTabNextID:
		pt, en, es = "Próxima aba", "Next tab", "Siguiente pestaña"
	case commandWorkspaceTabPreviousID:
		pt, en, es = "Aba anterior", "Previous tab", "Pestaña anterior"
	case commandWorkspaceTabFirstID, commandWorkspaceTabSecondID, commandWorkspaceTabThirdID, commandWorkspaceTabFourthID, commandWorkspaceTabFifthID, commandWorkspaceTabSixthID, commandWorkspaceTabSeventhID, commandWorkspaceTabEighthID, commandWorkspaceTabNinthID:
		pt, en, es = navigationPositionMetadata(position)
	default:
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "ui/workspace/tab/navigate", Classification: commandcatalog.HandlerUI}
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: pt, Description: "Navega para outra aba no workspace atual", Category: "Workspace"},
		"en":    {Name: en, Description: "Navigates to another tab in the current workspace", Category: "Workspace"},
		"es":    {Name: es, Description: "Navega a otra pestaña en el espacio de trabajo actual", Category: "Workspace"},
	}
	return commandcatalog.Definition{
		ID: commandID, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources:  []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck, commandcatalog.UI},
		Context:         commandcatalog.ContextPolicy{None: true},
		Presentation:    &commandcatalog.Presentation{Version: "workspace-tab-navigation-v1", Locales: locales},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceNever},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
	}, contract
}

func navigationPositionMetadata(position int) (pt, en, es string) {
	words := []struct{ pt, en, es string }{
		{"Primeira aba", "First tab", "Primera pestaña"}, {"Segunda aba", "Second tab", "Segunda pestaña"}, {"Terceira aba", "Third tab", "Tercera pestaña"},
		{"Quarta aba", "Fourth tab", "Cuarta pestaña"}, {"Quinta aba", "Fifth tab", "Quinta pestaña"}, {"Sexta aba", "Sixth tab", "Sexta pestaña"},
		{"Sétima aba", "Seventh tab", "Séptima pestaña"}, {"Oitava aba", "Eighth tab", "Octava pestaña"}, {"Nona aba", "Ninth tab", "Novena pestaña"},
	}
	if position < 1 || position > len(words) {
		return "", "", ""
	}
	item := words[position-1]
	return item.pt, item.en, item.es
}

func commandWorkspaceTabCreateRegistration(commandID string) (commandcatalog.Definition, commandcatalog.HandlerContract) {
	tabType, ok := workspaceTabTypeForCommand(commandID)
	if !ok {
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	contract := commandcatalog.HandlerContract{
		Effect:           commandcatalog.Write,
		HasMutableTarget: true,
		Route:            "contextual/workspace/tab/" + string(tabType) + "/create",
		Classification:   commandcatalog.HandlerBackend,
	}
	var pt, en, es commandcatalog.LocalizedMetadata
	switch tabType {
	case workspace.TabTypeChat:
		pt = commandcatalog.LocalizedMetadata{Name: "Criar aba de chat", Description: "Cria uma nova aba de chat no workspace atual", Category: "Workspace", Aliases: []string{"novo chat", "aba de chat"}}
		en = commandcatalog.LocalizedMetadata{Name: "Create chat tab", Description: "Creates a new chat tab in the current workspace", Category: "Workspace", Aliases: []string{"new chat", "chat tab"}}
		es = commandcatalog.LocalizedMetadata{Name: "Crear pestaña de chat", Description: "Crea una nueva pestaña de chat en el espacio de trabajo actual", Category: "Workspace", Aliases: []string{"nuevo chat", "pestaña de chat"}}
	case workspace.TabTypeEditor:
		pt = commandcatalog.LocalizedMetadata{Name: "Criar aba de editor", Description: "Cria uma nova aba de editor no workspace atual", Category: "Workspace", Aliases: []string{"novo editor", "aba de editor"}}
		en = commandcatalog.LocalizedMetadata{Name: "Create editor tab", Description: "Creates a new editor tab in the current workspace", Category: "Workspace", Aliases: []string{"new editor", "editor tab"}}
		es = commandcatalog.LocalizedMetadata{Name: "Crear pestaña de editor", Description: "Crea una nueva pestaña de editor en el espacio de trabajo actual", Category: "Workspace", Aliases: []string{"nuevo editor", "pestaña de editor"}}
	case workspace.TabTypeTasklist:
		pt = commandcatalog.LocalizedMetadata{Name: "Criar aba de lista de tarefas", Description: "Cria uma nova aba de lista de tarefas no workspace atual", Category: "Workspace", Aliases: []string{"nova lista de tarefas", "aba de tarefas"}}
		en = commandcatalog.LocalizedMetadata{Name: "Create task list tab", Description: "Creates a new task list tab in the current workspace", Category: "Workspace", Aliases: []string{"new task list", "task list tab"}}
		es = commandcatalog.LocalizedMetadata{Name: "Crear pestaña de lista de tareas", Description: "Crea una nueva pestaña de lista de tareas en el espacio de trabajo actual", Category: "Workspace", Aliases: []string{"nueva lista de tareas", "pestaña de tareas"}}
	case workspace.TabTypeTerminal:
		pt = commandcatalog.LocalizedMetadata{Name: "Criar aba de terminal", Description: "Cria uma nova aba e uma nova sessão de terminal no workspace atual", Category: "Workspace", Aliases: []string{"novo terminal", "aba de terminal"}}
		en = commandcatalog.LocalizedMetadata{Name: "Create terminal tab", Description: "Creates a new terminal tab and terminal session in the current workspace", Category: "Workspace", Aliases: []string{"new terminal", "terminal tab"}}
		es = commandcatalog.LocalizedMetadata{Name: "Crear pestaña de terminal", Description: "Crea una nueva pestaña y una nueva sesión de terminal en el espacio de trabajo actual", Category: "Workspace", Aliases: []string{"nuevo terminal", "pestaña de terminal"}}
	default:
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	return commandcatalog.Definition{
		ID:                    commandID,
		Effect:                commandcatalog.Write,
		Decision:              commandcatalog.NoDecision,
		HasMutableTarget:      true,
		AllowedSources:        []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context:               commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		Presentation:          &commandcatalog.Presentation{Version: "workspace-tab-" + string(tabType) + "-create-v1", Locales: map[string]commandcatalog.LocalizedMetadata{"pt-BR": pt, "en": en, "es": es}},
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

func commandWorkspaceTabCloseRegistration() (commandcatalog.Definition, commandcatalog.HandlerContract) {
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Write, HasMutableTarget: true, Route: "contextual/workspace/tab/close", Classification: commandcatalog.HandlerBackend}
	return commandcatalog.Definition{
		ID: commandWorkspaceTabCloseID, Effect: commandcatalog.Write, Decision: commandcatalog.NoDecision, HasMutableTarget: true,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context:        commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		Presentation: &commandcatalog.Presentation{Version: "workspace-tab-close-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Fechar aba ativa", Description: "Fecha a aba ativa no workspace atual", Category: "Workspace", Aliases: []string{"fechar aba", "encerrar aba"}},
			"en":    {Name: "Close active tab", Description: "Closes the active tab in the current workspace", Category: "Workspace", Aliases: []string{"close tab", "close active tab"}},
			"es":    {Name: "Cerrar pestaña activa", Description: "Cierra la pestaña activa en el espacio de trabajo actual", Category: "Workspace", Aliases: []string{"cerrar pestaña", "cerrar pestaña activa"}},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
	}, contract
}
