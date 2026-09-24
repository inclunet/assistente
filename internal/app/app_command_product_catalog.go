package app

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

const (
	// commandProductWorkspaceListID é o ID canônico de D2 para a primeira operação
	// produtiva de leitura do workspace. Ele é definido pelo bootstrap, nunca
	// recebido ou derivado de um cliente.
	commandProductWorkspaceListID = "workspace.list"
	commandProductShortcutsShowID = "help.shortcuts.show"
	commandProductRegistryVersion = "product-v40-agent-commands"
)

// CommandWorkspaceMetadata é deliberadamente menor que workspace.WorkspaceInfo:
// o path do usuário não faz parte do resultado persistível do comando.
type CommandWorkspaceMetadata struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Profile  string `json:"profile"`
	TabCount int    `json:"tab_count"`
	IsActive bool   `json:"is_active"`
}

type workspaceListResult struct {
	Workspaces []CommandWorkspaceMetadata `json:"workspaces"`
}

// commandProductCatalog monta o snapshot produtivo do catálogo e seus
// handlers backend. O snapshot é independente do lifecycle central para que o
// bootstrap possa incorporá-lo sem criar uma rota alternativa de execução.
func (a *App) commandProductCatalog() (*commandcatalog.Registry, map[string]commandexecution.Handler, error) {
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Listar workspaces", Description: "Lista os workspaces disponíveis", Category: "Workspace"},
		"en":    {Name: "List workspaces", Description: "Lists available workspaces", Category: "Workspace"},
		"es":    {Name: "Listar workspaces", Description: "Lista los workspaces disponibles", Category: "Workspace"},
	}
	definition := commandcatalog.Definition{
		ID:              commandProductWorkspaceListID,
		Effect:          commandcatalog.Read,
		Decision:        commandcatalog.NoDecision,
		AllowedSources:  []commandcatalog.Source{commandcatalog.KeyboardLocal, commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat},
		Context:         commandcatalog.ContextPolicy{None: true},
		Presentation:    &commandcatalog.Presentation{Version: "workspace-v1", Locales: locales},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema: &commandcatalog.Schema{
			Type: commandcatalog.SchemaObject,
			Properties: map[string]commandcatalog.Schema{
				"workspaces": {
					Type: commandcatalog.SchemaArray,
					Items: &commandcatalog.Schema{
						Type: commandcatalog.SchemaObject,
						Properties: map[string]commandcatalog.Schema{
							"id":        {Type: commandcatalog.SchemaString},
							"name":      {Type: commandcatalog.SchemaString},
							"profile":   {Type: commandcatalog.SchemaString},
							"tab_count": {Type: commandcatalog.SchemaInteger},
							"is_active": {Type: commandcatalog.SchemaBoolean},
						},
						Required: []string{"id", "name", "profile", "tab_count", "is_active"},
					},
				},
			},
			Required: []string{"workspaces"},
		},
		Risk:                  commandcatalog.RiskLow,
		Persistence:           commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceRedacted, Audit: commandcatalog.PersistenceRedacted},
		Scopes:                []commandcatalog.Scope{commandcatalog.ScopeSession},
		Availability:          commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute:          "internal/workspace/list",
		HandlerClassification: commandcatalog.HandlerBackend,
	}
	handler := commandcatalog.HandlerContract{
		Effect:         commandcatalog.Read,
		Route:          definition.HandlerRoute,
		Classification: commandcatalog.HandlerBackend,
	}
	uiDefinition, uiHandler := commandShortcutsRegistration()
	focusDefinition, focusHandler := commandWorkspacePanelFocusRegistration()
	chatDefinition, chatHandler := commandWorkspaceTabCreateRegistration(commandWorkspaceTabChatCreateID)
	editorDefinition, editorHandler := commandWorkspaceTabCreateRegistration(commandWorkspaceTabEditorCreateID)
	tasklistDefinition, tasklistHandler := commandWorkspaceTabCreateRegistration(commandWorkspaceTabTasklistCreateID)
	terminalDefinition, terminalHandler := commandWorkspaceTabCreateRegistration(commandWorkspaceTabTerminalCreateID)
	closeDefinition, closeHandler := commandWorkspaceTabCloseRegistration()
	workspaceCreateDefinition, workspaceCreateHandler := commandWorkspaceCreateRegistration()
	workspaceChatOpenDefinition, workspaceChatOpenHandler := commandWorkspaceChatOpenRegistration()
	modeRegistrations := make([]commandcatalog.Registration, 0, 3)
	for _, commandID := range []string{commandEditorModeMarkdownID, commandEditorModeRichID, commandEditorModeViewID} {
		modeDefinition, modeHandler := commandEditorModeRegistration(commandID)
		modeRegistrations = append(modeRegistrations, commandcatalog.Registration{Definition: modeDefinition, Handler: modeHandler})
	}
	fileRegistrations := make([]commandcatalog.Registration, 0, 3)
	for _, commandID := range []string{commandEditorFileOpenID, commandEditorFileSaveID, commandEditorFileSaveCopyID} {
		definition, handler := editorFileCommandRegistration(commandID)
		fileRegistrations = append(fileRegistrations, commandcatalog.Registration{Definition: definition, Handler: handler})
	}
	formatRegistrations := make([]commandcatalog.Registration, 0, len(commandEditorFormatIDs))
	for _, commandID := range commandEditorFormatIDs {
		definition, handler := editorFormatCommandRegistration(commandID)
		formatRegistrations = append(formatRegistrations, commandcatalog.Registration{Definition: definition, Handler: handler})
	}
	registrations := []commandcatalog.Registration{{Definition: definition, Handler: handler}, {Definition: uiDefinition, Handler: uiHandler}, {Definition: focusDefinition, Handler: focusHandler}, {Definition: chatDefinition, Handler: chatHandler}, {Definition: editorDefinition, Handler: editorHandler}, {Definition: tasklistDefinition, Handler: tasklistHandler}, {Definition: terminalDefinition, Handler: terminalHandler}, {Definition: closeDefinition, Handler: closeHandler}, {Definition: workspaceCreateDefinition, Handler: workspaceCreateHandler}, {Definition: workspaceChatOpenDefinition, Handler: workspaceChatOpenHandler}}
	registrations = append(registrations, commandTerminalSessionRegistrations()...)
	registrations = append(registrations, modeRegistrations...)
	registrations = append(registrations, fileRegistrations...)
	registrations = append(registrations, formatRegistrations...)
	for _, commandID := range commandWorkspaceTabNavigationIDs {
		navigationDefinition, navigationHandler := commandWorkspaceTabNavigationRegistration(commandID)
		registrations = append(registrations, commandcatalog.Registration{Definition: navigationDefinition, Handler: navigationHandler})
	}
	registrations = append(registrations, commandNavigationRegistrations()...)
	registrations = append(registrations, commandChatPickerRegistrations()...)
	registrations = append(registrations, commandEditorMenuRegistrations()...)
	registrations = append(registrations, commandPagePresentationRegistrations()...)
	registrations = append(registrations, commandPageMutationRegistrations()...)
	clearDefinition, clearHandler := commandConversationClearRegistration()
	registrations = append(registrations, commandcatalog.Registration{Definition: clearDefinition, Handler: clearHandler})
	registrations = append(registrations, commandChatActionRegistrations()...)
	interruptDefinition, interruptHandler := commandTerminalInterruptRegistration()
	registrations = append(registrations, commandcatalog.Registration{Definition: interruptDefinition, Handler: interruptHandler})
	registrations = append(registrations, commandChatMessageRegistrations()...)
	registrations = append(registrations, commandGlobalRegistrations()...)
	registrations = append(registrations, commandLayerActionRegistrations()...)
	toolRegistrations, toolHandlers := a.commandToolRegistrations()
	registrations = append(registrations, toolRegistrations...)
	registry, err := commandcatalog.NewComplete(registrations)
	if err != nil {
		return nil, nil, err
	}
	if a == nil {
		return registry, nil, commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	managerReady := a.workspaceMgr != nil
	a.authMu.RUnlock()
	if !managerReady {
		return registry, nil, commandexecution.ErrInvalidConfiguration
	}
	handlers := map[string]commandexecution.Handler{
		commandTerminalInterruptID:          {Contract: interruptHandler, Start: a.startCommandUI},
		commandProductWorkspaceListID:       {Contract: handler, Start: a.startWorkspaceList},
		commandProductShortcutsShowID:       {Contract: uiHandler, Start: a.startCommandUI},
		commandWorkspacePanelFocusID:        {Contract: focusHandler, Start: a.startCommandUI},
		commandWorkspaceTabChatCreateID:     {Contract: chatHandler, Start: a.startCommandUI},
		commandWorkspaceTabEditorCreateID:   {Contract: editorHandler, Start: a.startCommandUI},
		commandWorkspaceTabTasklistCreateID: {Contract: tasklistHandler, Start: a.startCommandUI},
		commandWorkspaceTabTerminalCreateID: {Contract: terminalHandler, Start: a.startCommandUI},
		commandWorkspaceTabCloseID:          {Contract: closeHandler, Start: a.startCommandUI},
		commandWorkspaceCreateID:            {Contract: workspaceCreateHandler, Start: a.startCommandUI},
		commandWorkspaceChatOpenID:          {Contract: workspaceChatOpenHandler, Start: a.startCommandUI},
	}
	for commandID, handler := range toolHandlers {
		handlers[commandID] = handler
	}
	for _, registration := range commandTerminalSessionRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI, ExecutionTimeout: 5 * time.Minute}
	}
	for _, registration := range modeRegistrations {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
	}
	for _, registration := range fileRegistrations {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI, ExecutionTimeout: 5 * time.Minute}
	}
	for _, registration := range formatRegistrations {
		handler := commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
		if isEditorFormatDialogCommand(registration.Definition.ID) {
			handler.ExecutionTimeout = 5 * time.Minute
		}
		handlers[registration.Definition.ID] = handler
	}
	for _, commandID := range commandWorkspaceTabNavigationIDs {
		_, navigationHandler := commandWorkspaceTabNavigationRegistration(commandID)
		handlers[commandID] = commandexecution.Handler{Contract: navigationHandler, Start: a.startCommandUI}
	}
	for _, registration := range commandNavigationRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
	}
	for _, registration := range commandChatPickerRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
	}
	for _, registration := range commandEditorMenuRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
	}
	for _, registration := range commandPagePresentationRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
	}
	for _, registration := range commandPageMutationRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI, ExecutionTimeout: 5 * time.Minute}
	}
	handlers[commandConversationClearID] = commandexecution.Handler{Contract: clearHandler, Start: a.startCommandUI, ExecutionTimeout: 5 * time.Minute}
	for _, registration := range commandChatActionRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
	}
	for _, registration := range commandChatMessageRegistrations() {
		handler := commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandUI}
		if registration.Definition.ID == commandMessageDeleteID {
			handler.ExecutionTimeout = 5 * time.Minute
		}
		handlers[registration.Definition.ID] = handler
	}
	for _, registration := range commandGlobalRegistrations() {
		start := a.startCommandUI
		var timeout time.Duration
		if registration.Definition.ID == commandGlobalJobID {
			start = a.startGlobalCommandJob
			timeout = 5 * time.Minute
		}
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: start, ExecutionTimeout: timeout, RuntimeOwnsDeadline: registration.Definition.ID == commandGlobalJobID}
	}
	for _, registration := range commandLayerActionRegistrations() {
		handlers[registration.Definition.ID] = commandexecution.Handler{Contract: registration.Handler, Start: a.startCommandLayerAction}
	}
	return registry, handlers, nil
}

