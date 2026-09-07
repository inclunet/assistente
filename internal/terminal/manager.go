package terminal

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
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

// Manager gerencia sessões PTY independentes compartilháveis entre chat e UI.
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
			"sessionId":  sessionID,
			"terminalId": sessionID,
			"commandId":  commandID,
			"output":     chunk,
		})
	}

	// Callback para raw output (streaming contínuo para Terminal Page)
	onRawOutput := func(sessionID, chunk string) {
		m.emitEvent("terminal:raw_output", map[string]string{
			"sessionId":  sessionID,
			"terminalId": sessionID,
			"output":     chunk,
		})
	}
	onCommandStart := func(sessionID, commandID, command, source string) {
		m.emitEvent("terminal:command_start", map[string]any{
			"sessionId":  sessionID,
			"terminalId": sessionID,
			"commandId":  commandID,
			"command":    command,
			"source":     source,
		})
	}
	onExit := func(sessionID string, exitErr error) {
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		m.emitEvent("terminal:exited", map[string]any{
			"sessionId":  sessionID,
			"terminalId": sessionID,
			"error":      errToString(exitErr),
		})
	}

	session, err := newSession(name, workDir, m.config.DefaultShell, onOutput, onRawOutput, onCommandStart, onExit)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[session.id] = session
	m.mu.Unlock()
	session.Start()

	// Emite evento de criação
	m.emitEvent("terminal:session_created", session.Info())

	return session, nil
}

// CreateInfo cria uma sessão e retorna somente seu contrato público.
func (m *Manager) CreateInfo(name, workDir string) (SessionInfo, error) {
	session, err := m.Create(name, workDir)
	if err != nil {
		return SessionInfo{}, err
	}
	return session.Info(), nil
}

// Acquire cria uma sessão nova.
//
// Mantido temporariamente para compatibilidade com consumidores antigos. A
// AEP-0089 proíbe adquirir implicitamente uma sessão idle global, pois isso
// permitiria ao chat capturar um terminal manual sem informar o usuário.
func (m *Manager) Acquire(ctx context.Context, workDir string) (*Session, error) {
	logging.Infof(ctx, "terminal.manager", "[Terminal] Criando sessão explícita para comando cwd=%s", workDir)
	return m.Create("", workDir)
}

// RunEphemeral executa um comando sem criar seção persistente visível.
// Não adiciona a sessão ao pool nem emite session_created/closed — evita
// flicker no Terminal e não ocupa o limite de sessões.
func (m *Manager) RunEphemeral(ctx context.Context, workDir, command string, timeout time.Duration, source string) (*HistoryEntry, error) {
	if timeout == 0 {
		timeout = m.config.DefaultTimeout
	}
	session, err := newSession("", workDir, m.config.DefaultShell,
		func(_, _, _ string) {},
		func(_, _ string) {},
		func(_, _, _, _ string) {},
		func(string, error) {},
	)
	if err != nil {
		return nil, err
	}
	session.Start()
	defer func() {
		if err := session.Close(); err != nil {
			logging.Warnf(ctx, "terminal.manager", "[Terminal] falha ao fechar sessão efêmera: %v", err)
		}
	}()

	commandID := uuid.NewString()
	entry := &HistoryEntry{ID: commandID, Command: command, Source: source, StartedAt: time.Now()}
	if err := session.beginCommand(); err != nil {
		return nil, err
	}
	result, err := session.RunCommand(ctx, command, timeout, source, commandID)
	return completeCommandEntry(entry, result, err), err
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

	commandID := uuid.NewString()
	entry := &HistoryEntry{
		ID:        commandID,
		Command:   command,
		Source:    source,
		StartedAt: time.Now(),
	}
	if err := session.beginCommand(); err != nil {
		return nil, err
	}

	// Executa o comando
	result, err := session.RunCommand(ctx, command, timeout, source, commandID)
	entry = completeCommandEntry(entry, result, err)

	// Emite evento de fim
	m.emitEvent("terminal:command_end", map[string]any{
		"sessionId":  sessionID,
		"terminalId": sessionID,
		"commandId":  entry.ID,
		"output":     entry.Output,
		"exitCode":   entry.ExitCode,
		"error":      errToString(err),
	})

	return entry, err
}

func completeCommandEntry(entry, result *HistoryEntry, err error) *HistoryEntry {
	if result != nil {
		return result
	}
	if err != nil {
		entry.ExitCode = -1
		entry.EndedAt = time.Now()
	}
	return entry
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

	commandID := uuid.NewString()
	entry, err := session.SendInput(input, commandID)
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

// Has informa se o ID ainda identifica uma sessão viva.
func (m *Manager) Has(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		return false
	}
	state := session.State()
	return state != StateClosing && state != StateExited
}

// Info retorna o contrato público de uma sessão viva.
func (m *Manager) Info(sessionID string) (SessionInfo, bool) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return SessionInfo{}, false
	}
	state := session.State()
	if state == StateClosing || state == StateExited {
		return SessionInfo{}, false
	}
	return session.Info(), true
}

// List retorna informações de todas as sessões ativas.
func (m *Manager) List() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		state := s.State()
		if state != StateClosing && state != StateExited {
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
			logging.Errorf(context.Background(), "terminal.manager", "[Terminal] Erro ao fechar sessão %s: %v", s.id, err)
		}
	}

	logging.Infof(context.Background(), "terminal.manager", "[Terminal] Todas as %d sessões encerradas", len(sessions))
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
