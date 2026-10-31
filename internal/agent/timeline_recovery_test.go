package agent

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestTurnRecoveryRuntimeCancellationBeforeFinalWriteRejectsOldPatch(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	h, err := svc.NewSimpleStreamHandler(ctx, conv, turn, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.OnChunk("recebido sem gravação")
	cancel()
	// Exercita o helper antes de qualquer escrita parcial. OnDone tem outro
	// percurso: tenta preservar o parcial antes de entregar o cancelamento.
	svc.emitFinalizationCancelledDone(ctx, conv, turn, h.AssistantMessageID, nil)
	done := em.find("chat:done")
	if len(done) != 1 {
		t.Fatalf("terminais: %+v", done)
	}
	if event := done[0].data.(ports.DoneEvent); event.TurnPatch != nil || event.Reason != "cancelled" {
		t.Fatalf("patch antigo no cancelamento: %+v", event)
	}
	recovery, err := svc.ReadTurnRecovery(context.WithoutCancel(ctx), conv, turn)
	if err != nil || len(recovery.Retained) != 1 || recovery.Retained[0].Streams[0].Fragments[0].Text != "recebido sem gravação" {
		t.Fatalf("conteúdo perdido: %+v %v", recovery, err)
	}
}

func TestTurnRecoveryRuntimeCancelledFinalizationPublishesConfirmedPartial(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	h, err := svc.NewSimpleStreamHandler(ctx, conv, turn, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.OnChunk("parcial preservado")
	cancel()
	h.OnDone("parcial preservado", llm.Usage{}, "modelo")
	done := em.find("chat:done")
	if len(done) != 1 {
		t.Fatalf("terminais: %+v", done)
	}
	event := done[0].data.(ports.DoneEvent)
	if event.Reason != "cancelled" || event.TurnPatch == nil || event.TurnPatch.Message.Content != "parcial preservado" {
		t.Fatalf("parcial confirmado ausente: %+v", event)
	}
	recovery, err := svc.ReadTurnRecovery(context.WithoutCancel(ctx), conv, turn)
	if err != nil || len(recovery.Retained) != 0 || !recovery.CanUsePersistedPatch(executionID(ctx)) {
		t.Fatalf("parcial confirmado classificado como não salvo: %+v %v", recovery, err)
	}
}

func TestTurnRecoveryRuntimeRequiresVersionMigrationBeforeSend(t *testing.T) {
	svc, ctx, _, _, _ := setupTurnRuntime(t)
	if err := svc.CheckTurnStorageReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Exec("DELETE FROM schema_migrations WHERE version = 43").Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckTurnStorageReady(ctx); !errors.Is(err, database.ErrChatTurnMigrationPending) {
		t.Fatalf("admissão sem v43: %v", err)
	}
}

func TestTurnRecoveryRuntimeRetainedRetryAndDiscard(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	h, err := svc.NewSimpleStreamHandler(ctx, conv, turn, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.OnChunk("conteúdo não salvo")
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	recovery, err := svc.ReadTurnRecovery(ctx, conv, turn)
	if err != nil || len(recovery.Retained) != 1 || recovery.Retained[0].Streams[0].Fragments[0].Text != "conteúdo não salvo" || recovery.CanUsePersistedPatch(executionID(ctx)) {
		t.Fatalf("retenção: %+v %v", recovery, err)
	}
	next, err := svc.BeginTurn(ctx, conv, turn, database.TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.EndUnfinishedTurn(next, conv, turn, nil)
	nextHandler, err := svc.NewSimpleStreamHandler(next, conv, turn, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	nextHandler.OnDone("resposta nova", llm.Usage{}, "modelo")
	recovery, err = svc.ReadTurnRecovery(next, conv, turn)
	if err != nil || len(recovery.Retained) != 1 || !recovery.CanUsePersistedPatch(executionID(next)) || recovery.CanUsePersistedPatch(executionID(ctx)) {
		t.Fatalf("retry: %+v %v", recovery, err)
	}
	if recovery.Persisted.Response.Representative.Content != "resposta nova" {
		t.Fatal("misturou tentativa antiga")
	}
	for i := 0; i < 2; i++ {
		if err := svc.DiscardRetainedTurn(next, conv, turn, executionID(ctx)); err != nil {
			t.Fatal(err)
		}
	}
	recovery, err = svc.ReadTurnRecovery(next, conv, turn)
	if err != nil || len(recovery.Retained) != 0 {
		t.Fatal("descarte não removeu somente tentativa antiga")
	}
}

func TestTurnRecoveryRuntimeNullableRepresentativeAndAuthorization(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	recovery, err := svc.ReadTurnRecovery(ctx, conv, turn)
	if err != nil || recovery.Active == nil || recovery.Persisted.Response.Representative != nil {
		t.Fatalf("preparação sem representante: %+v %v", recovery, err)
	}
	other := database.WithUserID(context.Background(), "other-user")
	if result, err := svc.ReadTurnRecovery(other, conv, turn); err == nil || result != nil {
		t.Fatal("leitura sem propriedade retornou conteúdo")
	}
	if err := svc.DiscardRetainedTurn(other, conv, turn, executionID(ctx)); err == nil {
		t.Fatal("descarte sem propriedade")
	}
	if err := database.DeleteConversationWithContext(ctx, conv); err != nil {
		t.Fatal(err)
	}
	if result, err := svc.ReadTurnRecovery(ctx, conv, turn); err == nil || result != nil {
		t.Fatal("fallback de memória após exclusão")
	}
}

func TestTurnRecoveryRuntimeTerminalVersionIsTransactional(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	h, err := svc.NewSimpleStreamHandler(ctx, conv, turn, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.OnDone("final", llm.Usage{}, "modelo")
	recovery, err := svc.ReadTurnRecovery(ctx, conv, turn)
	if err != nil || !recovery.CanUsePersistedPatch(executionID(ctx)) {
		t.Fatalf("commit sem prova: %+v %v", recovery, err)
	}
	before := recovery.Persisted.Version
	if err := database.DB().Exec("UPDATE chat_messages SET content = ? WHERE id = ?", "edição externa", h.AssistantMessageID).Error; err != nil {
		t.Fatal(err)
	}
	recovery, err = svc.ReadTurnRecovery(ctx, conv, turn)
	if err != nil || recovery.Persisted.Version.Revision <= before.Revision || recovery.Persisted.Response.Representative.Content != "edição externa" || recovery.CanUsePersistedPatch(executionID(ctx)) {
		t.Fatalf("mudança externa atribuída ao terminal: %+v %v", recovery, err)
	}
	if patch, err := svc.buildTurnPatch(ctx, conv, turn); patch != nil || err == nil {
		t.Fatal("patch velho liberado após edição externa")
	}
}

func TestTurnRecoveryRuntimeVersionReadFailureRollsBackTerminal(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	id, err := svc.EnsureTurnResponse(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	before, err := svc.timeline.ReadVersion(ctx, conv)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Exec(`CREATE TRIGGER remove_terminal_version AFTER UPDATE ON chat_turns WHEN NEW.state='completed' BEGIN DELETE FROM chat_timeline_versions WHERE conversation_id=NEW.conversation_id; END`).Error; err != nil {
		t.Fatal(err)
	}
	_, err = svc.turns.FinishResponseWithVersion(ctx, conv, turn, executionID(ctx), database.TurnCompleted, "completed", id, &database.MessageOptions{Content: "não deve confirmar"})
	if err == nil {
		t.Fatal("confirmou terminal sem prova de versão")
	}
	after, err := svc.timeline.ReadTurn(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before || after.Turn.ActiveExecutionID == nil || *after.Turn.ActiveExecutionID != executionID(ctx) || after.Response.Representative.Content != "" {
		t.Fatalf("rollback incompleto: %+v", after)
	}
}
