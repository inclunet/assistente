package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
	"assistente/internal/questionnaire"
	"gorm.io/gorm"
)

type appCommandDesktopBatchOutcome struct {
	result commandMutationBatchResult
	err    error
}

type appCommandDesktopMutationOutcome struct {
	result commandMutationResult
	err    error
}

func appCommandDesktopMutationInputs(t *testing.T) commandCompleteMutationInputs {
	t.Helper()
	registry := appCommandPortabilityRegistry(t)
	return commandCompleteMutationInputs{
		Projection: func(context.Context, commandconfig.Scope) (commandconfig.CompleteProjection, error) {
			return mutationProjectionOptions(registry), nil
		},
		Authorize: func(context.Context, auth.LocalSessionPrincipal, commandconfig.Scope, commandconfig.Operation) error {
			return nil
		},
		Version:      func(context.Context) (string, error) { return "desktop-mutation-test-v1", nil },
		Render:       func(commandconfig.MutationDiff) (string, error) { return "desktop-mutation-test", nil },
		OnMutationTx: func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil },
		DecisionTTL:  time.Minute,
	}
}

func appCommandDesktopMutationApplier(t *testing.T, a *App) *commandMutationApplier {
	t.Helper()
	applier, err := a.newCommandDesktopMutationApplier(appCommandDesktopMutationInputs(t))
	if err != nil {
		t.Fatalf("fábrica desktop: %v", err)
	}
	return applier
}

func startAppCommandDesktopBatch(t *testing.T, applier *commandMutationApplier, ctx context.Context, cancel context.CancelFunc, raw []byte, token string) (<-chan appCommandDesktopBatchOutcome, *bool) {
	t.Helper()
	result := make(chan appCommandDesktopBatchOutcome, 1)
	finished := new(bool)
	go func() {
		got, err := applier.ImportEnvelopeBatch(ctx, token, raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, appCommandPortabilityBatchRefs(t))
		result <- appCommandDesktopBatchOutcome{result: got, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		if *finished {
			return
		}
		select {
		case <-result:
		case <-time.After(5 * time.Second):
			t.Error("worker desktop não encerrou após cancelamento")
		}
	})
	return result, finished
}

func startAppCommandDesktopImport(t *testing.T, applier *commandMutationApplier, ctx context.Context, cancel context.CancelFunc, raw []byte, token string) (<-chan appCommandDesktopMutationOutcome, *bool) {
	t.Helper()
	result := make(chan appCommandDesktopMutationOutcome, 1)
	finished := new(bool)
	go func() {
		got, err := applier.ImportEnvelope(ctx, token, nil, raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, appCommandPortabilityRefs(t))
		result <- appCommandDesktopMutationOutcome{result: got, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		if *finished {
			return
		}
		select {
		case <-result:
		case <-time.After(5 * time.Second):
			t.Error("worker de import desktop não encerrou após cancelamento")
		}
	})
	return result, finished
}

func appCommandDesktopDecisionManager(t *testing.T) (<-chan map[string]any, *questionnaire.Manager) {
	t.Helper()
	decisions := make(chan map[string]any, 2)
	manager := questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			decisions <- data.(map[string]any)
		}
	})
	return decisions, manager
}

func acceptAppCommandDesktopDecisions(t *testing.T, decisions <-chan map[string]any, manager *questionnaire.Manager, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		var payload map[string]any
		select {
		case payload = <-decisions:
		case <-time.After(5 * time.Second):
			t.Fatalf("decisão desktop %d não foi apresentada", i+1)
		}
		id, ok := payload["id"].(string)
		if !ok || id == "" {
			t.Fatalf("decisão desktop sem ID: %#v", payload)
		}
		if err := manager.Respond(id, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
			t.Fatalf("resposta da decisão desktop %d: %v", i+1, err)
		}
	}
}

