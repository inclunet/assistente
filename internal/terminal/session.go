package terminal

import (
	"assistente/internal/logging"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/KennethanCeyer/ptyx"
	"github.com/google/uuid"
)

// SessionState representa o estado atual de uma sessão PTY.
type SessionState int

const (
	// StateIdle indica que a sessão está livre para receber comandos.
	StateIdle SessionState = iota

	// StateRunning indica que a sessão está executando um comando estruturado.
	StateRunning

	// StateClosing indica encerramento solicitado, ainda liberando recursos.
	StateClosing

	// StateExited indica que o processo terminou e o ID não é reconectável.
	StateExited

	// Aliases legados para consumidores durante a migração da AEP-0089.
	StateBusy   = StateRunning
	StateClosed = StateExited
)

// String retorna a representação textual do estado.
func (s SessionState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StateClosing:
		return "closing"
	case StateExited:
		return "exited"
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

type commandStartCallback func(sessionID, commandID, command, source string)

// Session encapsula uma sessão PTY persistente com um shell.
type Session struct {
	id                string
	sessionVersion    uint64
	name              string
	ptySession        ptyx.Session
	ptyReader         io.Reader
	ptyWriter         io.Writer
	state             SessionState
	commandGeneration uint64
	// commandPending cobre a janela entre beginCommand e o write inicial.
	// Não é uma prova de PID nem do estado de um subprocesso natural.
	commandPending   bool
	managedCommandID string
	shell            string
	cwd              string
	mu               sync.Mutex
	ioMu             sync.Mutex
	history          []HistoryEntry
	createdAt        time.Time
	lastUsed         time.Time

	// outputBuf acumula todo o output do PTY em background (para RunCommand com markers)
	outputBuf bytes.Buffer
	outputMu  sync.Mutex

	// onOutput é chamado com chunks de output filtrado durante RunCommand (LLM)
	onOutput outputCallback

	// onRawOutput é chamado para cada chunk lido do PTY (para Terminal Page)
	onRawOutput    rawOutputCallback
	onCommandStart commandStartCallback

	// suppressRawOutput quando true, readLoop não emite via onRawOutput (durante RunCommand)
	suppressRawOutput bool

	// cancelReader para cancelar o goroutine de leitura
	cancelReader    context.CancelFunc
	readerCtx       context.Context
	readerDone      chan struct{}
	readerDoneOnce  sync.Once
	processDone     chan struct{}
	processWaitOnce sync.Once
	processErr      error
	closeDone       chan struct{}
	closeDoneOnce   sync.Once
	closeErr        error
	onExit          func(sessionID string, err error)
	exitOnce        sync.Once
	ptyCloseOnce    sync.Once
	explicitClose   bool
}

var nextSessionVersion atomic.Uint64

func newSessionVersion() uint64 {
	version := nextSessionVersion.Add(1)
	if version == 0 {
		version = nextSessionVersion.Add(1)
	}
	return version
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

	// readerDrainTimeout limita apenas o fallback de fechamento do descritor.
	// O caminho normal aguarda EOF depois que o processo já terminou.
	readerDrainTimeout = 2 * time.Second

	// interruptDrainTimeout dá ao Ctrl+C tempo para produzir o restante da
	// saída e, quando possível, o marker final antes do cleanup.
	interruptDrainTimeout = 500 * time.Millisecond
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
func newSession(name, workDir, shell string, onOutput outputCallback, onRawOutput rawOutputCallback, onCommandStart commandStartCallback, onExit func(string, error)) (*Session, error) {
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
	// ptyx fecha e zera os campos internos do ConPTY em uma goroutine quando o
	// processo termina. Capture os handles enquanto Spawn ainda os expõe, para
	// que o readLoop e os writes não concorram com esse fechamento interno.
	ptyReader := ptySession.PtyReader()
	ptyWriter := ptySession.PtyWriter()

	s := &Session{
		id:             uuid.NewString(),
		sessionVersion: newSessionVersion(),
		name:           name,
		ptySession:     ptySession,
		ptyReader:      ptyReader,
		ptyWriter:      ptyWriter,
		state:          StateIdle,
		shell:          shell,
		cwd:            workDir,
		history:        make([]HistoryEntry, 0, 32),
		createdAt:      time.Now(),
		lastUsed:       time.Now(),
		onOutput:       onOutput,
		onRawOutput:    onRawOutput,
		onCommandStart: onCommandStart,
		cancelReader:   cancel,
		readerCtx:      ctx,
		readerDone:     make(chan struct{}),
		processDone:    make(chan struct{}),
		onExit:         onExit,
	}

	logging.Infof(ctx, "terminal.session", "[Terminal] Sessão criada: id=%s name=%s shell=%s cwd=%s", s.id, s.name, s.shell, s.cwd)
	return s, nil
}

// Start inicia a leitura somente depois que o Manager registrou a sessão.
func (s *Session) Start() {
	s.ensureLifecycleChannels()
	s.ensurePTYIO()
	go s.readLoop(s.readerCtx)
}

func (s *Session) ensurePTYIO() {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	if s.ptySession == nil {
		return
	}
	if s.ptyReader == nil {
		s.ptyReader = s.ptySession.PtyReader()
	}
	if s.ptyWriter == nil {
		s.ptyWriter = s.ptySession.PtyWriter()
	}
}

// readLoop lê continuamente do PTY e acumula no buffer.
// Também emite raw output para o frontend (quando não suprimido por RunCommand).
func (s *Session) readLoop(ctx context.Context) {
	defer s.signalReaderDone()

	buf := make([]byte, 4096)
	reader := s.ptyReader

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
			if err != io.EOF && !s.expectedReaderClose(err) {
				logging.Errorf(ctx, "terminal.session", "[Terminal] Erro de leitura na sessão %s: %v", s.id, err)
			} else if err != io.EOF {
				logging.Infof(ctx, "terminal.session", "[Terminal] Leitor encerrado durante cleanup da sessão %s: %v", s.id, err)
			} else {
				err = nil
			}
			waitErr := s.waitProcess()
			s.closePTY(false)
			if err == nil {
				err = waitErr
			}
			s.markExited(err)
			return
		}
	}
}

