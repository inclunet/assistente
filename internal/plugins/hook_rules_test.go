package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/tools/invocationctx"
	"gorm.io/gorm"
)

func TestHookRulesFollowInstallationLifecycle(t *testing.T) {
	for _, action := range []string{"uninstall", "disable", "update"} {
		t.Run(action, func(t *testing.T) {
			db := managedJobTestDB(t)
			inventory := NewInventory(db)
			rules := authorization.NewRepository(func() *gorm.DB { return db })
			alice := database.WithUserID(context.Background(), "alice")
			bob := database.WithUserID(context.Background(), "bob")
			admit := func(_ *gorm.DB, commit func() error) error { return commit() }
			pkg := t.TempDir()
			if err := os.WriteFile(filepath.Join(pkg, "plugin.json"), []byte(`{"$schema":"`+pluginpackage.ManifestSchema+`","name":"example"}`), 0600); err != nil {
				t.Fatal(err)
			}
			seed, err := pluginpackage.InspectLocal(pkg)
			if err != nil {
				t.Fatal(err)
			}
			install := func(ctx context.Context) authorization.PluginHook {
				t.Helper()
				row, err := inventory.InstallLocal(ctx, seed.Origin, seed.Digest, "")
				if err != nil {
					t.Fatal(err)
				}
				return authorization.PluginHook{Name: row.Name, Origin: row.Origin, Generation: row.HookGeneration, InstallationID: row.ID, Digest: row.Digest, Component: "hooks/hooks.json#PreToolUse"}
			}
			hook, other := install(alice), install(bob)
			save := func(ctx context.Context, target authorization.PluginHook, scope authorization.Scope, key, effect string) {
				t.Helper()
				if err := rules.RememberPluginHook(ctx, target, scope, key, effect, admit); err != nil {
					t.Fatal(err)
				}
			}
			save(alice, hook, authorization.ScopeGlobal, "", "allow")
			save(alice, hook, authorization.ScopeWorkspace, "w", "deny")
			save(bob, other, authorization.ScopeGlobal, "", "allow")
			secondDir := t.TempDir()
			manifest, err := os.ReadFile(filepath.Join(pkg, "plugin.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(secondDir, "plugin.json"), manifest, 0600); err != nil {
				t.Fatal(err)
			}
			second, err := pluginpackage.InspectLocal(secondDir)
			if err != nil {
				t.Fatal(err)
			}
			sibling, err := inventory.InstallLocal(alice, second.Origin, second.Digest, "")
			if err != nil {
				t.Fatal(err)
			}
			siblingHook := hook
			siblingHook.InstallationID = sibling.ID
			siblingHook.Origin = sibling.Origin
			save(alice, siblingHook, authorization.ScopeGlobal, "", "allow")
			network := database.AuthorizationRule{Domain: "network", Scope: "global", SelectorKey: "example.test", Selector: `{"host":"example.test"}`, SelectorVersion: 1, Effect: "allow", Origin: "consent"}
			if err := rules.Upsert(alice, network); err != nil {
				t.Fatal(err)
			}
			save(alice, hook, authorization.ScopeWorkspace, "previous", "allow")
			prior, err := rules.List(alice, authorization.PluginHookDomain, "workspace", "previous")
			if err != nil || len(prior) != 1 {
				t.Fatalf("regras: %v, %v", prior, err)
			}
			if err := rules.RevokeID(alice, prior[0].ID); err != nil {
				t.Fatal(err)
			}
			check := func(ctx context.Context, target authorization.PluginHook, workspace string, want bool) {
				t.Helper()
				got, err := rules.PluginHookAllowed(invocationctx.WithVerifiedWorkspaceID(ctx, workspace), target)
				if err != nil || got != want {
					t.Fatalf("decisão em %q = %v, esperado %v, erro %v", workspace, got, want, err)
				}
			}
			check(alice, hook, "outro", true)
			check(alice, hook, "w", false)
			check(alice, hook, "", false)
			check(bob, hook, "outro", false)
			count := func(ctx context.Context, want int) {
				t.Helper()
				rows, err := rules.ManagementRules(ctx)
				if err != nil || len(rows) != want {
					t.Fatalf("gestão: %d regras, esperado %d, err=%v", len(rows), want, err)
				}
			}
			denied := errors.New("sessão encerrada")
			err = inventory.ChangeInstallation(alice, hook.InstallationID, hook.Digest, "uninstall", func(*gorm.DB, func() error) error { return denied })
			if !errors.Is(err, denied) {
				t.Fatalf("admissão: %v", err)
			}
			count(alice, 5)
			switch action {
			case "update":
				if err := os.WriteFile(filepath.Join(pkg, "README.md"), []byte("nova revisão"), 0600); err != nil {
					t.Fatal(err)
				}
				next, err := pluginpackage.InspectLocal(pkg)
				if err != nil {
					t.Fatal(err)
				}
				updated, err := inventory.InstallLocal(alice, next.Origin, next.Digest, hook.Digest)
				if err != nil {
					t.Fatal(err)
				}
				fresh := hook
				fresh.Digest, fresh.Generation = updated.Digest, updated.HookGeneration
				save(alice, fresh, authorization.ScopeGlobal, "", "allow")
				check(alice, fresh, "outro", true)
			default:
				if err := inventory.ChangeInstallation(alice, hook.InstallationID, hook.Digest, action, admit); err != nil {
					t.Fatal(err)
				}
				if action == "uninstall" {
					if reinstalled := install(alice); reinstalled.InstallationID == hook.InstallationID {
						t.Fatal("reinstalação reutilizou a identidade anterior")
					}
				}
			}
			if err := rules.RememberPluginHook(alice, hook, authorization.ScopeGlobal, "", "allow", admit); !errors.Is(err, authorization.ErrNotFound) {
				t.Fatalf("aceite atrasado: %v", err)
			}
			if action == "disable" {
				if err := inventory.ChangeInstallation(alice, hook.InstallationID, hook.Digest, "enable", admit); err != nil {
					t.Fatal(err)
				}
			}
			check(alice, hook, "outro", false)
			check(bob, other, "outro", true)
			if action == "update" {
				if err := os.Remove(filepath.Join(pkg, "README.md")); err != nil {
					t.Fatal(err)
				}
				rows, err := inventory.List(alice)
				if err != nil {
					t.Fatal(err)
				}
				var previous string
				for _, row := range rows {
					if row.ID == hook.InstallationID {
						previous = row.Digest
					}
				}
				if _, err = inventory.InstallLocal(alice, seed.Origin, seed.Digest, previous); err != nil {
					t.Fatal(err)
				}
			}
			if err := rules.RememberPluginHook(alice, hook, authorization.ScopeGlobal, "", "allow", admit); !errors.Is(err, authorization.ErrNotFound) {
				t.Fatalf("aceite antigo após retorno: %v", err)
			}
			check(alice, siblingHook, "outro", true)
			retained, err := rules.List(alice, "network", "global", "")
			if err != nil || len(retained) != 1 {
				t.Fatalf("rede após limpeza: %v, %v", retained, err)
			}
			count(alice, 2)
			count(bob, 1)
		})
	}
}
