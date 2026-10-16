package commandconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commanddecision"
	"gorm.io/gorm"
)

func TestImportBatchServiceAtomicAdmission(t *testing.T) {
	for _, mode := range []string{"success", "deny_scope", "deny_second_decision", "stale_reference", "last_hook_failure", "combined_invalid"} {
		t.Run(mode, func(t *testing.T) {
			f := newActivationHookFixture(t)
			service := completeServiceWithActiveLayerProof(t, f, func() []string { return nil })
			workspace := "batch-workspace"
			ctx := context.Background()
			if err := f.config.EnsureScope(ctx, Scope{UserID: f.owner.UserID, WorkspaceID: &workspace}); err != nil {
				t.Fatal(err)
			}
			globalID, localID := importMutationUUID(t), importMutationUUID(t)
			calls, reads, suspensions := 0, 0, 0
			validate := service.service.config.Validate
			service.service.config.Validate = func(ctx context.Context, snapshot Snapshot) error {
				if err := validate(ctx, snapshot); err != nil {
					return err
				}
				if mode == "combined_invalid" && snapshot.Scope.WorkspaceID != nil && hasLayer(snapshot.Layers, globalID) && hasLayer(snapshot.Layers, localID) {
					return ErrInvalid
				}
				return nil
			}
			service.service.config.Authorize = func(_ context.Context, _ auth.LocalSessionPrincipal, scope Scope, op Operation) error {
				if op != ConfigImport {
					t.Fatal("operação errada")
				}
				if mode == "deny_scope" && scope.WorkspaceID != nil {
					return ErrInvalid
				}
				return nil
			}
			receipts, err := commanddecision.New(f.db, mutationPresenterFunc(func(_ context.Context, r commanddecision.Request) (commanddecision.Response, error) {
				calls++
				// Nem a aceitação anterior permite efeito antes do fim do lote.
				var count int64
				if err := f.db.Model(&Layer{}).Where("id IN ?", []string{globalID, localID}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("gravação anterior à última decisão")
				}
				action := commanddecision.ApplyAction
				if mode == "deny_second_decision" && calls == 2 {
					action = commanddecision.DenyAction
				}
				return commanddecision.Response{DecisionID: r.DecisionID, ActionID: action}, nil
			}), time.Now)
			if err != nil {
				t.Fatal(err)
			}
			service.service.config.Receipts = receipts
			service.service.config.BeforeCommit = func(_ context.Context, scope Scope) error {
				suspensions++
				if scope.UserID != f.owner.UserID || scope.WorkspaceID != nil {
					t.Fatal("suspensão deve abranger usuário")
				}
				return nil
			}
			hook, err := NewActivationMutationHook(f.act, f.grants, func(_ context.Context, scope Scope) (commandactivation.Owner, error) {
				owner := f.owner
				owner.WorkspaceID = cloneWorkspace(scope.WorkspaceID)
				return owner, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			service.service.config.OnMutationTx = func(ctx context.Context, tx *gorm.DB, diff MutationDiff) error {
				if err := hook(ctx, tx, diff); err != nil {
					return err
				}
				if mode == "last_hook_failure" && diff.Scope.WorkspaceID == nil {
					return ErrInvalid
				}
				return nil
			}
			provider := func(_ context.Context, scope Scope, current Snapshot) (ImportedSnapshot, error) {
				reads++
				id, name := globalID, "global-batch"
				if scope.WorkspaceID != nil {
					id, name = localID, "local-batch"
				}
				if hasLayer(current.Layers, id) {
					return ImportedSnapshot{Snapshot: current}, nil
				}
				current.Layers = append(current.Layers, Layer{ID: id, UserID: scope.UserID, WorkspaceID: cloneWorkspace(scope.WorkspaceID), Name: name, Enabled: true, Source: "user"})
				return ImportedSnapshot{Snapshot: current, TouchedLayerIDs: []string{id}}, nil
			}
			revalidate := func(context.Context, Scope) error {
				if mode == "stale_reference" {
					return ErrStale
				}
				return nil
			}
			diffs, err := service.ImportBatch(ctx, "token", []*string{nil, &workspace}, provider, revalidate)
			if mode == "success" {
				if err != nil || len(diffs) != 2 || calls != 2 || suspensions != 1 {
					t.Fatalf("diffs=%d calls=%d susp=%d err=%v", len(diffs), calls, suspensions, err)
				}
				if _, err := service.ImportBatch(ctx, "token", []*string{nil, &workspace}, provider, revalidate); !errors.Is(err, ErrNoChanges) {
					t.Fatalf("repetição: %v", err)
				}
				if calls != 2 || suspensions != 1 {
					t.Fatal("no-op teve decisão ou suspensão")
				}
				return
			}
			if err == nil {
				t.Fatal("lote deveria falhar")
			}
			if mode == "deny_scope" && (reads != 0 || calls != 0) {
				t.Fatal("leitura/decisão antes de autorizar todos escopos")
			}
			if mode == "combined_invalid" && calls != 0 {
				t.Fatal("união final inválida chegou à decisão")
			}
			var count int64
			if err := f.db.Model(&Layer{}).Where("id IN ?", []string{globalID, localID}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 || countRows(t, f.db, "command_config_mutations") != 0 {
				t.Fatal("falha deixou dados ou auditoria parcial")
			}
			if err := f.db.Table("command_decision_receipts").Where("consumed_at IS NOT NULL").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("falha consumiu receipt")
			}
		})
	}
}
