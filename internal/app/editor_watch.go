package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type editorDirWatch struct {
	dir     string
	watcher *fsnotify.Watcher
	done    chan struct{}

	mu       sync.Mutex
	files    map[string]int
	lastEmit map[string]time.Time
}

type editorAssistedWrite struct {
	origin    string
	expiresAt time.Time
	size      int64
	modTime   time.Time
	committed bool
	token     int64
}

const editorAssistedWriteTTL = 5 * time.Second
const editorAssistedWriteCommitWait = 200 * time.Millisecond

func normalizeWatchPath(p string) (string, error) {
	s := strings.TrimSpace(p)
	if s == "" {
		return "", fmt.Errorf("path vazio")
	}
	s = filepath.Clean(s)
	abs, err := filepath.Abs(s)
	if err == nil {
		s = abs
	}
	if runtime.GOOS == "windows" {
		s = strings.ToLower(s)
	}
	return s, nil
}

func (dw *editorDirWatch) isWatchingFile(normalizedAbsPath string) bool {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	_, ok := dw.files[normalizedAbsPath]
	return ok
}

func (dw *editorDirWatch) bumpEmitIfAllowed(normalizedAbsPath string, minInterval time.Duration) bool {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	now := time.Now()
	last := dw.lastEmit[normalizedAbsPath]
	if !last.IsZero() && now.Sub(last) < minInterval {
		return false
	}
	dw.lastEmit[normalizedAbsPath] = now
	return true
}

func (dw *editorDirWatch) addFile(normalizedAbsPath string) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	dw.files[normalizedAbsPath] = dw.files[normalizedAbsPath] + 1
}

func (dw *editorDirWatch) removeFile(normalizedAbsPath string) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	if dw.files == nil {
		return
	}
	cur := dw.files[normalizedAbsPath]
	if cur <= 1 {
		delete(dw.files, normalizedAbsPath)
		delete(dw.lastEmit, normalizedAbsPath)
		return
	}
	dw.files[normalizedAbsPath] = cur - 1
}

func (dw *editorDirWatch) isEmpty() bool {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	return len(dw.files) == 0
}

func (a *App) ensureEditorWatchInit() {
	a.editorWatchMu.Lock()
	defer a.editorWatchMu.Unlock()
	if a.editorDirWatches == nil {
		a.editorDirWatches = map[string]*editorDirWatch{}
	}
	if a.editorAssistedWriteByPath == nil {
		a.editorAssistedWriteByPath = map[string]editorAssistedWrite{}
	}
}

func (a *App) markEditorAssistedWrite(path string) func(bool) {
	a.ensureEditorWatchInit()
	norm, err := normalizeWatchPath(path)
	if err != nil {
		return nil
	}
	normDir, err := normalizeWatchPath(filepath.Dir(norm))
	if err != nil {
		return nil
	}
	a.editorWatchMu.Lock()
	dw := a.editorDirWatches[normDir]
	a.editorAssistedWriteSeq++
	token := a.editorAssistedWriteSeq
	a.editorWatchMu.Unlock()
	if dw == nil || !dw.isWatchingFile(norm) {
		return nil
	}
	a.editorWatchMu.Lock()
	currentWatch := a.editorDirWatches[normDir]
	if currentWatch == nil || !currentWatch.isWatchingFile(norm) {
		a.editorWatchMu.Unlock()
		return nil
	}
	a.editorAssistedWriteByPath[norm] = editorAssistedWrite{
		origin:    "assistant_tool",
		expiresAt: time.Now().Add(editorAssistedWriteTTL),
		token:     token,
	}
	a.editorWatchMu.Unlock()

	return func(committed bool) {
		if !committed {
			a.deleteEditorAssistedWriteIfToken(norm, token)
			return
		}
		info, err := os.Stat(norm)
		if err != nil || info.IsDir() {
			a.deleteEditorAssistedWriteIfToken(norm, token)
			return
		}
		a.editorWatchMu.Lock()
		defer a.editorWatchMu.Unlock()
		write, ok := a.editorAssistedWriteByPath[norm]
		if !ok || write.token != token || time.Now().After(write.expiresAt) {
			return
		}
		write.size = info.Size()
		write.modTime = info.ModTime()
		write.committed = true
		write.expiresAt = time.Now().Add(editorAssistedWriteTTL)
		a.editorAssistedWriteByPath[norm] = write
	}
}

func (a *App) deleteEditorAssistedWriteIfToken(normalizedAbsPath string, token int64) {
	a.ensureEditorWatchInit()
	a.editorWatchMu.Lock()
	defer a.editorWatchMu.Unlock()
	write, ok := a.editorAssistedWriteByPath[normalizedAbsPath]
	if ok && write.token == token {
		delete(a.editorAssistedWriteByPath, normalizedAbsPath)
	}
}

func (a *App) clearEditorAssistedWrite(normalizedAbsPath string) {
	a.ensureEditorWatchInit()
	a.editorWatchMu.Lock()
	defer a.editorWatchMu.Unlock()
	delete(a.editorAssistedWriteByPath, normalizedAbsPath)
}

func (a *App) clearExpiredEditorAssistedWritesLocked(now time.Time) {
	for path, write := range a.editorAssistedWriteByPath {
		if now.After(write.expiresAt) {
			delete(a.editorAssistedWriteByPath, path)
		}
	}
}

