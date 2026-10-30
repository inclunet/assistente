package agent

import (
	"context"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestSummarySurvivesCompletedExecution(t *testing.T) {
	manager := chat.NewStreamingManager(nil)
	ctx, _, finish, err := manager.Begin(database.WithUserID(context.Background(), "owner"), "conversation")
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	released := make(chan struct{})
	result := make(chan error, 1)
	service := NewService(ServiceConfig{
		Emitter: &mockEmitter{}, MsgRepo: &mockMsgRepo{},
		TriggerSummarize: func(summaryCtx context.Context, _, _ string, outputLimit int) {
			<-released
			if outputLimit != 2048 {
				t.Errorf("resumo recebeu limite %d, esperado o limite efetivamente enviado (2048)", outputLimit)
			}
			if _, ownerErr := database.RequireUserID(summaryCtx); ownerErr != nil {
				result <- ownerErr
				return
			}
			result <- summaryCtx.Err()
		},
	})
	service.SaveAndFinish(ctx, "conversation", "turn", "assistant", AgenticResult{FullResponse: "response", Finish: llm.FinishInfo{OutputLimit: 2048}}, "", nil, nil)
	finish()
	close(released)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("resumo perdeu contexto autenticado após conclusão: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("resumo não disparou")
	}
}
