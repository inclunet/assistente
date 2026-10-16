package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
)

// Usa a fixture autenticada e o banco temporário do teste de execução, sem
// sessão nativa, Credential Manager ou configuração pessoal.
func exerciseCommandRebuildCancellation(t *testing.T, app *App, store *commandconfig.Store,
	state *commandexecution.HostState, token, userID string, options commandconfig.LocalReadProjection,
) {
	t.Helper()
	for _, known := range []bool{true, false} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		entered := make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			finished <- app.rebuildPersistedCommandConfiguration(ctx, token, store,
				func(buildCtx context.Context, _ commandconfig.Snapshot) (*commandbindings.Configuration, error) {
					close(entered)
					<-buildCtx.Done()
					return nil, buildCtx.Err()
				})
		}()
		select {
		case <-entered:
		case <-ctx.Done():
			cancel()
			t.Fatal("projetor não iniciou")
		}
		// Tanto lock quanto perda da observação do SO devem encerrar o I/O.
		if err := state.SetOSSessionState(ctx, known, true); err != nil {
			cancel()
			t.Fatal(err)
		}
		select {
		case err := <-finished:
			if !errors.Is(err, context.Canceled) {
				cancel()
				t.Fatal("carregamento não foi cancelado pelo host", err)
			}
		case <-ctx.Done():
			cancel()
			t.Fatal("carregamento ficou esperando após invalidação")
		}
		if _, _, err := state.UserConfiguration(ctx, userID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
			cancel()
			t.Fatal("mapa sobreviveu à invalidação", err)
		}
		cancel()
		if err := state.SetOSSessionState(context.Background(), true, false); err != nil {
			t.Fatal(err)
		}
		if err := app.rebuildPersistedLocalReadConfiguration(context.Background(), token, store, options); err != nil {
			t.Fatal("nova reconstrução autenticada falhou", err)
		}
	}
}
