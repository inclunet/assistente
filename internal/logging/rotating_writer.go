package logging

import (
	"assistente/internal/commandinstance"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RotatingWriter mantém o arquivo atual e até backups arquivos anteriores.
// Chamadas concorrentes são serializadas; cada registro é escrito inteiro.
type RotatingWriter struct {
	mu          sync.Mutex
	path        string
	archive     string
	maxSize     int64
	backups     int
	file        *os.File
	size        int64
	lock        *commandinstance.Lock
	afterMove   func() error // injeção de falhas para teste de rollback
	afterCommit func() error // injeção de falhas para teste de limpeza
	cleanupErr  error
	started     time.Time
	interval    time.Duration
}

func NewRotatingWriter(path string, maxSize int64, backups int) (*RotatingWriter, error) {
	return newRotatingWriter(path, maxSize, backups, nil)
}

func newRotatingWriter(path string, maxSize int64, backups int, afterCreate func() error) (_ *RotatingWriter, resultErr error) {
	if path == "" || maxSize <= 0 || backups < 1 || backups > 100 {
		return nil, fmt.Errorf("configuração de rotação inválida")
	}
	archive := path + ".assistente-archives"
	marker := filepath.Join(archive, ".assistente-log")
	_, statErr := os.Lstat(archive)
	created := false
	var err error
	var archiveInfo, markerInfo, createdFileInfo os.FileInfo
	defer func() {
		if resultErr == nil || !created {
			return
		}
		removeIfSame := func(path string, original os.FileInfo) {
			if original == nil {
				return
			}
			if current, err := os.Lstat(path); err == nil && os.SameFile(original, current) {
				_ = os.Remove(path)
			}
		}
		// Não remova um arquivo criado aqui se outro escritor já depositou dados.
		if current, err := os.Lstat(path); err == nil && createdFileInfo != nil && current.Size() == 0 && os.SameFile(createdFileInfo, current) {
			_ = os.Remove(path)
		}
		removeIfSame(marker, markerInfo)
		removeIfSame(archive, archiveInfo) // Somente se permanecer vazio.
	}()
	if os.IsNotExist(statErr) {
		// Nunca adote um arquivo preexistente que pode conter outros dados.
		if _, err := os.Lstat(path); err == nil {
			return nil, fmt.Errorf("o arquivo de log já existe e não pertence ao Assistente")
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.Mkdir(archive, 0o700); err != nil {
			return nil, err
		}
		created = true
		archiveInfo, err = os.Lstat(archive)
		if err != nil {
			return nil, err
		}
		initialMarker, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		markerInfo, err = initialMarker.Stat()
		if err != nil {
			_ = initialMarker.Close()
			return nil, err
		}
		_, writeErr := initialMarker.WriteString("assistente-log-v1\n")
		if err := errors.Join(writeErr, initialMarker.Close()); err != nil {
			return nil, err
		}
	} else if statErr != nil {
		return nil, statErr
	} else {
		info, err := os.Lstat(archive)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("diretório de arquivos anteriores inválido")
		}
		data, err := os.ReadFile(marker)
		if err != nil || !strings.HasPrefix(string(data), "assistente-log-v1\n") {
			return nil, fmt.Errorf("o diretório de arquivos anteriores não pertence ao Assistente")
		}
	}
	lock, err := commandinstance.Acquire(context.Background(), marker)
	if err != nil {
		if errors.Is(err, commandinstance.ErrBusy) {
			created = false
		}
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(archive, ".rotation-pending")); err == nil {
		_ = lock.Close()
		return nil, errors.New("rotação interrompida; escolha outro arquivo para retomar os logs")
	} else if !os.IsNotExist(err) {
		_ = lock.Close()
		return nil, err
	}
	flags := os.O_APPEND | os.O_WRONLY
	if created {
		flags |= os.O_CREATE | os.O_EXCL
	}
	started := time.Now()
	if !created {
		if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
			_ = lock.Close()
			return nil, fmt.Errorf("o destino de logs não é um arquivo regular")
		} else if err != nil {
			_ = lock.Close()
			return nil, err
		}
		identity, err := logFileIdentity(path)
		if err != nil {
			_ = lock.Close()
			return nil, err
		}
		data, err := os.ReadFile(marker)
		lines := strings.Split(string(data), "\n")
		if err != nil || len(lines) != 4 || lines[0] != "assistente-log-v1" || lines[1] != identity || lines[3] != "" {
			_ = lock.Close()
			return nil, fmt.Errorf("o arquivo de log foi substituído")
		}
		stamp, parseErr := strconv.ParseInt(lines[2], 10, 64)
		if parseErr != nil || stamp <= 0 {
			_ = lock.Close()
			return nil, errors.New("marcador de log inválido")
		}
		started = time.Unix(0, stamp)
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	if err := requireSingleLogLink(file); err != nil {
		_ = file.Close()
		_ = lock.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = lock.Close()
		return nil, err
	}
	if created {
		createdFileInfo = info
		if afterCreate != nil {
			if err := afterCreate(); err != nil {
				_ = file.Close()
				_ = lock.Close()
				return nil, err
			}
		}
		if err := writeLogIdentity(path, marker, started); err != nil {
			_ = file.Close()
			_ = lock.Close()
			return nil, err
		}
	}
	return &RotatingWriter{path: path, archive: archive, maxSize: maxSize, backups: backups, file: file, size: info.Size(), lock: lock, started: started}, nil
}

func logFileIdentity(path string) (string, error) {
	lock, err := commandinstance.Acquire(context.Background(), path)
	if err != nil {
		return "", err
	}
	identity := lock.Identity()
	return identity, lock.Close()
}

func writeLogIdentity(path, marker string, started time.Time) error {
	identity, err := logFileIdentity(path)
	if err != nil {
		return err
	}
	return os.WriteFile(marker, []byte("assistente-log-v1\n"+identity+"\n"+strconv.FormatInt(started.UnixNano(), 10)+"\n"), 0o600)
}

func (w *RotatingWriter) configure(maxSize int64, backups int, interval time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.maxSize, w.backups = maxSize, backups
	w.interval = interval
}

func (w *RotatingWriter) checkReady() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return errors.New("gravação interrompida; escolha outro arquivo")
	}
	if _, err := os.Lstat(filepath.Join(w.archive, ".rotation-pending")); err == nil {
		return errors.New("rotação pendente; escolha outro arquivo")
	} else if !os.IsNotExist(err) {
		return err
	}
	openInfo, err := w.file.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(w.path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(openInfo, pathInfo) {
		return errors.New("arquivo de log indisponível ou substituído")
	}
	return nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && (w.size+int64(len(p)) > w.maxSize || (w.interval > 0 && time.Since(w.started) >= w.interval)) {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	cleanupErr := w.cleanupErr
	w.cleanupErr = nil
	return n, errors.Join(err, cleanupErr)
}

func (w *RotatingWriter) rotate() (result error) {
	if err := requireSingleLogLink(w.file); err != nil {
		return err
	}
	oldInfo, err := w.file.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(w.path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(oldInfo, pathInfo) {
		return errors.New("o arquivo de log foi substituído durante a execução")
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	pending := filepath.Join(w.archive, ".rotation-pending")
	marker, err := os.OpenFile(pending, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if info, statErr := os.Lstat(w.path); statErr == nil && os.SameFile(oldInfo, info) {
			w.file, _ = os.OpenFile(w.path, os.O_APPEND|os.O_WRONLY, 0o600)
		}
		return err
	}
	if err := errors.Join(marker.Sync(), marker.Close()); err != nil {
		if info, statErr := os.Lstat(w.path); statErr == nil && os.SameFile(oldInfo, info) {
			w.file, _ = os.OpenFile(w.path, os.O_APPEND|os.O_WRONLY, 0o600)
		}
		return err
	}
	moved := false
	committed := false
	var shifted []int
	var savedOldest string
	var newInfo os.FileInfo
	defer func() {
		if result == nil || committed {
			return
		}
		if moved {
			// Só remova um arquivo novo cuja identidade ainda é a que criamos.
			if current, err := os.Lstat(w.path); err == nil {
				if newInfo == nil || !os.SameFile(newInfo, current) {
					return
				}
				if err := os.Remove(w.path); err != nil {
					return
				}
			} else if !os.IsNotExist(err) {
				return
			}
			backup := filepath.Join(w.archive, "1")
			if info, err := os.Lstat(backup); err != nil || !os.SameFile(oldInfo, info) {
				return
			}
			if err := os.Rename(backup, w.path); err != nil {
				return
			}
		}
		for i := len(shifted) - 1; i >= 0; i-- {
			n := shifted[i]
			if err := os.Rename(filepath.Join(w.archive, strconv.Itoa(n+1)), filepath.Join(w.archive, strconv.Itoa(n))); err != nil {
				return
			}
		}
		if savedOldest != "" {
			if err := os.Rename(savedOldest, filepath.Join(w.archive, strconv.Itoa(w.backups))); err != nil {
				return
			}
		}
		if info, err := os.Lstat(w.path); err != nil || !os.SameFile(oldInfo, info) {
			return
		}
		// A identidade original volta a valer, mesmo se o marcador tiver falhado.
		if err := writeLogIdentity(w.path, filepath.Join(w.archive, ".assistente-log"), w.started); err != nil {
			return
		}
		file, err := os.OpenFile(w.path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			w.file, w.size = file, oldInfo.Size()
			_ = os.Remove(pending)
		}
	}()
	if w.backups > 0 {
		oldest := filepath.Join(w.archive, strconv.Itoa(w.backups))
		if info, err := os.Lstat(oldest); err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("arquivo anterior de log inválido")
			}
			tmp, err := os.CreateTemp(w.archive, ".oldest-*")
			if err != nil {
				return err
			}
			tmpPath := tmp.Name()
			if err := tmp.Close(); err != nil {
				return err
			}
			if err := os.Remove(tmpPath); err != nil {
				return err
			}
			if err := os.Rename(oldest, tmpPath); err != nil {
				return err
			}
			savedOldest = tmpPath
		} else if !os.IsNotExist(err) {
			return err
		}
		for i := w.backups - 1; i >= 1; i-- {
			old := filepath.Join(w.archive, strconv.Itoa(i))
			if info, err := os.Lstat(old); err == nil && !info.Mode().IsRegular() {
				return errors.New("arquivo anterior de log inválido")
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Rename(old, filepath.Join(w.archive, strconv.Itoa(i+1))); err != nil {
				if !os.IsNotExist(err) {
					return err
				}
			} else {
				shifted = append(shifted, i)
			}
		}
		if err := os.Rename(w.path, filepath.Join(w.archive, "1")); err != nil {
			return err
		}
		moved = true
	}
	if w.afterMove != nil {
		if err := w.afterMove(); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_EXCL|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	newInfo, err = file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	started := time.Now()
	if err := writeLogIdentity(w.path, filepath.Join(w.archive, ".assistente-log"), started); err != nil {
		_ = file.Close()
		return err
	}
	w.file, w.size = file, 0
	w.started = started
	committed = true
	if w.afterCommit != nil {
		if err := w.afterCommit(); err != nil {
			w.cleanupErr = err
			return nil
		}
	}
	if savedOldest != "" {
		if err := os.Remove(savedOldest); err != nil {
			w.cleanupErr = err
			return nil
		}
	}
	for i := w.backups + 1; i <= 100; i++ {
		if err := os.Remove(filepath.Join(w.archive, strconv.Itoa(i))); err != nil && !os.IsNotExist(err) {
			w.cleanupErr = err
			return nil
		}
	}
	if err := os.Remove(pending); err != nil {
		w.cleanupErr = err
		return nil
	}
	return nil
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var fileErr error
	if w.file != nil {
		fileErr = w.file.Close()
	}
	err := errors.Join(fileErr, w.lock.Close())
	w.file = nil
	return err
}
