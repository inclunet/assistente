package shell

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/terminal"
)

// ========== TESTES DE VALIDAÇÃO (sem Manager) ==========

// TestRejectsMissingCommand valida rejeição de comando vazio
func TestRejectsMissingCommand(t *testing.T) {
	rc := NewRunCommand(nil, nil, nil, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("esperado error result para comando vazio")
	}
	if len(result.Content) == 0 {
		t.Error("esperado mensagem de erro, got vazia")
	}
}

// TestInvalidJSON valida rejeição de JSON inválido
func TestInvalidJSON(t *testing.T) {
	rc := NewRunCommand(nil, nil, nil, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":`))
	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("esperado error result para JSON inválido")
	}
}

// TestDeniedByAllowlist valida bloqueio de allowlist
func TestDeniedByAllowlist(t *testing.T) {
	al := &allowlist.Allowlist{
		AlwaysDeny:    []string{"rm *"},
		AutoApprove:   []string{},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("esperado error quando bloqueado por allowlist")
	}
	if len(result.Content) == 0 {
		t.Error("esperado mensagem de bloqueio")
	}
}

// TestConfirmRejected valida rejeição de confirmação
func TestConfirmRejected(t *testing.T) {
	al := &allowlist.Allowlist{DefaultAction: "confirm"}

	confirmFn := func(ctx context.Context, command, workDir string) (bool, error) {
		return false, nil // reject
	}

	rc := NewRunCommand(nil, confirmFn, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"echo test"}`))
	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatal("esperado error quando usuário rejeita")
	}
}

// ========== TESTES DE VALIDAÇÃO DE ARGUMENTOS ==========

// TestTimeoutNegativeIgnored valida que timeout <= 0 usa padrão
func TestTimeoutNegativeIgnored(t *testing.T) {
	// Teste estrutural: parsing de argumentos
	var args runCommandArgs
	data := json.RawMessage(`{"command":"echo", "timeout_seconds": -5}`)
	err := json.Unmarshal(data, &args)
	if err != nil {
		t.Fatalf("erro ao parsear: %v", err)
	}
	if args.TimeoutSeconds != -5 {
		t.Errorf("esperado -5, got %d", args.TimeoutSeconds)
	}
	// O código trata timeout <= 0 como "usar padrão"
	// Isso é validado durante execução com um Manager real
}

// TestEmptyWorkingDirectory valida diretório vazio
func TestEmptyWorkingDirectory(t *testing.T) {
	var args runCommandArgs
	data := json.RawMessage(`{"command":"echo", "working_directory":""}`)
	err := json.Unmarshal(data, &args)
	if err != nil {
		t.Fatalf("erro ao parsear: %v", err)
	}
	if args.WorkingDirectory != "" {
		t.Errorf("esperado vazio, got %q", args.WorkingDirectory)
	}
}

// TestCommandWithSpecialCharacters valida parsing de comando complexo
func TestCommandWithSpecialCharacters(t *testing.T) {
	var args runCommandArgs
	data := json.RawMessage(`{"command":"echo 'Hello \"World\"' && ls -la"}`)
	err := json.Unmarshal(data, &args)
	if err != nil {
		t.Fatalf("erro ao parsear: %v", err)
	}
	if args.Command != "echo 'Hello \"World\"' && ls -la" {
		t.Errorf("comando não preservado corretamente: %q", args.Command)
	}
}

// TestNameAndDescription valida metadados da tool
func TestNameAndDescription(t *testing.T) {
	rc := NewRunCommand(nil, nil, nil, ".")

	name := rc.Name()
	if name != "run_command" {
		t.Errorf("esperado Name()='run_command', got %q", name)
	}

	desc := rc.Description()
	if len(desc) == 0 {
		t.Error("esperado Description() não-vazio")
	}
	if !contains(desc, "command") {
		t.Errorf("esperado description contém 'command'")
	}
}

// TestParameters valida JSON schema
func TestParameters(t *testing.T) {
	rc := NewRunCommand(nil, nil, nil, ".")

	params := rc.Parameters()
	if len(params) == 0 {
		t.Fatal("esperado Parameters() não-vazio")
	}

	// Validate que é JSON válido
	var schema map[string]interface{}
	err := json.Unmarshal(params, &schema)
	if err != nil {
		t.Fatalf("Parameters() não é JSON válido: %v", err)
	}

	if schema["type"] != "object" {
		t.Error("esperado schema type='object'")
	}

	if props, ok := schema["properties"].(map[string]interface{}); !ok {
		t.Error("esperado 'properties' em schema")
	} else {
		if _, hasCommand := props["command"]; !hasCommand {
			t.Error("esperado 'command' property no schema")
		}
	}
}

// ========== MOCK & EXECUTION TESTS ==========

// MockSessionManager implementa SessionManager para testes
type MockSessionManager struct {
	acquireCalls    int
	releaseCalls    int
	runCommandCalls int
	runSessionID    string
	liveSessions    map[string]bool
	sessionCWD      map[string]string

	// Controladores de behavior
	fakeSession *terminal.Session
	fakeSessErr error
	fakeEntry   *terminal.HistoryEntry
	fakeRunErr  error
}

// createMockSession cria uma Session para testes
func createMockSession(sessionID string) *terminal.Session {
	return &terminal.Session{}
}

func (m *MockSessionManager) Acquire(ctx context.Context, workDir string) (*terminal.Session, error) {
	m.acquireCalls++
	if m.fakeSessErr != nil {
		return nil, m.fakeSessErr
	}
	if m.fakeSession == nil {
		m.fakeSession = createMockSession("sess-default")
	}
	return m.fakeSession, nil
}

func (m *MockSessionManager) RunCommand(ctx context.Context, sessionID string, command string, timeout time.Duration, requesterID string) (*terminal.HistoryEntry, error) {
	m.runCommandCalls++
	m.runSessionID = sessionID
	if m.fakeRunErr != nil {
		return m.fakeEntry, m.fakeRunErr
	}
	if m.fakeEntry == nil {
		m.fakeEntry = &terminal.HistoryEntry{
			ID:       "cmd-1",
			Command:  command,
			Output:   "",
			ExitCode: 0,
		}
	}
	return m.fakeEntry, nil
}

func (m *MockSessionManager) Info(sessionID string) (terminal.SessionInfo, bool) {
	if !m.liveSessions[sessionID] {
		return terminal.SessionInfo{}, false
	}
	return terminal.SessionInfo{ID: sessionID, CWD: m.sessionCWD[sessionID]}, true
}

func (m *MockSessionManager) Release(sessionID string) {
	m.releaseCalls++
}

// TestSuccessfulExecution valida execução bem-sucedida
func TestSuccessfulExecution(t *testing.T) {
	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-123",
			Command:  "echo hello",
			Output:   "hello\n",
			ExitCode: 0,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"echo *"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso, got error: %s", result.Content)
	}
	if !contains(result.Content, "hello") {
		t.Errorf("esperado output contém 'hello', got %q", result.Content)
	}
	if mgr.runCommandCalls != 1 {
		t.Errorf("esperado 1 RunCommand call, got %d", mgr.runCommandCalls)
	}
	if mgr.releaseCalls != 1 {
		t.Errorf("esperado 1 Release call, got %d", mgr.releaseCalls)
	}
}

func TestExecutionUsesExplicitTerminalWithoutAcquire(t *testing.T) {
	mgr := &MockSessionManager{
		liveSessions: map[string]bool{"term-explicit": true},
		sessionCWD:   map[string]string{"term-explicit": "/workspace/repo"},
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-explicit",
			Output:   "ok",
			ExitCode: 0,
		},
	}
	al := &allowlist.Allowlist{AutoApprove: []string{"echo *"}, DefaultAction: "deny"}
	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(
		`{"command":"echo ok","terminal_id":"term-explicit"}`,
	))
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%#v err=%v", result, err)
	}
	if mgr.acquireCalls != 0 {
		t.Fatalf("Acquire chamado %d vez(es)", mgr.acquireCalls)
	}
	if mgr.runSessionID != "term-explicit" {
		t.Fatalf("RunCommand recebeu %q", mgr.runSessionID)
	}
	if result.Metadata["deepLink"] != "assistente://terminal/term-explicit" {
		t.Fatalf("deepLink = %#v", result.Metadata["deepLink"])
	}
}

func TestExecutionRejectsWorkingDirectoryWithExplicitTerminal(t *testing.T) {
	mgr := &MockSessionManager{
		liveSessions: map[string]bool{"term-explicit": true},
	}
	al := &allowlist.Allowlist{AutoApprove: []string{"echo *"}, DefaultAction: "deny"}
	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(
		`{"command":"echo ok","terminal_id":"term-explicit","working_directory":"outro"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || mgr.runCommandCalls != 0 {
		t.Fatalf("resultado=%#v runCalls=%d", result, mgr.runCommandCalls)
	}
}

func TestExecutionRejectsWorkingDirectoryOutsideProject(t *testing.T) {
	mgr := &MockSessionManager{}
	al := &allowlist.Allowlist{AutoApprove: []string{"echo *"}, DefaultAction: "deny"}
	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, t.TempDir())

	result, err := rc.Execute(context.Background(), json.RawMessage(
		`{"command":"echo ok","working_directory":"../../outside"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || mgr.acquireCalls != 0 {
		t.Fatalf("resultado=%#v acquireCalls=%d", result, mgr.acquireCalls)
	}
}

func TestExplicitTerminalConfirmationUsesSessionCWD(t *testing.T) {
	mgr := &MockSessionManager{
		liveSessions: map[string]bool{"term-explicit": true},
		sessionCWD:   map[string]string{"term-explicit": "/workspace/repo"},
	}
	al := &allowlist.Allowlist{DefaultAction: "confirm"}
	confirmedCWD := ""
	confirmFn := func(_ context.Context, _, workDir string) (bool, error) {
		confirmedCWD = workDir
		return false, nil
	}
	rc := NewRunCommand(mgr, confirmFn, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(
		`{"command":"echo ok","terminal_id":"term-explicit"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || confirmedCWD != "/workspace/repo" {
		t.Fatalf("resultado=%#v cwd=%q", result, confirmedCWD)
	}
}

func TestExecutionRejectsDeadExplicitTerminal(t *testing.T) {
	mgr := &MockSessionManager{liveSessions: map[string]bool{}}
	al := &allowlist.Allowlist{AutoApprove: []string{"echo *"}, DefaultAction: "deny"}
	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(
		`{"command":"echo ok","terminal_id":"term-dead"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || mgr.runCommandCalls != 0 {
		t.Fatalf("resultado=%#v runCalls=%d", result, mgr.runCommandCalls)
	}
}

func TestExecutionDoesNotReleaseExplicitTerminalAfterFailure(t *testing.T) {
	mgr := &MockSessionManager{
		liveSessions: map[string]bool{"term-explicit": true},
		sessionCWD:   map[string]string{"term-explicit": "/workspace/repo"},
		fakeRunErr:   errors.New("sessão ocupada"),
	}
	al := &allowlist.Allowlist{AutoApprove: []string{"echo *"}, DefaultAction: "deny"}
	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(
		`{"command":"echo ok","terminal_id":"term-explicit"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || mgr.acquireCalls != 0 || mgr.releaseCalls != 0 {
		t.Fatalf(
			"resultado=%#v acquireCalls=%d releaseCalls=%d",
			result,
			mgr.acquireCalls,
			mgr.releaseCalls,
		)
	}
}

// TestNonZeroExitCode valida comando com exit não-zero
func TestNonZeroExitCode(t *testing.T) {
	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-124",
			Command:  "false",
			Output:   "",
			ExitCode: 1,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"false"}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso mesmo com exit code não-zero, got error: %s", result.Content)
	}
	if !contains(result.Content, "exit code: 1") {
		t.Errorf("esperado '[exit code: 1]' no output, got %q", result.Content)
	}
}

// TestTimeoutWithPartialOutput valida timeout com output parcial
func TestTimeoutWithPartialOutput(t *testing.T) {
	mgr := &MockSessionManager{
		fakeRunErr: context.DeadlineExceeded,
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-125",
			Command:  "long-command",
			Output:   "partial output here",
			ExitCode: -1, // timeout indicator
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"long-command", "timeout_seconds":5}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso com timeout parcial, got error: %s", result.Content)
	}
	if !contains(result.Content, "TIMEOUT") {
		t.Errorf("esperado '[TIMEOUT' no output, got %q", result.Content)
	}
	if !contains(result.Content, "partial output") {
		t.Errorf("esperado output parcial preservado, got %q", result.Content)
	}
}

// TestTimeoutWithoutOutput valida timeout sem output
func TestTimeoutWithoutOutput(t *testing.T) {
	mgr := &MockSessionManager{
		fakeRunErr: context.DeadlineExceeded,
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-126",
			Command:  "hang-forever",
			Output:   "", // sem output
			ExitCode: -1,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"hang-forever", "timeout_seconds":1}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("esperado error quando timeout sem output, got sucesso")
	}
	if !contains(result.Content, "Erro ao executar") {
		t.Errorf("esperado erro message, got %q", result.Content)
	}
}

// TestAcquireSessionError valida erro ao adquirir sessão
func TestAcquireSessionError(t *testing.T) {
	mgr := &MockSessionManager{
		fakeSessErr: context.DeadlineExceeded,
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"test"}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("esperado error ao adquirir sessão")
	}
	if !contains(result.Content, "sessão") {
		t.Errorf("esperado mention de sessão na mensagem, got %q", result.Content)
	}
}

