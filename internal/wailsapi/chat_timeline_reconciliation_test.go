package wailsapi

import (
	"assistente/internal/chat"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/database"
	"github.com/google/uuid"
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

func TestTimelineAPIReconcileRecoveryReturnsActiveExecution(t *testing.T) {
	f := newTimelineAPIFixture(t)
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.projection.Admit(f.ctx, f.conversation, f.message, id.String(), func() (database.ChatTimelineVersion, error) {
		version, readErr := f.reader.ReadVersion(f.ctx, f.conversation)
		if readErr != nil {
			return version, readErr
		}
		_, writeErr := database.NewChatTurnRepository(database.DB()).BeginExecution(f.ctx, f.conversation, f.message, "", database.TurnModeNew, id.String(), "")
		return version, writeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	var reference *apidto.TimelineRecoveryReference
	for _, item := range opened.Items {
		if item.RecoveryReference != nil {
			reference = item.RecoveryReference
			break
		}
	}
	if reference == nil {
		t.Fatal("snapshot não publicou referência ativa")
	}
	result, err := f.api.ReconcileRecovery(opened.WindowID, *reference)
	if err != nil || result.State != "active" || result.Reference != *reference || result.ConversationID != f.conversation {
		t.Fatalf("resultado ativo: %+v %v", result, err)
	}
}

type observedReconciliationSession struct {
	*timelineTestSession
	entered chan struct{}
}

func (s *observedReconciliationSession) AuthenticatedContext() (context.Context, error) {
	ctx, err := s.timelineTestSession.AuthenticatedContext()
	select {
	case s.entered <- struct{}{}:
	default:
	}
	return ctx, err
}

func TestTimelineAPICloseCancelsReconciliationWaitingForWriter(t *testing.T) {
	f := newTimelineAPIFixture(t)
	opened, err := f.api.Open(f.request())
	if err != nil {
		t.Fatal(err)
	}
	writerEntered, writerRelease, writerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(writerRelease) })
	defer release()
	var writerErr error
	go func() {
		writerErr = f.projection.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) {
			close(writerEntered)
			<-writerRelease
			return nil, nil
		})
		close(writerDone)
	}()
	waitTimelineEventTest(t, writerEntered)
	observed := &observedReconciliationSession{f.session, make(chan struct{}, 1)}
	f.api.mu.Lock()
	f.api.session = observed
	f.api.mu.Unlock()
	queried, closed := make(chan struct{}), make(chan struct{})
	var queryErr, closeErr error
	go func() {
		_, queryErr = f.api.ReconcileRecovery(opened.WindowID, apidto.TimelineRecoveryReference{ProjectionID: "old", Incarnation: "old", TurnID: f.message, ExecutionID: "old"})
		close(queried)
	}()
	waitTimelineEventTest(t, observed.entered)
	go func() { closeErr = f.api.Close(opened.WindowID); close(closed) }()
	waitTimelineEventTest(t, queried) // writer permanece ocupado, Close ainda drena a inscrição
	if !errors.Is(queryErr, context.Canceled) {
		t.Fatal("consulta não cancelou", queryErr)
	}
	release()
	waitTimelineEventTest(t, writerDone)
	waitTimelineEventTest(t, closed)
	if writerErr != nil || closeErr != nil {
		t.Fatal(writerErr, closeErr)
	}
}
