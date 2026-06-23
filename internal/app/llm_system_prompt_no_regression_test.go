package app

import (
	"testing"

	"assistente/internal/contextprovider"
	"assistente/internal/profiles"
)

func buildFullSystemPromptForTest(app *App, messages []Message, enabledSkills []string, disableSkills bool, disableOnDemand bool, skillTplData any, slashSkillContent string) []Message {
	return app.effectivePromptBuilder().BuildWithContextBlocks(
		messages,
		enabledSkills,
		disableSkills,
		disableOnDemand,
		skillTplData,
		slashSkillContent,
		[]contextprovider.Block{},
	)
}

// TestBuildFullSystemPrompt_DisableSkills_DefaultSystemMessage valida que
// skills desabilitados não removem a identidade base do assistente.
func TestBuildFullSystemPrompt_DisableSkills_DefaultSystemMessage(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "oi"},
	}

	app := &App{}
	app.skillMgr = nil // Sem skill manager

	// Simular: DisableSkills=true
	enabledSkills := []string{}

	result := buildFullSystemPromptForTest(app, messages, enabledSkills, true, false, nil, "")

	if len(result) != len(messages)+1 {
		t.Fatalf("expected system+user messages, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Fatalf("expected first message to be system, got %s", result[0].Role)
	}
	if !contains(result[0].Content.(string), "helpful, intelligent assistant") {
		t.Fatalf("expected default system prompt, got %s", result[0].Content)
	}
}

// TestBuildFullSystemPrompt_WithoutSkillManager_AddsDefaultSystemMessage valida que
// a ausência do skill manager não remove o prompt base.
func TestBuildFullSystemPrompt_WithoutSkillManager_AddsDefaultSystemMessage(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "oi"},
	}

	app := &App{}
	app.skillMgr = nil // Sem skill manager = nenhum skill

	enabledSkills := []string(nil)

	result := buildFullSystemPromptForTest(app, messages, enabledSkills, false, false, nil, "")

	if len(result) != len(messages)+1 {
		t.Fatalf("expected system+user messages, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Fatalf("expected system message, got %s", result[0].Role)
	}
}

// TestBuildFullSystemPrompt_WithSlashSkill_AddsSystemMessage valida que
// quando há slash skill, um system message é adicionado mesmo que skills desabilitadas
func TestBuildFullSystemPrompt_WithSlashSkill_AddsSystemMessage(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "oi"},
	}

	app := &App{}
	app.skillMgr = nil

	// Simular: skills desabilitados, MAS com slash skill
	enabledSkills := []string{}
	slashSkillContent := "# Skill invocado via /slash"

	result := buildFullSystemPromptForTest(app, messages, enabledSkills, true, false, nil, slashSkillContent)

	// Verificar: primeiro message é system (por causa do slash skill)
	if len(result) < 1 {
		t.Fatal("Expected at least one message")
	}

	if result[0].Role != "system" {
		t.Errorf("Expected first message to be 'system' when slash skill invoked, got '%s'", result[0].Role)
	}

	systemContent, ok := result[0].Content.(string)
	if !ok {
		t.Fatal("System content should be a string")
	}

	if systemContent == "" {
		t.Error("System message should not be empty when slash skill is invoked")
	}

	if !contains(systemContent, "Skill invocado via /slash") {
		t.Error("System message should contain slash skill content")
	}
}

