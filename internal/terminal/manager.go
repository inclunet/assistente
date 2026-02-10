package terminal

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	// DefaultMaxSessions é o número máximo de sessões simultâneas.
	DefaultMaxSessions = 10

	// DefaultCommandTimeout é o timeout padrão para execução de comandos.
	DefaultCommandTimeout = 30 * time.Second
)

// ManagerConfig contém a configuração do gerenciador de sessões.
type ManagerConfig struct {
	MaxSessions    int
	DefaultTimeout time.Duration
	DefaultShell   string
}

// DefaultManagerConfig retorna a configuração padrão.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		MaxSessions:    DefaultMaxSessions,
		DefaultTimeout: DefaultCommandTimeout,
		DefaultShell:   defaultShell(),
	}
}

// ManagerStats contém estatísticas do gerenciador.
type ManagerStats struct {
	TotalSessions int `json:"totalSessions"`
	IdleSessions  int `json:"idleSessions"`
	BusySessions  int `json:"busySessions"`
	MaxSessions   int `json:"maxSessions"`
}

// Manager gerencia o pool de sessões PTY compartilhado entre LLM e usuário.
type Manager struct {
	sessions  map[string]*Session
	mu        sync.RWMutex
	config    ManagerConfig
	emitEvent func(event string, data any) // callback para emitir eventos Wails
	nextNum   int                          // contador para nomes automáticos
}

// NewManager cria um novo gerenciador de sessões.
// emitEvent é o callback usado para emitir eventos Wails ao frontend.
func NewManager(config ManagerConfig, emitEvent func(event string, data any)) *Manager {
	if emitEvent == nil {
		emitEvent = func(string, any) {} // noop
	}

	return &Manager{
		sessions:  make(map[string]*Session),
		config:    config,
		emitEvent: emitEvent,
		nextNum:   1,
	}
}

// Create cria uma nova sessão PTY com nome e diretório de trabalho.
// Usada pelo usuário via UI para criar terminais manualmente.
func (m *Manager) Create(name, workDir string) (*Session, error) {
	m.mu.Lock()

	if len(m.sessions) >= m.config.MaxSessions {
		m.mu.Unlock()
		return nil, fmt.Errorf("limite de sessões atingido (%d/%d)", len(m.sessions), m.config.MaxSessions)
	}

	if name == "" {
		name = fmt.Sprintf("Terminal %d", m.nextNum)
		m.nextNum++
	}

	m.mu.Unlock()

	// Callback para output filtrado (durante RunCommand com markers)
	onOutput := func(sessionID, commandID, chunk string) {
		m.emitEvent("terminal:command_output", map[string]string{
			"sessionId": sessionID,
			"commandId": commandID,
			"output":    chunk,
		})
	}

	// Callback para raw output (streaming contínuo para Terminal Page)
	onRawOutput := func(sessionID, chunk string) {
		m.emitEvent("terminal:raw_output", map[string]string{
			"sessionId": sessionID,
			"output":    chunk,
		})
	}

	session, err := newSession(name, workDir, m.config.DefaultShell, onOutput, onRawOutput)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[session.id] = session
	m.mu.Unlock()

	// Emite evento de criação
	m.emitEvent("terminal:session_created", session.Info())

	return session, nil
}

// Acquire busca uma sessão idle ou cria uma nova.
// Usada pelo LLM (via run_command tool) para obter uma sessão automaticamente.
func (m *Manager) Acquire(ctx context.Context, workDir string) (*Session, error) {
	m.mu.RLock()

	// Procura sessão idle (preferencialmente com mesmo cwd)
	var bestSession *Session
	for _, s := range m.sessions {
		if s.State() == StateIdle {
			if s.cwd == workDir {
				bestSession = s
				break // match perfeito
			}
			if bestSession == nil {
				bestSession = s
			}
		}
	}

	m.mu.RUnlock()

	if bestSession != nil {
		log.Printf("[Terminal] Reutilizando sessão idle: id=%s cwd=%s", bestSession.id, bestSession.cwd)
		return bestSession, nil
	}

	// Nenhuma sessão idle — cria uma nova
	return m.Create("", workDir)
}