func (s *Session) ensureLifecycleChannels() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readerDone == nil {
		s.readerDone = make(chan struct{})
	}
	if s.processDone == nil {
		s.processDone = make(chan struct{})
	}
	if s.closeDone == nil {
		s.closeDone = make(chan struct{})
	}
}

func (s *Session) signalReaderDone() {
	s.readerDoneOnce.Do(func() {
		s.ensureLifecycleChannels()
		close(s.readerDone)
	})
}

func (s *Session) expectedReaderClose(err error) bool {
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	return state == StateClosing || state == StateExited ||
		errors.Is(err, os.ErrClosed) ||
		strings.Contains(strings.ToLower(err.Error()), "file already closed")
}

// waitProcess é o único ponto que chama Session.Wait. Além de colher o
// processo no Unix, ele garante que o handle do processo ConPTY não seja
// fechado enquanto outra goroutine ainda o aguarda.
func (s *Session) waitProcess() error {
	s.ensureLifecycleChannels()
	s.processWaitOnce.Do(func() {
		if s.ptySession != nil {
			s.processErr = s.ptySession.Wait()
		}
		close(s.processDone)
	})
	<-s.processDone
	return s.processErr
}

func (s *Session) waitReaderDrain() {
	if s.ptySession == nil {
		return
	}
	s.ensureLifecycleChannels()
	select {
	case <-s.readerDone:
		return
	case <-time.After(readerDrainTimeout):
		// Defesa para implementações de PTY que não entregam EOF após o
		// processo terminar. Só então fechamos o descritor para desbloquear Read.
		s.closePTY(false)
		<-s.readerDone
	}
}

func (s *Session) closePTY(kill bool) {
	s.ptyCloseOnce.Do(func() {
		s.ioMu.Lock()
		defer s.ioMu.Unlock()
		if s.ptySession == nil {
			return
		}
		if kill {
			_ = s.ptySession.Kill()
		}
		_ = s.ptySession.Close()
	})
}

func (s *Session) markExited(err error) {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	s.exitOnce.Do(func() {
		s.mu.Lock()
		s.state = StateExited
		explicitClose := s.explicitClose
		s.mu.Unlock()
		if !explicitClose && s.onExit != nil {
			s.onExit(s.id, err)
		}
	})
}

func (s *Session) beginCommandWithID(commandID string) error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateIdle {
		return fmt.Errorf("sessão %s não está disponível (estado: %s)", s.id, s.state.String())
	}
	if commandID == "" {
		commandID = uuid.NewString()
	}
	s.state = StateRunning
	s.commandGeneration++
	s.commandPending = true
	s.managedCommandID = commandID
	s.lastUsed = time.Now()
	return nil
}

func (s *Session) finishCommand() {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateRunning {
		s.state = StateIdle
	}
	s.commandPending = false
}

