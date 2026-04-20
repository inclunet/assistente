package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"assistente/controllers"
)

// ---------------------------------------------------------------------------
// Mock setupBackend
// ---------------------------------------------------------------------------

type mockSetupBackend struct {
	needsWizard     bool
	hasMasterKey    bool
	setupPwdReturn  string
	setupPwdErr     error
	testProviderOK  bool
	testProviderErr error
	listModels      []string
	listModelsErr   error
	createErr       error
	defaultProvErr  error
	setChatModelErr error

	setupPwdCalled   string
	createdType      string
	createdKey       string
	defaultProvID    string
	chatModelSet     string
}

func (m *mockSetupBackend) NeedsWelcomeWizard() bool { return m.needsWizard }
func (m *mockSetupBackend) HasMasterKey() bool        { return m.hasMasterKey }

func (m *mockSetupBackend) SetupMasterPassword(password string) (string, error) {
	m.setupPwdCalled = password
	return m.setupPwdReturn, m.setupPwdErr
}

func (m *mockSetupBackend) TestLLMProvider(req controllers.TestLLMProviderRequest) (bool, error) {
	return m.testProviderOK, m.testProviderErr
}

func (m *mockSetupBackend) ListModelsRaw(req controllers.TestLLMProviderRequest) ([]string, error) {
	return m.listModels, m.listModelsErr
}

func (m *mockSetupBackend) CreateDefaultLLMProvider(providerType, apiKey string) error {
	m.createdType = providerType
	m.createdKey = apiKey
	return m.createErr
}

func (m *mockSetupBackend) SetDefaultProvider(id string) error {
	m.defaultProvID = id
	return m.defaultProvErr
}

