package chat

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"assistente/internal/database"
)

type requestTimelineSession struct{}

func (requestTimelineSession) AuthorizeTimeline(ctx context.Context) (context.Context, string, error) {
	return ctx, "request-session", nil
}

type cancelLifecycleReader struct {
	*DBTimelineReader
	cancel context.CancelFunc
}

func (r *cancelLifecycleReader) ReadTurnLifecycles(ctx context.Context, conversation string, ids []string) (*TimelineSelectionSnapshot, error) {
	snapshot, err := r.DBTimelineReader.ReadTurnLifecycles(ctx, conversation, ids)
	r.cancel() // Cancelamento entre a leitura concluída e a aplicação da prova.
	return snapshot, err
}

func TestWindowLifecycleCancellationPreservesSubscription(t *testing.T) {
	for _, operation := range []string{"refresh", "navigate"} {
		t.Run(operation, func(t *testing.T) {
			f := newSnapshotFixture(t)
			ctx, cancel := context.WithCancel(f.ctx)
			defer cancel()
			probe := &cancelLifecycleReader{DBTimelineReader: f.reader, cancel: cancel}
			p := NewLiveProjection(probe)
			sub, initial, err := p.SubscribeTimeline(ctx, requestTimelineSession{}, "reading", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			c := sub.conversation
			c.turns[f.question] = &liveTurn{turnID: f.question, executionID: "obsolete", incarnation: c.incarnation}
			windows, published := cloneSubscribedWindows(c.windows), c.publication.fork()
			var result *SubscribedTimelineSnapshot
			if operation == "refresh" {
				result, err = sub.Refresh(ctx, initial.WindowRevision, nil)
			} else {
				result, err = sub.Navigate(ctx, TimelineWindowNavigation{Epoch: initial.Epoch, WindowRevision: initial.WindowRevision, TimelineWindowPosition: TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 1}})
			}
			if !errors.Is(err, context.Canceled) || result != nil {
				t.Fatalf("pedido cancelado retornou snapshot: %+v %v", result, err)
			}
			if !reflect.DeepEqual(windows, c.windows) || !reflect.DeepEqual(published, c.publication) || c.turns[f.question].detached {
				t.Fatal("pedido cancelado alterou janela, journal ou lifecycle")
			}
		})
	}
}

func TestVersionedSelectionsPreservesEarlierDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	visited := false
	_, err := visitVersionedSelections(ctx, "conversation", []string{"turn"}, nil, func(ctx context.Context, _ string, _ []string) (*TimelineSelectionSnapshot, error) {
		actual, ok := ctx.Deadline()
		if !ok || !actual.Equal(deadline) {
			t.Fatal("prazo do pedido substituído pelo orçamento interno", actual)
		}
		return &TimelineSelectionSnapshot{}, nil
	}, func([]string, *TimelineSelectionSnapshot) error {
		visited = true
		return nil
	})
	if err != nil || !visited {
		t.Fatal("leitura válida recusada", err)
	}
}

func TestWindowLifecycleCommittedWriteSurvivesRequestCancellation(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "history", 2)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	err := p.ApplyStorageMutation(ctx, f.conversation, []string{"message:first"}, func() ([]string, error) {
		err := database.UpdateMessageTextWithContext(ctx, "first", "confirmado antes de cancelar")
		cancel()
		return nil, err
	})
	if err != nil || sub.conversation.publicationErrorLocked() != nil {
		t.Fatal("cancelamento abandonou escrita confirmada", err, sub.conversation.publicationErrorLocked())
	}
	for _, batch := range membershipReplay(t, f, sub, initial) {
		for _, operation := range batch.Operations {
			if operation.Kind == "segment_replace" && operation.Segment != nil && operation.ItemID == "message:first" && operation.Segment.Text == "confirmado antes de cancelar" {
				return
			}
		}
	}
	t.Fatal("escrita confirmada ausente do replay")
}

func TestWindowRetentionCancellationDoesNotReleaseReadingItem(t *testing.T) {
	for _, phase := range []string{"version", "selection", "deadline"} {
		t.Run(phase, func(t *testing.T) {
			f, p := membershipFixture(t)
			sub, initial, err := p.SubscribeTimeline(f.ctx, requestTimelineSession{}, "reading", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 2}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			followTail(t, f, sub, initial, 2)
			retainWindow(t, f, sub, "message:first")
			membershipWrite(t, f, p, "new", 30)
			c := sub.conversation
			windows, published := cloneSubscribedWindows(c.windows), c.publication.fork()
			ctx, cancel := context.WithCancel(f.ctx)
			defer cancel()
			wantErr := error(context.Canceled)
			var deadlineProbe *retentionDeadlineReader
			var deadline time.Time
			switch phase {
			case "version":
				p.reader = &retentionVersionCancelReader{DBTimelineReader: f.reader, cancel: cancel}
			case "selection":
				p.reader = &failingSelectionReader{DBTimelineReader: f.reader, afterSelection: cancel}
			case "deadline":
				deadline = time.Now().Add(2 * time.Second)
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, deadline)
				defer stop()
				deadlineProbe = &retentionDeadlineReader{DBTimelineReader: f.reader}
				p.reader = deadlineProbe
				wantErr = context.DeadlineExceeded
			}
			err = sub.SetRetention(ctx, TimelineRetentionRequest{Epoch: c.epoch, WindowRevision: strconv.FormatUint(c.windows[sub.windowID].revision, 10)})
			if !errors.Is(err, wantErr) {
				t.Fatal("liberação cancelada não retornou cancelamento", err)
			}
			if deadlineProbe != nil && !deadlineProbe.observed.Equal(deadline) {
				t.Fatal("prazo menor foi perdido na seleção", deadlineProbe.observed)
			}
			if !reflect.DeepEqual(windows, c.windows) || !reflect.DeepEqual(published, c.publication) {
				t.Fatal("liberação cancelada cortou item em leitura ou publicou confirmação")
			}
		})
	}
}

type retentionVersionCancelReader struct {
	*DBTimelineReader
	cancel context.CancelFunc
}

func (r *retentionVersionCancelReader) ReadVersion(ctx context.Context, conversation string) (database.ChatTimelineVersion, error) {
	version, err := r.DBTimelineReader.ReadVersion(ctx, conversation)
	r.cancel()
	return version, err
}

type retentionDeadlineReader struct {
	*DBTimelineReader
	observed time.Time
}

func (r *retentionDeadlineReader) ReadSelection(ctx context.Context, _ string, _ []string) (*TimelineSelectionSnapshot, error) {
	r.observed, _ = ctx.Deadline()
	// Falha determinística da leitura; não depende de sleeps nem carga do runner.
	return nil, context.DeadlineExceeded
}