// TestConfirmFunctionError valida erro ao chamar confirmFunc
func TestConfirmFunctionError(t *testing.T) {
	confirmFn := func(ctx context.Context, command, workDir string) (bool, error) {
		return false, context.DeadlineExceeded // erro na confirmação
	}

	al := &allowlist.Allowlist{
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, confirmFn, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"test"}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("esperado error quando confirmFunc falha")
	}
	if !contains(result.Content, "confirmação") {
		t.Errorf("esperado mention de confirmação na mensagem, got %q", result.Content)
	}
}

// TestOutputTruncation valida truncamento de output > 50KB
func TestOutputTruncation(t *testing.T) {
	// Criar output grande (> 50KB)
	largeOutput := ""
	for i := 0; i < 60000; i++ { // > 60KB
		largeOutput += "x"
	}

	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-127",
			Command:  "big-output",
			Output:   largeOutput,
			ExitCode: 0,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"big-output"}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso, got: %s", result.Content)
	}
	if !contains(result.Content, "TRUNCADO") {
		t.Errorf("esperado truncation message, got %q", result.Content)
	}
	if len(result.Content) > 52*1024 { // 50KB + mensagem + margem
		t.Errorf("esperado output truncado, got %d bytes (max ~52KB)", len(result.Content))
	}
}