// RunCommand executa um comando na sessão PTY e retorna o output.
// O comando é envolvido com markers para detectar início/fim e exit code.
func (s *Session) RunCommand(ctx context.Context, command string, timeout time.Duration, source, commandID string) (*HistoryEntry, error) {
	defer s.finishCommand()

	s.mu.Lock()
	if commandID == "" {
		commandID = s.managedCommandID
		if commandID == "" {
			commandID = uuid.NewString()
		}
	}
	s.managedCommandID = commandID
	s.mu.Unlock()
	entry := &HistoryEntry{
		ID:        commandID,
		Command:   command,
		StartedAt: time.Now(),
		Source:    source,
	}
	if s.onCommandStart != nil {
		s.onCommandStart(s.id, entry.ID, entry.Command, entry.Source)
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
	logging.Debugf(ctx, "terminal.session", "[Terminal] RunCommand session=%s os=%s enter=%q cmdLen=%d shell=%s",
		s.id, runtime.GOOS, enter, len(wrappedCmd), s.shell)
	s.ioMu.Lock()
	s.mu.Lock()
	canWrite := s.state == StateRunning
	s.mu.Unlock()
	if !canWrite {
		s.ioMu.Unlock()
		return nil, fmt.Errorf("sessão %s foi encerrada antes do início do comando", s.id)
	}
	if s.ptyWriter == nil {
		s.ioMu.Unlock()
		return nil, fmt.Errorf("sessão %s não possui writer PTY", s.id)
	}
	nWritten, err := s.ptyWriter.Write([]byte(wrappedCmd + enter))
	s.mu.Lock()
	s.commandPending = false
	s.mu.Unlock()
	s.ioMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("falha ao enviar comando para sessão %s: %w", s.id, err)
	}
	logging.Debugf(ctx, "terminal.session", "[Terminal] Write OK: %d bytes escritos para sessão %s", nWritten, s.id)

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
			// Timeout ou cancelamento: envia Ctrl+C e aguarda brevemente a
			// drenagem. RunEphemeral fará em seguida o cleanup completo
			// (Kill + Wait + EOF do leitor + Close), antes de retornar.
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				logging.Errorf(context.Background(), "terminal.session", "[Terminal] TIMEOUT session=%s", s.id)
			} else {
				logging.Infof(context.Background(), "terminal.session", "[Terminal] Comando cancelado na sessão %s", s.id)
			}
			if writeErr := s.Interrupt(); writeErr != nil {
				logging.Errorf(context.Background(), "terminal.session", "[Terminal] Erro ao enviar Ctrl+C após timeout: %v", writeErr)
			}

			raw := s.drainAfterInterrupt(marker)
			cleaned := StripANSI(raw)
			cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")

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

			return output, -1, fmt.Errorf("fim do comando não observado: %w", ctx.Err())

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

func (s *Session) drainAfterInterrupt(marker *CommandMarker) string {
	deadline := time.NewTimer(interruptDrainTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	snapshot := func() string {
		s.outputMu.Lock()
		defer s.outputMu.Unlock()
		return s.outputBuf.String()
	}
	for {
		raw := snapshot()
		if marker.ParseOutput(raw).Found {
			return raw
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return snapshot()
		}
	}
}

// addHistoryEntry adiciona uma entrada ao histórico, respeitando o limite.
func (s *Session) addHistoryEntry(entry *HistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// O histórico recebe uma cópia limitada; o chamador conserva o output bruto
	// para que a tool aplique seu contrato model-facing sem conteúdo já mutilado.
	historyEntry := limitedHistoryEntry(entry)

	s.history = append(s.history, historyEntry)

	// Mantém apenas as últimas N entradas
	if len(s.history) > maxHistoryEntries {
		s.history = s.history[len(s.history)-maxHistoryEntries:]
	}
}

func limitedHistoryEntry(entry *HistoryEntry) HistoryEntry {
	historyEntry := *entry
	if len(historyEntry.Output) > maxOutputSize {
		end := maxOutputSize
		for end > 0 && !utf8.RuneStart(historyEntry.Output[end]) {
			end--
		}
		historyEntry.Output = historyEntry.Output[:end] + fmt.Sprintf(
			"\n\n[TRUNCADO: output original tinha %d bytes]", len(historyEntry.Output),
		)
	}
	return historyEntry
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
func (s *Session) SendInput(input, commandID string) (*HistoryEntry, error) {
	// Windows ConPTY espera CR (\r) para simular Enter.
	// Unix PTY espera LF (\n) para executar o comando.
	enter := "\n"
	if runtime.GOOS == "windows" {
		enter = "\r"
	}

	if commandID == "" {
		commandID = uuid.NewString()
	}
	if err := s.beginCommandWithID(commandID); err != nil {
		return nil, err
	}
	defer s.finishCommand()
	entry := &HistoryEntry{
		ID:        commandID,
		Command:   input,
		StartedAt: time.Now(),
		ExitCode:  -999, // sentinel: "raw/interativo, sem exit code"
		Source:    "user-raw",
	}
	if s.onCommandStart != nil {
		s.onCommandStart(s.id, entry.ID, entry.Command, entry.Source)
	}
	s.ioMu.Lock()
	s.mu.Lock()
	canWrite := s.state == StateRunning
	s.mu.Unlock()
	if !canWrite {
		s.ioMu.Unlock()
		return nil, fmt.Errorf("sessão %s foi encerrada antes do envio do input", s.id)
	}
	if s.ptyWriter == nil {
		s.ioMu.Unlock()
		return nil, fmt.Errorf("sessão %s não possui writer PTY", s.id)
	}
	_, err := s.ptyWriter.Write([]byte(input + enter))
	s.mu.Lock()
	s.commandPending = false
	s.mu.Unlock()
	s.ioMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("falha ao enviar input para sessão %s: %w", s.id, err)
	}

	s.addHistoryEntry(entry)

	return entry, nil
}

// Interrupt envia Ctrl+C (byte 0x03) ao PTY para interromper o processo em execução.
func (s *Session) Interrupt() error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	s.mu.Lock()
	if s.state == StateClosing || s.state == StateExited {
		s.mu.Unlock()
		return fmt.Errorf("sessão %s está fechada", s.id)
	}
	s.mu.Unlock()

	if s.ptyWriter == nil {
		return fmt.Errorf("sessão %s não possui writer PTY", s.id)
	}
	if err := writeInterruptByte(s.ptyWriter, s.id); err != nil {
		return err
	}
	logging.Infof(context.Background(), "terminal.session", "[Terminal] Ctrl+C enviado para sessão %s", s.id)
	return nil
}

