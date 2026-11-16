package logging

import (
	"assistente/internal/commandinstance"
	"assistente/internal/configdir"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Settings é a configuração persistida do log do aplicativo para este usuário.
type Settings struct {
	Enabled     bool   `json:"enabled"`
	Path        string `json:"path"`
	MaxMiB      int    `json:"maxMiB"`
	Backups     int    `json:"backups"`
	RotateHours int    `json:"rotateHours"`
}

func DefaultSettings() Settings {
	path := ""
	if dir := configdir.GetHomeDir(); dir != "" {
		path = filepath.Join(dir, "assistente.log")
	}
	return Settings{Path: path, MaxMiB: 50, Backups: 5, RotateHours: 24}
}

func (s Settings) validate() error {
	if s.MaxMiB < 1 || s.MaxMiB > 10240 || s.Backups < 1 || s.Backups > 100 || s.RotateHours < 0 || s.RotateHours > 8760 {
		return errors.New("limite ou retenção de logs inválidos")
	}
	if s.Enabled && (strings.TrimSpace(s.Path) == "" || !filepath.IsAbs(s.Path)) {
		return errors.New("o arquivo de log deve ter caminho absoluto")
	}
	return nil
}

// Manager é um destino estável, compartilhado por slog e GORM, que permite
// trocar a configuração durante a execução sem reconstruir seus loggers.
type Manager struct {
	mu                     sync.Mutex
	config                 string
	settings               Settings
	sink                   io.WriteCloser
	override               string
	loadErr                error
	writeErr               error
	writeHealthyAfterError bool
}

func NewManager(configPath, override string) (*Manager, error) {
	m := &Manager{config: configPath, settings: DefaultSettings(), override: override}
	if override != "" && ConflictsWithSettings(configPath, override) {
		return nil, &FileOpenError{Path: override, Err: errors.New("o arquivo de log coincide com a configuração de logs")}
	}
	data, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		m.loadErr = err
	} else if err == nil {
		if err = json.Unmarshal(data, &m.settings); err != nil {
			m.loadErr = err
			m.settings = DefaultSettings()
		}
	}
	if err = m.settings.validate(); err != nil {
		m.loadErr = err
		m.settings = DefaultSettings()
	}
	if override != "" {
		m.sink, err = OpenExplicitLog(override)
		if err != nil {
			return nil, &FileOpenError{Path: override, Err: err}
		}
	} else if m.settings.Enabled {
		if ConflictsWithSettings(configPath, m.settings.Path) {
			m.loadErr = errors.New("o arquivo de log coincide com a configuração de logs")
			return m, nil
		}
		var writer *RotatingWriter
		writer, err = NewRotatingWriter(m.settings.Path, int64(m.settings.MaxMiB)*1024*1024, m.settings.Backups)
		if err != nil {
			m.loadErr = err
		} else {
			writer.configure(int64(m.settings.MaxMiB)*1024*1024, m.settings.Backups, time.Duration(m.settings.RotateHours)*time.Hour)
			m.sink = writer
		}
	}
	return m, nil
}

type explicitLog struct {
	file *os.File
	lock *commandinstance.Lock
}

func (e *explicitLog) Write(p []byte) (int, error) { return e.file.Write(p) }
func (e *explicitLog) Close() error {
	return errors.Join(e.file.Close(), e.lock.Close())
}

// OpenExplicitLog mantém a compatibilidade append da flag, mas não permite
// que ela grave simultaneamente no arquivo sob rotação de outra instância.
func OpenExplicitLog(path string) (io.WriteCloser, error) {
	lockArchive := func() (*commandinstance.Lock, error) {
		resolved := path
		if target, err := filepath.EvalSymlinks(path); err == nil {
			resolved = target
		}
		archive := resolved + ".assistente-archives"
		info, err := os.Lstat(archive)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errors.New("diretório de arquivos anteriores inválido")
		}
		marker := filepath.Join(archive, ".assistente-log")
		data, err := os.ReadFile(marker)
		if err != nil || !strings.HasPrefix(string(data), "assistente-log-v1\n") {
			return nil, errors.New("diretório de arquivos anteriores indisponível")
		}
		return commandinstance.Acquire(context.Background(), marker)
	}
	lock, err := lockArchive()
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	if err := requireSingleLogLink(file); err != nil {
		_ = file.Close()
		_ = lock.Close()
		return nil, err
	}
	if lock == nil {
		lock, err = lockArchive()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
	}
	return &explicitLog{file: file, lock: lock}, nil
}

