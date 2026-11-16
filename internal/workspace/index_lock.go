package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"assistente/internal/commandinstance"
)

// lockIndex serializa a transação de leitura e publicação entre processos que
// compartilham o índice global, inclusive aqueles com bancos diferentes.
func (m *Manager) lockIndex(ctx context.Context) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	dir := filepath.Join(m.homeDir, workspacesDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		cancel()
		return nil, err
	}
	path := filepath.Join(dir, ".index.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := f.Close(); err != nil {
		cancel()
		return nil, err
	}
	for {
		lock, err := commandinstance.Acquire(ctx, path)
		if err == nil {
			return func() { _ = lock.Close(); cancel() }, nil
		}
		if !errors.Is(err, commandinstance.ErrBusy) {
			cancel()
			return nil, fmt.Errorf("lock workspace index: %w", err)
		}
		select {
		case <-ctx.Done():
			cancel()
			return nil, fmt.Errorf("lock workspace index: %w", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}