func TestNewCommandDesktopMutationApplierImportBatchSucceedsWithoutJWT(t *testing.T) {
	a, _, raw, globalID, workspaceID, workspaceIDValue := appCommandPortabilityBatchFixture(t)
	decisions, manager := appCommandDesktopDecisionManager(t)
	a.questionnaireMgr = manager
	applier := appCommandDesktopMutationApplier(t, a)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	resultCh, finished := startAppCommandDesktopBatch(t, applier, ctx, cancel, raw, "")
	acceptAppCommandDesktopDecisions(t, decisions, manager, 2)

	select {
	case outcome := <-resultCh:
		*finished = true
		if outcome.err != nil || !outcome.result.Committed || !outcome.result.Rebuilt || len(outcome.result.Diffs) != 2 {
			t.Fatalf("import batch desktop sem JWT: result=%+v err=%v", outcome.result, outcome.err)
		}
		if outcome.result.Report == nil || outcome.result.Report.NoChanges {
			t.Fatalf("relatório do import batch desktop: %+v", outcome.result.Report)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("import batch desktop não terminou após as confirmações")
	}
	assertCommandPortabilityBatchPersisted(t, a, globalID, workspaceID, workspaceIDValue)
	assertCommandPortabilityBatchMap(t, a, workspaceIDValue)
}

func TestNewCommandDesktopMutationApplierRejectsNonEmptyTokenBeforeDecision(t *testing.T) {
	a, _, raw, globalID, workspaceID, workspaceIDValue := appCommandPortabilityBatchFixture(t)
	decisions, manager := appCommandDesktopDecisionManager(t)
	a.questionnaireMgr = manager
	applier := appCommandDesktopMutationApplier(t, a)

	result, err := applier.ImportEnvelopeBatch(context.Background(), "jwt-must-not-be-used", raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, appCommandPortabilityBatchRefs(t))
	if !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("token não vazio: err=%v, want ErrDenied", err)
	}
	if result.Committed || result.Rebuilt || len(result.Diffs) != 0 {
		t.Fatalf("token não vazio alterou o resultado: %+v", result)
	}
	select {
	case decision := <-decisions:
		t.Fatalf("token não vazio abriu decisão: %#v", decision)
	default:
	}
	assertCommandPortabilityBatchAbsent(t, a, globalID, workspaceID, workspaceIDValue)
}

func TestNewCommandDesktopMutationApplierRevokedSessionCannotCommitWhileQuestionnaireOpen(t *testing.T) {
	a, _, batchRaw, globalID, workspaceID, workspaceIDValue := appCommandPortabilityBatchFixture(t)
	var envelope portability.ExportFile
	if err := json.Unmarshal(batchRaw, &envelope); err != nil {
		t.Fatalf("envelope do fixture: %v", err)
	}
	// A importação unitária mantém exatamente uma decisão aberta. O lote é
	// usado apenas para obter o envelope real com uma camada global e uma de
	// workspace; a segunda confirmação não deve mascarar a revogação testada.
	if len(envelope.Resources.CommandLayers) < 1 {
		t.Fatal("fixture não trouxe camada global")
	}
	envelope.Resources.CommandLayers = envelope.Resources.CommandLayers[:1]
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("envelope unitário: %v", err)
	}
	// O refresh é capturado antes da fábrica. A operação desktop não recebe
	// JWT: o logout abaixo só prova a revogação da sessão congelada na fábrica.
	_, refreshToken := appCommandPortabilitySession(t, a)
	decisions, manager := appCommandDesktopDecisionManager(t)
	a.questionnaireMgr = manager
	applier := appCommandDesktopMutationApplier(t, a)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	resultCh, finished := startAppCommandDesktopImport(t, applier, ctx, cancel, raw, "")
	var first map[string]any
	select {
	case first = <-decisions:
	case outcome := <-resultCh:
		*finished = true
		t.Fatalf("import terminou antes do questionário: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("questionário da mutação revogável não foi apresentado")
	}
	if err := a.sessionSvc.Logout(context.Background(), refreshToken); err != nil {
		t.Fatalf("logout da sessão desktop: %v", err)
	}
	finishCommandDecision(t, manager, first, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)

	select {
	case outcome := <-resultCh:
		*finished = true
		if outcome.err == nil || outcome.result.Committed || outcome.result.Rebuilt {
			t.Fatalf("sessão revogada alcançou commit: result=%+v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mutação revogada não encerrou após a resposta")
	}
	assertCommandPortabilityBatchAbsent(t, a, globalID, workspaceID, workspaceIDValue)
}

func TestNewCommandDesktopMutationApplierDoesNotAdoptSessionSwappedAfterFactory(t *testing.T) {
	a, _, raw, globalID, workspaceID, workspaceIDValue := appCommandPortabilityBatchFixture(t)
	decisions, manager := appCommandDesktopDecisionManager(t)
	a.questionnaireMgr = manager
	applier := appCommandDesktopMutationApplier(t, a)

	var user database.User
	if err := database.DB().Where("id = ?", a.currentUserID).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	second, err := a.sessionSvc.IssueSession(context.Background(), &user, "desktop-session-swap")
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: second.SessionID, Role: user.Role})

	result, err := applier.ImportEnvelopeBatch(context.Background(), "", raw, commandportability.PlanOptions{Mode: commandportability.ReplaceMode}, appCommandPortabilityBatchRefs(t))
	if !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("troca de sessão após factory: err=%v, want ErrDenied", err)
	}
	if result.Committed || result.Rebuilt || len(result.Diffs) != 0 {
		t.Fatalf("troca de sessão alterou o resultado: %+v", result)
	}
	select {
	case decision := <-decisions:
		t.Fatalf("troca de sessão abriu decisão: %#v", decision)
	default:
	}
	assertCommandPortabilityBatchAbsent(t, a, globalID, workspaceID, workspaceIDValue)
}