func (a *App) assistedWriteMatchesCurrentFile(normalizedAbsPath string, write editorAssistedWrite) bool {
	if !write.committed {
		return false
	}
	info, err := os.Stat(normalizedAbsPath)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() == write.size && info.ModTime().Equal(write.modTime)
}

func (a *App) consumeEditorAssistedWrite(normalizedAbsPath string) (string, bool) {
	deadline := time.Now().Add(editorAssistedWriteCommitWait)
	for {
		a.ensureEditorWatchInit()
		a.editorWatchMu.Lock()
		now := time.Now()
		a.clearExpiredEditorAssistedWritesLocked(now)
		write, ok := a.editorAssistedWriteByPath[normalizedAbsPath]
		if !ok {
			a.editorWatchMu.Unlock()
			return "", false
		}
		if write.committed {
			a.editorWatchMu.Unlock()
			matches := !time.Now().After(write.expiresAt) && a.assistedWriteMatchesCurrentFile(normalizedAbsPath, write)
			a.editorWatchMu.Lock()
			current, stillCurrent := a.editorAssistedWriteByPath[normalizedAbsPath]
			if stillCurrent && current.token == write.token {
				delete(a.editorAssistedWriteByPath, normalizedAbsPath)
			}
			a.editorWatchMu.Unlock()
			if !stillCurrent || current.token != write.token {
				return "", false
			}
			if !matches {
				return "", false
			}
			return write.origin, true
		}
		a.editorWatchMu.Unlock()
		if time.Now().After(deadline) {
			return "", false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (a *App) EditorWatchFile(path string) error {
	if a.ctx == nil {
		return fmt.Errorf("app não inicializado")
	}
	return a.editorWatchFile(path)
}

func (a *App) EditorUnwatchFile(path string) error {
	if a.ctx == nil {
		return fmt.Errorf("app não inicializado")
	}
	return a.editorUnwatchFile(path)
}

func (a *App) editorWatchFile(path string) error {
	a.ensureEditorWatchInit()

	normFile, err := normalizeWatchPath(path)
	if err != nil {
		return err
	}
	normDir, err := normalizeWatchPath(filepath.Dir(normFile))
	if err != nil {
		return err
	}

	a.editorWatchMu.Lock()
	dw := a.editorDirWatches[normDir]
	if dw == nil {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			a.editorWatchMu.Unlock()
			return fmt.Errorf("falha ao criar watcher: %w", err)
		}
		if err := watcher.Add(normDir); err != nil {
			_ = watcher.Close()
			a.editorWatchMu.Unlock()
			return fmt.Errorf("falha ao observar diretório: %w", err)
		}

		dw = &editorDirWatch{
			dir:      normDir,
			watcher:  watcher,
			done:     make(chan struct{}),
			files:    map[string]int{},
			lastEmit: map[string]time.Time{},
		}
		a.editorDirWatches[normDir] = dw

		go a.runEditorDirWatch(dw)
	}
	a.editorWatchMu.Unlock()

	dw.addFile(normFile)
	return nil
}

func (a *App) editorUnwatchFile(path string) error {
	a.ensureEditorWatchInit()

	normFile, err := normalizeWatchPath(path)
	if err != nil {
		return err
	}
	normDir, err := normalizeWatchPath(filepath.Dir(normFile))
	if err != nil {
		return err
	}

	a.editorWatchMu.Lock()
	dw := a.editorDirWatches[normDir]
	a.editorWatchMu.Unlock()
	if dw == nil {
		return nil
	}

	dw.removeFile(normFile)
	if !dw.isWatchingFile(normFile) {
		a.clearEditorAssistedWrite(normFile)
	}

	if !dw.isEmpty() {
		return nil
	}

	a.editorWatchMu.Lock()
	defer a.editorWatchMu.Unlock()
	// Re-check sob lock
	dw = a.editorDirWatches[normDir]
	if dw == nil || !dw.isEmpty() {
		return nil
	}
	delete(a.editorDirWatches, normDir)
	close(dw.done)
	_ = dw.watcher.Close()
	return nil
}

func (a *App) stopAllEditorWatches() {
	a.ensureEditorWatchInit()

	a.editorWatchMu.Lock()
	watches := a.editorDirWatches
	a.editorDirWatches = map[string]*editorDirWatch{}
	a.editorWatchMu.Unlock()

	for _, dw := range watches {
		select {
		case <-dw.done:
			// already closed
		default:
			close(dw.done)
		}
		_ = dw.watcher.Close()
	}
}

func (a *App) runEditorDirWatch(dw *editorDirWatch) {
	minInterval := 200 * time.Millisecond

	for {
		select {
		case <-dw.done:
			return
		case ev, ok := <-dw.watcher.Events:
			if !ok {
				return
			}

			normEv, err := normalizeWatchPath(ev.Name)
			if err != nil {
				continue
			}
			if !dw.isWatchingFile(normEv) {
				continue
			}
			if !dw.bumpEmitIfAllowed(normEv, minInterval) {
				continue
			}

			if a.ctx != nil {
				payload := map[string]any{
					"path": ev.Name,
					"op":   ev.Op.String(),
					"ts":   time.Now().UnixMilli(),
				}
				if origin, ok := a.consumeEditorAssistedWrite(normEv); ok {
					payload["origin"] = origin
					payload["assisted"] = true
				}
				a.emitter.Emit("editor:fileChanged", payload)
			}
		case <-dw.watcher.Errors:
			// best-effort: ignore
		}
	}
}
