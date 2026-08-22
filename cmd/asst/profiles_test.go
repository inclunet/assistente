package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/profiles"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Mock profilesBackend
// ---------------------------------------------------------------------------

type mockProfilesBackend struct {
	profiles      []profiles.ProfileInfo
	profilesErr   error
	activeSlug    string
	profile       *profiles.Profile
	profileErr    error
	activateErr   error
	createSlug    string
	createErr     error
	updateErr     error
	duplicateSlug string
	duplicateErr  error
	deleteErr     error

	// Capture calls
	activatedSlug  string
	createdProfile *profiles.Profile
	updatedSlug    string
	updatedProfile *profiles.Profile
	deletedSlug    string
	duplicatedSlug string
}

func (m *mockProfilesBackend) GetProfiles() ([]profiles.ProfileInfo, error) {
	return m.profiles, m.profilesErr
}

func (m *mockProfilesBackend) GetActiveProfileSlug() string {
	return m.activeSlug
}

func (m *mockProfilesBackend) GetProfile(slug string) (*profiles.Profile, error) {
	if m.profileErr != nil {
		return nil, m.profileErr
	}
	if m.profile != nil {
		return m.profile, nil
	}
	// Return a basic profile by default
	return &profiles.Profile{
		Name:        "Test",
		Description: "Perfil de teste",
		Chat: profiles.ChatConfig{
			Temperature: 0.7,
		},
	}, nil
}

func (m *mockProfilesBackend) SetActiveProfile(slug string) error {
	m.activatedSlug = slug
	return m.activateErr
}

func (m *mockProfilesBackend) CreateProfile(p profiles.Profile) (string, error) {
	m.createdProfile = &p
	return m.createSlug, m.createErr
}

func (m *mockProfilesBackend) UpdateProfile(slug string, p profiles.Profile) error {
	m.updatedSlug = slug
	m.updatedProfile = &p
	return m.updateErr
}

func (m *mockProfilesBackend) DuplicateProfile(slug string) (string, error) {
	m.duplicatedSlug = slug
	return m.duplicateSlug, m.duplicateErr
}

func (m *mockProfilesBackend) DeleteProfile(slug string) error {
	m.deletedSlug = slug
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// runProfilesList
// ---------------------------------------------------------------------------

func TestProfilesList_Success(t *testing.T) {
	mock := &mockProfilesBackend{
		activeSlug: "coder",
		profiles: []profiles.ProfileInfo{
			{Slug: "padrao", Name: "Padrão", Source: "exe"},
			{Slug: "coder", Name: "Coder", Source: "home"},
		},
	}

	var out bytes.Buffer
	err := runProfilesList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "SLUG") {
		t.Error("expected header row")
	}
	if !strings.Contains(output, "padrao") {
		t.Error("expected 'padrao' profile in output")
	}
	if !strings.Contains(output, "coder") {
		t.Error("expected 'coder' profile in output")
	}
	if !strings.Contains(output, "*") {
		t.Error("expected '*' marker for active profile")
	}
}

func TestProfilesList_Empty(t *testing.T) {
	mock := &mockProfilesBackend{
		profiles: []profiles.ProfileInfo{},
	}

	var out bytes.Buffer
	err := runProfilesList(mock, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "SLUG") {
		t.Error("expected header even with no profiles")
	}
}

func TestProfilesList_Error(t *testing.T) {
	mock := &mockProfilesBackend{
		profilesErr: fmt.Errorf("db error"),
	}

	var out bytes.Buffer
	err := runProfilesList(mock, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao listar perfis") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProfilesShow
// ---------------------------------------------------------------------------

func TestProfilesShow_Success(t *testing.T) {
	mock := &mockProfilesBackend{
		profile: &profiles.Profile{
			Name:        "Coder",
			Description: "Perfil para programação",
			Icon:        "💻",
			Chat: profiles.ChatConfig{
				LLMProvider:  "openai-default",
				Model:        "gpt-4o",
				Temperature:  0.3,
				MaxTokens:    4096,
				EnabledTools: []string{"http_request", "code_exec"},
			},
		},
	}

	var out bytes.Buffer
	err := runProfilesShow(mock, &out, "coder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	for _, expected := range []string{"Coder", "programação", "💻", "openai-default", "gpt-4o", "0.3", "4096", "http_request"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got: %s", expected, output)
		}
	}
}

