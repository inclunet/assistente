package chat

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
)

func liveFixture(t *testing.T) (*LiveProjection, context.Context, *ExecutionProjection) {
	t.Helper()
	p := NewLiveProjection()
	ctx := database.WithUserID(context.Background(), "user")
	e, err := beginTestProjection(p, ctx, "conversation", "question", "execution")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, c := range p.conversations {
			if c.timer != nil {
				c.timer.Stop()
			}
		}
	})
	return p, ctx, e
}

func TestLiveProjectionIterationAndTransportIdentity(t *testing.T) {
	p, ctx, e := liveFixture(t)
	old := e.StartStream(1)
	old.Append("tentativa de rede")
	current := e.StartStream(1)
	if old.Append("atrasado") || old.Seal("errado", "") {
		t.Fatal("callback antigo aceito")
	}
	current.Initial("prefixo ")
	current.Append("primeiro")
	current.Cut()
	current.Append(" segundo")
	current.Reasoning("raciocínio", false)
	before, _ := p.Snapshot(ctx, "conversation", "question")
	if !current.Seal("prefixo primeiro segundo", "raciocínio") {
		t.Fatal("seal recusado")
	}
	if current.Append("tardio") {
		t.Fatal("conteúdo depois de seal")
	}
	e.StartStream(2).Append("terceiro")
	after, _ := p.Snapshot(ctx, "conversation", "question")
	if len(after.Streams) != 2 || len(after.Streams[0].Fragments) != 2 || after.Streams[0].Fragments[0].ID != before.Streams[0].Fragments[0].ID || after.Streams[0].Fragments[1].Text != " segundo" {
		t.Fatalf("fragmentos e iterações: %+v", after)
	}
	if before.Streams[0].Fragments[1].Reasoning != "raciocínio" {
		t.Fatal("snapshot foi alterado posteriormente")
	}
}

func TestLiveProjectionTerminalPreservesUncommittedContent(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "storage_failure", true: "metadata_only"}[committed], func(t *testing.T) {
			p, ctx, e := liveFixture(t)
			s := e.StartStream(0)
			s.Append("conteúdo recebido")
			s.Reasoning("pensamento", false)
			finishTestProjection(e, database.TurnFailed, "internal_error", committed, false)
			if s.Append("atrasado") {
				t.Fatal("terminal aceitou delta")
			}
			snap, _ := p.Snapshot(ctx, "conversation", "question")
			if !snap.Volatile || !snap.Finished || snap.Streams[0].Fragments[0].Text != "conteúdo recebido" || snap.Streams[0].Fragments[0].Reasoning != "pensamento" {
				t.Fatalf("perdeu conteúdo: %+v", snap)
			}
			if committed && snap.Reason != "internal_error" {
				t.Fatal("metadado confirmado virou erro de persistência")
			}
			if !committed && snap.Reason != "persistence_error" {
				t.Fatal("falha de persistência oculta")
			}
			next, err := beginTestProjection(p, ctx, "conversation", "question", "retry")
			if err != nil {
				t.Fatal(err)
			}
			next.StartStream(0).Append("novo")
			if len(e.conversation.retained) != 1 || e.conversation.retained["execution"].streams[0].parts[0].text.String() != "conteúdo recebido" {
				t.Fatal("retry apagou tentativa volatile")
			}
		})
	}
}

func TestLiveProjectionIsolationAndConcurrentSnapshot(t *testing.T) {
	p, ctx, e := liveFixture(t)
	s := e.StartStream(0)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			s.Append("x")
		}
	}()
	for i := 0; i < 100; i++ {
		snap, err := p.Snapshot(ctx, "conversation", "question")
		if err != nil || snap.ExecutionID != "execution" {
			t.Fatalf("snapshot: %+v %v", snap, err)
		}
	}
	wg.Wait()
	final, _ := p.Snapshot(ctx, "conversation", "question")
	if len(final.Streams[0].Fragments[0].Text) != 1000 {
		t.Fatal("delta perdido")
	}
	other := database.WithUserID(context.Background(), "other")
	if snap, _ := p.Snapshot(other, "conversation", "question"); snap != nil {
		t.Fatal("vazamento entre usuários")
	}
	if snap, _ := p.Snapshot(ctx, "other", "question"); snap != nil {
		t.Fatal("vazamento entre conversas")
	}
	if _, err := p.Snapshot(context.Background(), "conversation", "question"); err == nil {
		t.Fatal("aceitou contexto sem usuário")
	}
}

