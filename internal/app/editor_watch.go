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

// editorAssistedWriteCommitWait é quanto o watcher espera pelo commit de uma
// marcação pendente antes de tratar o evento como externo. Com margem sobre o
// throttle de emissão (200ms): um evento pode chegar enquanto a gravação da
// geração mais nova ainda não foi commitada (ex.: autosave logo após a tool).
const editorAssistedWriteCommitWait = 500 * time.Millisecond

// editorAssistedWriteMaxGenerations limita quantas marcações recentes são
// mantidas por path (escritas rápidas em sequência: tool + autosave + retry).
const editorAssistedWriteMaxGenerations = 8

// Origens de escrita registradas para o watcher do editor.
const (
	editorWriteOriginAssistantTool = "assistant_tool"
	editorWriteOriginEditorUI      = "editor_ui"
)

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
		a.editorAssistedWriteByPath = map[string][]editorAssistedWrite{}
	}
}

// markEditorAssistedWrite registra uma escrita feita por tool do assistente
// (edit_file/write_file). Mantida com esta assinatura porque é usada como
// observer em app_tool_registry.go.
func (a *App) markEditorAssistedWrite(path string) func(bool) {
	return a.markEditorWrite(path, editorWriteOriginAssistantTool)
}

// markEditorSelfWrite registra uma escrita feita pelo próprio editor
// (salvar/autosave via EditorWriteFile/EditorWriteDraft).
func (a *App) markEditorSelfWrite(path string) func(bool) {
	return a.markEditorWrite(path, editorWriteOriginEditorUI)
}

// markEditorWrite registra uma escrita iniciada pelo app no path informado e
// retorna uma função de commit: commit(true) grava size+mtime do arquivo
// recém-escrito na marcação; commit(false) cancela a marcação.
//
// Cada chamada cria uma GERAÇÃO independente por path (lista pequena com TTL):
// escritas rápidas em sequência (tool do assistente + autosave do editor, ou
// múltiplas tools) não sobrescrevem a marcação umas das outras, então um evento
// do watcher que chegue atrasado ainda casa com a geração correspondente.
func (a *App) markEditorWrite(path string, origin string) func(bool) {
	a.ensureEditorWatchInit()
	norm, err := normalizeWatchPath(path)
	if err != nil {
		return nil
	}
	a.editorWatchMu.Lock()
	a.clearExpiredEditorAssistedWritesLocked(time.Now())
	a.editorAssistedWriteSeq++
	token := a.editorAssistedWriteSeq
	writes := append(a.editorAssistedWriteByPath[norm], editorAssistedWrite{
		origin:    origin,
		expiresAt: time.Now().Add(editorAssistedWriteTTL),
		token:     token,
	})
	// Limita o número de gerações vivas por path (as mais antigas saem primeiro).
	if len(writes) > editorAssistedWriteMaxGenerations {
		writes = writes[len(writes)-editorAssistedWriteMaxGenerations:]
	}
	a.editorAssistedWriteByPath[norm] = writes
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
		for i, write := range a.editorAssistedWriteByPath[norm] {
			if write.token != token {
				continue
			}
			if time.Now().After(write.expiresAt) {
				return
			}
			write.size = info.Size()
			write.modTime = info.ModTime()
			write.committed = true
			write.expiresAt = time.Now().Add(editorAssistedWriteTTL)
			a.editorAssistedWriteByPath[norm][i] = write
			return
		}
	}
}

func (a *App) deleteEditorAssistedWriteIfToken(normalizedAbsPath string, token int64) {
	a.ensureEditorWatchInit()
	a.editorWatchMu.Lock()
	defer a.editorWatchMu.Unlock()
	writes := a.editorAssistedWriteByPath[normalizedAbsPath]
	for i, write := range writes {
		if write.token == token {
			a.setEditorAssistedWritesLocked(normalizedAbsPath, append(writes[:i:i], writes[i+1:]...))
			return
		}
	}
}