func TestProfilesShow_ExibeToolPolicyEToolPolicyDefault(t *testing.T) {
	mock := &mockProfilesBackend{
		profile: &profiles.Profile{
			Name: "Programação",
			Chat: profiles.ChatConfig{
				Temperature:       0,
				ToolPolicyDefault: "on_demand",
				ToolPolicy: map[string]string{
					"write_file": "preloaded",
					"read_file":  "preloaded",
				},
			},
		},
	}

	var out bytes.Buffer
	if err := runProfilesShow(mock, &out, "programacao"); err != nil {
		t.Fatalf("runProfilesShow: %v", err)
	}

	output := out.String()
	for _, expected := range []string{
		"Tools default: on_demand",
		"Tool read_file:",
		"preloaded",
		"Tool write_file:",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got: %s", expected, output)
		}
	}
	if strings.Index(output, "read_file") > strings.Index(output, "write_file") {
		t.Errorf("tool policy should be sorted, got: %s", output)
	}
}

func TestProfilesShow_MinimalProfile(t *testing.T) {
	mock := &mockProfilesBackend{
		profile: &profiles.Profile{
			Name:        "Simples",
			Description: "Sem extras",
			Chat: profiles.ChatConfig{
				Temperature: 0.7,
			},
		},
	}

	var out bytes.Buffer
	err := runProfilesShow(mock, &out, "simples")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	// Should NOT show Provider, Model, MaxTokens, Tools lines
	if strings.Contains(output, "Provider:") {
		t.Error("should not show Provider when empty")
	}
	if strings.Contains(output, "Modelo:") {
		t.Error("should not show Modelo when empty")
	}
	if strings.Contains(output, "Max Tokens:") {
		t.Error("should not show Max Tokens when 0")
	}
	if strings.Contains(output, "Tools:") {
		t.Error("should not show Tools when empty")
	}
}

func TestProfilesShow_NotFound(t *testing.T) {
	mock := &mockProfilesBackend{
		profileErr: fmt.Errorf("not found"),
	}

	var out bytes.Buffer
	err := runProfilesShow(mock, &out, "inexistente")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "perfil 'inexistente' não encontrado") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProfilesActivate
// ---------------------------------------------------------------------------

func TestProfilesActivate_Success(t *testing.T) {
	mock := &mockProfilesBackend{}

	var out bytes.Buffer
	err := runProfilesActivate(mock, &out, "coder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.activatedSlug != "coder" {
		t.Errorf("expected activated slug 'coder', got %q", mock.activatedSlug)
	}
	if !strings.Contains(out.String(), "ativado") {
		t.Error("expected success message")
	}
}

