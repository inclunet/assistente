package app

import (
	"bytes"
	"context"
	"testing"

	"assistente/internal/commandexecution"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
)

func TestCommandStorageSettingsRestart(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	db := database.DB()
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.CredentialKeyWrap{}); err != nil {
		t.Fatal(err)
	}

	dek := bytes.Repeat([]byte{0x42}, 32)
	persistentManager := credentials.NewManagerWithStore(dek, credentials.NewDBStore(), true)
	app.credMgr = persistentManager
	if err := persistentManager.LoadInstanceSecrets(ctx); err != nil {
		t.Fatal("carregar cofre persistente inicial", err)
	}
	if err := app.prepareCommandStorage(ctx, db, persistentManager); err != nil {
		t.Fatal("preparar armazenamento antes da montagem", err)
	}

	decisions := appCommandImportWailsCopyDecisions(t, app)
	if err := app.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		t.Fatal("montar produto real", err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal("projetar configuração inicial", err)
	}
	if err := BootstrapCommandLifecycle(ctx, app); err != nil {
		t.Fatal("bootstrap inicial", err)
	}

	layer := settingsActivationSecurityConfirmed(t, app, decisions, func() (CommandSettingsMutation, error) {
		return app.SaveCommandLayer(CommandLayerEdit{Name: "Camada persistente de restart", Enabled: true})
	})
	shortcutSpec := `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`
	binding := settingsActivationSecurityConfirmed(t, app, decisions, func() (CommandSettingsMutation, error) {
		return app.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer.ID, CommandID: commandProductWorkspaceListID,
			TriggerType: "keyboard.local", TriggerSpec: shortcutSpec, Enabled: true,
		})
	})
	settingsActivationSecurityConfirmed(t, app, decisions, func() (CommandSettingsMutation, error) {
		return app.PrepareManualCommandLayer(layer.ID)
	})
	if _, err := app.SetCommandLayerActive(layer.ID, true); err != nil {
		t.Fatal("ativar camada", err)
	}

	before, err := app.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal("ler configuração antes do restart", err)
	}
	if !commandStorageSettingsContains(before, layer.ID, binding.ID, shortcutSpec) {
		t.Fatalf("configuração salva não encontrada antes do restart: %+v", before)
	}
	keyboard, err := app.GetLocalCommandKeyboardMap()
	if err != nil || len(keyboard.Bindings) != 63 {
		t.Fatalf("mapa de teclado antes do restart: %+v err=%v", keyboard, err)
	}
	custom := commandKeyboardBindingFor(t, keyboard, LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}})
	if result, err := app.DispatchLocalCommandKey(keyboard.Generation, custom.Shortcut, "down", false); err != nil || result == nil || result.Status != "succeeded" {
		t.Fatalf("dispatch antes do restart: %+v err=%v", result, err)
	}

	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal("shutdown do lifecycle", err)
	}
	if err := app.drainCommandExecutors(ctx); err != nil {
		t.Fatal("shutdown do executor", err)
	}

	reloadedManager := credentials.NewManagerWithStore(dek, credentials.NewDBStore(), true)
	if err := reloadedManager.LoadInstanceSecrets(ctx); err != nil {
		t.Fatal("recarregar cofre persistente", err)
	}
	reloaded := &App{
		ctx: app.ctx, sessionSvc: app.sessionSvc, credMgr: reloadedManager,
		workspaceMgr: app.workspaceMgr, questionnaireMgr: questionnaire.NewManager(func(string, any) {}),
		authKeyringDelete: func() error { return nil },
	}
	t.Cleanup(func() {
		_ = ShutdownCommandLifecycle(ctx, reloaded)
		_ = reloaded.drainCommandExecutors(ctx)
		_ = reloaded.shutdownCommandBridgeIfConfigured(ctx)
	})
	reloaded.setCurrentUserID(app.currentUserID)
	reloaded.setCurrentAuthUser(&AuthUser{UserID: app.currentAuthUser.UserID, SessionID: app.currentAuthUser.SessionID, Role: app.currentAuthUser.Role})
	epochs, err := reloaded.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	reloaded.commandHost, err = commandexecution.NewHostState(epochs, commandProductRegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.commandHost.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.prepareCommandStorage(ctx, db, reloadedManager); err != nil {
		t.Fatalf("preparar armazenamento após restart: %v", err)
	}
	_ = appCommandImportWailsCopyDecisions(t, reloaded)
	if err := reloaded.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		t.Fatal("montar produto após restart", err)
	}
	if err := reloaded.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal("reprojetar configuração após restart", err)
	}
	if err := BootstrapCommandLifecycle(ctx, reloaded); err != nil {
		t.Fatal("bootstrap após restart", err)
	}

	after, err := reloaded.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal("ler configuração após restart", err)
	}
	if !commandStorageSettingsContains(after, layer.ID, binding.ID, shortcutSpec) {
		t.Fatalf("camada/binding não sobreviveram ao restart: %+v", after)
	}
	keyboard, err = reloaded.GetLocalCommandKeyboardMap()
	if err != nil || len(keyboard.Bindings) != 63 {
		t.Fatalf("mapa de teclado após restart: %+v err=%v", keyboard, err)
	}
	custom = commandKeyboardBindingFor(t, keyboard, LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}})
	if result, err := reloaded.DispatchLocalCommandKey(keyboard.Generation, custom.Shortcut, "down", false); err != nil || result == nil || result.Status != "succeeded" {
		t.Fatalf("dispatch após restart: %+v err=%v", result, err)
	}
}

func commandStorageSettingsContains(snapshot CommandSettingsSnapshot, layerID, bindingID, shortcutSpec string) bool {
	for _, layer := range snapshot.Layers {
		if layer.ID != layerID || layer.Name != "Camada persistente de restart" {
			continue
		}
		for _, binding := range snapshot.Bindings {
			if binding.ID == bindingID && binding.LayerID == layerID && binding.TriggerSpec == shortcutSpec && binding.TriggerType == "keyboard.local" && binding.Enabled {
				return true
			}
		}
	}
	return false
}
