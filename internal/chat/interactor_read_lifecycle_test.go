package chat

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/events"
	"assistente/internal/providers"
)

type pendingConversationRead struct {
	noopConvRepo
	cancelled chan struct{}
	release   chan struct{}
	finished  chan struct{}
}

func (r pendingConversationRead) GetConversationInfo(ctx context.Context, _ string) (*Conversation, error) {
	<-ctx.Done()
	close(r.cancelled)
	<-r.release
	close(r.finished)
	return nil, ctx.Err()
}

func TestPrepareContextWaitsForCancelledReadBeforeReturningProviderError(t *testing.T) {
	repo := pendingConversationRead{cancelled: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	release := sync.OnceFunc(func() { close(repo.release) })
	t.Cleanup(release)
	interactor := NewInteractor(InteractorConfig{
		Emitter: events.NoopEmitter{}, ConvRepo: repo,
		ProviderSvc: providers.NewService(providers.ServiceConfig{Store: providers.NewMemoryStore()}),
	})
	result := make(chan error, 1)
	go func() {
		_, err := interactor.PrepareContext(t.Context(), PrepareContextRequest{ConversationID: "conversation", UserContent: "hello"})
		result <- err
	}()
	select {
	case <-repo.cancelled:
	case <-time.After(time.Second):
		t.Fatal("a falha de provedor não cancelou a consulta concorrente")
	}
	select {
	case err := <-result:
		t.Fatalf("preparação retornou com consulta ainda ativa: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "nenhum provedor LLM") {
			t.Fatalf("precedência do erro de provedor mudou: %v", err)
		}
		select {
		case <-repo.finished:
		default:
			t.Fatal("consulta ainda ativa após retorno")
		}
	case <-time.After(time.Second):
		t.Fatal("preparação não concluiu após a consulta sair")
	}
}