func TestProfilesActivate_Error(t *testing.T) {
	mock := &mockProfilesBackend{
		activateErr: fmt.Errorf("not found"),
	}

	var out bytes.Buffer
	err := runProfilesActivate(mock, &out, "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao ativar perfil 'nope'") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProfilesCreate
// ---------------------------------------------------------------------------

func TestProfilesCreate_Success(t *testing.T) {
	mock := &mockProfilesBackend{createSlug: "meu-perfil"}
	defer func() { profileCreateName = "" }()

	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileCreateName, "name", "", "")
	cmd.Flags().StringVar(&profileCreateModel, "model", "", "")
	cmd.Flags().StringVar(&profileCreateProvider, "provider", "", "")
	cmd.Flags().Float64Var(&profileCreateTemp, "temperature", 0.7, "")
	_ = cmd.Flags().Set("name", "Meu Perfil")

	var out bytes.Buffer
	err := runProfilesCreate(mock, &out, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.createdProfile == nil {
		t.Fatal("expected profile to be created")
	}
	if mock.createdProfile.Name != "Meu Perfil" {
		t.Errorf("expected name 'Meu Perfil', got %q", mock.createdProfile.Name)
	}
	if !strings.Contains(out.String(), "meu-perfil") {
		t.Error("expected slug in output")
	}
}

func TestProfilesCreate_MissingName(t *testing.T) {
	profileCreateName = ""
	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileCreateName, "name", "", "")

	var out bytes.Buffer
	err := runProfilesCreate(&mockProfilesBackend{}, &out, cmd)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "--name é obrigatório") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProfilesCreate_WithFlags(t *testing.T) {
	mock := &mockProfilesBackend{createSlug: "coder"}
	defer func() {
		profileCreateName = ""
		profileCreateModel = ""
		profileCreateProvider = ""
		profileCreateTemp = 0.7
	}()

	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileCreateName, "name", "", "")
	cmd.Flags().StringVar(&profileCreateModel, "model", "", "")
	cmd.Flags().StringVar(&profileCreateProvider, "provider", "", "")
	cmd.Flags().Float64Var(&profileCreateTemp, "temperature", 0.7, "")
	_ = cmd.Flags().Set("name", "Coder")
	_ = cmd.Flags().Set("model", "gpt-4o")
	_ = cmd.Flags().Set("provider", "openai-default")
	_ = cmd.Flags().Set("temperature", "0.3")

	var out bytes.Buffer
	err := runProfilesCreate(mock, &out, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.createdProfile.Chat.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %q", mock.createdProfile.Chat.Model)
	}
	if mock.createdProfile.Chat.LLMProvider != "openai-default" {
		t.Errorf("expected provider openai-default, got %q", mock.createdProfile.Chat.LLMProvider)
	}
	if mock.createdProfile.Chat.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %f", mock.createdProfile.Chat.Temperature)
	}
}

func TestProfilesCreate_BackendError(t *testing.T) {
	mock := &mockProfilesBackend{createErr: fmt.Errorf("slug conflict")}
	defer func() { profileCreateName = "" }()

	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileCreateName, "name", "", "")
	_ = cmd.Flags().Set("name", "Duplicado")

	var out bytes.Buffer
	err := runProfilesCreate(mock, &out, cmd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao criar perfil") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProfilesEdit
// ---------------------------------------------------------------------------

func TestProfilesEdit_Success(t *testing.T) {
	mock := &mockProfilesBackend{
		profile: &profiles.Profile{
			Name: "Original",
			Chat: profiles.ChatConfig{
				Model:       "gpt-3.5",
				Temperature: 0.7,
			},
		},
	}
	defer func() {
		profileEditName = ""
		profileEditModel = ""
		profileEditProvider = ""
		profileEditTemp = 0
	}()

	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileEditName, "name", "", "")
	cmd.Flags().StringVar(&profileEditModel, "model", "", "")
	cmd.Flags().StringVar(&profileEditProvider, "provider", "", "")
	cmd.Flags().Float64Var(&profileEditTemp, "temperature", 0, "")
	_ = cmd.Flags().Set("name", "Novo Nome")
	_ = cmd.Flags().Set("model", "gpt-4o")

	var out bytes.Buffer
	err := runProfilesEdit(mock, &out, cmd, "original")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.updatedSlug != "original" {
		t.Errorf("expected update slug 'original', got %q", mock.updatedSlug)
	}
	if mock.updatedProfile.Name != "Novo Nome" {
		t.Errorf("expected updated name 'Novo Nome', got %q", mock.updatedProfile.Name)
	}
	if mock.updatedProfile.Chat.Model != "gpt-4o" {
		t.Errorf("expected updated model 'gpt-4o', got %q", mock.updatedProfile.Chat.Model)
	}
	if !strings.Contains(out.String(), "atualizado") {
		t.Error("expected success message")
	}
}