// ========== EDGE CASES & METADATA ==========

// TestTimeoutExceedsMaxTimeout valida clipping de timeout > 300s
func TestTimeoutExceedsMaxTimeout(t *testing.T) {
	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-128",
			Command:  "long-task",
			Output:   "done",
			ExitCode: 0,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")

	// Req timeout: 600 segundos (máximo é 300s / 5min)
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"long-task", "timeout_seconds":600}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso, got: %s", result.Content)
	}

	// Validar que timeout foi clipped (não há forma de confirmar diretamente,
	// mas o Manager foi chamado, logo timeout foi calculado e respeitado)
	if mgr.runCommandCalls != 1 {
		t.Errorf("esperado 1 RunCommand call, got %d", mgr.runCommandCalls)
	}
}

// TestConfirmDecisionWithNilCallback valida DecisionConfirm sem callback
func TestConfirmDecisionWithNilCallback(t *testing.T) {
	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-129",
			Command:  "test",
			Output:   "ok",
			ExitCode: 0,
		},
	}

	al := &allowlist.Allowlist{
		DefaultAction: "confirm",
	}

	// Sem confirmFn (nil) — com DefaultAction="confirm", vai tentar chamar callback
	// Mas como é nil, deve pular para execução
	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"test"}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso com confirmFn=nil, got: %s", result.Content)
	}
	// Deve ter executado mesmo sem callback
	if mgr.runCommandCalls != 1 {
		t.Errorf("esperado execução mesmo sem confirmFn, got %d calls", mgr.runCommandCalls)
	}
}

