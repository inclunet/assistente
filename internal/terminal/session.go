package terminal

import (
	"assistente/internal/logging"
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/KennethanCeyer/ptyx"
	"github.com/google/uuid"
)

// SessionState representa o estado atual de uma sessão PTY.
type SessionState int

const (
	// StateIdle indica que a sessão está livre para receber comandos.
	StateIdle SessionState = iota

	// StateBusy indica que a sessão está executando um comando.
	StateBusy

	// StateClosed indica que a sessão foi encerrada.
	StateClosed
)

// String retorna a representação textual do estado.
func (s SessionState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateBusy:
		return "busy"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// HistoryEntry representa uma execução de comando no histórico da sessão.
type HistoryEntry struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	Output    string    `json:"output"`
	ExitCode  int       `json:"exitCode"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
	Source    string    `json:"source"` // "user" ou "llm"
}

// SessionInfo contém informações públicas sobre uma sessão (para frontend).
type SessionInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CWD       string `json:"cwd"`
	State     string `json:"state"`
	Shell     string `json:"shell"`
	CreatedAt string `json:"createdAt"`
	LastUsed  string `json:"lastUsed"`
}

// outputCallback é chamado quando novo output é recebido (para streaming em tempo real).
// Usado pelo waitForMarker para emitir output filtrado (sem echo/markers) durante RunCommand.
type outputCallback func(sessionID, commandID, chunk string)

// rawOutputCallback é chamado para CADA chunk lido do PTY (output bruto, com ANSI limpo).
// Usado para streaming contínuo no Terminal Page (modo raw).
type rawOutputCallback func(sessionID, chunk string)

// Session encapsula uma sessão PTY persistente com um shell.
type Session struct {
	id         string
	name       string
	ptySession ptyx.Session
	state      SessionState
	shell      string
	cwd        string
	mu         sync.Mutex
	history    []HistoryEntry
	createdAt  time.Time
	lastUsed   time.Time

	// outputBuf acumula todo o output do PTY em background (para RunCommand com markers)
	outputBuf bytes.Buffer
	outputMu  sync.Mutex

	// onOutput é chamado com chunks de output filtrado durante RunCommand (LLM)
	onOutput outputCallback

	// onRawOutput é chamado para cada chunk lido do PTY (para Terminal Page)
	onRawOutput rawOutputCallback

	// suppressRawOutput quando true, readLoop não emite via onRawOutput (durante RunCommand)
	suppressRawOutput bool

	// cancelReader para cancelar o goroutine de leitura
	cancelReader context.CancelFunc
}

const (
	// maxHistoryEntries é o número máximo de entradas no histórico por sessão
	maxHistoryEntries = 200

	// maxOutputSize é o tamanho máximo de output por comando (50KB)
	maxOutputSize = 50 * 1024

	// defaultCols é o número padrão de colunas do terminal
	defaultCols = 120

	// defaultRows é o número padrão de linhas do terminal
	defaultRows = 40
)

// defaultShell retorna o shell padrão para o SO atual.
func defaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell.exe"
	}
	return "bash"
}

// shellType retorna o tipo do shell normalizado para uso em markers.
func shellType(shell string) string {
	switch shell {
	case "powershell.exe", "powershell", "pwsh.exe", "pwsh":
		return "powershell"
	default:
		return "bash"
	}
}

// newSession cria e inicializa uma nova sessão PTY.
func newSession(name, workDir, shell string, onOutput outputCallback, onRawOutput rawOutputCallback) (*Session, error) {
	if shell == "" {
		shell = defaultShell()
	}

	ctx, cancel := context.WithCancel(context.Background())

	opts := ptyx.SpawnOpts{
		Prog: shell,
		Dir:  workDir,
		Cols: defaultCols,
		Rows: defaultRows,
	}

	ptySession, err := ptyx.Spawn(ctx, opts)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("falha ao criar sessão PTY (%s): %w", shell, err)
	}

	s := &Session{
		id:           uuid.New().String()[:8],
		name:         name,
		ptySession:   ptySession,
		state:        StateIdle,
		shell:        shell,
		cwd:          workDir,
		history:      make([]HistoryEntry, 0, 32),
		createdAt:    time.Now(),
		lastUsed:     time.Now(),
		onOutput:     onOutput,
		onRawOutput:  onRawOutput,
		cancelReader: cancel,
	}

	// Goroutine para ler output do PTY em background
	go s.readLoop(ctx)

	logging.Infof(context.Background(), "terminal.session", "[Terminal] Sessão criada: id=%s name=%s shell=%s cwd=%s", s.id, s.name, s.shell, s.cwd)
	return s, nil
}

// readLoop lê continuamente do PTY e acumula no buffer.
// Também emite raw output para o frontend (quando não suprimido por RunCommand).
func (s *Session) readLoop(ctx context.Context) {
	buf := make([]byte, 4096)
	reader := s.ptySession.PtyReader()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])

			// Acumula no buffer (para RunCommand com markers)
			s.outputMu.Lock()
			s.outputBuf.WriteString(chunk)
			suppress := s.suppressRawOutput
			s.outputMu.Unlock()

			// Emite raw output para Terminal Page (quando não está em RunCommand/marker mode)
			if !suppress && s.onRawOutput != nil {
				// Limpa ANSI e normaliza line endings para exibição
				cleaned := StripANSI(chunk)
				cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
				cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
				if cleaned != "" {
					s.onRawOutput(s.id, cleaned)
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				logging.Errorf(ctx, "terminal.session", "[Terminal] Erro de leitura na sessão %s: %v", s.id, err)
			}
			return
		}
	}
}

// RunCommand executa um comando na sessão PTY e retorna o output.
// O comando é envolvido com markers para detectar início/fim e exit code.
func (s *Session) RunCommand(ctx context.Context, command string, timeout time.Duration, source string) (*HistoryEntry, error) {
	s.mu.Lock()
	if s.state == StateClosed {
		s.mu.Unlock()
		return nil, fmt.Errorf("sessão %s está fechada", s.id)
	}
	if s.state == StateBusy {
		s.mu.Unlock()
		return nil, fmt.Errorf("sessão %s está ocupada", s.id)
	}
	s.state = StateBusy
	s.lastUsed = time.Now()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.state != StateClosed {
			s.state = StateIdle
		}
		s.mu.Unlock()
	}()

	entry := &HistoryEntry{
		ID:        uuid.New().String()[:8],
		Command:   command,
		StartedAt: time.Now(),
		Source:    source,
	}

	// Cria marker único para esta execução
	marker := NewCommandMarker()

	// Suprime raw output durante execução com markers (para não poluir o Terminal Page)
	s.outputMu.Lock()
	s.outputBuf.Reset()
	s.suppressRawOutput = true
	s.outputMu.Unlock()

	defer func() {
		s.outputMu.Lock()
		s.suppressRawOutput = false
		s.outputMu.Unlock()
	}()

	// Envia o comando wrapped com markers
	wrappedCmd := marker.WrapCommand(command, shellType(s.shell))
	// Windows ConPTY espera CR (\r) para simular Enter.
	// Unix PTY espera LF (\n) para executar o comando.
	enter := "\n"
	if runtime.GOOS == "windows" {
		enter = "\r"
	}
	logging.Errorf(ctx, "terminal.session", "[Terminal] RunCommand session=%s os=%s enter=%q cmdLen=%d shell=%s",
		s.id, runtime.GOOS, enter, len(wrappedCmd), s.shell)
	nWritten, err := s.ptySession.PtyWriter().Write([]byte(wrappedCmd + enter))
	if err != nil {
		return nil, fmt.Errorf("falha ao enviar comando para sessão %s: %w", s.id, err)
	}
	logging.Warnf(ctx, "terminal.session", "[Terminal] Write OK: %d bytes escritos para sessão %s", nWritten, s.id)

	// Aguarda o end marker aparecer no output (com timeout)
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, exitCode, err := s.waitForMarker(timeoutCtx, marker, entry.ID)
	entry.EndedAt = time.Now()

	if err != nil {
		entry.Output = output // output parcial
		entry.ExitCode = -1
		s.addHistoryEntry(entry)
		return entry, err
	}

	entry.Output = output
	entry.ExitCode = exitCode
	s.addHistoryEntry(entry)

	return entry, nil
}

// waitForMarker espera até que o end marker apareça no output do PTY.
// Emite chunks de output limpo via onOutput callback para streaming em tempo real,
// filtrando echo do comando e markers.
func (s *Session) waitForMarker(ctx context.Context, marker *CommandMarker, commandID string) (string, int, error) {
	ticker := time.NewTicker(50 * time.Millisecond) // polling a cada 50ms
	defer ticker.Stop()

	// streamSentLen rastreia quanto do output "real" (entre markers) já foi enviado via callback.
	streamSentLen := 0
	// startFound indica se já encontramos o start marker (para começar a emitir output).
	startFound := false

	for {
		select {
		case <-ctx.Done():
			// Timeout ou cancelamento — interrompe o comando e retorna output parcial
			s.outputMu.Lock()
			raw := s.outputBuf.String()
			s.outputMu.Unlock()
			cleaned := StripANSI(raw)
			cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")

			logging.Errorf(ctx, "terminal.session", "[Terminal] TIMEOUT session=%s bufLen=%d", s.id, len(raw))

			// Envia Ctrl+C para interromper o comando travado e liberar o shell
			if _, writeErr := s.ptySession.PtyWriter().Write([]byte{0x03}); writeErr != nil {
				logging.Errorf(ctx, "terminal.session", "[Terminal] Erro ao enviar Ctrl+C após timeout: %v", writeErr)
			} else {
				logging.Warnf(ctx, "terminal.session", "[Terminal] Ctrl+C enviado após timeout na sessão %s", s.id)
			}

			// Extrai output útil (entre start marker e o fim, se houver start marker)
			output := cleaned
			if idx := strings.Index(cleaned, "\n"+marker.StartTag()); idx != -1 {
				contentStart := idx + 1 + len(marker.StartTag())
				if contentStart < len(cleaned) && cleaned[contentStart] == '\n' {
					contentStart++
				}
				output = strings.TrimSpace(cleaned[contentStart:])
			} else if strings.HasPrefix(cleaned, marker.StartTag()) {
				contentStart := len(marker.StartTag())
				if contentStart < len(cleaned) && cleaned[contentStart] == '\n' {
					contentStart++
				}
				output = strings.TrimSpace(cleaned[contentStart:])
			}

			return output, -1, fmt.Errorf("timeout aguardando fim do comando")

		case <-ticker.C:
			s.outputMu.Lock()
			raw := s.outputBuf.String()
			s.outputMu.Unlock()

			// Tenta parsear markers (detecta conclusão)
			result := marker.ParseOutput(raw)
			if result.Found {
				// Emite o trecho final de output que ainda não foi enviado
				if s.onOutput != nil && len(result.Output) > streamSentLen {
					s.onOutput(s.id, commandID, result.Output[streamSentLen:])
				}
				return result.Output, result.ExitCode, nil
			}

			// Se ainda não encontrou o end marker, tenta emitir output parcial
			// entre o start marker e o final do buffer atual (sem incluir echo/markers).
			if s.onOutput != nil {
				cleaned := StripANSI(raw)
				cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")

				if !startFound {
					// Procura start marker no início de linha
					nlStart := strings.Index(cleaned, "\n"+marker.StartTag())
					if nlStart != -1 {
						startFound = true
					} else if strings.HasPrefix(cleaned, marker.StartTag()) {
						startFound = true
					}
				}

				if startFound {
					// Extrai output parcial: tudo após (start marker + \n) até o fim do buffer
					startTag := marker.StartTag()
					idx := strings.Index(cleaned, "\n"+startTag)
					contentStart := 0
					if idx != -1 {
						contentStart = idx + 1 + len(startTag)
					} else if strings.HasPrefix(cleaned, startTag) {
						contentStart = len(startTag)
					}
					if contentStart < len(cleaned) && cleaned[contentStart] == '\n' {
						contentStart++
					}

					// O output parcial é do contentStart até o fim (excluindo linhas com end marker)
					partial := cleaned[contentStart:]
					// Remove linhas que contêm o end marker (se parcialmente recebido)
					if endIdx := strings.Index(partial, marker.EndTag()); endIdx >= 0 {
						partial = partial[:endIdx]
					}
					partial = strings.TrimRight(partial, "\n")

					if len(partial) > streamSentLen {
						s.onOutput(s.id, commandID, partial[streamSentLen:])
						streamSentLen = len(partial)
					}
				}
			}
		}
	}
}

// addHistoryEntry adiciona uma entrada ao histórico, respeitando o limite.
func (s *Session) addHistoryEntry(entry *HistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Trunca output se necessário
	if len(entry.Output) > maxOutputSize {
		entry.Output = entry.Output[:maxOutputSize] + fmt.Sprintf(
			"\n\n[TRUNCADO: output original tinha %d bytes]", len(entry.Output),
		)
	}

	s.history = append(s.history, *entry)

	// Mantém apenas as últimas N entradas
	if len(s.history) > maxHistoryEntries {
		s.history = s.history[len(s.history)-maxHistoryEntries:]
	}
}

// GetHistory retorna uma cópia do histórico de comandos.
func (s *Session) GetHistory() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]HistoryEntry, len(s.history))
	copy(result, s.history)
	return result
}

// Info retorna informações públicas da sessão.
func (s *Session) Info() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	return SessionInfo{
		ID:        s.id,
		Name:      s.name,
		CWD:       s.cwd,
		State:     s.state.String(),
		Shell:     s.shell,
		CreatedAt: s.createdAt.Format(time.RFC3339),
		LastUsed:  s.lastUsed.Format(time.RFC3339),
	}
}

// ID retorna o identificador da sessão.
func (s *Session) ID() string {
	return s.id
}

// State retorna o estado atual da sessão.
func (s *Session) State() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// SendInput envia input raw para o PTY sem markers.
// Usado para comandos do usuário no Terminal Page e para input de programas interativos.
// Não bloqueia — o output vem via streaming (onRawOutput).
func (s *Session) SendInput(input string) (*HistoryEntry, error) {
	s.mu.Lock()
	if s.state == StateClosed {
		s.mu.Unlock()
		return nil, fmt.Errorf("sessão %s está fechada", s.id)
	}
	s.lastUsed = time.Now()
	s.mu.Unlock()

	// Windows ConPTY espera CR (\r) para simular Enter.
	// Unix PTY espera LF (\n) para executar o comando.
	enter := "\n"
	if runtime.GOOS == "windows" {
		enter = "\r"
	}

	_, err := s.ptySession.PtyWriter().Write([]byte(input + enter))
	if err != nil {
		return nil, fmt.Errorf("falha ao enviar input para sessão %s: %w", s.id, err)
	}

	entry := &HistoryEntry{
		ID:        uuid.New().String()[:8],
		Command:   input,
		StartedAt: time.Now(),
		ExitCode:  -999, // sentinel: "raw/interativo, sem exit code"
		Source:    "user-raw",
	}
	s.addHistoryEntry(entry)

	return entry, nil
}

// Interrupt envia Ctrl+C (byte 0x03) ao PTY para interromper o processo em execução.
func (s *Session) Interrupt() error {
	s.mu.Lock()
	if s.state == StateClosed {
		s.mu.Unlock()
		return fmt.Errorf("sessão %s está fechada", s.id)
	}
	s.mu.Unlock()

	_, err := s.ptySession.PtyWriter().Write([]byte{0x03}) // Ctrl+C = ETX
	if err != nil {
		return fmt.Errorf("falha ao enviar Ctrl+C para sessão %s: %w", s.id, err)
	}
	logging.Errorf(context.Background(), "terminal.session", "[Terminal] Ctrl+C enviado para sessão %s", s.id)
	return nil
}

// Close encerra a sessão PTY e libera recursos.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateClosed {
		return nil
	}

	s.state = StateClosed

	// Para o goroutine de leitura
	if s.cancelReader != nil {
		s.cancelReader()
	}

	// Encerra a sessão PTY
	if s.ptySession != nil {
		_ = s.ptySession.Kill()
		_ = s.ptySession.Close()
	}

	logging.Infof(context.Background(), "terminal.session", "[Terminal] Sessão encerrada: id=%s name=%s", s.id, s.name)
	return nil
}
