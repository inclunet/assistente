package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/apidto"
)

// ---------------------------------------------------------------------------
// Mock providersBackend
// ---------------------------------------------------------------------------

type mockProvidersBackend struct {
	mockProviderSetup
	providersWithStatus []map[string]interface{}
	testOK              bool
	testErr             error
	models              []string
	modelsErr           error
	defaultProviderID   string
	setDefaultErr       error
	deleteErr           error

	// Capture calls
	testedReq apidto.TestLLMProviderRequest
	deletedID string
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
func (m *mockProvidersBackend) SetDefaultProvider(id string) error {
	m.defaultProviderID = id
	return m.setDefaultErr
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