func (m *Manager) Settings() (Settings, bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	problem := ""
	if m.loadErr != nil {
		problem = m.loadErr.Error()
	} else if m.writeErr != nil {
		problem = m.writeErr.Error()
	}
	return m.settings, m.override != "", problem
}

// Save verifica o destino antes de persistir e mantém o anterior em caso de erro.
func (m *Manager) Save(settings Settings) error {
	if err := settings.validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config == "" {
		return errors.New("diretório de configurações indisponível")
	}
	if settings.Enabled && ConflictsWithSettings(m.config, settings.Path) {
		return errors.New("o arquivo de log coincide com a configuração de logs")
	}
	var next io.WriteCloser
	current, samePath := m.sink.(*RotatingWriter)
	samePath = samePath && settings.Enabled && current.path == settings.Path && m.override == ""
	if samePath {
		if err := current.checkReady(); err != nil {
			return err
		}
		if m.writeErr != nil && !m.writeHealthyAfterError {
			return fmt.Errorf("gravação ainda não foi restabelecida: %w", m.writeErr)
		}
	}
	if m.override == "" && settings.Enabled && !samePath {
		var err error
		next, err = NewRotatingWriter(settings.Path, int64(settings.MaxMiB)*1024*1024, settings.Backups)
		if err != nil {
			return err
		}
		next.(*RotatingWriter).configure(int64(settings.MaxMiB)*1024*1024, settings.Backups, time.Duration(settings.RotateHours)*time.Hour)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err == nil {
		err = writeSettings(m.config, data)
	}
	if err != nil {
		if next != nil {
			_ = next.Close()
		}
		return err
	}
	m.settings = settings
	m.loadErr = nil
	if m.override == "" || m.writeHealthyAfterError {
		m.writeErr = nil
		m.writeHealthyAfterError = false
	}
	if samePath {
		current.configure(int64(settings.MaxMiB)*1024*1024, settings.Backups, time.Duration(settings.RotateHours)*time.Hour)
		return nil
	}
	if m.override == "" {
		old := m.sink
		m.sink = next
		if old != nil {
			return old.Close()
		}
	}
	return nil
}

// ConflictsWithSettings identifica caminhos iguais, aliases e hardlinks existentes.
func ConflictsWithSettings(configPath, logPath string) bool {
	if configPath == "" || logPath == "" {
		return false
	}
	if a, err := os.Stat(configPath); err == nil {
		if b, err := os.Stat(logPath); err == nil && os.SameFile(a, b) {
			return true
		}
	}
	canonical := func(path string) string {
		abs, err := filepath.Abs(path)
		if err != nil {
			return filepath.Clean(path)
		}
		if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
			abs = filepath.Join(dir, filepath.Base(abs))
		}
		return filepath.Clean(abs)
	}
	a, b := canonical(configPath), canonical(logPath)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func writeSettings(path string, data []byte) error {
	if path == "" {
		return errors.New("diretório de configurações indisponível")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".logging-settings-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("salvar configuração de log: %w", err)
	}
	return nil
}

func (m *Manager) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sink == nil {
		return len(p), nil
	}
	n, err := m.sink.Write(p)
	if err != nil {
		m.writeErr = err
		m.writeHealthyAfterError = false
	} else if m.writeErr != nil {
		m.writeHealthyAfterError = true
	}
	return n, err
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sink == nil {
		return nil
	}
	err := m.sink.Close()
	m.sink = nil
	return err
}
