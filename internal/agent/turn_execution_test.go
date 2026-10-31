package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
)

func setupTurnRuntime(t *testing.T) (*Service, context.Context, string, string, *captureEmitter) {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	restore := database.SetDB(database.DB())
	if err := database.InitPath(filepath.Join(t.TempDir(), "runtime.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(); restore() })
	if err := database.DB().Create(&database.User{UUIDModel: database.UUIDModel{ID: "runtime-user"}, Username: "runtime-user"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "runtime-user")
	conv, err := database.CreateConversationWithContext(ctx, "runtime", "")
	if err != nil {
		t.Fatal(err)
	}
	question, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pergunta"})
	if err != nil {
		t.Fatal(err)
	}
	emitter := &captureEmitter{}
	svc := NewService(ServiceConfig{Turns: database.NewChatTurnRepository(database.DB()), Timeline: chat.NewDBTimelineReader(database.DB()), MsgRepo: chat.NewDBMessageStore(), Emitter: emitter})
	ctx, err = svc.BeginTurn(ctx, conv.ID, question.ID, database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.EndUnfinishedTurn(ctx, conv.ID, question.ID, nil) })
	return svc, ctx, conv.ID, question.ID, emitter
}

func assertRuntimeTerminal(t *testing.T, ctx context.Context, conv, turn, state, reason string, em *captureEmitter) ports.DoneEvent {
	t.Helper()
	stored, err := database.NewChatTurnRepository(database.DB()).Get(context.WithoutCancel(ctx), conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != state || stored.ActiveExecutionID != nil {
		t.Fatalf("turno: %+v", stored)
	}
	var attempts []database.ChatTurnAttempt
	if err := database.DB().Where("turn_id = ?", turn).Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].TerminalReason != reason || attempts[0].State != state {
		t.Fatalf("tentativas: %+v", attempts)
	}
	events := em.find("chat:done")
	if len(events) != 1 {
		t.Fatalf("terminais: %+v", events)
	}
	return events[0].data.(ports.DoneEvent)
}

func TestTurnRuntimeRecoveryKeepsOneAttemptAndResponse(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){
		func(h llm.StreamHandler) { h.OnChunk("parcial"); h.OnError("rede") },
		func(h llm.StreamHandler) {
			h.OnChunk("final")
			h.OnDone("final", llm.Usage{TotalTokens: 7, CacheReadTokens: 2}, "modelo")
		},
	}}
	svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, conv, turn, "", nil, true, 2)
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	done := assertRuntimeTerminal(t, ctx, conv, turn, database.TurnCompleted, "completed", em)
	var messages []database.ChatMessage
	if err := database.DB().Where("conversation_id = ? AND role = 'assistant'", conv).Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != done.AssistantMessageID || messages[0].Content != "final" || messages[0].TotalTokens != 7 || messages[0].CacheReadTokens != 2 {
		t.Fatalf("resposta: %+v", messages)
	}
	snapshot, err := chat.NewDBTimelineReader(database.DB()).ReadWindow(ctx, database.MessageWindowQuery{ConversationID: conv, Anchor: "end", Direction: "before", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, node := range snapshot.Window.Nodes {
		if node.ItemID != done.ItemID {
			continue
		}
		matched = true
		if done.TurnPatch == nil || done.TurnPatch.Message.ID != node.Message.ID || done.TurnPatch.Message.Content != node.Message.Content || done.TurnPatch.Message.TotalTokens != node.Message.TotalTokens || done.TurnPatch.Message.CacheReadTokens != node.Message.CacheReadTokens {
			t.Fatalf("terminal e janela divergentes: %+v %+v", done.TurnPatch, node)
		}
	}
	if !matched {
		t.Fatal("janela perdeu item terminal")
	}
	// Retry explícito ganha execução, mas preserva identidade da resposta.
	next, err := svc.BeginTurn(ctx, conv, turn, database.TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.EnsureTurnResponse(next, conv, turn)
	if err != nil || id != done.AssistantMessageID || executionID(next) == executionID(ctx) {
		t.Fatalf("retry id=%q err=%v", id, err)
	}
	if svc.SaveAndFinish(ctx, conv, turn, id, AgenticResult{FullResponse: "atrasada"}, "", nil, nil) {
		t.Fatal("aceitou execução antiga")
	}
	if len(em.find("chat:done")) != 1 {
		t.Fatal("terminal antigo duplicado")
	}
	svc.EndUnfinishedTurn(next, conv, turn, nil)
}

func TestTurnRuntimeCancellationBeforeWorker(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	streamer := &recoveryStreamer{}
	svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, conv, turn, "", nil, false, 1)
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	done := assertRuntimeTerminal(t, ctx, conv, turn, database.TurnInterrupted, "cancelled", em)
	if streamer.calls != 0 || done.ErrorMessage == ports.ChatErrorPersistence {
		t.Fatalf("cancelamento: %+v", done)
	}
}

func TestTurnRuntimePartialWriteFailureNeverPublishesOldPatch(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider_error", true: "cancelled"}[cancelled], func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if err := database.DB().Exec(`CREATE TRIGGER reject_partial BEFORE UPDATE ON chat_messages BEGIN SELECT RAISE(ABORT,'disco indisponivel'); END`).Error; err != nil {
				t.Fatal(err)
			}
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
				h.OnChunk("texto visível")
				if cancelled {
					cancel()
				} else {
					h.OnError("rede")
				}
			}}}
			svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, conv, turn, "", nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			done := assertRuntimeTerminal(t, ctx, conv, turn, database.TurnFailed, "persistence_error", em)
			if done.TurnPatch != nil || done.ErrorMessage != ports.ChatErrorPersistence {
				t.Fatalf("patch obsoleto: %+v", done)
			}
		})
	}
}

