package logging

import (
	"assistente/internal/configdir"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		m.sink, err = os.OpenFile(override, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, &FileOpenError{Path: override, Err: err}
		}
	} else if m.settings.Enabled {
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