func TestNewCommandDesktopMutationApplierTimeoutCancelsAndJoinsQuestionnaire(t *testing.T) {
	a, _, raw, _, _, _ := appCommandPortabilityBatchFixture(t)
	decisions, manager := appCommandDesktopDecisionManager(t)
	a.questionnaireMgr = manager
	applier := appCommandDesktopMutationApplier(t, a)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	resultCh, finished := startAppCommandDesktopBatch(t, applier, ctx, cancel, raw, "")
	select {
	case <-decisions:
	case outcome := <-resultCh:
		*finished = true
		t.Fatalf("timeout terminou antes de abrir o questionário: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("questionário do timeout não foi apresentado")
	}
	select {
	case <-ctx.Done():
	case outcome := <-resultCh:
		*finished = true
		t.Fatalf("worker terminou antes do timeout: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("contexto não expirou")
	}
	select {
	case outcome := <-resultCh:
		*finished = true
		if !errors.Is(outcome.err, context.DeadlineExceeded) || outcome.result.Committed || outcome.result.Rebuilt {
			t.Fatalf("timeout desktop: result=%+v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout não fez join do worker do questionário")
	}
}

func TestNewCommandDesktopMutationApplierBuildConfigurationGuardLeavesCommittedMapSuspended(t *testing.T) {
	a, _, raw, globalID, workspaceID, workspaceIDValue := appCommandPortabilityBatchFixture(t)
	decisions, manager := appCommandDesktopDecisionManager(t)
	a.questionnaireMgr = manager
	inputs := appCommandDesktopMutationInputs(t)
	manual, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	guardErr := errors.New("guard de publicação desktop rejeitou a configuração")
	inputs.BuildConfiguration = func(context.Context, commandconfig.Scope, commandconfig.Snapshot, commandconfig.CompleteProjection) (*commandbindings.Configuration, []string, func(context.Context) error, error) {
		return manual, nil, func(context.Context) error { return guardErr }, nil
	}
	applier, err := a.newCommandDesktopMutationApplier(inputs)
	if err != nil {
		t.Fatalf("fábrica desktop com BuildConfiguration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	resultCh, finished := startAppCommandDesktopBatch(t, applier, ctx, cancel, raw, "")
	acceptAppCommandDesktopDecisions(t, decisions, manager, 2)
	select {
	case outcome := <-resultCh:
		*finished = true
		if !errors.Is(outcome.err, guardErr) || !outcome.result.Committed || outcome.result.Rebuilt {
			t.Fatalf("guard pós-commit: result=%+v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("guard pós-commit não terminou")
	}

	assertCommandPortabilityBatchPersisted(t, a, globalID, workspaceID, workspaceIDValue)
	if _, _, err := a.commandHost.UserConfiguration(context.Background(), a.currentUserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("mapa após guard deveria estar suspenso: %v", err)
	}
}
