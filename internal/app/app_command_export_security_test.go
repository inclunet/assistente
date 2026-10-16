package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"assistente/internal/database"
	"assistente/internal/portability"
	"gorm.io/gorm"
)

func TestCommandExportReportsEmptyAndEntryLimitWithoutOutput(t *testing.T) {
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a.ctx = ctx
	ctx = database.WithUserID(ctx, a.currentUserID)
	req := portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true}
	if raw, err := a.exportCommandLayers(ctx, req); raw != "" || !errors.Is(err, errCommandExportEmpty) {
		t.Fatalf("vazio: bytes=%d erro=%v", len(raw), err)
	}
	rows := make([]commandconfig.Layer, 65)
	for i := range rows {
		id := appCommandPortabilityUUID(t)
		rows[i] = commandconfig.Layer{ID: id, UserID: a.currentUserID, Name: id, Source: "user", Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	}
	if err := database.DB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if raw, err := a.exportCommandLayers(ctx, req); raw != "" || !errors.Is(err, errCommandExportLimit) {
		t.Fatalf("65 entradas: bytes=%d erro=%v", len(raw), err)
	}
}

func TestCommandExportRejectsSecurityChangeDuringRead(t *testing.T) {
	for _, scenario := range []string{"epoch", "os_lock", "product"} {
		t.Run(scenario, func(t *testing.T) {
			a := readyCommandProduct(t)
			product := a.commandProduct.Load()
			t.Cleanup(func() { a.commandProduct.Store(product) })
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			a.ctx = ctx
			db := database.DB()
			layer := commandconfig.Layer{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "PRIVATE_EXPORT_MARKER", Enabled: true, Source: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if err := db.Create(&layer).Error; err != nil {
				t.Fatal(err)
			}
			fired := false
			const callback = "test:command-export-security-change"
			if err := db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if fired || tx.Statement.Table != "command_layers" {
					return
				}
				fired = true
				switch scenario {
				case "epoch":
					if err := a.commandEpochs.InvalidateSecurity(ctx); err != nil {
						_ = tx.AddError(err)
					}
				case "os_lock":
					if err := a.commandHost.SetOSSessionState(ctx, true, true); err != nil {
						_ = tx.AddError(err)
					}
				case "product":
					a.commandProduct.Store(nil)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
			raw, err := a.exportCommandLayers(database.WithUserID(ctx, a.currentUserID), portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true})
			if !fired {
				t.Fatal("teste não alcançou leitura")
			}
			if err == nil || raw != "" || strings.Contains(err.Error(), layer.Name) {
				t.Fatalf("export liberou conteúdo após mudança: bytes=%d erro=%v", len(raw), err)
			}
		})
	}
}

func TestCommandExportSingleConnectionReadOnlyAndInvalidLegacyData(t *testing.T) {
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.ctx = ctx
	db := database.DB()
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	previousMax := pool.Stats().MaxOpenConnections
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.SetMaxOpenConns(previousMax) })
	layer := commandconfig.Layer{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "read-only", Source: "user", Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(db)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: a.currentUserID})
	if err != nil {
		t.Fatal(err)
	}
	request := portability.ExportRequest{ExplicitSelection: true, IncludeCommandLayers: true}
	ownerContext := database.WithUserID(ctx, a.currentUserID)
	if raw, err := a.exportCommandLayers(ownerContext, request); err != nil || raw == "" {
		t.Fatalf("export com pool unitário: bytes=%d erro=%v", len(raw), err)
	}
	if err := store.CheckCurrent(ctx, snapshot); err != nil {
		t.Fatalf("export alterou configuração: %v", err)
	}
	commandID := commandProductWorkspaceListID
	binding := commandconfig.Binding{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, LayerRefKind: "user", LayerRef: layer.ID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`, CommandID: &commandID, Arguments: `{"password":"LEGACY_SECRET_MARKER"}`, Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{"version":1}`}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := a.exportCommandLayers(ownerContext, request)
	if err == nil || raw != "" || strings.Contains(err.Error(), "LEGACY_SECRET_MARKER") {
		t.Fatalf("registro inválido virou export parcial: bytes=%d erro=%v", len(raw), err)
	}
}