// setEditorAssistedWritesLocked grava a lista de gerações do path, removendo a
// chave quando vazia. Chamar com editorWatchMu já adquirido.
func (a *App) setEditorAssistedWritesLocked(normalizedAbsPath string, writes []editorAssistedWrite) {
	if len(writes) == 0 {
		delete(a.editorAssistedWriteByPath, normalizedAbsPath)
		return
	}
	a.editorAssistedWriteByPath[normalizedAbsPath] = writes
}

func (a *App) clearEditorAssistedWrite(normalizedAbsPath string) {
	a.ensureEditorWatchInit()
	a.editorWatchMu.Lock()
	defer a.editorWatchMu.Unlock()
	delete(a.editorAssistedWriteByPath, normalizedAbsPath)
}

func (a *App) clearExpiredEditorAssistedWritesLocked(now time.Time) {
	for path, writes := range a.editorAssistedWriteByPath {
		alive := writes[:0]
		for _, write := range writes {
			if !now.After(write.expiresAt) {
				alive = append(alive, write)
			}
		}
		a.setEditorAssistedWritesLocked(path, alive)
	}
}

func assistedWriteMatchesFileInfo(write editorAssistedWrite, info os.FileInfo) bool {
	return write.committed && info.Size() == write.size && info.ModTime().Equal(write.modTime)
}

// resolveEditorSelfWrite verifica se o evento do watcher para o path
// corresponde a uma escrita registrada pelo próprio app e retorna o origin.
//
// As marcações NÃO são consumidas no primeiro match: o Windows costuma emitir
// múltiplos eventos Write para uma única gravação e o throttle de 200ms não
// garante coalescência. Enquanto size+mtime do arquivo continuarem batendo com
// ALGUMA geração commitada não expirada, os eventos são atribuídos àquela
// escrita (a mais recente vence em caso de empate). Gerações commitadas que já
// não batem com o arquivo atual são invalidadas (foram sucedidas por outra
// escrita ou por mudança externa real). Se restarem gerações ainda não
// commitadas (gravação em andamento), o watcher espera pelo commit até o
// deadline antes de tratar o evento como externo.
func (a *App) resolveEditorSelfWrite(normalizedAbsPath string) (string, bool) {
	deadline := time.Now().Add(editorAssistedWriteCommitWait)
	for {
		a.ensureEditorWatchInit()

		info, statErr := os.Stat(normalizedAbsPath)
		fileStatOK := statErr == nil && !info.IsDir()

		a.editorWatchMu.Lock()
		a.clearExpiredEditorAssistedWritesLocked(time.Now())
		writes := a.editorAssistedWriteByPath[normalizedAbsPath]
		if len(writes) == 0 {
			a.editorWatchMu.Unlock()
			return "", false
		}

		hasPending := false
		matchedOrigin := ""
		matched := false
		alive := make([]editorAssistedWrite, 0, len(writes))
		// Itera da geração mais nova para a mais antiga: a escrita mais
		// recente é a dona do estado atual do arquivo em caso de empate.
		for i := len(writes) - 1; i >= 0; i-- {
			write := writes[i]
			if !write.committed {
				hasPending = true
				alive = append([]editorAssistedWrite{write}, alive...)
				continue
			}
			if fileStatOK && assistedWriteMatchesFileInfo(write, info) {
				if !matched {
					matchedOrigin = write.origin
					matched = true
				}
				alive = append([]editorAssistedWrite{write}, alive...)
				continue
			}
			// Geração commitada que não bate com o arquivo atual: invalidada
			// (sucedida por outra escrita ou por mudança externa real).
		}
		a.setEditorAssistedWritesLocked(normalizedAbsPath, alive)
		a.editorWatchMu.Unlock()

		if matched {
			return matchedOrigin, true
		}
		if !hasPending || time.Now().After(deadline) {
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
				if origin, ok := a.resolveEditorSelfWrite(normEv); ok {
					payload["origin"] = origin
					switch origin {
					case editorWriteOriginEditorUI:
						payload["selfWrite"] = true
					default:
						payload["assisted"] = true
					}
				}
				a.emitter.Emit("editor:fileChanged", payload)
			}
		case <-dw.watcher.Errors:
			// best-effort: ignore
		}
	}
}
