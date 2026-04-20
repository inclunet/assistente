package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"assistente/controllers"
)

// ---------------------------------------------------------------------------
// Mock credentialsBackend
// ---------------------------------------------------------------------------

type mockCredentialsBackend struct {
	credentials []controllers.CredentialSummary
	listErr     error
	upsertErr   error
	deleteErr   error

	// Capture calls
	upsertedInput controllers.CredentialInput
	deletedPattern string
}

func (m *mockCredentialsBackend) ListCredentials() ([]controllers.CredentialSummary, error) {
	return m.credentials, m.listErr
}

func (m *mockCredentialsBackend) UpsertCredential(input controllers.CredentialInput) error {
	m.upsertedInput = input
	return m.upsertErr
}

func (m *mockCredentialsBackend) DeleteCredential(pattern string) error {
	m.deletedPattern = pattern
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// runCredentialsList
// ---------------------------------------------------------------------------

func TestCredentialsList_Success(t *testing.T) {
	mock := &mockCredentialsBackend{
		credentials: []controllers.CredentialSummary{
			{Pattern: "api.openai.com", Type: "bearer", Masked: "sk-...abc", Managed: true},
			{Pattern: "api.anthropic.com", Type: "bearer", Masked: "sk-...xyz"},
		},
	}

	var out bytes.Buffer
	err := runCredentialsList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "PADRÃO") {
		t.Error("expected header row")
	}
	if !strings.Contains(output, "api.openai.com") {
		t.Error("expected openai credential")
	}
	if !strings.Contains(output, "api.anthropic.com") {
		t.Error("expected anthropic credential")
	}
	if !strings.Contains(output, "sim") {
		t.Error("expected 'sim' for managed credential")
	}
}

func TestCredentialsList_Empty(t *testing.T) {
	mock := &mockCredentialsBackend{
		credentials: []controllers.CredentialSummary{},
	}

	var out bytes.Buffer
	err := runCredentialsList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhuma credencial registrada") {
		t.Error("expected empty message")
	}
}

func TestCredentialsList_Error(t *testing.T) {
	mock := &mockCredentialsBackend{
		listErr: fmt.Errorf("db error"),
	}

	var out bytes.Buffer
	err := runCredentialsList(mock, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao listar credenciais") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runCredentialsSet
// ---------------------------------------------------------------------------

func TestCredentialsSet_WithValue(t *testing.T) {
	mock := &mockCredentialsBackend{}
	fakePwd := func(prompt string) (string, error) { return "", nil }

	var out bytes.Buffer
	err := runCredentialsSet(mock, &out, "api.openai.com", "sk-test123", "bearer", fakePwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.upsertedInput.Pattern != "api.openai.com" {
		t.Errorf("expected pattern 'api.openai.com', got %q", mock.upsertedInput.Pattern)
	}
	if mock.upsertedInput.Token != "sk-test123" {
		t.Errorf("expected token 'sk-test123', got %q", mock.upsertedInput.Token)
	}
	if mock.upsertedInput.Type != "bearer" {
		t.Errorf("expected type 'bearer', got %q", mock.upsertedInput.Type)
	}
	if !strings.Contains(out.String(), "salva") {
		t.Error("expected success message")
	}
}

func TestCredentialsSet_DefaultType(t *testing.T) {
	mock := &mockCredentialsBackend{}
	fakePwd := func(prompt string) (string, error) { return "", nil }

	var out bytes.Buffer
	err := runCredentialsSet(mock, &out, "api.test.com", "sk-val", "", fakePwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.upsertedInput.Type != "bearer" {
		t.Errorf("expected default type 'bearer', got %q", mock.upsertedInput.Type)
	}
}

func TestCredentialsSet_UpsertError(t *testing.T) {
	mock := &mockCredentialsBackend{
		upsertErr: fmt.Errorf("storage error"),
	}
	fakePwd := func(prompt string) (string, error) { return "", nil }

	var out bytes.Buffer
	err := runCredentialsSet(mock, &out, "api.test.com", "sk-val", "bearer", fakePwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao salvar credencial") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCredentialsSet_EmptyValueError(t *testing.T) {
	mock := &mockCredentialsBackend{}
	// readPassword also returns empty — should fail
	fakePwd := func(prompt string) (string, error) { return "", nil }

	var out bytes.Buffer
	// Pass empty value; stdin is a terminal (no pipe), so it will try readPassword which returns empty
	err := runCredentialsSet(mock, &out, "api.test.com", "", "bearer", fakePwd)
	if err == nil {
		t.Fatal("expected error for empty value")
	}
	if !strings.Contains(err.Error(), "valor não pode ser vazio") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCredentialsSet_PasswordReaderError(t *testing.T) {
	mock := &mockCredentialsBackend{}
	fakePwd := func(prompt string) (string, error) { return "", fmt.Errorf("terminal error") }

	var out bytes.Buffer
	err := runCredentialsSet(mock, &out, "api.test.com", "", "bearer", fakePwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao ler valor") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runCredentialsRemove
// ---------------------------------------------------------------------------

func TestCredentialsRemove_Success(t *testing.T) {
	mock := &mockCredentialsBackend{}

	var out bytes.Buffer
	err := runCredentialsRemove(mock, &out, "api.openai.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deletedPattern != "api.openai.com" {
		t.Errorf("expected deleted pattern 'api.openai.com', got %q", mock.deletedPattern)
	}
	if !strings.Contains(out.String(), "removida") {
		t.Error("expected success message")
	}
}

func TestCredentialsRemove_Error(t *testing.T) {
	mock := &mockCredentialsBackend{
		deleteErr: fmt.Errorf("not found"),
	}

	var out bytes.Buffer
	err := runCredentialsRemove(mock, &out, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao remover credencial") {
		t.Errorf("unexpected error: %v", err)
	}
}
