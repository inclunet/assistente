// Package commandinstance fornece exclusão local de participantes de uma
// instância; não é prova de morte nem executa recovery.
package commandinstance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	ErrBusy        = errors.New("instância de comandos já está em uso")
	ErrUnsupported = errors.New("exclusão de instância não suportada nesta plataforma")
	ErrInvalid     = errors.New("caminho de banco inválido")
)

// Lock representa a posse exclusiva do arquivo. Use sempre um ponteiro.
type Lock struct {
	mu       sync.Mutex
	file     *os.File
	identity string
	closed   bool
	closeErr error
	release  func() error
}

func Acquire(ctx context.Context, databasePath string) (*Lock, error) {
	if ctx == nil || strings.TrimSpace(databasePath) == "" || strings.TrimSpace(databasePath) != databasePath {
		return nil, ErrInvalid
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	file, err := os.OpenFile(databasePath, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, ErrInvalid
	}
	identity, release, err := acquirePlatformLock(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	select {
	case <-ctx.Done():
		_ = file.Close()
		return nil, ctx.Err()
	default:
	}
	return &Lock{file: file, identity: identity, release: release}, nil
}

func (l *Lock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return l.closeErr
	}
	l.closed = true
	if l.release == nil {
		l.closeErr = fmt.Errorf("%w: lock sem release", ErrInvalid)
		return l.closeErr
	}
	l.closeErr = l.release()
	return l.closeErr
}

func (l *Lock) Identity() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.identity
}
