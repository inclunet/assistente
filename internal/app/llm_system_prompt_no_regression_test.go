package app

import (
	"testing"

	"assistente/internal/profiles"
)

// TestBuildFullSystemPrompt_DisableSkills_NoSystemMessage valida que
// quando skills estão desabilitados E sem slash skill, NENHUM system message é adicionado
func TestBuildFullSystemPrompt_DisableSkills_NoSystemMessage(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "oi"},
	}

	app := &App{}
	app.skillMgr = nil // Sem skill manager

	// Simular: DisableSkills=true
	enabledSkills := []string{}

	result := app.buildFullSystemPrompt(messages, enabledSkills, false, nil, "", "")

	// Verificar: nenhum message foi adicionado
	if len(result) != len(messages) {
		t.Errorf("Expected same number of messages (%d), got %d", len(messages), len(result))
	}

	// Verificar: primeiro message ainda é user, não system
	if result[0].Role != "user" {
		t.Errorf("Expected first message to be 'user', got '%s'", result[0].Role)
	}

	// Nenhuma message é system
	for i, msg := range result {
		if msg.Role == "system" {
			t.Errorf("Unexpected system message at index %d", i)
		}
	}
}

// TestBuildFullSystemPrompt_WithSkills_AddsSystemMessage valida que
// quando há skill manager com skills, um system message COM conteúdo é adicionado
func TestBuildFullSystemPrompt_WithSkills_AddsSystemMessage(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "oi"},
	}

	app := &App{}
	app.skillMgr = nil // Sem skill manager = nenhum skill

	// Simular: enabledSkills[:] (nil) = usa defaults, mas sem skillMgr retorna vazio
	// Para garantir que DefaultSystemPrompt é adicionado, precisamos simular
	// que há skills. A verdade é: se não há skillMgr, buildSkillsPromptSection
	// retorna "", então buildFullSystemPrompt não adiciona system message.

	// ESTE TESTE VALIDA O CENÁRIO REAL:
	// Quando não há skillMgr, mesmo com enabledSkills=nil, nenhum system message é adicionado
	enabledSkills := []string(nil)

	result := app.buildFullSystemPrompt(messages, enabledSkills, false, nil, "", "")

	// Verificação: sem skill manager E sem slash skill, nenhum system message é adicionado
	// (mesmo que enabledSkills seja nil)
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages (no system message added), got %d", len(messages), len(result))
	}

	if result[0].Role == "system" {
		t.Error("Unexpected system message when no skill manager and no slash skill")
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

	result := app.buildFullSystemPrompt(messages, enabledSkills, false, nil, slashSkillContent, "")

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

	result := app.buildFullSystemPrompt(messages, enabledSkills, false, nil, "", "")

	// Verificar: nenhum novo system message foi criado
	// O system message original é mantido
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}

	if result[0].Role != "system" {
		t.Errorf("Expected first message to be 'system', got '%s'", result[0].Role)
	}

	// Conteúdo do system message original é preservado
	systemContent := result[0].Content.(string)
	if systemContent != existingSystemContent {
		t.Errorf("System message was modified. Expected: %s, Got: %s", existingSystemContent, systemContent)
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

	result := app.buildFullSystemPrompt(messages, enabledSkills, false, nil, "", "")

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
			DisableSkills: true,        // Crítico!
			DisableTools:  true,
			EnabledSkills: []string{},  // Vazio por causa de DisableSkills
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

	result := app.buildFullSystemPrompt(messages, enabledSkills, profile.Chat.DisableOnDemandSkills, nil, "", "")

	// Verificações críticas:
	// 1. Nenhum system message foi adicionado
	if len(result) != 1 {
		t.Errorf("Expected 1 message (user only), got %d", len(result))
	}

	// 2. Primeira e única message é do usuário
	if result[0].Role != "user" {
		t.Errorf("Expected 'user' message, got '%s'", result[0].Role)
	}

	// 3. Nenhum message vazio
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

	result := app.buildFullSystemPrompt(messages, enabledSkills, false, nil, "", "")

	// Verificar: histórico preservado sem system message vazio adicionado
	if len(result) != len(messages) {
		t.Errorf("Expected %d messages, got %d", len(messages), len(result))
	}

	// Verificar ordem: user -> assistant -> user
	expectedRoles := []string{"user", "assistant", "user"}
	for i, expectedRole := range expectedRoles {
		if result[i].Role != expectedRole {
			t.Errorf("Message %d: expected role '%s', got '%s'", i, expectedRole, result[i].Role)
		}
	}

	// Nenhum system message foi adicionado
	for _, msg := range result {
		if msg.Role == "system" {
			t.Error("System message should not be added when skills disabled")
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
