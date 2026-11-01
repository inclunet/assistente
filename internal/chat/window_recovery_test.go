package chat

import (
	"assistente/internal/database"
	"context"
	"errors"
	"testing"
	"time"
)

type windowRecoveryReader func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error)

func (r windowRecoveryReader) ReadWindow(ctx context.Context, q database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	return r(ctx, q)
}

func TestWindowRecoverySerializesDeltasAndPreservesOffPageExecution(t *testing.T) {
	p, ctx, e := liveFixture(t)
	stream := e.StartStream(0)
	stream.Append("recebido")
	other, err := p.Admit(ctx, "conversation", "other", "other-execution", func() (database.ChatTimelineVersion, error) {
		return database.ChatTimelineVersion{Incarnation: "storage"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	otherStream := other.StartStream(0)
	reading, release := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, release)
	done := make(chan error, 1)
	go func() {
		_, err := p.ReadWindow(ctx, database.MessageWindowQuery{ConversationID: "conversation"}, windowRecoveryReader(func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
			close(reading)
			<-release
			return &TimelineWindowSnapshot{Version: database.ChatTimelineVersion{Incarnation: "storage", Revision: 2}, TurnIDs: []string{"question"}}, nil
		}))
		done <- err
	}()
	<-reading
	appended := make(chan bool, 1)
	go func() { appended <- stream.Append(" antigo") }()
	select {
	case <-appended:
		t.Fatal("delta atravessou a consulta da janela")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if <-appended {
		t.Fatal("execução que perdeu vínculo continuou escrevendo")
	}
	if !otherStream.Append("fora da página") {
		t.Fatal("ausência na página foi interpretada como exclusão")
	}
	recovery, err := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) { return recoveryStored("", 2), nil }})
	if err != nil || len(recovery.Retained) != 1 || testLiveSegment(recovery.Retained[0].Streams[0], "text", 0).Text != "recebido" {
		t.Fatalf("conteúdo desvinculado perdido: %+v %v", recovery, err)
	}
}

func TestWindowRecoveryFailureDoesNotChangeExecution(t *testing.T) {
	p, ctx, e := liveFixture(t)
	stream := e.StartStream(0)
	denied := errors.New("consulta recusada")
	result, err := p.ReadWindow(ctx, database.MessageWindowQuery{ConversationID: "conversation"}, windowRecoveryReader(func(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
		return nil, denied
	}))
	if !errors.Is(err, denied) || result != nil {
		t.Fatalf("erro perdido: %+v %v", result, err)
	}
	if !stream.Append("preservado") {
		t.Fatal("falha de consulta invalidou execução")
	}
}

func TestWindowRecoveryUsesAuthorizedTransactionalReader(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(nil)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	if result, err := p.ReadWindow(database.WithUserID(context.Background(), "other-user"), query, f.reader); err == nil || result != nil {
		t.Fatal("janela de outro usuário exposta")
	}
	result, err := p.ReadWindow(f.ctx, query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TurnIDs) != 1 || result.TurnIDs[0] != f.question || responseNode(t, result.Window.Nodes, f.question).Message.ID != f.answer {
		t.Fatalf("janela autorizada alterada: %+v", result)
	}
}

func BenchmarkTimelineWindowBarrier(b *testing.B) {
	f := newSnapshotFixture(b)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	p := NewLiveProjection(nil)
	for _, coordinated := range []bool{false, true} {
		name := "direct"
		if coordinated {
			name = "coordinated"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var err error
				if coordinated {
					_, err = p.ReadWindow(f.ctx, query, f.reader)
				} else {
					_, err = f.reader.ReadWindow(f.ctx, query)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