// Release marca uma sessão como idle após uso pelo LLM.
func (m *Manager) Release(sessionID string) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return
	}

	session.mu.Lock()
	if session.state == StateBusy {
		session.state = StateIdle
	}
	session.mu.Unlock()
}

// RunCommand executa um comando em uma sessão específica.
// Emite eventos Wails de início e fim para o frontend.
func (m *Manager) RunCommand(ctx context.Context, sessionID, command string, timeout time.Duration, source string) (*HistoryEntry, error) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("sessão '%s' não encontrada", sessionID)
	}

	if timeout == 0 {
		timeout = m.config.DefaultTimeout
	}

	// Emite evento de início
	entry := &HistoryEntry{
		Command:   command,
		Source:    source,
		StartedAt: time.Now(),
	}
	m.emitEvent("terminal:command_start", map[string]any{
		"sessionId": sessionID,
		"command":   command,
		"source":    source,
	})

	// Executa o comando
	result, err := session.RunCommand(ctx, command, timeout, source)

	if result != nil {
		entry = result
	}

	// Emite evento de fim
	m.emitEvent("terminal:command_end", map[string]any{
		"sessionId": sessionID,
		"commandId": entry.ID,
		"output":    entry.Output,
		"exitCode":  entry.ExitCode,
		"error":     errToString(err),
	})

	return result, err
}

// SendInput envia input raw para uma sessão PTY (modo interativo, sem markers).
// Usado para comandos do usuário no Terminal Page.
func (m *Manager) SendInput(sessionID, input string) (*HistoryEntry, error) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("sessão '%s' não encontrada", sessionID)
	}

	// Emite evento de início de comando
	m.emitEvent("terminal:command_start", map[string]any{
		"sessionId": sessionID,
		"command":   input,
		"source":    "user-raw",
	})

	entry, err := session.SendInput(input)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// Interrupt envia Ctrl+C para uma sessão PTY.
func (m *Manager) Interrupt(sessionID string) error {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("sessão '%s' não encontrada", sessionID)
	}

	return session.Interrupt()
}

// Get retorna uma sessão pelo ID.
func (m *Manager) Get(sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	return s, ok
}

// List retorna informações de todas as sessões ativas.
func (m *Manager) List() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		if s.State() != StateClosed {
			result = append(result, s.Info())
		}
	}
	return result
}

// Close encerra uma sessão específica e remove do pool.
func (m *Manager) Close(sessionID string) error {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("sessão '%s' não encontrada", sessionID)
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	err := session.Close()

	// Emite evento de fechamento
	m.emitEvent("terminal:session_closed", map[string]string{
		"sessionId": sessionID,
	})

	return err
}

// CloseAll encerra todas as sessões. Chamado no shutdown do app.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, s := range sessions {
		if err := s.Close(); err != nil {
			log.Printf("[Terminal] Erro ao fechar sessão %s: %v", s.id, err)
		}
	}

	log.Printf("[Terminal] Todas as %d sessões encerradas", len(sessions))
}

// Stats retorna estatísticas do gerenciador.
func (m *Manager) Stats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := ManagerStats{
		TotalSessions: len(m.sessions),
		MaxSessions:   m.config.MaxSessions,
	}

	for _, s := range m.sessions {
		switch s.State() {
		case StateIdle:
			stats.IdleSessions++
		case StateBusy:
			stats.BusySessions++
		}
	}

	return stats
}

// GetHistory retorna o histórico de comandos de uma sessão.
func (m *Manager) GetHistory(sessionID string) ([]HistoryEntry, error) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("sessão '%s' não encontrada", sessionID)
	}

	return session.GetHistory(), nil
}

// errToString converte um erro para string (ou vazio se nil).
func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
