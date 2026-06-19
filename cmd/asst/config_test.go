package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// ---------------------------------------------------------------------------
// Mock configBackend
// ---------------------------------------------------------------------------

type mockConfigBackend struct {
	profile    *profiles.Profile
	profileErr error
	providers  []*llm.ProviderConfig
	activeSlug string
	updateErr  error

	// Capture calls
	updatedSlug  string
	updatedModel string
}

func (m *mockConfigBackend) GetActiveProfile() (*profiles.Profile, error) {
	return m.profile, m.profileErr
}

func (m *mockConfigBackend) GetActiveProfileSlug() string {
	return m.activeSlug
}

func (m *mockConfigBackend) GetProfile(_ string) (*profiles.Profile, error) {
	return m.profile, m.profileErr
}

func (m *mockConfigBackend) GetLLMProviders() []*llm.ProviderConfig {
	return m.providers
}

func (m *mockConfigBackend) UpdateProfile(slug string, p profiles.Profile) error {
	m.updatedSlug = slug
	m.updatedModel = p.Chat.Model
	return m.updateErr
}

// ---------------------------------------------------------------------------
// runConfigShow
// ---------------------------------------------------------------------------

func TestConfigShow_Success(t *testing.T) {
	mock := &mockConfigBackend{
		profile: &profiles.Profile{
			Name: "Coder",
			Chat: profiles.ChatConfig{
				LLMProvider:     "openai-default",
				Model:           "gpt-4o",
				Temperature:     0.3,
				MaxTokens:       4096,
				ResponseTimeout: 60,
			},
		},
	}

	var out bytes.Buffer
	err := runConfigShow(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	for _, expected := range []string{"Coder", "openai-default", "gpt-4o", "0.3", "4096", "60"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got: %s", expected, output)
		}
	}
}

func TestConfigShow_Error(t *testing.T) {
	mock := &mockConfigBackend{
		profileErr: fmt.Errorf("no profile"),
	}

	var out bytes.Buffer
	err := runConfigShow(mock, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao obter perfil ativo") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runConfigProviders
// ---------------------------------------------------------------------------

func TestConfigProviders_Success(t *testing.T) {
	mock := &mockConfigBackend{
		providers: []*llm.ProviderConfig{
			{ID: "openai-default", Type: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com", IsDefault: true},
			{ID: "anthropic-1", Type: "claude", Name: "Anthropic", BaseURL: "https://api.anthropic.com"},
		},
	}

	var out bytes.Buffer
	err := runConfigProviders(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "openai-default") {
		t.Error("expected openai-default in output")
	}
	if !strings.Contains(output, "anthropic-1") {
		t.Error("expected anthropic-1 in output")
	}
	if !strings.Contains(output, "*") {
		t.Error("expected '*' marker for default provider")
	}
}

func TestConfigProviders_Empty(t *testing.T) {
	mock := &mockConfigBackend{
		providers: []*llm.ProviderConfig{},
	}

	var out bytes.Buffer
	err := runConfigProviders(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhum provedor LLM configurado") {
		t.Error("expected empty message")
	}
}

// ---------------------------------------------------------------------------
// runConfigModel
// ---------------------------------------------------------------------------

func TestConfigModel_Success(t *testing.T) {
	mock := &mockConfigBackend{
		profile:    &profiles.Profile{Chat: profiles.ChatConfig{Model: "gpt-4o"}},
		activeSlug: "padrao",
	}

	var out bytes.Buffer
	err := runConfigModel(mock, &out, "gpt-4-turbo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.updatedModel != "gpt-4-turbo" {
		t.Errorf("expected model 'gpt-4-turbo', got %q", mock.updatedModel)
	}
	if mock.updatedSlug != "padrao" {
		t.Errorf("expected slug 'padrao', got %q", mock.updatedSlug)
	}
	if !strings.Contains(out.String(), "gpt-4-turbo") {
		t.Error("expected success message")
	}
}

func TestConfigModel_Error(t *testing.T) {
	mock := &mockConfigBackend{
		profile:    &profiles.Profile{},
		activeSlug: "padrao",
		updateErr:  fmt.Errorf("invalid model"),
	}

	var out bytes.Buffer
	err := runConfigModel(mock, &out, "bad-model")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao definir modelo") {
		t.Errorf("unexpected error: %v", err)
	}
}
