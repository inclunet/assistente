package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
)

func TestCommandLifecycleReloadFailureWithdrawsPreviouslyReadyMap(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "storage_failure"
		if canceled {
			name = "canceled_reload"
		}
		t.Run(name, func(t *testing.T) {
			app, inputs := appLifecycleProductMountFixture(t)
			inputs.Runtime = commandruntime.Config{}
			ctx := context.Background()
			if err := ConfigureCommandLifecycleForApp(app, inputs); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ShutdownCommandLifecycle(context.Background(), app) })
			if err := BootstrapCommandLifecycle(ctx, app); err != nil {
				t.Fatal(err)
			}
			before, _ := CommandLifecycleSnapshot(app)
			if before.State != commandruntime.StateReady || !before.Published {
				t.Fatalf("pré-condição: runtime não estava pronto: %+v", before)
			}
			if canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			} else if err := database.DB().Migrator().DropTable(&commandconfig.Generation{}); err != nil {
				t.Fatal(err)
			}
			principal := *app.currentAuthUser
			app.bootstrapCommandLifecycleAfterAuth(ctx, &principal, nil)
			after, err := CommandLifecycleSnapshot(app)
			if err != nil || after.Published || after.State == commandruntime.StateReady {
				t.Fatalf("falha de recarga preservou/publicou mapa antigo: %+v err=%v", after, err)
			}
			if !app.authResultStillCurrent(&principal) {
				t.Fatal("falha de comandos desfez a autenticação válida")
			}
			if _, _, err := app.commandHost.UserConfiguration(context.Background(), principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				t.Fatalf("executor ainda pode observar mapa anterior: %v", err)
			}
		})
	}
}
