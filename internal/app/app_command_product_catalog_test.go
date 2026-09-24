package app

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandProductCatalogWorkspaceListIsCompleteAndReadsRealManager(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	manager := workspace.NewManager(homeDir)
	if err := manager.Initialize(workDir); err != nil {
		t.Fatal(err)
	}
	userID := uuid.Must(uuid.NewV7()).String()
	sessionID := uuid.Must(uuid.NewV7()).String()
	app := &App{workspaceMgr: manager, currentUserID: userID, currentAuthUser: &AuthUser{UserID: userID, SessionID: sessionID}}

	registry, handlers, err := app.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Lookup(commandProductWorkspaceListID)
	if !ok || !registry.Complete() || definition.Effect != commandcatalog.Read || definition.Decision != commandcatalog.NoDecision {
		t.Fatalf("registro workspace.list incompleto: complete=%v definition=%+v", registry.Complete(), definition)
	}
	if definition.ArgumentsSchema == nil || definition.ArgumentsSchema.Type != commandcatalog.SchemaObject || definition.ResultSchema == nil || definition.ResultSchema.Type != commandcatalog.SchemaObject {
		t.Fatalf("schemas inesperados: args=%+v result=%+v", definition.ArgumentsSchema, definition.ResultSchema)
	}
	handler, ok := handlers[commandProductWorkspaceListID]
	if !ok || handler.Start == nil || handler.Contract.Route != definition.HandlerRoute {
		t.Fatalf("handler workspace.list ausente ou divergente: %+v", handler)
	}

	started := time.Now()
	handle, err := handler.Start(context.Background(), commandexecution.Invocation{CommandID: commandProductWorkspaceListID, Principal: auth.LocalSessionPrincipal{UserID: userID, SessionID: sessionID}})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("Start bloqueou por %s", time.Since(started))
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Succeeded {
			t.Fatalf("status=%q, esperado succeeded", outcome.Status)
		}
		var result workspaceListResult
		if err := json.Unmarshal(outcome.Result, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Workspaces) != 1 || result.Workspaces[0].ID == "" || result.Workspaces[0].Name == "" {
			t.Fatalf("resultado inesperado: %+v", result)
		}
		if len(outcome.Result) == 0 || bytes.Contains(outcome.Result, []byte(workDir)) {
			t.Fatal("resultado expôs path bruto do workspace")
		}
	case <-time.After(time.Second):
		t.Fatal("workspace.list não concluiu")
	}

	handle, err = handler.Start(context.Background(), commandexecution.Invocation{CommandID: commandProductWorkspaceListID, Principal: auth.LocalSessionPrincipal{UserID: userID, SessionID: sessionID}})
	if err != nil {
		t.Fatal(err)
	}
	handle.Cancel()
	handle.Cancel()
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Succeeded && outcome.Status != commandledger.Cancelled {
			t.Fatalf("cancel retornou status inesperado: %q", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("Cancel não produziu desfecho")
	}
}

