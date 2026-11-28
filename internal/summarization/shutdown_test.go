package summarization

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/profiles"
)

type shutdownRepository struct {
	repoDeMentira
	messages   []chat.Message
	cleanupErr error
}

func (r *shutdownRepository) GetMessages(context.Context, string) ([]chat.Message, error) {
	return r.messages, nil
}

func (r *shutdownRepository) SetSummarizingInProgress(ctx context.Context, id string, inProgress bool) error {
	if !inProgress {
		r.cleanupErr = ctx.Err()
	}
	return r.repoDeMentira.SetSummarizingInProgress(ctx, id, inProgress)
}

func TestShutdownCancelaEAguardaSumarizacao(t *testing.T) {
	for _, tc := range []struct {
		name    string
		blockAt int
		markers []bool
	}{
		{name: "resolução antes da checagem", blockAt: 1},
		{name: "execução com resumo em andamento", blockAt: 2, markers: []bool{true, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profileMgr := setupProfileTestEnv(t)
			slug, err := profileMgr.Create(perfilDeAgente("openai"))
			if err != nil {
				t.Fatal(err)
			}
			repo := &shutdownRepository{}
			for range 24 {
				repo.messages = append(repo.messages, chat.Message{Role: "user", Content: strings.Repeat("texto ", 2000)})
			}
			begun, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			releaseWorker := sync.OnceFunc(func() { close(release) })
			resolverCalls := 0
			svc := NewService(ServiceConfig{
				Repo: repo, ProfileManager: profileMgr,
				ProfileResolver: func(ctx context.Context, p *profiles.Profile) *profiles.Profile {
					resolverCalls++
					if resolverCalls == tc.blockAt {
						close(begun)
						<-ctx.Done()
						close(canceled)
						<-release
						return nil
					}
					return p
				},
			})

			workerCtx, stopWorker := context.WithCancel(t.Context())
			waitCtx, stopWaiting := context.WithTimeout(t.Context(), 5*time.Second)
			defer stopWaiting()
			defer func() {
				stopWorker()
				releaseWorker()
				svc.Shutdown()
			}()

			go svc.CheckAndTriggerSummarization(workerCtx, "conversa", slug, 0)
			wait := func(ch <-chan struct{}, operation string) {
				t.Helper()
				select {
				case <-ch:
				case <-waitCtx.Done():
					t.Fatalf("tempo esgotado ao aguardar %s", operation)
				}
			}
			wait(begun, "início")
			checkDone := make(chan []bool, 1)
			shutdownDone := make(chan struct{})
			go func() {
				svc.Shutdown()
				checkDone <- repo.emAndamento
				close(shutdownDone)
			}()
			wait(canceled, "cancelamento")
			select {
			case <-shutdownDone:
				t.Fatal("Shutdown retornou enquanto a resolução ainda estava em andamento")
			case <-time.After(20 * time.Millisecond):
			}
			releaseWorker()
			wait(shutdownDone, "drenagem")
			if actual := <-checkDone; !slices.Equal(actual, tc.markers) {
				t.Fatalf("marcadores quando Shutdown retornou = %v, esperados %v", actual, tc.markers)
			}
			if repo.cleanupErr != nil {
				t.Fatalf("o contexto de limpeza está cancelado: %v", repo.cleanupErr)
			}
			svc.CheckAndTriggerSummarization(workerCtx, "conversa", slug, 0)
			if resolverCalls != tc.blockAt {
				t.Fatalf("admitiu nova resolução após Shutdown: %d chamadas", resolverCalls)
			}
		})
	}
}
