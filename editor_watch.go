package main

import (
	"fmt"
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
				a.emitter.Emit( "editor:fileChanged", map[string]any{
					"path": ev.Name,
					"op":   ev.Op.String(),
					"ts":   time.Now().UnixMilli(),
				})
			}
		case <-dw.watcher.Errors:
			// best-effort: ignore
		}
	}
}
