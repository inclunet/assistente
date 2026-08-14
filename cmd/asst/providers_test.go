package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/profiles"
)

// ---------------------------------------------------------------------------
// Mock providersBackend
// ---------------------------------------------------------------------------

type mockProvidersBackend struct {
	providersWithStatus []map[string]interface{}
	testOK              bool
	testErr             error
	models              []string
	modelsErr           error
	createDefaultErr    error
	createResult        map[string]interface{}
	createErr           error
	setDefaultErr       error
	updateProfileErr    error
	deleteErr           error

	// Capture calls
	testedReq         apidto.TestLLMProviderRequest
	createdDefault    string
	createdDefaultKey string
	createdReq        apidto.CreateLLMProviderRequest
	defaultProviderID string
	chatModelSet      string
	deletedID         string
}

func (m *mockProvidersBackend) GetLLMProvidersWithStatus() []map[string]interface{} {
	return m.providersWithStatus
}

func (m *mockProvidersBackend) TestLLMProvider(req apidto.TestLLMProviderRequest) (bool, error) {
	m.testedReq = req
	return m.testOK, m.testErr
}

func (m *mockProvidersBackend) ListModelsRaw(req apidto.TestLLMProviderRequest) ([]string, error) {
	return m.models, m.modelsErr
}

func (m *mockProvidersBackend) CreateDefaultLLMProvider(providerType, apiKey string) error {
	m.createdDefault = providerType
	m.createdDefaultKey = apiKey
	return m.createDefaultErr
}

func (m *mockProvidersBackend) CreateLLMProvider(req apidto.CreateLLMProviderRequest) (map[string]interface{}, error) {
	m.createdReq = req
	return m.createResult, m.createErr
}

func (m *mockProvidersBackend) SetDefaultProvider(id string) error {
	m.defaultProviderID = id
	return m.setDefaultErr
}

func (m *mockProvidersBackend) GetActiveProfileAndSlug() (*profiles.ActiveProfile, error) {
	return &profiles.ActiveProfile{Profile: &profiles.Profile{}, Slug: "padrao"}, nil
}

func (m *mockProvidersBackend) UpdateProfile(_ string, p profiles.Profile) error {
	m.chatModelSet = p.Chat.Model
	return m.updateProfileErr
}

func (m *mockProvidersBackend) DeleteLLMProvider(_ context.Context, id string) error {
	m.deletedID = id
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// runProvidersList
// ---------------------------------------------------------------------------

func TestProvidersList_Success(t *testing.T) {
	mock := &mockProvidersBackend{
		providersWithStatus: []map[string]interface{}{
			{"id": "openai-1", "type": "openai", "name": "OpenAI", "defaultModel": "gpt-4o", "status": "ok", "isDefault": true},
			{"id": "anthropic-1", "type": "claude", "name": "Anthropic", "defaultModel": "claude-3.5-sonnet", "status": "ok"},
		},
	}

	var out bytes.Buffer
	err := runProvidersList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "ID") {
		t.Error("expected header row")
	}
	if !strings.Contains(output, "openai-1") {
		t.Error("expected openai provider")
	}
	if !strings.Contains(output, "anthropic-1") {
		t.Error("expected anthropic provider")
	}
	if !strings.Contains(output, "*") {
		t.Error("expected '*' marker for default provider")
	}
}

func TestProvidersList_Empty(t *testing.T) {
	mock := &mockProvidersBackend{
		providersWithStatus: []map[string]interface{}{},
	}

	var out bytes.Buffer
	err := runProvidersList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhum provedor configurado") {
		t.Error("expected empty message")
	}
}

func TestProvidersList_MissingStatus(t *testing.T) {
	mock := &mockProvidersBackend{
		providersWithStatus: []map[string]interface{}{
			{"id": "p1", "type": "openai", "name": "Test"},
		},
	}

	var out bytes.Buffer
	err := runProvidersList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "-") {
		t.Error("expected '-' for missing status")
	}
}

// ---------------------------------------------------------------------------
// runProvidersTest
// ---------------------------------------------------------------------------

func TestProvidersTest_Success(t *testing.T) {
	mock := &mockProvidersBackend{testOK: true}

	var out bytes.Buffer
	err := runProvidersTest(mock, &out, "openai-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.testedReq.ProviderID != "openai-1" {
		t.Errorf("expected provider ID 'openai-1', got %q", mock.testedReq.ProviderID)
	}
	if !strings.Contains(out.String(), "OK") {
		t.Error("expected OK in output")
	}
}

