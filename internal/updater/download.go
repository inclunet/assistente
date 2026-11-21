package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrDownloadIdle permite distinguir uma rede parada do cancelamento do app.
var ErrDownloadIdle = fmt.Errorf("download sem resposta da rede: %w", context.DeadlineExceeded)

// doDownload mantém a autenticação do cliente compartilhado, mas usa os limites
// de transferência. O timer de inatividade cobre apenas espera pela rede, não
// escrita em disco nem callbacks de progresso.
func (u *Updater) doDownload(ctx context.Context, req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(u.downloadIdle, func() { cancel(ErrDownloadIdle) })
	resp, err := u.downloadClient.Do(ctx, req)
	timer.Stop()
	if err != nil {
		cause := context.Cause(ctx)
		cancel(nil)
		if cause != nil {
			return nil, cause
		}
		return nil, err
	}
	resp.Body = &downloadBody{ReadCloser: resp.Body, ctx: ctx, cancel: cancel, idle: u.downloadIdle}
	return resp, nil
}

type downloadBody struct {
	io.ReadCloser
	ctx    context.Context
	cancel context.CancelCauseFunc
	idle   time.Duration
}

func (b *downloadBody) Read(p []byte) (int, error) {
	timer := time.AfterFunc(b.idle, func() { b.cancel(ErrDownloadIdle) })
	n, err := b.ReadCloser.Read(p)
	timer.Stop()
	if err != nil && context.Cause(b.ctx) != nil {
		return n, context.Cause(b.ctx)
	}
	return n, err
}

func (b *downloadBody) Close() error {
	b.cancel(nil)
	return b.ReadCloser.Close()
}
