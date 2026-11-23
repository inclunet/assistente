package terminal

import (
	"assistente/internal/logging"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

// rawOutputCallback entrega texto da sessão manual após interpretar os controles do PTY.
type rawOutputCallback func(sessionID, commandID, chunk string)

type commandStartCallback func(sessionID, commandID, command, source string)

// Session encapsula uma sessão PTY persistente com um shell.
type Session struct {
	powerShellKey  string
	id             string
	sessionVersion uint64
	name           string
	ptySession     ptyx.Session
	ptyReader      io.Reader
	ptyWriter      io.Writer
	// Sob outputMu, largura e escape pendente ao fim do último bloco lido.
	viewportControl   terminalControl
	viewportColumns   int
	state             SessionState
	commandGeneration uint64
	commandReserved   bool // A chamada mantém a sessão até a entrega do resultado.
	// commandPending cobre a janela entre beginCommand e o write inicial.
	// Não é uma prova de PID nem do estado de um subprocesso natural.
	commandPending    bool
	lastDispatchedID  string // Última entrada enviada; uma reserva invalida snapshots pela geração.
	interruptIsManual bool
	shell             string
	cwd               string
	mu                sync.Mutex
	ioMu              sync.Mutex
	history           []HistoryEntry
	createdAt         time.Time
	lastUsed          time.Time

	// outputBuf contém apenas os bytes pendentes de um comando estruturado.
	outputBuf    bytes.Buffer
	outputMu     sync.Mutex
	rawOutput    rawTerminalOutput
	rawOwner     *rawDeliveryOwner
	rawCommandID string
	rawFinished  bool

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
	readerErr       error // Escrito sob mu antes de fechar readerDone; leitores aguardam o canal ou tomam mu.
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
	writeErr        error            // sob mu; identifica o dono do cleanup e a causa de escrita
	commandDelivery *commandDelivery // Sob mu: preserva o resultado antes dos eventos de encerramento.
	discardOutput   bool             // Sob outputMu: a entrada encerrou antes do cleanup do PTY.
}

type commandDelivery struct {
	interruptAccepted chan struct{} // Fechado sob mu quando a escrita de Ctrl+C é aceita.
	afterEnd          []func()      // Sob mu: eventos de encerramento pendentes.
	ended             bool
	err               error
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
	switch strings.ToLower(filepath.Base(shell)) {
	case "powershell.exe", "powershell", "pwsh.exe", "pwsh":
		return "powershell"
	case "zsh", "zsh.exe":
		return "zsh"
	default:
		return "bash"
	}
}

// newSession cria e inicializa uma nova sessão PTY.
func newSession(name, workDir, shell string, env []string, onOutput outputCallback, onRawOutput rawOutputCallback, onCommandStart commandStartCallback, onExit func(string, error)) (*Session, error) {
	if shell == "" {
		shell = defaultShell()
	}

	opts := ptyx.SpawnOpts{
		Prog: shell,

		Env:  env,
		Dir:  workDir,
		Cols: defaultCols,
		Rows: defaultRows,
	}

	key := ""
	if shellType(shell) == "powershell" {
		key = uuid.NewString()
		opts.Args = []string{"-NoExit", "-Command", powerShellSessionLibrary(key)}
	}
	return spawnSession(name, opts, key, onOutput, onRawOutput, onCommandStart, onExit)
}

func spawnSession(name string, opts ptyx.SpawnOpts, key string, onOutput outputCallback, onRawOutput rawOutputCallback, onCommandStart commandStartCallback, onExit func(string, error)) (*Session, error) {
	ctx, cancel := context.WithCancel(context.Background())
	ptySession, err := ptyx.Spawn(ctx, opts)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("falha ao criar sessão PTY (%s): %w", opts.Prog, err)
	}
	// ptyx fecha e zera os campos internos do ConPTY em uma goroutine quando o
	// processo termina. Capture os handles enquanto Spawn ainda os expõe, para
	// que o readLoop e os writes não concorram com esse fechamento interno.
	ptyReader := ptySession.PtyReader()
	ptyWriter := ptySession.PtyWriter()

	s := &Session{
		powerShellKey:   key,
		id:              uuid.NewString(),
		sessionVersion:  newSessionVersion(),
		name:            name,
		ptySession:      ptySession,
		ptyReader:       ptyReader,
		ptyWriter:       ptyWriter,
		viewportColumns: opts.Cols,
		state:           StateIdle,
		shell:           opts.Prog,
		cwd:             opts.Dir,
		history:         make([]HistoryEntry, 0, 32),
		createdAt:       time.Now(),
		lastUsed:        time.Now(),
		onOutput:        onOutput,
		onRawOutput:     onRawOutput,
		onCommandStart:  onCommandStart,
		cancelReader:    cancel,
		readerCtx:       ctx,
		readerDone:      make(chan struct{}),
		processDone:     make(chan struct{}),
		onExit:          onExit,
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

// A troca de comando apenas copia o estado fixo. O leitor interpreta cada
// bloco uma vez, sem copiar o buffer anterior nem rever o histórico sob o lock.
func (s *Session) recordOutput(chunk string) {
	s.outputMu.Lock()
	if s.discardOutput {
		s.outputMu.Unlock()
		return
	}
	if s.suppressRawOutput {
		s.outputBuf.WriteString(chunk)
	}
	if runtime.GOOS == "windows" && shellType(s.shell) == "powershell" {
		s.viewportControl.trackViewport(chunk, &s.viewportColumns)
	}
	var raw string
	if s.onRawOutput != nil {
		raw = s.rawOutput.append(chunk, s.suppressRawOutput)
	}
	s.deliverRawAndUnlock(raw)
}

// O leitor atribui o bloco ao dono sob outputMu e pode aguardar espaço ou
// executar o callback sem impedir a troca de comando.
func (s *Session) deliverRawAndUnlock(raw string) {
	if raw == "" {
		s.outputMu.Unlock()
		return
	}
	if s.rawOwner == nil {
		s.rawOwner = newRawDeliveryOwner(false, s.publishRaw)
	}
	owner := s.rawOwner
	owner.reserve()
	chunk := rawChunk{commandID: s.rawCommandID, text: raw}
	s.outputMu.Unlock()
	owner.submit(chunk)
}

func (s *Session) publishRaw(chunk rawChunk) {
	s.onRawOutput(s.id, chunk.commandID, chunk.text)
}

func (s *Session) finishRawOutput() {
	s.outputMu.Lock()
	s.rawFinished = true
	s.deliverRawAndUnlock(s.rawOutput.finish())
}

func (s *Session) startManagedOutput(ctx context.Context, entry *HistoryEntry) {
	s.outputMu.Lock()
	owner := s.rawOwner
	s.outputMu.Unlock()
	if owner != nil {
		owner.waitBeforeDispatch(ctx)
	}
	s.announceCommandStart(entry)
}

func (s *Session) announceCommandStart(entry *HistoryEntry) {
	if s.onCommandStart != nil {
		s.onCommandStart(s.id, entry.ID, entry.Command, entry.Source)
	}
}

func (s *Session) startManualOutput(entry *HistoryEntry) {
	s.outputMu.Lock()
	previousOwner := s.rawOwner
	previousID := s.rawCommandID
	s.rawCommandID = entry.ID
	owner := newRawDeliveryOwner(true, s.publishRaw)
	s.rawOwner = owner
	previous := s.rawOutput.changeOwner()
	s.outputMu.Unlock()

	if previousOwner != nil {
		previousOwner.wait()
	}
	if previous != "" {
		s.publishRaw(rawChunk{commandID: previousID, text: previous})
	}
	s.announceCommandStart(entry)
	owner.opened()
}

func (s *Session) beginMarkerCapture(marker *CommandMarker) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	marker.viewportColumns = s.viewportColumns
	marker.control = s.viewportControl
	s.outputBuf.Reset()
	s.suppressRawOutput = true
}

// readLoop lê continuamente do PTY e acumula no buffer.
// Também emite raw output para o frontend (quando não suprimido por RunCommand).
func (s *Session) readLoop(ctx context.Context) {
	defer s.signalReaderDone()

	buf := make([]byte, ptyReadSize)
	reader := s.ptyReader

	for {
		select {
		case <-ctx.Done():
			s.finishRawOutput()
			return
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])

			// Acumula no buffer (para RunCommand com markers)
			s.recordOutput(chunk)
		}
		if err != nil {
			s.finishRawOutput()
			if errors.Is(err, io.EOF) || runtime.GOOS == "linux" && errors.Is(err, syscall.EIO) {
				// O PTY nativo do Linux reporta EIO ao fechar o último slave. Nos
				// demais backends só EOF comprova esse fim sem consultar o processo.
				err = nil
			} else if s.expectedReaderClose(err) {
				logging.Infof(ctx, "terminal.session", "[Terminal] Leitor encerrado durante cleanup da sessão %s: %v", s.id, err)
				err = nil
			} else {
				logging.Errorf(ctx, "terminal.session", "[Terminal] Erro de leitura na sessão %s: %v", s.id, err)
				s.closeAfterReadFailure(err)
				return
			}
			waitErr := s.waitProcess()
			s.closePTY()
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
	state, writeClosing := s.state, s.writeErr != nil
	s.mu.Unlock()
	if state == StateClosing || state == StateExited {
		return !writeClosing || errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
	}
	return errors.Is(err, os.ErrClosed) && s.ptySession != nil && conPTYProcessExited(s.ptySession.Pid())
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
		// processo terminar. Pare as novas leituras e libere a entrega do bloco
		// já lido e da cauda do decodificador, mesmo se onCommandStart chamou Close.
		if s.cancelReader != nil {
			s.cancelReader()
			s.outputMu.Lock()
			if s.rawOwner != nil {
				s.rawOwner.finishReader()
			}
			s.outputMu.Unlock()
		}
		s.closePTY()
		<-s.readerDone
	}
}