func (m *mockSetupBackend) SetChatModel(model string) error {
	m.chatModelSet = model
	return m.setChatModelErr
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// fakePasswordReader returns a passwordReader that serves passwords in order.
func fakePasswordReader(passwords ...string) passwordReader {
	idx := 0
	return func(prompt string) (string, error) {
		if idx >= len(passwords) {
			return "", fmt.Errorf("no more passwords")
		}
		p := passwords[idx]
		idx++
		return p, nil
	}
}

// withStdin replaces os.Stdin with a pipe containing the given input.
// Returns a cleanup function to restore the original stdin.
func withStdin(t *testing.T, input string) func() {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	go func() {
		_, _ = w.WriteString(input)
		w.Close()
	}()
	return func() { os.Stdin = oldStdin; r.Close() }
}

// ---------------------------------------------------------------------------
// readMasterPassword
// ---------------------------------------------------------------------------

func TestReadMasterPassword_Success(t *testing.T) {
	readPwd := fakePasswordReader("senhasegura", "senhasegura")
	pwd, err := readMasterPassword(readPwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pwd != "senhasegura" {
		t.Errorf("expected 'senhasegura', got %q", pwd)
	}
}

func TestReadMasterPassword_TooShort(t *testing.T) {
	readPwd := fakePasswordReader("short", "short")
	_, err := readMasterPassword(readPwd)
	if err == nil {
		t.Fatal("expected error for short password")
	}
	if !strings.Contains(err.Error(), "pelo menos 8 caracteres") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadMasterPassword_Mismatch(t *testing.T) {
	readPwd := fakePasswordReader("senhasegura", "outrasenha!")
	_, err := readMasterPassword(readPwd)
	if err == nil {
		t.Fatal("expected error for mismatched passwords")
	}
	if !strings.Contains(err.Error(), "não coincidem") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadMasterPassword_ReadError(t *testing.T) {
	readPwd := func(prompt string) (string, error) {
		return "", fmt.Errorf("terminal closed")
	}
	_, err := readMasterPassword(readPwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao ler senha") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// readProviderChoice
// ---------------------------------------------------------------------------

func TestReadProviderChoice_Valid(t *testing.T) {
	cleanup := withStdin(t, "1\n")
	defer cleanup()

	var out bytes.Buffer
	reader := bufio.NewReader(os.Stdin)
	choice, err := readProviderChoice(reader, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if choice != "OpenAI" {
		t.Errorf("expected OpenAI, got %q", choice)
	}
}

func TestReadProviderChoice_LastItem(t *testing.T) {
	cleanup := withStdin(t, fmt.Sprintf("%d\n", len(providerChoices)))
	defer cleanup()

	var out bytes.Buffer
	reader := bufio.NewReader(os.Stdin)
	choice, err := readProviderChoice(reader, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if choice != "LiteLLM" {
		t.Errorf("expected LiteLLM, got %q", choice)
	}
}

func TestReadProviderChoice_Invalid(t *testing.T) {
	cleanup := withStdin(t, "99\n")
	defer cleanup()

	var out bytes.Buffer
	reader := bufio.NewReader(os.Stdin)
	_, err := readProviderChoice(reader, &out)
	if err == nil {
		t.Fatal("expected error for invalid choice")
	}
	if !strings.Contains(err.Error(), "escolha inválida") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadProviderChoice_NonNumeric(t *testing.T) {
	cleanup := withStdin(t, "abc\n")
	defer cleanup()

	var out bytes.Buffer
	reader := bufio.NewReader(os.Stdin)
	_, err := readProviderChoice(reader, &out)
	if err == nil {
		t.Fatal("expected error for non-numeric input")
	}
}

// ---------------------------------------------------------------------------
// runSetup — full wizard flow
// ---------------------------------------------------------------------------

func TestRunSetup_FullHappyPath(t *testing.T) {
	// Stdin sequence: Enter (recovery key ack) + "1" (OpenAI) + Enter (default model)
	cleanup := withStdin(t, "\n1\n\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:    true,
		hasMasterKey:   false,
		setupPwdReturn: "recovery-key-123",
		testProviderOK: true,
		listModels:     []string{"gpt-4o", "gpt-4o-mini"},
	}

	readPwd := fakePasswordReader("senhasegura", "senhasegura", "sk-test-key")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify password was set
	if mock.setupPwdCalled != "senhasegura" {
		t.Errorf("expected password 'senhasegura', got %q", mock.setupPwdCalled)
	}

	// Verify provider was created
	if mock.createdType == "" {
		t.Error("expected provider to be created")
	}
	if mock.createdKey != "sk-test-key" {
		t.Errorf("expected API key 'sk-test-key', got %q", mock.createdKey)
	}

	// Verify output contains success
	output := out.String()
	if !strings.Contains(output, "configurado com sucesso") {
		t.Errorf("expected success message in output, got: %s", output)
	}
	if !strings.Contains(output, "recovery-key-123") {
		t.Errorf("expected recovery key in output")
	}
}

func TestRunSetup_AlreadyConfigured_DeclinesReconfig(t *testing.T) {
	cleanup := withStdin(t, "n\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard: false, // already configured
	}

	var out bytes.Buffer
	err := runSetup(mock, fakePasswordReader(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "já está configurado") {
		t.Error("expected 'already configured' message")
	}
}

func TestRunSetup_AlreadyConfigured_AcceptsReconfig(t *testing.T) {
	// Accepts reconfig: "s" + provider choice "13" (Ollama, no API key) + Enter for model
	cleanup := withStdin(t, "s\n13\n\nqualquer-modelo\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:    false,
		hasMasterKey:   true,
		testProviderOK: true,
		listModelsErr:  fmt.Errorf("no models"),
	}

	var out bytes.Buffer
	err := runSetup(mock, fakePasswordReader(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Ollama doesn't need API key
	if mock.createdKey != "" {
		t.Errorf("Ollama should not have API key, got %q", mock.createdKey)
	}
	if !strings.Contains(out.String(), "configurado com sucesso") {
		t.Error("expected success message")
	}
}

func TestRunSetup_MasterPasswordError(t *testing.T) {
	cleanup := withStdin(t, "1\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:  true,
		hasMasterKey: false,
		setupPwdErr:  fmt.Errorf("encryption failed"),
	}

	readPwd := fakePasswordReader("senhasegura", "senhasegura")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err == nil {
		t.Fatal("expected error from SetupMasterPassword")
	}
	if !strings.Contains(err.Error(), "erro ao configurar senha mestre") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSetup_EmptyAPIKey(t *testing.T) {
	// Provider choice "1" (OpenAI) + Enter for recovery key
	cleanup := withStdin(t, "1\n\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:    true,
		hasMasterKey:   true, // skip password step
		testProviderOK: true,
	}

	// Return empty API key
	readPwd := fakePasswordReader("")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
	if !strings.Contains(err.Error(), "API key não pode ser vazia") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSetup_TestProviderFails_UserCancels(t *testing.T) {
	// Provider "1" + API key + user declines to continue after test failure
	cleanup := withStdin(t, "1\nn\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:     true,
		hasMasterKey:    true,
		testProviderOK:  false,
		testProviderErr: fmt.Errorf("connection refused"),
	}

	readPwd := fakePasswordReader("sk-test")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err == nil {
		t.Fatal("expected error when user cancels after test failure")
	}
	if !strings.Contains(err.Error(), "configuração cancelada") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "FALHOU") {
		t.Error("expected FALHOU in output")
	}
}

func TestRunSetup_TestProviderFails_UserContinues(t *testing.T) {
	// Provider "1" + test fails + user says "s" to continue + Enter for model
	cleanup := withStdin(t, "1\ns\n\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:     true,
		hasMasterKey:    true,
		testProviderOK:  false,
		testProviderErr: fmt.Errorf("timeout"),
		listModels:      []string{"gpt-4o"},
	}

	readPwd := fakePasswordReader("sk-key")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "configurado com sucesso") {
		t.Error("expected success despite test failure")
	}
}

func TestRunSetup_CreateProviderFails(t *testing.T) {
	cleanup := withStdin(t, "1\n\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:    true,
		hasMasterKey:   true,
		testProviderOK: true,
		listModelsErr:  fmt.Errorf("no models"),
		createErr:      fmt.Errorf("db write error"),
	}

	readPwd := fakePasswordReader("sk-key")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err == nil {
		t.Fatal("expected error from CreateDefaultLLMProvider")
	}
	if !strings.Contains(err.Error(), "erro ao criar provedor") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSetup_ModelSelectionByNumber(t *testing.T) {
	// Provider "1" + Enter for model "2" (selects second model)
	cleanup := withStdin(t, "1\n2\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:    true,
		hasMasterKey:   true,
		testProviderOK: true,
		listModels:     []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
	}

	readPwd := fakePasswordReader("sk-key")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.chatModelSet != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", mock.chatModelSet)
	}
}

func TestRunSetup_ModelSelectionByName(t *testing.T) {
	// Provider "1" + type model name directly
	cleanup := withStdin(t, "1\nmy-custom-model\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:    true,
		hasMasterKey:   true,
		testProviderOK: true,
		listModels:     []string{"gpt-4o"},
	}

	readPwd := fakePasswordReader("sk-key")
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.chatModelSet != "my-custom-model" {
		t.Errorf("expected model 'my-custom-model', got %q", mock.chatModelSet)
	}
}

func TestRunSetup_OllamaSkipsAPIKey(t *testing.T) {
	// Provider "13" (Ollama) + Enter for model
	cleanup := withStdin(t, "13\nllama3\n")
	defer cleanup()

	mock := &mockSetupBackend{
		needsWizard:    true,
		hasMasterKey:   true,
		testProviderOK: true,
		listModelsErr:  fmt.Errorf("no endpoint"),
	}

	readPwd := fakePasswordReader() // no password calls expected
	var out bytes.Buffer

	err := runSetup(mock, readPwd, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.createdKey != "" {
		t.Errorf("Ollama should not have API key, got %q", mock.createdKey)
	}
	if !strings.Contains(out.String(), "Ollama") {
		t.Error("expected Ollama in output")
	}
}
