package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func lifecycleWorkspaceOptions(t *testing.T, app *App) commandconfig.LocalReadProjection {
	t.Helper()
	registry, _, err := app.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return commandconfig.LocalReadProjection{Registry: registry}
}

func TestLifecycleWorkspaceNovoPreservaGlobalEInicializaGeracaoLocal(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := database.DB().Create(&commandconfig.Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: principal.UserID, Generation: 7, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatalf("rebuild com workspace novo: %v", err)
	}
	var generations []commandconfig.Generation
	if err := database.DB().Where("user_id = ?", principal.UserID).Order("workspace_id").Find(&generations).Error; err != nil {
		t.Fatal(err)
	}
	if len(generations) != 2 || generations[0].Generation != 7 || generations[1].WorkspaceID == nil || generations[1].Generation != 1 {
		t.Fatalf("gerações global/local incorretas: %+v", generations)
	}
	if _, _, err := app.commandHost.UserConfiguration(ctx, principal.UserID); err != nil {
		t.Fatalf("configuração global foi descartada: %v", err)
	}
}

func TestLifecyclePublishRecusaMudancaDeWorkspaceAntesDaPublicacao(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Create(&commandconfig.Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: principal.UserID, Generation: 1, UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := app.loadCommandLifecyclePersistedConfiguration(ctx, store, lifecycleWorkspaceOptions(t, app))
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	other, err := app.workspaceMgr.Create("outro")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.workspaceMgr.Switch(other.ID); err != nil {
		t.Fatal(err)
	}
	if err := loaded.publish(ctx); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("mudança de workspace permitiu publicação: %v", err)
	}
}

func TestLifecycleLoadErroSQLNaoPublica(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	conn, err := database.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := app.loadCommandLifecyclePersistedConfiguration(ctx, store, lifecycleWorkspaceOptions(t, app)); err == nil || ok {
		t.Fatalf("load SQL deveria falhar fechado: ok=%v err=%v", ok, err)
	}
}