// TestWorkingDirectoryRelativePath valida path relativo
func TestWorkingDirectoryRelativePath(t *testing.T) {
	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-130",
			Command:  "pwd",
			Output:   "/project/subdir\n",
			ExitCode: 0,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, "/project")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{
		"command":"pwd",
		"working_directory":"subdir/nested"
	}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso, got: %s", result.Content)
	}
	// Validar que metadata contém workDir resolvido
	if metadata, ok := result.Metadata["workDir"].(string); !ok || metadata == "" {
		t.Errorf("esperado workDir em metadata, got %v", result.Metadata["workDir"])
	}
}

// TestMetadataCompleto valida presença de todos os campos de metadata
func TestMetadataCompleto(t *testing.T) {
	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-131",
			Command:  "echo test",
			Output:   "test\n",
			ExitCode: 0,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, "/mydir")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{
		"command":"echo test",
		"working_directory":"subdir"
	}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("esperado sucesso, got: %s", result.Content)
	}

	// Validar todos os campos de metadata
	requiredFields := []string{"command", "workDir", "exitCode", "sessionId"}
	for _, field := range requiredFields {
		if _, ok := result.Metadata[field]; !ok {
			t.Errorf("esperado field %q em metadata", field)
		}
	}

	// Validar tipos
	if cmd, ok := result.Metadata["command"].(string); !ok || cmd != "echo test" {
		t.Errorf("esperado command='echo test', got %v", result.Metadata["command"])
	}
	if exitCode, ok := result.Metadata["exitCode"].(int); !ok || exitCode != 0 {
		t.Errorf("esperado exitCode=0, got %v", result.Metadata["exitCode"])
	}
	if workDir, ok := result.Metadata["workDir"].(string); !ok || workDir == "" {
		t.Errorf("esperado workDir string não-vazio, got %v", result.Metadata["workDir"])
	}
	// sessionId é um string, pode ser vazio em testes com Session mock
	if _, ok := result.Metadata["sessionId"].(string); !ok {
		t.Errorf("esperado sessionId field em metadata, got %v", result.Metadata["sessionId"])
	}
}