func TestProvidersTest_Failure(t *testing.T) {
	mock := &mockProvidersBackend{testOK: false}

	var out bytes.Buffer
	err := runProvidersTest(mock, &out, "bad-provider")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out.String(), "FALHOU") {
		t.Error("expected FALHOU in output")
	}
}

func TestProvidersTest_Error(t *testing.T) {
	mock := &mockProvidersBackend{
		testErr: fmt.Errorf("network error"),
	}

	var out bytes.Buffer
	err := runProvidersTest(mock, &out, "err-provider")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out.String(), "FALHOU") {
		t.Error("expected FALHOU in output")
	}
}

// ---------------------------------------------------------------------------
// runProvidersModels
// ---------------------------------------------------------------------------

func TestProvidersModels_Success(t *testing.T) {
	mock := &mockProvidersBackend{
		models: []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
	}

	var out bytes.Buffer
	err := runProvidersModels(mock, &out, "openai-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "gpt-4o") {
		t.Error("expected gpt-4o")
	}
	if !strings.Contains(output, "gpt-4o-mini") {
		t.Error("expected gpt-4o-mini")
	}
}

func TestProvidersModels_Empty(t *testing.T) {
	mock := &mockProvidersBackend{
		models: []string{},
	}

	var out bytes.Buffer
	err := runProvidersModels(mock, &out, "empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhum modelo encontrado") {
		t.Error("expected empty message")
	}
}