// TestBuildFullSystemPrompt_ExistingSystemMessage_Combines valida que
// quando já existe system message, é preservado (não adicionado novamente)
func TestBuildFullSystemPrompt_ExistingSystemMessage_Combines(t *testing.T) {
	existingSystemContent := "Existing system message"
	messages := []Message{
		{Role: "system", Content: existingSystemContent},
		{Role: "user", Content: "oi"},
	}

	app := &App{}
	app.skillMgr = nil

	// Simular: skills desabilitados
	enabledSkills := []string{}

	result := buildFullSystemPromptForTest(app, messages, enabledSkills, true, false, nil, "")

	// Verificar: nenhum novo system message foi criado
	// O system message original é mantido
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}

	if result[0].Role != "system" {
		t.Errorf("Expected first message to be 'system', got '%s'", result[0].Role)
	}

	// Conteúdo do system message original é preservado junto do prompt base.
	systemContent := result[0].Content.(string)
	if !contains(systemContent, existingSystemContent) || !contains(systemContent, "helpful, intelligent assistant") {
		t.Errorf("System message should combine default and existing content, got: %s", systemContent)
	}
}

// TestBuildFullSystemPrompt_NoEmptySystemMessage valida a correção crítica:
// NUNCA deve enviar system message vazio para o Google
func TestBuildFullSystemPrompt_NoEmptySystemMessage(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "oi"},
	}

	app := &App{}
	app.skillMgr = nil

	// Cenário: DisableSkills=true, sem slash skill
	enabledSkills := []string{}

	result := buildFullSystemPromptForTest(app, messages, enabledSkills, true, false, nil, "")

	// Validação crítica: nenhum message com role=system E content=""
	for i, msg := range result {
		if msg.Role == "system" {
			content, ok := msg.Content.(string)
			if ok && content == "" {
				t.Errorf("REGRESSION: Empty system message at index %d (this breaks Google API)", i)
			}
		}
	}
}

// TestProfile_DisableSkills_Integration testa o fluxo completo com Profile
func TestProfile_DisableSkills_Integration(t *testing.T) {
	profile := &profiles.Profile{
		Name: "Gemini Test",
		Chat: profiles.ChatConfig{
			LLMProvider:   "google-gemini",
			Model:         "gemini-2.0-flash",
			DisableSkills: true, // Crítico!
			DisableTools:  true,
			EnabledSkills: []string{}, // Vazio por causa de DisableSkills
		},
	}

	app := &App{}
	app.skillMgr = nil

	messages := []Message{
		{Role: "user", Content: "oi"},
	}

	// Simular o que acontece quando DisableSkills=true
	enabledSkills := profile.Chat.EnabledSkills
	if profile.Chat.DisableSkills {
		enabledSkills = []string{}
	}

	result := buildFullSystemPromptForTest(app, messages, enabledSkills, profile.Chat.DisableSkills, profile.Chat.DisableOnDemandSkills, nil, "")

	if len(result) != 2 {
		t.Errorf("expected system+user messages, got %d", len(result))
	}

	if result[0].Role != "system" || result[1].Role != "user" {
		t.Errorf("expected system then user, got %+v", result)
	}

	for i, msg := range result {
		if content, ok := msg.Content.(string); ok {
			if content == "" {
				t.Errorf("Message at index %d has empty content", i)
			}
		}
	}
}

// TestBuildFullSystemPrompt_ConversationHistory testa com múltiplas mensagens
func TestBuildFullSystemPrompt_ConversationHistory(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "oi"},
		{Role: "assistant", Content: "olá!"},
		{Role: "user", Content: "como vai?"},
	}

	app := &App{}
	app.skillMgr = nil

	// DisableSkills=true, sem slash skill
	enabledSkills := []string{}

	result := buildFullSystemPromptForTest(app, messages, enabledSkills, true, false, nil, "")

	// Verificar: histórico preservado após o system prompt base.
	if len(result) != len(messages)+1 {
		t.Errorf("Expected %d messages, got %d", len(messages)+1, len(result))
	}

	expectedRoles := []string{"system", "user", "assistant", "user"}
	for i, expectedRole := range expectedRoles {
		if result[i].Role != expectedRole {
			t.Errorf("Message %d: expected role '%s', got '%s'", i, expectedRole, result[i].Role)
		}
	}
}

// Helper function
func contains(s, substring string) bool {
	for i := range s {
		if i+len(substring) <= len(s) && s[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