func TestCommandProductCatalogNavigationMetadataPaletteAndKeyboard(t *testing.T) {
	a := &App{}
	registry, handlers, err := a.commandProductCatalog()
	if err == nil || registry == nil {
		t.Fatal("catálogo sem workspace manager deveria recusar a montagem")
	}

	// Montagem com manager real é exercitada pelo teste acima; este teste valida
	// o snapshot completo sem depender de estado de UI ou de URL.
	manager := workspace.NewManager(t.TempDir())
	if err := manager.Initialize(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	a.workspaceMgr = manager
	registry, handlers, err = a.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	help, ok := registry.Lookup(commandProductShortcutsShowID)
	if !ok || !help.AllowsSource(commandcatalog.Palette) || !help.AllowsSource(commandcatalog.KeyboardLocal) || !help.AllowsSource(commandcatalog.StreamDeck) || len(help.AllowedSources) != 3 {
		t.Fatalf("help.shortcuts.show deve aceitar Palette, teclado local e StreamDeck: %+v", help)
	}
	if help.Persistence.Result != commandcatalog.PersistenceNever || help.Persistence.Audit != commandcatalog.PersistenceNever {
		t.Fatalf("help local não deve declarar persistência de resultado/auditoria: %+v", help.Persistence)
	}
	focus, ok := registry.Lookup(commandWorkspacePanelFocusID)
	if !ok || focus.Effect != commandcatalog.Read || focus.Decision != commandcatalog.NoDecision ||
		focus.HandlerClassification != commandcatalog.HandlerUI || !focus.Context.None ||
		len(focus.AllowedSources) != 3 || !focus.AllowsSource(commandcatalog.Palette) ||
		!focus.AllowsSource(commandcatalog.KeyboardLocal) || !focus.AllowsSource(commandcatalog.StreamDeck) {
		t.Fatalf("workspace.panel.focus deve ser UI read-only nas três origens: %+v", focus)
	}
	if focus.Persistence.Result != commandcatalog.PersistenceNever || focus.Persistence.Audit != commandcatalog.PersistenceNever {
		t.Fatalf("foco local não deve declarar persistência de resultado/auditoria: %+v", focus.Persistence)
	}
	if focus.Presentation == nil || focus.Presentation.Locales["pt-BR"].Description != "Foca o conteúdo da aba ativa; disponível somente no workspace com painel pronto" ||
		focus.Presentation.Locales["en"].Description != "Focuses the active tab's content; available only in a workspace with a ready panel" ||
		focus.Presentation.Locales["es"].Description != "Enfoca el contenido de la pestaña activa; disponible solo en el espacio de trabajo con un panel listo" {
		t.Fatalf("descrições de foco do workspace inesperadas: %+v", focus.Presentation)
	}
	if handler := handlers[commandWorkspacePanelFocusID]; handler.Start == nil || handler.Contract.Route != focus.HandlerRoute || handler.Contract.Classification != commandcatalog.HandlerUI {
		t.Fatalf("handler UI de foco ausente ou divergente: %+v", handler)
	}

	for _, navigation := range commandProductUINavigation {
		definition, ok := registry.Lookup(navigation.id)
		if !ok {
			t.Fatalf("comando ausente: %s", navigation.id)
		}
		wantSources := 3
		if commandExternalUICommandSupported(navigation.id) {
			wantSources++
		}
		if definition.Effect != commandcatalog.Read || definition.Decision != commandcatalog.NoDecision || definition.HandlerClassification != commandcatalog.HandlerUI || len(definition.AllowedSources) != wantSources || definition.AllowsSource(commandcatalog.UI) != (navigation.uiAction || commandExternalUICommandSupported(navigation.id)) || !definition.AllowsSource(commandcatalog.Palette) || !definition.AllowsSource(commandcatalog.KeyboardLocal) || !definition.AllowsSource(commandcatalog.StreamDeck) {
			t.Fatalf("contrato de navegação inesperado: %+v", definition)
		}
		if definition.Persistence.Result != commandcatalog.PersistenceNever || definition.Persistence.Audit != commandcatalog.PersistenceNever {
			t.Fatalf("navegação local não deve declarar persistência de resultado/auditoria: %+v", definition.Persistence)
		}
		if definition.ArgumentsSchema == nil || definition.ArgumentsSchema.Type != commandcatalog.SchemaObject || len(definition.ArgumentsSchema.Properties) != 0 || len(definition.ArgumentsSchema.Required) != 0 {
			t.Fatalf("schema de argumentos não vazio: %+v", definition.ArgumentsSchema)
		}
		for locale, metadata := range definition.Presentation.Locales {
			if metadata.Name == "" || metadata.Description == "" || metadata.Category == "" || len(metadata.Aliases) == 0 {
				t.Fatalf("metadata incompleto em %s/%s: %+v", navigation.id, locale, metadata)
			}
		}
		if handler := handlers[navigation.id]; handler.Start == nil || handler.Contract.Route != definition.HandlerRoute {
			t.Fatalf("handler UI ausente ou divergente em %s: %+v", navigation.id, handler)
		}
	}
	for _, picker := range commandProductChatPickers {
		wantSources := 3
		if picker.uiAction {
			wantSources++
		}
		definition, ok := registry.Lookup(picker.id)
		if !ok {
			t.Fatalf("picker ausente: %s", picker.id)
		}
		if definition.Effect != commandcatalog.Read || definition.Decision != commandcatalog.NoDecision || definition.HandlerClassification != commandcatalog.HandlerUI || !definition.Context.None || len(definition.AllowedSources) != wantSources || definition.AllowsSource(commandcatalog.UI) != picker.uiAction || !definition.AllowsSource(commandcatalog.Palette) || !definition.AllowsSource(commandcatalog.KeyboardLocal) || !definition.AllowsSource(commandcatalog.StreamDeck) {
			t.Fatalf("contrato de picker inesperado: %+v", definition)
		}
		if definition.Persistence.Arguments != commandcatalog.PersistenceNever || definition.Persistence.Result != commandcatalog.PersistenceNever || definition.Persistence.Audit != commandcatalog.PersistenceNever {
			t.Fatalf("picker local possui política de persistência: %+v", definition.Persistence)
		}
		if definition.Presentation == nil || len(definition.Presentation.Locales) != 3 {
			t.Fatalf("metadata de picker incompleto: %+v", definition.Presentation)
		}
		for _, locale := range []string{"pt-BR", "en", "es"} {
			metadata := definition.Presentation.Locales[locale]
			if metadata.Name == "" || metadata.Description == "" || metadata.Category != "Chat" || len(metadata.Aliases) == 0 {
				t.Fatalf("metadata incompleto em %s/%s: %+v", picker.id, locale, metadata)
			}
		}
		if handler := handlers[picker.id]; handler.Start == nil || handler.Contract.Route != definition.HandlerRoute || handler.Contract.Classification != commandcatalog.HandlerUI {
			t.Fatalf("handler UI ausente ou divergente em %s: %+v", picker.id, handler)
		}
	}
	for _, editorCommand := range commandProductEditorMenus {
		definition, ok := registry.Lookup(editorCommand.id)
		if !ok || definition.Effect != commandcatalog.Read || definition.Decision != commandcatalog.NoDecision || definition.HandlerClassification != commandcatalog.HandlerUI || !definition.Context.None || len(definition.AllowedSources) != 3 || definition.Persistence.Arguments != commandcatalog.PersistenceNever || definition.Persistence.Result != commandcatalog.PersistenceNever || definition.Persistence.Audit != commandcatalog.PersistenceNever {
			t.Fatalf("contrato de comando editor inesperado: %s %+v", editorCommand.id, definition)
		}
		for _, locale := range []string{"pt-BR", "en", "es"} {
			metadata := definition.Presentation.Locales[locale]
			if metadata.Name == "" || metadata.Description == "" || metadata.Category != "Editor" || len(metadata.Aliases) == 0 {
				t.Fatalf("metadata editor incompleto em %s/%s: %+v", editorCommand.id, locale, metadata)
			}
		}
		if handler := handlers[editorCommand.id]; handler.Start == nil || handler.Contract.Route != definition.HandlerRoute || handler.Contract.Classification != commandcatalog.HandlerUI {
			t.Fatalf("handler editor ausente ou divergente em %s: %+v", editorCommand.id, handler)
		}
	}
	for _, id := range commandEditorFormatIDs {
		handler, ok := handlers[id]
		if !ok || handler.Start == nil {
			t.Fatalf("handler de formatação ausente: %s %+v", id, handler)
		}
		if isEditorFormatDialogCommand(id) {
			if handler.ExecutionTimeout != 5*time.Minute || commandUIRunTimeout(id) != 5*time.Minute || commandUIResultTTL(id) <= commandUIRunTimeout(id) {
				t.Fatalf("diálogo de formatação sem janela de 5 minutos: %s handler=%s run=%s ttl=%s", id, handler.ExecutionTimeout, commandUIRunTimeout(id), commandUIResultTTL(id))
			}
		} else if handler.ExecutionTimeout != 0 || commandUIRunTimeout(id) != 30*time.Second {
			t.Fatalf("formatação não-dialogada recebeu timeout especial: %s handler=%s run=%s", id, handler.ExecutionTimeout, commandUIRunTimeout(id))
		}
	}
}

func TestCommandProductLocalUIClassificationIsClosedAndFailClosed(t *testing.T) {
	a := &App{}
	registry, _, err := func() (*commandcatalog.Registry, map[string]commandexecution.Handler, error) {
		manager := workspace.NewManager(t.TempDir())
		if err := manager.Initialize(t.TempDir()); err != nil {
			t.Fatal(err)
		}
		a.workspaceMgr = manager
		return a.commandProductCatalog()
	}()
	if err != nil {
		t.Fatal(err)
	}
	localIDs := append(append([]string{}, commandWorkspaceTabNavigationIDs...), commandProductShortcutsShowID, commandWorkspacePanelFocusID)
	for _, item := range commandProductUINavigation {
		localIDs = append(localIDs, item.id)
	}
	for _, item := range commandProductChatPickers {
		localIDs = append(localIDs, item.id)
	}
	for _, item := range commandProductEditorMenus {
		localIDs = append(localIDs, item.id)
	}
	for _, id := range localIDs {
		definition, ok := registry.Lookup(id)
		if !ok || commandExecutionClassForDefinition(definition) != commandExecutionLocalUI {
			t.Fatalf("%s não foi classificado como local_ui: %+v", id, definition)
		}
		if definition.Persistence.Result != commandcatalog.PersistenceNever || definition.Persistence.Audit != commandcatalog.PersistenceNever {
			t.Fatalf("%s local possui política de persistência: %+v", id, definition.Persistence)
		}
		mutated := definition
		mutated.Effect = commandcatalog.Write
		if commandExecutionClassForDefinition(mutated) == commandExecutionLocalUI {
			t.Fatalf("%s permaneceu local_ui após alteração de efeito", id)
		}
		mutated = definition
		mutated.HandlerClassification = commandcatalog.HandlerBackend
		if commandExecutionClassForDefinition(mutated) == commandExecutionLocalUI {
			t.Fatalf("%s permaneceu local_ui após alteração de handler", id)
		}
	}
}