func (s *Session) closePTY() {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	s.releasePTY()
}

func (s *Session) releasePTY() {
	s.ptyCloseOnce.Do(func() {
		if s.ptySession != nil {
			_ = s.ptySession.Close()
		}
	})
}

// A saída natural publica exited. Quando já existe cleanup, seu dono só
// muda o estado para exited depois de registrar a causa final do fechamento.
func (s *Session) markExited(err error) {
	s.ioMu.Lock()
	s.mu.Lock()
	if s.state == StateClosing || s.state == StateExited {
		s.mu.Unlock()
		s.ioMu.Unlock()
		return
	}
	s.state = StateExited
	s.mu.Unlock()
	s.ioMu.Unlock()
	if s.onExit != nil {
		s.afterCommandEnd(func() { s.exitOnce.Do(func() { s.deliverExit(err) }) })
	}
}

// A sessão já está encerrada. O emissor pode concluir o teardown sem bloquear
// a entrega do resultado do comando nem executar callbacks sob mu.
func (s *Session) afterCommandEnd(notify func()) {
	s.mu.Lock()
	if d := s.commandDelivery; d != nil && !d.ended {
		d.afterEnd = append(d.afterEnd, notify)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	notify()
}

func (s *Session) deliverExit(err error) {
	s.afterRawDelivery(func() { s.onExit(s.id, err) })
}

func (s *Session) afterRawDelivery(notify func()) {
	s.outputMu.Lock()
	owner := s.rawOwner
	s.outputMu.Unlock()
	if owner != nil {
		owner.afterDelivery(notify)
		return
	}
	notify()
}

func (s *Session) reserveCommand(delivery *commandDelivery) error {
	if s.faultOwnsCleanup() {
		return fmt.Errorf("sessão %s indisponível após falha no PTY", s.id)
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateIdle {
		return fmt.Errorf("sessão %s não está disponível (estado: %s)", s.id, s.state.String())
	}
	s.state = StateRunning
	s.commandDelivery = delivery
	s.commandReserved = true
	s.commandGeneration++
	s.commandPending = true
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
	s.commandReserved = false
	s.commandPending = false
}

func (s *Session) runCommandInDirectory(ctx context.Context, command, directory string, timeout time.Duration, source, commandID string, onEnd func(*HistoryEntry, error, string)) (resultEntry *HistoryEntry, resultErr error) {
	if commandID == "" {
		commandID = uuid.NewString()
	}
	delivery := &commandDelivery{interruptAccepted: make(chan struct{})}
	if err := s.reserveCommand(delivery); err != nil {
		return nil, err
	}
	defer func() {
		if onEnd != nil {
			s.mu.Lock()
			var activeManualID string
			if s.state == StateRunning && s.commandPending && s.interruptIsManual {
				activeManualID = s.lastDispatchedID
			}
			s.mu.Unlock()
			onEnd(resultEntry, resultErr, activeManualID)
		}
		s.mu.Lock()
		delivery.ended, delivery.err = true, resultErr
		pending := delivery.afterEnd
		delivery.afterEnd = nil
		if s.writeErr == nil && s.readerErr == nil {
			s.commandDelivery = nil
		}
		s.mu.Unlock()
		if !s.faultOwnsCleanup() {
			s.finishCommand()
		}
		for _, notify := range pending {
			notify()
		}
		s.notifyFaultExit()
	}()
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	entry := &HistoryEntry{
		ID:        commandID,
		Command:   command,
		StartedAt: time.Now(),
		Source:    source,
	}
	// Cria marker único para esta execução
	marker := newCommandMarkerForShell(shellType(s.shell))

	// Durante o anúncio a saída ainda é da entrada manual anterior.
	s.startManagedOutput(commandCtx, entry)

	defer func() {
		s.outputMu.Lock()
		if s.suppressRawOutput {
			// A saída posterior à moldura é da sessão. Blocos manuais que ainda
			// aguardam entrega já carregam o seu próprio ID.
			s.rawCommandID = ""
		}
		s.suppressRawOutput = false
		s.outputBuf = bytes.Buffer{}
		s.outputMu.Unlock()
	}()

	runFrame := func(frame string) error {
		if err := s.writeManagedFrame(commandCtx, marker, entry, frame); err != nil {
			entry.ExitCode = -1
			return err
		}
		var err error
		entry.Output, entry.ExitCode, err = s.waitForMarker(commandCtx, marker, entry.ID)
		return err
	}
	if s.powerShellKey != "" {
		marker.probeInterrupt = delivery.interruptAccepted
		resultErr = runFrame(marker.powerShellProbe(s.powerShellKey))
		if resultErr == nil && entry.ExitCode != 0 && entry.ExitCode != 1 {
			resultErr = fmt.Errorf("consulta da biblioteca PowerShell retornou status %d", entry.ExitCode)
		}
		if resultErr == nil {
			key := ""
			if entry.ExitCode == 0 {
				key = s.powerShellKey
			}
			marker = newCommandMarkerForShell("powershell")
			resultErr = runFrame(marker.wrapPowerShell(command, directory, key))
		}
	} else {
		resultErr = runFrame(marker.wrapCommandInDirectory(command, directory, shellType(s.shell)))
	}
	entry.EndedAt = time.Now()
	s.addHistoryEntry(entry)
	return entry, resultErr
}

func (s *Session) writeManagedFrame(ctx context.Context, marker *CommandMarker, entry *HistoryEntry, wrappedCmd string) error {
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
	if s.state != StateRunning {
		cause := s.readerErr
		if cause == nil && s.explicitClose {
			cause = ErrSessionClosed
		}
		s.mu.Unlock()
		s.ioMu.Unlock()
		if cause == nil {
			select {
			case <-s.processDone:
				cause = s.processErr
			default:
			}
		}
		if cause == nil {
			cause = fmt.Errorf("sessão %s foi encerrada antes do início do comando", s.id)
		}
		return cause
	}
	if s.commandDelivery != nil {
		select {
		case <-s.commandDelivery.interruptAccepted:
			s.mu.Unlock()
			s.ioMu.Unlock()
			return context.Canceled
		default:
		}
	}
	s.mu.Unlock()
	if s.ptyWriter == nil {
		s.ioMu.Unlock()
		return fmt.Errorf("sessão %s não possui writer PTY", s.id)
	}
	request := []byte(wrappedCmd + enter)
	if err := ctx.Err(); err != nil {
		s.ioMu.Unlock()
		return err
	}
	s.beginMarkerCapture(marker)
	retire, writeErr := s.writeInputAndUnlock(request, entry.ID, false)
	if writeErr != nil {
		if retire {
			select {
			case <-s.closeDone:
			case <-ctx.Done():
			}
		}
		// Fim da entrega: o leitor continua drenando os bytes posteriores sem
		// acumular outra cópia nem enviá-los como entrada manual.
		s.outputMu.Lock()
		s.discardOutput = true
		s.outputMu.Unlock()
		// O leitor pode ter recebido mais bytes durante o encerramento.
		// Consuma o lote final uma única vez, antes de entregar a entrada.
		stream := newMarkerStream(marker)
		result := stream.finish(s.takeCommandOutput())
		if result.Found {
			entry.Output = stream.outputBeforeFooter()
		} else {
			entry.Output = stream.partialOutput()
		}
		if marker.probeInterrupt != nil {
			entry.Output = ""
		}
		if s.onOutput != nil && entry.Output != "" {
			s.onOutput(s.id, entry.ID, entry.Output)
		}
		s.mu.Lock()
		readErr, cleanupErr := s.readerErr, s.closeErr
		s.mu.Unlock()
		return errors.Join(writeErr, readErr, ctx.Err(), cleanupErr)
	}
	logging.Debugf(ctx, "terminal.session", "[Terminal] Write OK: %d bytes escritos para sessão %s", len(request), s.id)

	return nil
}

// ErrSessionClosed indica que o fechamento explícito interrompeu o comando.
var ErrSessionClosed = errors.New("sessão encerrada explicitamente")

// ErrSessionWrite indica falha de escrita de um comando no PTY.
var ErrSessionWrite = errors.New("falha ao enviar comando para sessão")

// ErrSessionRead indica uma falha do leitor do PTY e acompanha a causa original.
var ErrSessionRead = errors.New("falha ao ler o resultado do comando")

// exitedCommandResult recebe a saída drenada quando o leitor fecha sem o marcador final.
func (s *Session) exitedCommandResult(stream *markerStream) (string, int, error) {
	output := stream.partialOutput()
	s.mu.Lock()
	closed := s.explicitClose
	s.mu.Unlock()
	if s.readerErr != nil {
		return output, -1, s.readerErr
	}
	if closed {
		return output, -1, ErrSessionClosed
	}
	select {
	case <-s.processDone:
		if stream.started && stream.marker.probeInterrupt == nil {
			if s.processErr == nil {
				return output, 0, nil
			}
			var exit *ptyx.ExitError
			if errors.As(s.processErr, &exit) && exit.ExitCode >= 0 {
				return output, exit.ExitCode, nil
			}
		}
		if s.processErr != nil {
			return output, -1, fmt.Errorf("processo da sessão encerrado: %w", s.processErr)
		}
	default:
	}
	return output, -1, errors.New("sessão encerrada sem resultado do comando")
}

// A leitura já pode ter colocado outros bytes em outputBuf desde o último tick.
// Transfere o sufixo e o lote concorrente antes de liberar a próxima leitura raw.
func (s *Session) resumeRawAfterFooter(result parseResult) {
	s.outputMu.Lock()
	if !s.suppressRawOutput || s.discardOutput {
		s.outputMu.Unlock()
		return
	}
	s.suppressRawOutput = false
	s.rawCommandID = ""
	s.rawOutput = rawTerminalOutput{footerBreak: true, text: parsedTerminalText{footerColumn: result.footerColumn}}
	previous := s.rawOwner
	owner := newRawDeliveryOwner(true, s.publishRaw)
	s.rawOwner = owner
	if s.onRawOutput != nil {
		text := s.rawOutput.append(result.afterFooter, false)
		text += s.rawOutput.append(s.outputBuf.String(), false)
		if s.rawFinished {
			text += s.rawOutput.finish()
		}
		if text != "" {
			owner.reserve()
			owner.submit(rawChunk{text: text})
		}
	}
	s.outputBuf = bytes.Buffer{}
	s.outputMu.Unlock()
	if previous != nil {
		previous.afterDelivery(owner.opened)
	} else {
		owner.opened()
	}
}

// waitForMarker acompanha a saída até o marcador final ou o encerramento do shell.
// O callback recebe o texto do comando durante a execução.
func (s *Session) waitForMarker(ctx context.Context, marker *CommandMarker, commandID string) (output string, code int, err error) {
	ticker := time.NewTicker(50 * time.Millisecond) // polling a cada 50ms
	defer ticker.Stop()

	// Conta os bytes dos deltas já entregues para emitir apenas o sufixo ao finalizar.
	streamSentLen := 0
	defer func() {
		if marker.probeInterrupt != nil {
			output = ""
			return
		}
		if s.onOutput != nil && len(output) > streamSentLen {
			s.onOutput(s.id, commandID, output[streamSentLen:])
		}
	}()
	stream := newMarkerStream(marker)

	for {
		select {
		case <-marker.probeInterrupt:
			// Ctrl+C já chegou ao PTY. Drene o rodapé que pode estar em trânsito
			// e devolva à sessão a saída posterior, inclusive no mesmo bloco.
			result := s.drainAfterInterrupt(stream)
			if !result.Found {
				result = stream.finish(s.takeCommandOutput())
			}
			if result.Found {
				s.resumeRawAfterFooter(result)
			}
			return "", -1, context.Canceled
		case <-ctx.Done():
			return s.canceledCommandResult(ctx, stream)

		case <-s.readerDone:
			// Um comando pode encerrar o próprio shell antes do marcador final.
			// Só aceite o status do processo após drenar o leitor e observar o
			// marcador de início. Fechamento externo e cancelamento são erros.
			if ctx.Err() != nil {
				return s.canceledCommandResult(ctx, stream)
			}
			result := stream.finish(s.takeCommandOutput())
			if result.Found {
				s.resumeRawAfterFooter(result)
				return result.Output, result.ExitCode, nil
			}
			return s.exitedCommandResult(stream)

		case <-ticker.C:
			if ctx.Err() != nil {
				return s.canceledCommandResult(ctx, stream)
			}
			chunk := s.takeCommandOutput()
			if chunk == "" {
				continue
			}
			delta, result := stream.append(chunk)
			if marker.probeInterrupt == nil && s.onOutput != nil && len(delta) > 0 {
				s.onOutput(s.id, commandID, string(delta))
				streamSentLen += len(delta)
			}
			if result.Found {
				s.resumeRawAfterFooter(result)
				return result.Output, result.ExitCode, nil
			}
		}
	}
}

// Reaproveite até 16 leituras do PTY entre ticks do mesmo comando.
const maxReusableOutputCapacity = 16 * ptyReadSize

// Para lotes pequenos, copie antes de permitir a próxima escrita no buffer.
// Em picos, transfira o lote e faça a cópia maior fora da seção crítica.
func (s *Session) takeCommandOutput() string {
	s.outputMu.Lock()
	if s.outputBuf.Cap() <= maxReusableOutputCapacity {
		pending := s.outputBuf.String()
		s.outputBuf.Reset()
		s.outputMu.Unlock()
		return pending
	}
	pending := s.outputBuf.Bytes()
	s.outputBuf = bytes.Buffer{}
	s.outputMu.Unlock()
	return string(pending)
}

func (stream *markerStream) finish(chunk string) parseResult {
	if _, result := stream.append(chunk); result.Found {
		return result
	}
	stream.text.finish()
	_, result := stream.progress()
	return result
}

func (stream *markerStream) partialOutput() string {
	if !stream.started {
		return ""
	}
	data := stream.text.data
	// Ao drenar, uma tag inteira com status ainda sem terminador já identifica
	// a moldura. Prefixos ambíguos ou candidatos inválidos pertencem à saída.
	if stream.candidate != 0 && stream.statusScan >= 0 {
		status := string(bytes.TrimSpace(data[stream.candidate:]))
		if _, err := strconv.Atoi(status); status == "" || status == "-" || status == "+" || err == nil {
			return stream.outputBeforeFooter()
		}
	}
	return string(data[stream.start:])
}

func (stream *markerStream) outputBeforeFooter() string {
	output := stream.text.data[stream.start : stream.candidate-len(stream.marker.endTag)]
	if !stream.marker.inlineEnd {
		output = bytes.TrimSuffix(output, []byte("\n")) // separador escrito pelo rodapé POSIX
	}
	return string(output)
}

func (s *Session) canceledCommandResult(ctx context.Context, stream *markerStream) (string, int, error) {
	// Timeout ou cancelamento: envia Ctrl+C e aguarda brevemente a
	// drenagem. RunEphemeral fará em seguida o cleanup completo
	// (Kill + Wait + EOF do leitor + Close), antes de retornar.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		logging.Errorf(context.Background(), "terminal.session", "[Terminal] TIMEOUT session=%s", s.id)
	} else {
		logging.Infof(context.Background(), "terminal.session", "[Terminal] Comando cancelado na sessão %s", s.id)
	}
	var result parseResult
	select {
	case <-s.readerDone:
	default:
		if err := s.Interrupt(); err != nil {
			logging.Errorf(context.Background(), "terminal.session", "[Terminal] Erro ao enviar Ctrl+C após timeout: %v", err)
		}
		result = s.drainAfterInterrupt(stream)
	}
	if !result.Found {
		result = stream.finish(s.takeCommandOutput())
	}
	if result.Found {
		s.resumeRawAfterFooter(result)
		result.Output = stream.outputBeforeFooter()
	} else {
		result.Output = stream.partialOutput()
	}
	cause := ctx.Err()
	s.mu.Lock()
	if s.explicitClose {
		cause = errors.Join(cause, ErrSessionClosed)
	}
	s.mu.Unlock()
	select {
	case <-s.readerDone:
		if s.readerErr != nil {
			cause = errors.Join(cause, s.readerErr)
		}
	default:
	}
	return result.Output, -1, fmt.Errorf("fim do comando não observado: %w", cause)
}

func (s *Session) drainAfterInterrupt(stream *markerStream) parseResult {
	deadline := time.NewTimer(interruptDrainTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		chunk := s.takeCommandOutput()
		if _, result := stream.append(chunk); result.Found {
			return result
		}
		select {
		case <-ticker.C:
		case <-s.readerDone:
			return parseResult{}
		case <-deadline.C:
			return parseResult{}
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

	// Uma interface que acabou de abrir deve poder identificar a entrada
	// manual ainda interrompível, mesmo após muitas tentativas sem despacho.
	if len(s.history) > maxHistoryEntries {
		if s.interruptIsManual && s.history[0].ID == s.lastDispatchedID {
			s.history[1] = s.history[0]
		}
		s.history[0] = HistoryEntry{}
		s.history = s.history[1:]
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
	if err := s.reserveCommand(nil); err != nil {
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
	s.startManualOutput(entry)
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
	if _, err := s.writeInputAndUnlock([]byte(input+enter), commandID, true); err != nil {
		return nil, err
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
	if err := s.writeInterruptLocked(s.ptyWriter); err != nil {
		return err
	}
	logging.Infof(context.Background(), "terminal.session", "[Terminal] Ctrl+C enviado para sessão %s", s.id)
	return nil
}

// O chamador mantém ioMu desde a validação até este ponto. Mesmo que Write
// tenha aceitado só um prefixo, a sessão entra em closing antes de liberar o lock.
func (s *Session) writeInputAndUnlock(request []byte, commandID string, manual bool) (bool, error) {
	n, err := s.ptyWriter.Write(request)
	if n != len(request) && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		err = fmt.Errorf("%w %s: %w", ErrSessionWrite, s.id, err)
	}
	s.mu.Lock()
	retire := err != nil && s.state == StateRunning
	if retire {
		s.state, s.writeErr = StateClosing, err
	}
	if err == nil {
		s.lastDispatchedID = commandID
	} else {
		s.lastDispatchedID = ""
	}
	s.interruptIsManual = manual && err == nil
	s.commandPending = false
	s.mu.Unlock()
	s.ioMu.Unlock()
	if retire {
		s.ensureLifecycleChannels()
		go func() {
			var killErr error
			if s.ptySession != nil {
				killErr = s.ptySession.Kill()
			}
			_ = s.finishClose(nil, killErr)
		}()
	}
	return retire, err
}

// Close encerra a sessão PTY e libera recursos.
func (s *Session) Close() error {
	s.ensureLifecycleChannels()
	s.ioMu.Lock()
	return s.closeWithIOLock(nil)
}

// closeWithIOLock executa o cleanup assumindo que ioMu está reservado pelo
// chamador. Ele libera ioMu assim que Kill/estado closing terminam, antes de
// aguardar o processo e drenar o leitor.
func (s *Session) closeWithIOLock(captured *closeSnapshotToken) error {
	s.mu.Lock()
	// A falha do leitor pode iniciar o cleanup sem ioMu. Valide a captura
	// sob o mesmo lock que atribui o encerramento à ação explícita.
	if captured != nil && !s.matchesCloseSnapshot(captured) {
		s.mu.Unlock()
		s.ioMu.Unlock()
		return ErrCloseStale
	}
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
	s.mu.Unlock()

	// Primeiro encerra a árvore de processos. Kill no ptyx usa
	// TerminateProcess no Windows; Wait confirma a saída antes de liberar os
	// handles. O leitor permanece aberto nesse intervalo para drenar o ConPTY.
	var killErr error
	if s.ptySession != nil {
		killErr = s.ptySession.Kill()
	}
	s.ioMu.Unlock()
	return s.finishClose(nil, killErr)
}

// Um Write no PTY pode reter ioMu até a morte do processo ou o fechamento
// do descritor. O próprio leitor é o único dono do cleanup ao fazer a
// transição para closing sob mu. Close concorrente aguardará closeDone.
func (s *Session) closeAfterReadFailure(err error) {
	s.mu.Lock()
	if s.state == StateClosing || s.state == StateExited {
		if s.writeErr != nil {
			s.readerErr = fmt.Errorf("%w: %w", ErrSessionRead, err)
		}
		s.mu.Unlock()
		s.signalReaderDone()
		return
	}
	s.state = StateClosing
	s.readerErr = fmt.Errorf("%w: %w", ErrSessionRead, err)
	s.mu.Unlock()
	s.signalReaderDone()
	var killErr error
	if s.ptySession != nil {
		killErr = s.ptySession.Kill()
	}
	_ = s.finishClose(err, killErr)
}

func (s *Session) finishClose(readErr, killErr error) error {
	waitErr := s.waitProcess()
	s.waitReaderDrain()
	if s.cancelReader != nil {
		// Só cancela depois do EOF: o mesmo contexto governa o readLoop, e
		// cancelá-lo antes descartaria bytes ainda bufferizados no ConPTY.
		s.cancelReader()
	}
	if readErr != nil {
		s.releasePTY()
	} else {
		s.closePTY()
	}

	logging.Infof(context.Background(), "terminal.session", "[Terminal] Sessão encerrada: id=%s name=%s", s.id, s.name)
	var closeErr error
	if killErr != nil {
		closeErr = fmt.Errorf("falha ao encerrar processo da sessão %s: %w", s.id, killErr)
	} else if waitErr != nil && !isExpectedTermination(waitErr) {
		closeErr = fmt.Errorf("falha ao aguardar processo da sessão %s: %w", s.id, waitErr)
	}
	s.mu.Lock()
	s.closeErr = closeErr
	s.state = StateExited
	s.closeDoneOnce.Do(func() { close(s.closeDone) })
	s.mu.Unlock()
	s.notifyFaultExit()
	return closeErr
}

// faultOwnsCleanup indica que o backend já tem um responsável pela drenagem e
// pela publicação de exited. Os chamadores de comando não repetem essa espera.
// A sessão continua ocupando uma vaga do gerenciador enquanto a limpeza estiver pendente.
func (s *Session) faultOwnsCleanup() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr != nil || s.readerErr != nil
}

// Somente um fechamento explícito produz session_closed. Uma saída natural
// ou falha pode já ter enfileirado exited atrás do resultado do comando.
func (s *Session) closeForManager() (bool, error) {
	err := s.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.explicitClose, err
}

// A segunda operação a terminar publica exited, mesmo se o contexto tiver
// vencido enquanto o backend continuava bloqueado em Kill, Wait ou Close.
func (s *Session) notifyFaultExit() {
	s.mu.Lock()
	if s.state != StateExited || (s.writeErr == nil && s.readerErr == nil) || s.explicitClose || s.onExit == nil {
		s.mu.Unlock()
		return
	}
	var cause error
	if d := s.commandDelivery; d != nil {
		if !d.ended {
			s.mu.Unlock()
			return
		}
		cause = d.err
	}
	// Se o comando venceu antes do cleanup, podem existir causas novas.
	for _, late := range []error{s.writeErr, s.readerErr, s.closeErr} {
		if late != nil && !errors.Is(cause, late) {
			cause = errors.Join(cause, late)
		}
	}
	s.mu.Unlock()
	s.exitOnce.Do(func() { s.deliverExit(cause) })
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
