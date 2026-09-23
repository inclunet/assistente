package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"gorm.io/gorm"
)

var errBatchProjectionSuspended = errors.New("projeção pós-commit suspensa deliberadamente")

func TestAppCommandPortabilityBatchCommitPersistsWhenRebuildLosesPublishedProjection(t *testing.T) {
	for _, mode := range []string{"projection_failure", "inactive_stale_build", "inactive_stale_final"} {
		t.Run(mode, func(t *testing.T) { testAppCommandBatchPostCommit(t, mode) })
	}
}

func testAppCommandBatchPostCommit(t *testing.T, mode string) {
	a, access, raw, globalID, importedWorkspaceLayerID, importedWorkspaceID := appCommandPortabilityBatchFixture(t)
	ctx := context.Background()

	// O lote importado é deliberadamente diferente do workspace ativo. Ambos
	// têm escopo persistido; somente o global e o workspace ativo participam da
	// publicação, enquanto o workspace importado apenas altera seu generation.
	active, err := a.workspaceMgr.Create("batch ativo independente")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.workspaceMgr.Switch(active.ID); err != nil {
		t.Fatal(err)
	}
	configStore, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	if err := configStore.EnsureScope(ctx, commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &active.ID}); err != nil {
		t.Fatal(err)
	}
	beforeImported, err := configStore.Load(ctx, commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &importedWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	beforeGeneration := workspaceGeneration(t, beforeImported, importedWorkspaceID)

	decisions := make(chan map[string]any, 2)
	a.questionnaireMgr = questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	registry := appCommandPortabilityRegistry(t)
	staleProjectionCalls := 0
	inputs := commandCompleteMutationInputs{
		Projection: func(_ context.Context, _ commandconfig.Scope) (commandconfig.CompleteProjection, error) {
			_, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID)
			if errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				staleProjectionCalls++
				if mode == "projection_failure" {
					return commandconfig.CompleteProjection{}, errors.Join(errBatchProjectionSuspended, err)
				}
				targetCall := 1
				if mode == "inactive_stale_final" {
					targetCall = 2
				}
				if staleProjectionCalls == targetCall {
					updated := database.DB().Model(&commandconfig.Generation{}).
						Where("user_id = ? AND workspace_id = ?", a.currentUserID, importedWorkspaceID).
						Update("generation", gorm.Expr("generation + 1"))
					if updated.Error != nil {
						return commandconfig.CompleteProjection{}, updated.Error
					}
					if updated.RowsAffected != 1 {
						return commandconfig.CompleteProjection{}, errors.New("geração alvo não encontrada")
					}
				}
				return mutationProjectionOptions(registry), nil
			}
			if err != nil {
				return commandconfig.CompleteProjection{}, err
			}
			return mutationProjectionOptions(registry), nil
		},
		Authorize: func(context.Context, auth.LocalSessionPrincipal, commandconfig.Scope, commandconfig.Operation) error {
			return nil
		},
		Version:      func(context.Context) (string, error) { return "portability-batch-rebuild-test-v1", nil },
		Render:       func(commandconfig.MutationDiff) (string, error) { return "batch-rebuild-test", nil },
		OnMutationTx: func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil },
		DecisionTTL:  time.Minute,
	}
	applier, err := a.newCommandMutationApplier(inputs)
	if err != nil {
		t.Fatal(err)
	}

	operationCtx, cancel := context.WithTimeout(database.WithUserID(ctx, "foreign-context-user"), 30*time.Second)
	defer cancel()
	type outcome struct {
		result commandMutationBatchResult
		err    error
	}
	resultCh := make(chan outcome, 1)
	finished := false
	go func() {
		result, runErr := applier.ImportEnvelopeBatch(operationCtx, access, raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, appCommandPortabilityBatchRefs(t))
		resultCh <- outcome{result: result, err: runErr}
	}()
	defer func() {
		cancel()
		if finished {
			return
		}
		select {
		case <-resultCh:
		case <-time.After(5 * time.Second):
			t.Error("worker de rebuild não encerrou após cancelamento")
		}
	}()

	for i := 0; i < 2; i++ {
		var decision map[string]any
		select {
		case decision = <-decisions:
		case completed := <-resultCh:
			finished = true
			t.Fatalf("batch terminou antes da confirmação %d: result=%+v err=%v", i+1, completed.result, completed.err)
		case <-time.After(30 * time.Second):
			t.Fatalf("confirmação %d não foi apresentada", i+1)
		}
		id, ok := decision["id"].(string)
		if !ok || id == "" {
			t.Fatalf("decisão sem id: %#v", decision)
		}
		if err := a.questionnaireMgr.Respond(id, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
			t.Fatal(err)
		}
	}

	var completed outcome
	select {
	case completed = <-resultCh:
	case <-time.After(30 * time.Second):
		t.Fatal("batch não encerrou após confirmação")
	}
	finished = true
	if completed.err == nil || !completed.result.Committed || completed.result.Rebuilt || len(completed.result.Diffs) != 2 {
		t.Fatalf("commit/rebuild inesperado: result=%+v err=%v", completed.result, completed.err)
	}
	if completed.result.Report == nil || completed.result.Report.NoChanges || len(completed.result.Report.Layers) != 2 {
		t.Fatal("falha de reconstrução não pode apagar relatório do lote já gravado")
	}
	wantErr := commandconfig.ErrStale
	if mode == "projection_failure" {
		wantErr = errBatchProjectionSuspended
	}
	if !errors.Is(completed.err, wantErr) {
		t.Fatalf("erro de rebuild inesperado: %v", completed.err)
	}

	assertCommandPortabilityBatchPersisted(t, a, globalID, importedWorkspaceLayerID, importedWorkspaceID)
	afterImported, err := configStore.Load(ctx, commandconfig.Scope{UserID: a.currentUserID, WorkspaceID: &importedWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	wantGeneration := beforeGeneration + 1
	if mode != "projection_failure" {
		wantGeneration++ // alteração concorrente deliberada, não repetição do lote
	}
	if got := workspaceGeneration(t, afterImported, importedWorkspaceID); got != wantGeneration {
		t.Fatalf("generation do workspace importado: got=%d want=%d", got, wantGeneration)
	}
	if _, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("mapa suspenso não preservado: %v", err)
	}
	wantCalls := 2
	if mode == "projection_failure" {
		wantCalls = 1
	}
	if staleProjectionCalls != wantCalls {
		t.Fatalf("Projection pós-commit: got=%d want=%d", staleProjectionCalls, wantCalls)
	}
	for _, diff := range completed.result.Diffs {
		var count int64
		if err := database.DB().Table("command_config_mutations").Where("mutation_id = ?", diff.MutationID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("auditoria da mutation %s: count=%d want=1", diff.MutationID, count)
		}
		var audit struct {
			BeforeGeneration int64
			AfterGeneration  int64
		}
		if err := database.DB().Table("command_config_mutations").Select("before_generation, after_generation").Where("mutation_id = ?", diff.MutationID).Take(&audit).Error; err != nil {
			t.Fatal(err)
		}
		if audit.AfterGeneration != audit.BeforeGeneration+1 {
			t.Fatalf("auditoria da mutation %s não incrementou uma vez: %+v", diff.MutationID, audit)
		}
	}
}

func workspaceGeneration(t *testing.T, snapshot commandconfig.Snapshot, workspaceID string) int64 {
	t.Helper()
	for _, generation := range snapshot.Generations {
		if generation.WorkspaceID != nil && *generation.WorkspaceID == workspaceID {
			return generation.Generation
		}
	}
	t.Fatalf("generation ausente para workspace %q", workspaceID)
	return 0
}