func TestLiveProjectionRetentionKeepsActiveAndVolatile(t *testing.T) {
	p, ctx, e := liveFixture(t)
	e.StartStream(0).Append("confirmado")
	finishTestProjection(e, database.TurnCompleted, "completed", true, true)
	active, _ := beginTestProjection(p, ctx, "conversation", "active", "active-execution")
	active.StartStream(0).Append("ativo")
	volatile, _ := beginTestProjection(p, ctx, "conversation", "volatile", "volatile-execution")
	volatile.StartStream(0).Append("não salvo")
	finishTestProjection(volatile, database.TurnFailed, "persistence_error", false, false)
	p.mu.Lock()
	c := e.conversation
	c.mu.Lock()
	c.timer.Stop()
	c.timer = nil
	p.pruneLocked(e.key, c, time.Now().Add(2*projectionRetention))
	c.mu.Unlock()
	p.mu.Unlock()
	if snap, _ := p.Snapshot(ctx, "conversation", "question"); snap != nil {
		t.Fatal("reteve conteúdo dispensável")
	}
	for _, turn := range []string{"active", "volatile"} {
		if snap, _ := p.Snapshot(ctx, "conversation", turn); snap == nil {
			t.Fatalf("expulsou %s", turn)
		}
	}
}

func BenchmarkLiveProjectionAppend(b *testing.B) {
	for i := 0; i < b.N; i++ {
		p := NewLiveProjection()
		e, _ := beginTestProjection(p, database.WithUserID(context.Background(), "user"), "conversation", "turn", "execution")
		s := e.StartStream(0)
		for j := 0; j < 1000; j++ {
			s.Append("um delta de texto ")
		}
	}
}

func TestLiveProjectionReasoningReplacementAndDiscardAcrossCuts(t *testing.T) {
	p, ctx, e := liveFixture(t)
	s := e.StartStream(0)
	s.Reasoning("A", false)
	s.Cut()
	s.Reasoning("B", false)
	s.Reasoning("AB", true)
	snap, _ := p.Snapshot(ctx, "conversation", "question")
	if snap.Streams[0].Fragments[0].Reasoning != "AB" || snap.Streams[0].Fragments[1].Reasoning != "" {
		t.Fatalf("raciocínio duplicado: %+v", snap.Streams)
	}
	s.Reasoning("", true)
	snap, _ = p.Snapshot(ctx, "conversation", "question")
	for _, fragment := range snap.Streams[0].Fragments {
		if fragment.Reasoning != "" {
			t.Fatal("raciocínio descartado permaneceu")
		}
	}
}

func TestLiveProjectionAcknowledgesOnlyWrittenContent(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed_partial", true: "new_delta"}[late], func(t *testing.T) {
			p, ctx, e := liveFixture(t)
			s := e.StartStream(0)
			s.Append("parcial")
			if late {
				s.Append(" novo")
			}
			_ = e.CommitContent(-1, "parcial", "", func() error { return nil })
			finishTestProjection(e, database.TurnInterrupted, "cancelled", true, false)
			snap, _ := p.Snapshot(ctx, "conversation", "question")
			if snap.Volatile != late {
				t.Fatalf("confirmação incorreta: %+v", snap)
			}
		})
	}
}

func TestLiveProjectionEmptyIntermediateDoesNotRequireRecovery(t *testing.T) {
	for _, meaningful := range []bool{false, true} {
		t.Run(map[bool]string{false: "whitespace", true: "uncommitted_content"}[meaningful], func(t *testing.T) {
			p, ctx, e := liveFixture(t)
			first := e.StartStream(1)
			text := " \t\n\u00a0"
			if meaningful {
				text = "intermediário não salvo"
			}
			first.Append(text)
			first.Seal(text, "\t\n")
			e.StartStream(2).Seal("resposta final", "")
			finishTestProjection(e, database.TurnCompleted, "completed", true, true)
			snapshot, _ := p.Snapshot(ctx, "conversation", "question")
			if snapshot.Volatile != meaningful {
				t.Fatalf("classificação incorreta: %+v", snapshot)
			}
		})
	}
}

func beginTestProjection(p *LiveProjection, ctx context.Context, conversationID, turnID, executionID string) (*ExecutionProjection, error) {
	return p.Admit(ctx, conversationID, turnID, executionID, func() (database.ChatTimelineVersion, error) {
		return database.ChatTimelineVersion{Incarnation: "storage", Revision: 1}, nil
	})
}

func finishTestProjection(e *ExecutionProjection, state, reason string, persisted, contentCommitted bool) {
	write := func() (database.ChatTimelineVersion, error) {
		if !persisted {
			return database.ChatTimelineVersion{}, errors.New("falha de gravação")
		}
		return database.ChatTimelineVersion{Incarnation: "storage", Revision: 2}, nil
	}
	if contentCommitted {
		snapshot, _ := e.owner.Snapshot(database.WithUserID(context.Background(), e.key.userID), e.key.conversationID, e.turnID)
		var text, reasoning string
		for _, part := range snapshot.Streams[len(snapshot.Streams)-1].Fragments {
			text += part.Text
			reasoning += part.Reasoning
		}
		_ = e.CommitResponse(state, reason, text, reasoning, write)
	} else {
		_ = e.CommitTerminal(state, reason, write)
	}
}
