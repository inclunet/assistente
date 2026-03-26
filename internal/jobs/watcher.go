package jobs

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatcherCallback e chamado quando um arquivo YAML e criado/modificado/removido.
type WatcherCallback struct {
	OnCreate func(path string, job *Job)
	OnUpdate func(path string, job *Job)
	OnRemove func(path string, jobID string)
}

// Watcher monitora o diretorio de jobs por mudancas em arquivos YAML.
type Watcher struct {
	dir       string
	watcher   *fsnotify.Watcher
	callback  WatcherCallback
	debounce  time.Duration
	stopCh    chan struct{}
	stopped   bool
	mu        sync.Mutex
	pending   map[string]time.Time // debounce por arquivo
}

// NewWatcher cria um watcher para o diretorio de jobs.
func NewWatcher(dir string, callback WatcherCallback) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		dir:      dir,
		watcher:  fw,
		callback: callback,
		debounce: 500 * time.Millisecond,
		stopCh:   make(chan struct{}),
		pending:  make(map[string]time.Time),
	}, nil
}

// Start comeca a monitorar o diretorio. Bloqueia ate Stop() ser chamado.
func (w *Watcher) Start() error {
	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return err
	}

	if err := w.watcher.Add(w.dir); err != nil {
		return err
	}

	log.Printf("[Jobs] Watcher started on %s", w.dir)

	debounceTicker := time.NewTicker(100 * time.Millisecond)
	defer debounceTicker.Stop()

	for {
		select {
		case <-w.stopCh:
			return nil

		case event, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}
			w.handleEvent(event)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("[Jobs] Watcher error: %v", err)

		case <-debounceTicker.C:
			w.processPending()
		}
	}
}

// Stop para o watcher.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stopped {
		return
	}

	w.stopped = true
	close(w.stopCh)
	w.watcher.Close()
	log.Printf("[Jobs] Watcher stopped")
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	if !isYAMLFile(event.Name) {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		// Remocao: processar imediatamente (sem debounce)
		jobID := jobIDFromPath(event.Name)
		if jobID != "" && w.callback.OnRemove != nil {
			log.Printf("[Jobs] Watcher: file removed %s", filepath.Base(event.Name))
			w.callback.OnRemove(event.Name, jobID)
		}
		delete(w.pending, event.Name)
		return
	}

	if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
		w.pending[event.Name] = time.Now()
	}
}

func (w *Watcher) processPending() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	for path, ts := range w.pending {
		if now.Sub(ts) < w.debounce {
			continue
		}

		delete(w.pending, path)

		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[Jobs] Watcher: cannot read %s: %v", filepath.Base(path), err)
			continue
		}

		job, err := Parse(data)
		if err != nil {
			log.Printf("[Jobs] Watcher: invalid YAML %s: %v", filepath.Base(path), err)
			continue
		}

		job.FilePath = path

		// Determinar se e create ou update pelo callback
		if w.callback.OnUpdate != nil {
			w.callback.OnUpdate(path, job)
		}
	}
}

func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// jobIDFromPath extrai o ID do job a partir do caminho do arquivo.
// Ex: "/path/to/fetch-jira-tickets.yaml" -> "fetch-jira-tickets"
func jobIDFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// LoadAllFromDir carrega todos os arquivos YAML de um diretorio.
// Retorna os jobs validos e erros dos invalidos.
func LoadAllFromDir(dir string) ([]*Job, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{err}
	}

	var jobs []*Job
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		// Ignora catalog.yaml (gerado automaticamente)
		if entry.Name() == "catalog.yaml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		job, err := Parse(data)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		job.FilePath = path
		jobs = append(jobs, job)
	}

	return jobs, errs
}