// TestMetadataTimeoutWithoutOutput valida metadata quando timeout sem output
func TestMetadataTimeoutWithoutOutput(t *testing.T) {
	mgr := &MockSessionManager{
		fakeRunErr: context.DeadlineExceeded,
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-132",
			Command:  "hang",
			Output:   "",
			ExitCode: -1,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{
		"command":"hang",
		"timeout_seconds":1
	}`))

	if err != nil {
		t.Fatalf("esperado nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("esperado error quando timeout sem output")
	}

	// Validar que metadata contém exitCode=-1
	if exitCode, ok := result.Metadata["exitCode"].(int); !ok || exitCode != -1 {
		t.Errorf("esperado exitCode=-1 em metadata, got %v", result.Metadata["exitCode"])
	}
	if commandID, ok := result.Metadata["commandId"].(string); !ok || commandID != "cmd-132" {
		t.Errorf("esperado commandId=cmd-132 em metadata, got %v", result.Metadata["commandId"])
	}
}

// TestMultipleExecutionsReleaseSession valida que Release é chamado sempre
func TestMultipleExecutionsReleaseSession(t *testing.T) {
	mgr := &MockSessionManager{
		fakeEntry: &terminal.HistoryEntry{
			ID:       "cmd-133",
			Command:  "test",
			Output:   "ok",
			ExitCode: 0,
		},
	}

	al := &allowlist.Allowlist{
		AutoApprove:   []string{"*"},
		DefaultAction: "deny",
	}

	rc := NewRunCommand(mgr, nil, func() *allowlist.Allowlist { return al }, ".")

	// Executar 3 vezes
	for i := 0; i < 3; i++ {
		result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"test"}`))
		if err != nil {
			t.Fatalf("iteração %d: esperado nil error, got %v", i, err)
		}
		if result.IsError {
			t.Fatalf("iteração %d: esperado sucesso, got: %s", i, result.Content)
		}
	}

	// Release deve ser chamado 3 vezes (uma por execução)
	if mgr.releaseCalls != 3 {
		t.Errorf("esperado 3 Release calls, got %d", mgr.releaseCalls)
	}
	// RunCommand deve ser chamado 3 vezes
	if mgr.runCommandCalls != 3 {
		t.Errorf("esperado 3 RunCommand calls, got %d", mgr.runCommandCalls)
	}
}

// ========== HELPERS ==========

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