func TestTurnRuntimePostCommitPanicStillEmitsCompleted(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	id, err := svc.EnsureTurnResponse(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	svc.onSpeechRequest = func(string, string, string, string, string, string, bool) { panic("TTS indisponível") }
	func() {
		defer svc.EndUnfinishedTurn(ctx, conv, turn, nil)
		defer func() { svc.HandleRecoveredPanic(ctx, conv, turn, "test", recover(), nil) }()
		svc.SaveAndFinish(ctx, conv, turn, id, AgenticResult{FullResponse: "confirmada"}, "", nil, nil)
	}()
	done := assertRuntimeTerminal(t, ctx, conv, turn, database.TurnCompleted, "completed", em)
	if done.Reason != "completed" {
		t.Fatalf("terminal: %+v", done)
	}
}

func TestTurnRuntimePlaceholderFailureAndMissingTerminal(t *testing.T) {
	for _, placeholderFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider_sem_terminal", true: "placeholder"}[placeholderFailure], func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			reason := "internal_error"
			if placeholderFailure {
				reason = "persistence_error"
				if err := database.DB().Exec(`CREATE TRIGGER reject_response BEFORE INSERT ON chat_messages WHEN NEW.role='assistant' BEGIN SELECT RAISE(ABORT,'sem espaço'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			svc.StreamSimpleWithRecovery(ctx, &recoveryStreamer{}, nil, llm.ChatParams{}, conv, turn, "", nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			assertRuntimeTerminal(t, ctx, conv, turn, database.TurnFailed, reason, em)
		})
	}
}

func TestTurnRuntimePrefillPreservesExactResponseID(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	for _, entry := range []struct{ id, content string }{{"assistant", "incorreta"}, {" assistant ", "canônica"}} {
		if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: entry.id}, ConversationID: conv, TurnID: &turn, Role: "assistant", Content: entry.content}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if got := svc.loadAssistantPrefill(ctx, " assistant "); got != "canônica" {
		t.Fatalf("prefill=%q", got)
	}
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
}

func TestTurnRuntimeLimitSpeechPanicStillEmitsInterrupted(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	id, err := svc.EnsureTurnResponse(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	svc.onSpeechRequest = func(string, string, string, string, string, string, bool) { panic("fala indisponível") }
	runner := &agenticLoopRunner{svc: svc, conversationID: conv, turnID: turn, assistantMessageID: id, maxIterations: 1}
	func() {
		defer svc.EndUnfinishedTurn(ctx, conv, turn, nil)
		defer func() { svc.HandleRecoveredPanic(ctx, conv, turn, "test", recover(), nil) }()
		runner.finishLimitReached(ctx)
	}()
	done := assertRuntimeTerminal(t, ctx, conv, turn, database.TurnInterrupted, "limit_reached", em)
	if done.Reason != "limit_reached" {
		t.Fatalf("terminal: %+v", done)
	}
}

func TestTurnRuntimeRetryAfterTerminalStorageRecovers(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	if err := database.DB().Exec(`CREATE TRIGGER reject_terminal BEFORE UPDATE ON chat_turn_attempts BEGIN SELECT RAISE(ABORT,'storage offline'); END`).Error; err != nil {
		t.Fatal(err)
	}
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	events := em.find("chat:done")
	if len(events) != 1 || events[0].data.(ports.DoneEvent).ErrorMessage != ports.ChatErrorPersistence {
		t.Fatalf("terminal: %+v", events)
	}
	if _, err := svc.BeginTurn(ctx, conv, turn, database.TurnModeRetry); err == nil {
		t.Fatal("admitiu retry enquanto storage indisponível")
	}
	if err := database.DB().Exec("DROP TRIGGER reject_terminal").Error; err != nil {
		t.Fatal(err)
	}
	next, err := svc.BeginTurn(ctx, conv, turn, database.TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	current, err := database.NewChatTurnRepository(database.DB()).Get(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	if current.ActiveExecutionID == nil || *current.ActiveExecutionID != executionID(next) {
		t.Fatalf("execução: %+v", current)
	}
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	if len(em.find("chat:done")) != 1 {
		t.Fatal("execução antiga repetiu terminal")
	}
	svc.EndUnfinishedTurn(next, conv, turn, nil)
}

func TestTurnRuntimeAgenticTerminalPaths(t *testing.T) {
	for _, scenario := range []string{"success", "provider_error", "cancelled", "partial_failure"} {
		t.Run(scenario, func(t *testing.T) {
			svc, ctx, conv, turn, em := setupTurnRuntime(t)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if scenario == "partial_failure" {
				if err := database.DB().Exec(`CREATE TRIGGER reject_partial BEFORE UPDATE ON chat_messages BEGIN SELECT RAISE(ABORT,'disco indisponivel'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) {
				h.OnChunk("conteúdo")
				switch scenario {
				case "success":
					h.OnDone("conteúdo", llm.Usage{}, "modelo")
				case "cancelled":
					cancel()
				default:
					h.OnError("rede")
				}
			}}}
			svc.RunAgenticLoop(ctx, nil, llm.ChatParams{MaxAgenticIterations: 1}, conv, turn, nil, streamer, nil,
				func(convID string, iter int) IterationHandler {
					return NewAgenticStreamHandler(em, convID, iter, nil, turn)
				}, nil, false, 1)
			svc.EndUnfinishedTurn(ctx, conv, turn, nil)
			state, reason := database.TurnFailed, scenario
			switch scenario {
			case "success":
				state, reason = database.TurnCompleted, "completed"
			case "cancelled":
				state = database.TurnInterrupted
			case "partial_failure":
				reason = "persistence_error"
			}
			done := assertRuntimeTerminal(t, ctx, conv, turn, state, reason, em)
			if scenario == "partial_failure" && (done.TurnPatch != nil || done.ErrorMessage != ports.ChatErrorPersistence) {
				t.Fatalf("patch obsoleto: %+v", done)
			}
		})
	}
}

