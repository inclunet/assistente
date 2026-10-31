package agent

import (
	"context"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestLiveProjectionRuntimeCancelledPartialIsConfirmed(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){func(h llm.StreamHandler) { h.OnChunk("parcial salvo"); cancel() }}}
	svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, conv, turn, "", nil, false, 1)
	snap, err := svc.projection.Snapshot(ctx, conv, turn)
	if err != nil || snap.Volatile || !snap.Finished || snap.State != database.TurnInterrupted {
		t.Fatalf("parcial confirmado retido como volatile: %+v %v", snap, err)
	}
}

func TestLiveProjectionRuntimeUnfinishedKeepsPendingChunk(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	h, err := svc.NewSimpleStreamHandler(ctx, conv, turn, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.OnChunk("recebido antes do timer")
	h.OnThinking("raciocínio recebido")
	// Mesmo caminho defer usado em panic e retorno sem callback terminal.
	svc.EndUnfinishedTurn(ctx, conv, turn, nil)
	h.FlushStream()
	h.OnChunk("callback tardio")
	snap, err := svc.projection.Snapshot(ctx, conv, turn)
	if err != nil || !snap.Volatile || snap.State != database.TurnFailed || snap.Reason != "internal_error" || snap.Streams[0].Fragments[0].Text != "recebido antes do timer" || snap.Streams[0].Fragments[0].Reasoning != "raciocínio recebido" {
		t.Fatalf("terminal sem conteúdo persistido: %+v %v", snap, err)
	}
}

func TestLiveProjectionRuntimeTransportRetryAndFinalReplacement(t *testing.T) {
	svc, ctx, conv, turn, _ := setupTurnRuntime(t)
	var old *SimpleStreamHandler
	streamer := &recoveryStreamer{steps: []func(llm.StreamHandler){
		func(h llm.StreamHandler) { old = h.(*SimpleStreamHandler); h.OnChunk("parcial"); h.OnError("rede") },
		func(h llm.StreamHandler) {
			old.OnChunk("tardio")
			h.OnChunk("atual")
			h.OnDone("final explícito", llm.Usage{}, "modelo")
		},
	}}
	svc.StreamSimpleWithRecovery(ctx, streamer, nil, llm.ChatParams{}, conv, turn, "", nil, true, 2)
	snap, err := svc.projection.Snapshot(ctx, conv, turn)
	if err != nil || snap.Volatile || !snap.Finished || snap.State != database.TurnCompleted || snap.Streams[0].Fragments[0].Text != "final explícito" {
		t.Fatalf("conclusão: %+v %v", snap, err)
	}
	next, err := svc.BeginTurn(ctx, conv, turn, database.TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.EndUnfinishedTurn(next, conv, turn, nil)
	old.OnChunk("execução anterior")
	current, _ := svc.projection.Snapshot(next, conv, turn)
	if current.ExecutionID == snap.ExecutionID || len(current.Streams) != 0 || current.ItemID != snap.ItemID {
		t.Fatalf("retry contaminado: %+v", current)
	}
}
