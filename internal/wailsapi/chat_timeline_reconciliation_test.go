package wailsapi

import (
	"assistente/internal/chat"
	"context"
	"errors"
	"fmt"
	"testing"

	"assistente/internal/apidto"
)

func TestTimelineAPIReconcileRecoveryAuthorizesCapability(t *testing.T) {
	f := newTimelineAPIFixture(t)
	opened, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	reference := apidto.TimelineRecoveryReference{ProjectionID: "old-projection", Incarnation: "old-storage", TurnID: f.message, ExecutionID: "old-execution"}
	result, err := f.api.ReconcileRecovery(opened.WindowID, reference)
	if err != nil || result.State != "unknown" || result.Reference != reference || result.ConversationID != f.conversation {
		t.Fatalf("resultado: %+v %v", result, err)
	}
	if result, err = f.api.ReconcileRecovery("wrong-window", reference); err == nil || result != nil {
		t.Fatal("capability incorreta aceita")
	}
	f.session.mu.Lock()
	f.session.session = "new-session"
	f.session.mu.Unlock()
	if result, err = f.api.ReconcileRecovery(opened.WindowID, reference); err == nil || result != nil {
		t.Fatal("sessão anterior aceita")
	}
}

type blockedReconciliationReader struct {
	*chat.DBTimelineReader
	entered, release chan struct{}
}

func (r *blockedReconciliationReader) ReadTurnLifecycles(ctx context.Context, _ string, _ []string) (*chat.TimelineSelectionSnapshot, error) {
	close(r.entered)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return nil, errors.New("teste liberado")
	}
}

func TestTimelineAPIClosingCancelsReconciliation(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprint(shutdown), func(t *testing.T) {
			f := newTimelineAPIFixture(t)
			reader := &blockedReconciliationReader{f.reader, make(chan struct{}), make(chan struct{})}
			defer close(reader.release) // também desbloqueia cleanup se a regressão reaparecer
			f.api.reader = reader
			opened, err := f.api.Open(f.request())
			if err != nil {
				t.Fatal(err)
			}
			reference := apidto.TimelineRecoveryReference{ProjectionID: "projection", Incarnation: "storage", TurnID: f.message, ExecutionID: "execution"}
			finished, closed := make(chan struct{}), make(chan struct{})
			var queryErr, closeErr error
			go func() { _, queryErr = f.api.ReconcileRecovery(opened.WindowID, reference); close(finished) }()
			waitTimelineEventTest(t, reader.entered)
			go func() {
				if shutdown {
					CloseChatTimeline(f.api)
				} else {
					closeErr = f.api.Close(opened.WindowID)
				}
				close(closed)
			}()
			waitTimelineEventTest(t, finished)
			waitTimelineEventTest(t, closed)
			if !errors.Is(queryErr, context.Canceled) || closeErr != nil {
				t.Fatal("consulta/fechamento", queryErr, closeErr)
			}
			if f.ctx.Err() != nil {
				t.Fatal("cancelou contexto pai")
			}
		})
	}
}