func (a *App) startWorkspaceList(ctx context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
	if ctx == nil {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidRequest
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	if principal != invocation.Principal {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
	}
	a.authMu.RLock()
	manager := a.workspaceMgr
	a.authMu.RUnlock()
	if manager == nil {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidConfiguration
	}
	id, err := uuid.NewV7()
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan commandexecution.Outcome, 1)
	var cancelOnce sync.Once
	go func() {
		defer cancel()
		if workCtx.Err() != nil {
			done <- commandexecution.Outcome{Status: commandledger.Cancelled}
			return
		}
		infos, listErr := manager.List()
		if workCtx.Err() != nil {
			done <- commandexecution.Outcome{Status: commandledger.Cancelled}
			return
		}
		currentPrincipal, principalErr := a.currentCommandPrincipal()
		a.authMu.RLock()
		sameManager := a.workspaceMgr == manager
		a.authMu.RUnlock()
		if principalErr != nil || currentPrincipal != invocation.Principal || !sameManager {
			done <- commandexecution.Outcome{Status: commandledger.Failed}
			return
		}
		if listErr == nil {
			result := workspaceListResult{Workspaces: make([]CommandWorkspaceMetadata, 0, len(infos))}
			for _, info := range infos {
				result.Workspaces = append(result.Workspaces, CommandWorkspaceMetadata{ID: info.ID, Name: info.Name, Profile: info.Profile, TabCount: info.TabCount, IsActive: info.IsActive})
			}
			payload, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				done <- commandexecution.Outcome{Status: commandledger.Failed}
				return
			}
			if workCtx.Err() != nil {
				done <- commandexecution.Outcome{Status: commandledger.Cancelled}
				return
			}
			select {
			case done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: payload}:
				return
			case <-workCtx.Done():
			}
		}
		if workCtx.Err() != nil {
			done <- commandexecution.Outcome{Status: commandledger.Cancelled}
			return
		}
		done <- commandexecution.Outcome{Status: commandledger.Failed}
	}()
	return commandexecution.ExecutionHandle{
		ID:   id.String(),
		Done: done,
		Cancel: func() {
			cancelOnce.Do(cancel)
		},
	}, nil
}