func TestProfilesEdit_TemperatureChanged(t *testing.T) {
	mock := &mockProfilesBackend{
		profile: &profiles.Profile{
			Name: "Test",
			Chat: profiles.ChatConfig{Temperature: 0.7},
		},
	}
	profileEditName = ""
	profileEditModel = ""
	profileEditProvider = ""
	profileEditTemp = 0.2
	defer func() { profileEditTemp = 0 }()

	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileEditName, "name", "", "")
	cmd.Flags().StringVar(&profileEditModel, "model", "", "")
	cmd.Flags().StringVar(&profileEditProvider, "provider", "", "")
	cmd.Flags().Float64Var(&profileEditTemp, "temperature", 0, "")
	_ = cmd.Flags().Set("temperature", "0.2")

	var out bytes.Buffer
	err := runProfilesEdit(mock, &out, cmd, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.updatedProfile.Chat.Temperature != 0.2 {
		t.Errorf("expected temperature 0.2, got %f", mock.updatedProfile.Chat.Temperature)
	}
}

func TestProfilesEdit_NotFound(t *testing.T) {
	mock := &mockProfilesBackend{profileErr: fmt.Errorf("not found")}

	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileEditName, "name", "", "")
	cmd.Flags().Float64Var(&profileEditTemp, "temperature", 0, "")

	var out bytes.Buffer
	err := runProfilesEdit(mock, &out, cmd, "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "perfil 'nope' não encontrado") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProfilesEdit_UpdateError(t *testing.T) {
	mock := &mockProfilesBackend{
		updateErr: fmt.Errorf("read-only"),
	}

	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileEditName, "name", "", "")
	cmd.Flags().StringVar(&profileEditModel, "model", "", "")
	cmd.Flags().StringVar(&profileEditProvider, "provider", "", "")
	cmd.Flags().Float64Var(&profileEditTemp, "temperature", 0, "")

	var out bytes.Buffer
	err := runProfilesEdit(mock, &out, cmd, "builtin")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao atualizar perfil") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProfilesDuplicate
// ---------------------------------------------------------------------------

func TestProfilesDuplicate_Success(t *testing.T) {
	mock := &mockProfilesBackend{duplicateSlug: "coder-copy"}

	var out bytes.Buffer
	err := runProfilesDuplicate(mock, &out, "coder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.duplicatedSlug != "coder" {
		t.Errorf("expected duplicated slug 'coder', got %q", mock.duplicatedSlug)
	}
	if !strings.Contains(out.String(), "coder-copy") {
		t.Error("expected new slug in output")
	}
}

func TestProfilesDuplicate_Error(t *testing.T) {
	mock := &mockProfilesBackend{duplicateErr: fmt.Errorf("not found")}

	var out bytes.Buffer
	err := runProfilesDuplicate(mock, &out, "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao duplicar perfil 'nope'") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runProfilesDelete
// ---------------------------------------------------------------------------

func TestProfilesDelete_Success(t *testing.T) {
	mock := &mockProfilesBackend{}

	var out bytes.Buffer
	err := runProfilesDelete(mock, &out, "velho")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deletedSlug != "velho" {
		t.Errorf("expected deleted slug 'velho', got %q", mock.deletedSlug)
	}
	if !strings.Contains(out.String(), "removido") {
		t.Error("expected success message")
	}
}

func TestProfilesDelete_Error(t *testing.T) {
	mock := &mockProfilesBackend{deleteErr: fmt.Errorf("builtin")}

	var out bytes.Buffer
	err := runProfilesDelete(mock, &out, "padrao")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao remover perfil 'padrao'") {
		t.Errorf("unexpected error: %v", err)
	}
}