func TestTurnRuntimeStartupPreservesLiveCLIAndRecoversReleasedExecution(t *testing.T) {
	svc, ctx, conv, turn, em := setupTurnRuntime(t)
	// Cache divergente entre CLI/desktop não muda o endereço persistido do lease.
	otherCache := t.TempDir()
	t.Setenv("LOCALAPPDATA", otherCache)
	t.Setenv("XDG_CACHE_HOME", otherCache)
	// Uma segunda abertura do banco (desktop ou CLI) não prova morte do worker.
	if err := database.RecoverChatTurnsBeforeWorkers(); err != nil {
		t.Fatal(err)
	}
	current, err := database.NewChatTurnRepository(database.DB()).Get(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	if current.ActiveExecutionID == nil || *current.ActiveExecutionID != executionID(ctx) {
		t.Fatalf("interrompeu worker vivo: %+v", current)
	}
	// Simula a liberação nativa que o SO garante ao encerrar o processo.
	if err := executionFrom(ctx).lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.RecoverChatTurnsBeforeWorkers(); err != nil {
		t.Fatal(err)
	}
	current, err = database.NewChatTurnRepository(database.DB()).Get(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != database.TurnInterrupted || current.ActiveExecutionID != nil {
		t.Fatalf("não recuperou execução sem posse: %+v", current)
	}
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	if len(em.find("chat:done")) != 0 {
		t.Fatal("worker antigo publicou terminal após recuperação")
	}
}

func TestTurnRuntimeRejectedAdmissionRemovesOnlyUnusedLease(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	path := executionFrom(ctx).lease.Path
	before, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BeginTurn(ctx, conv, turn, database.TurnModeRetry); !errors.Is(err, database.ErrChatTurnActive) {
		t.Fatalf("admissão concorrente: %v", err)
	}
	after, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("lease órfão: antes=%v depois=%v", before, after)
	}
	if err := database.RecoverChatTurnsBeforeWorkers(); err != nil {
		t.Fatal(err)
	}
	current, err := database.NewChatTurnRepository(database.DB()).Get(ctx, conv, turn)
	if err != nil {
		t.Fatal(err)
	}
	if current.ActiveExecutionID == nil || *current.ActiveExecutionID != executionID(ctx) {
		t.Fatal("removeu posse da tentativa viva")
	}
}