func TestProvidersModels_Error(t *testing.T) {
	mock := &mockProvidersBackend{
		modelsErr: fmt.Errorf("api error"),
	}

	var out bytes.Buffer
	err := runProvidersModels(mock, &out, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao listar modelos") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProvidersDefault
// ---------------------------------------------------------------------------

func TestProvidersDefault_Success(t *testing.T) {
	mock := &mockProvidersBackend{}

	var out bytes.Buffer
	err := runProvidersDefault(mock, &out, "openai-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.defaultProviderID != "openai-1" {
		t.Errorf("expected provider ID 'openai-1', got %q", mock.defaultProviderID)
	}
	if !strings.Contains(out.String(), "padrão") {
		t.Error("expected success message")
	}
}

func TestProvidersDefault_Error(t *testing.T) {
	mock := &mockProvidersBackend{
		setDefaultErr: fmt.Errorf("not found"),
	}

	var out bytes.Buffer
	err := runProvidersDefault(mock, &out, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao definir provedor padrão") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProvidersRemove
// ---------------------------------------------------------------------------

func TestProvidersRemove_Success(t *testing.T) {
	mock := &mockProvidersBackend{}

	var out bytes.Buffer
	err := runProvidersRemove(mock, &out, "openai-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deletedID != "openai-1" {
		t.Errorf("expected deleted ID 'openai-1', got %q", mock.deletedID)
	}
	if !strings.Contains(out.String(), "removido") {
		t.Error("expected success message")
	}
}

func TestProvidersRemove_Error(t *testing.T) {
	mock := &mockProvidersBackend{
		deleteErr: fmt.Errorf("delete failed"),
	}

	var out bytes.Buffer
	err := runProvidersRemove(mock, &out, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao remover provedor") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProvidersAdd (wizard)
// ---------------------------------------------------------------------------

func TestProvidersAdd_OpenAI_Success(t *testing.T) {
	mock := &mockProvidersBackend{
		testOK:    true,
		models:    []string{"gpt-4o", "gpt-4o-mini"},
		modelsErr: nil,
	}

	// Simulate input: choose OpenAI (1), select model by Enter (default)
	input := "1\n\n"
	reader := bufio.NewReader(strings.NewReader(input))

	fakePwd := func(prompt string) (string, error) { return "sk-test-key", nil }

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.createdDefault != "openai" {
		t.Errorf("expected provider type 'openai', got %q", mock.createdDefault)
	}
	if mock.createdDefaultKey != "sk-test-key" {
		t.Errorf("expected API key 'sk-test-key', got %q", mock.createdDefaultKey)
	}
	if !strings.Contains(out.String(), "criado com sucesso") {
		t.Error("expected success message")
	}
}

func TestProvidersAdd_Ollama_NoAPIKey(t *testing.T) {
	// Ollama (Local) is choice 13 — does not require API key
	mock := &mockProvidersBackend{
		testOK:    true,
		models:    []string{"llama3", "codellama"},
		modelsErr: nil,
	}

	input := "13\n1\n"
	reader := bufio.NewReader(strings.NewReader(input))

	fakePwd := func(prompt string) (string, error) {
		t.Fatal("should not call readPassword for Ollama")
		return "", nil
	}

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.createdDefault == "" {
		t.Error("expected CreateDefaultLLMProvider to be called")
	}
}

func TestProvidersAdd_TestFailed_UserCancels(t *testing.T) {
	mock := &mockProvidersBackend{
		testOK:  false,
		testErr: fmt.Errorf("connection refused"),
	}

	// Choose OpenAI (1), then decline to continue (N)
	input := "1\nN\n"
	reader := bufio.NewReader(strings.NewReader(input))
	fakePwd := func(prompt string) (string, error) { return "sk-key", nil }

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err == nil {
		t.Fatal("expected error (cancelled)")
	}
	if !strings.Contains(err.Error(), "cancelado") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProvidersAdd_TestFailed_UserContinues(t *testing.T) {
	mock := &mockProvidersBackend{
		testOK:    false,
		testErr:   fmt.Errorf("timeout"),
		models:    []string{"gpt-4o"},
		modelsErr: nil,
	}

	// Choose OpenAI (1), continue despite failure (s), select default model
	input := "1\ns\n\n"
	reader := bufio.NewReader(strings.NewReader(input))
	fakePwd := func(prompt string) (string, error) { return "sk-key", nil }

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "criado com sucesso") {
		t.Error("expected success message")
	}
}

func TestProvidersAdd_EmptyAPIKey(t *testing.T) {
	mock := &mockProvidersBackend{}

	input := "1\n"
	reader := bufio.NewReader(strings.NewReader(input))
	fakePwd := func(prompt string) (string, error) { return "", nil }

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
	if !strings.Contains(err.Error(), "API key não pode ser vazia") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProvidersAdd_SelectModelByNumber(t *testing.T) {
	mock := &mockProvidersBackend{
		testOK: true,
		models: []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
	}

	// Choose OpenAI (1), select model 1 (gpt-4o) which differs from default gpt-4o-mini
	input := "1\n1\n"
	reader := bufio.NewReader(strings.NewReader(input))
	fakePwd := func(prompt string) (string, error) { return "sk-key", nil }

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default for OpenAI is gpt-4o-mini; user chose gpt-4o so SetChatModel should be called
	if mock.chatModelSet != "gpt-4o" {
		t.Errorf("expected chat model 'gpt-4o', got %q", mock.chatModelSet)
	}
}

func TestProvidersAdd_ModelApplyFailsWarns(t *testing.T) {
	mock := &mockProvidersBackend{
		testOK:           true,
		models:           []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"},
		updateProfileErr: fmt.Errorf("perfil corrompido"),
	}

	// OpenAI (1) + model 1 (gpt-4o) difere do default gpt-4o-mini → tenta aplicar
	input := "1\n1\n"
	reader := bufio.NewReader(strings.NewReader(input))
	fakePwd := func(prompt string) (string, error) { return "sk-key", nil }

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err != nil {
		t.Fatalf("provedor foi criado; comando não deveria falhar, got: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "não foi possível aplicar o modelo") {
		t.Errorf("esperava aviso de falha ao aplicar modelo, got: %s", output)
	}
	if !strings.Contains(output, "criado com sucesso") {
		t.Errorf("provedor foi criado; esperava mensagem de sucesso, got: %s", output)
	}
}

func TestProvidersAdd_CreateError(t *testing.T) {
	mock := &mockProvidersBackend{
		testOK:           true,
		models:           []string{"gpt-4o"},
		createDefaultErr: fmt.Errorf("storage error"),
	}

	input := "1\n\n"
	reader := bufio.NewReader(strings.NewReader(input))
	fakePwd := func(prompt string) (string, error) { return "sk-key", nil }

	var out bytes.Buffer
	err := runProvidersAdd(mock, &out, reader, fakePwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao criar provedor") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// str helper
// ---------------------------------------------------------------------------

func TestStr_NilValue(t *testing.T) {
	if str(nil) != "" {
		t.Error("expected empty string for nil")
	}
}

func TestStr_StringValue(t *testing.T) {
	if str("hello") != "hello" {
		t.Error("expected 'hello'")
	}
}

func TestStr_NonStringValue(t *testing.T) {
	if str(42) != "42" {
		t.Error("expected '42'")
	}
}