// Close encerra a sessão PTY e libera recursos.
func (s *Session) Close() error {
	s.ensureLifecycleChannels()
	s.ioMu.Lock()
	return s.closeWithIOLock()
}

// closeWithIOLock executa o cleanup assumindo que ioMu está reservado pelo
// chamador. Ele libera ioMu assim que Kill/estado closing terminam, antes de
// aguardar o processo e drenar o leitor.
func (s *Session) closeWithIOLock() error {
	s.mu.Lock()
	if s.state == StateClosing {
		closeDone := s.closeDone
		s.mu.Unlock()
		s.ioMu.Unlock()
		<-closeDone
		s.mu.Lock()
		err := s.closeErr
		s.mu.Unlock()
		return err
	}
	if s.state == StateExited {
		err := s.closeErr
		s.mu.Unlock()
		s.ioMu.Unlock()
		return err
	}

	s.state = StateClosing
	s.explicitClose = true
	cancelReader := s.cancelReader
	id, name := s.id, s.name
	s.mu.Unlock()

	// Primeiro encerra a árvore de processos. Kill no ptyx usa
	// TerminateProcess no Windows; Wait confirma a saída antes de liberar os
	// handles. O leitor permanece aberto nesse intervalo para drenar o ConPTY.
	var killErr error
	if s.ptySession != nil {
		killErr = s.ptySession.Kill()
	}
	s.ioMu.Unlock()

	waitErr := s.waitProcess()
	s.waitReaderDrain()
	if cancelReader != nil {
		// Só cancela depois do EOF: o mesmo contexto governa o readLoop, e
		// cancelá-lo antes descartaria bytes ainda bufferizados no ConPTY.
		cancelReader()
	}
	s.closePTY(false)

	s.markExited(nil)

	logging.Infof(context.Background(), "terminal.session", "[Terminal] Sessão encerrada: id=%s name=%s", id, name)
	var closeErr error
	if killErr != nil {
		closeErr = fmt.Errorf("falha ao encerrar processo da sessão %s: %w", id, killErr)
	} else if waitErr != nil && !isExpectedTermination(waitErr) {
		closeErr = fmt.Errorf("falha ao aguardar processo da sessão %s: %w", id, waitErr)
	}
	s.mu.Lock()
	s.closeErr = closeErr
	s.mu.Unlock()
	s.closeDoneOnce.Do(func() { close(s.closeDone) })
	return closeErr
}

// closeCapturedSession é usado somente por CloseOperation depois de
// PrepareClose reservar ioMu. A segunda comparação fecha a janela entre a
// captura e o efeito, inclusive contra saída natural da sessão.
func (s *Session) closeCapturedSession(token *closeSnapshotToken) error {
	s.mu.Lock()
	if !s.matchesCloseSnapshot(token) {
		s.mu.Unlock()
		s.ioMu.Unlock()
		return ErrCloseStale
	}
	s.mu.Unlock()
	return s.closeWithIOLock()
}

func isExpectedTermination(err error) bool {
	if err == nil {
		return true
	}
	// Kill produz ExitError tanto no ptyx Unix quanto no Windows. Depois de um
	// fechamento explícito esse status é esperado e não representa falha de
	// cleanup.
	var exitErr *ptyx.ExitError
	return errors.As(err, &exitErr)
}
