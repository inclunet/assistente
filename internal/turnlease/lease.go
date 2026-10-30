// Package turnlease prova posse de execuções de chat entre desktop e CLI.
// Não usa PID nem timeout: o sistema operacional libera a posse após crash.
package turnlease

import (
	"assistente/internal/commandinstance"
	"context"
	"errors"
	"github.com/google/uuid"
	"os"
	"path/filepath"
)

var ErrBusy = commandinstance.ErrBusy

type Lease struct {
	lock *commandinstance.Lock
	Path string
}

func validID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.Version() == 7 && parsed.String() == id
}

// Create antecede o INSERT da tentativa; seu caminho absoluto é persistido com ela.
func Create(ctx context.Context, directory, executionID string) (*Lease, error) {
	if !validID(executionID) || !filepath.IsAbs(directory) {
		return nil, errors.New("identidade de lease inválida")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, executionID+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return Open(ctx, path, executionID)
}

// Open usa o caminho persistido, nunca o cache/ambiente do processo que recupera.
// Arquivo ausente não prova morte: não recriar um inode para sondar uma posse.
func Open(ctx context.Context, path, executionID string) (*Lease, error) {
	if !validID(executionID) || !filepath.IsAbs(path) || filepath.Base(path) != executionID+".lock" {
		return nil, errors.New("identidade de lease inválida")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("lease não é arquivo regular")
	}
	lock, err := commandinstance.Acquire(ctx, path)
	if err != nil {
		return nil, err
	}
	return &Lease{lock: lock, Path: path}, nil
}
func (l *Lease) Close() error { return l.lock.Close() }

// RemoveUnadmitted só pode ser usado quando a admissão comprovadamente não
// chegou a inserir a tentativa. Erro/resultado ambíguo de COMMIT não autoriza unlink.
func (l *Lease) RemoveUnadmitted() error {
	if err := l.Close(); err != nil {
		return err
	}
	return os.Remove(l.Path)
}
