package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMemorySkill(t *testing.T) {
	// Lê o skill memory do diretório do projeto (exe dir simulado)
	data, err := os.ReadFile("../../.assistente/skills/memory/SKILL.md")
	if err != nil {
		t.Fatalf("Falha ao ler SKILL.md: %v", err)
	}

	meta, content, err := Parse(string(data))
	if err != nil {
		t.Fatalf("Falha ao parsear SKILL.md: %v", err)
	}

	// === Required Fields ===
	if meta.Name != "memory-manager" {
		t.Errorf("Name esperado 'memory-manager', got %q", meta.Name)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Version esperado '1.0.0', got %q", meta.Version)
	}
	if meta.Description == "" {
		t.Error("Description não deve ser vazio")
	}

	// === Identity Fields ===
	if meta.DisplayName != "Memory Manager" {
		t.Errorf("DisplayName esperado 'Memory Manager', got %q", meta.DisplayName)
	}
	if meta.Author != "Assistente" {
		t.Errorf("Author esperado 'Assistente', got %q", meta.Author)
	}

	// === Categorization ===
	if meta.Type != "agent" {
		t.Errorf("Type esperado 'agent', got %q", meta.Type)
	}
	if meta.Category != "memory" {
		t.Errorf("Category esperado 'memory', got %q", meta.Category)
	}
	if meta.Difficulty != "beginner" {
		t.Errorf("Difficulty esperado 'beginner', got %q", meta.Difficulty)
	}

	// === AutoLoad ===
	if !meta.IsAutoLoad() {
		t.Error("IsAutoLoad() deveria retornar true")
	}

	// === Platforms ===
	if len(meta.Platforms) != 3 {
		t.Errorf("Platforms esperado 3, got %d", len(meta.Platforms))
	}

	// === Tools (structured format) ===
	if meta.Tools == nil {
		t.Fatal("Tools não deve ser nil")
	}
	if len(meta.Tools.Allowed) != 4 {
		t.Errorf("Tools.Allowed esperado 4, got %d: %v", len(meta.Tools.Allowed), meta.Tools.Allowed)
	}
	expectedTools := []string{"read_file", "write_file", "edit_file", "list_directory"}
	for i, expected := range expectedTools {
		if i < len(meta.Tools.Allowed) && meta.Tools.Allowed[i] != expected {
			t.Errorf("Tools.Allowed[%d] esperado %q, got %q", i, expected, meta.Tools.Allowed[i])
		}
	}

	// === Filesystem ===
	if meta.Filesystem == nil {
		t.Fatal("Filesystem não deve ser nil")
	}
	if len(meta.Filesystem.Read) != 1 {
		t.Errorf("Filesystem.Read esperado 1 entry, got %d", len(meta.Filesystem.Read))
	}
	if len(meta.Filesystem.Write) != 1 {
		t.Errorf("Filesystem.Write esperado 1 entry, got %d", len(meta.Filesystem.Write))
	}

	// === Behavior ===
	if meta.Behavior == nil {
		t.Fatal("Behavior não deve ser nil")
	}
	if meta.Behavior.Interactive == nil {
		t.Fatal("Behavior.Interactive não deve ser nil")
	}

	// === Output ===
	if meta.Output == nil {
		t.Fatal("Output não deve ser nil")
	}
	if meta.Output.Format != "markdown" {
		t.Errorf("Output.Format esperado 'markdown', got %q", meta.Output.Format)
	}

	// === Content ===
	if content == "" {
		t.Error("Content (corpo Markdown) não deve ser vazio")
	}

	// Verifica que o content tem as seções chave do sistema hierárquico
	requiredSections := []string{
		"## memory.md",
		"## daily/",
		"## Ciclo de Vida",
		"Rollup Semanal",
		"Rollup Mensal",
		"Rollup Anual",
		"## Checklist de Início de Conversa",
	}
	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			t.Errorf("Content deveria conter seção %q", section)
		}
	}

	// === Strict validation (deve passar pois está spec-compliant) ===
	if err := validateSpec(meta); err != nil {
		t.Errorf("validateSpec (strict) falhou: %v", err)
	}

	t.Logf("Skill parseado com sucesso: %s v%s (%s)", meta.Name, meta.Version, meta.GetDisplayName())
	t.Logf("Tools: %v", meta.GetToolsAllowed())
	t.Logf("Content: %d bytes", len(content))
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"memory-manager", "memory-manager"},
		{"Memory Manager", "memory-manager"},
		{"Código Review", "codigo-review"},
		{"test--skill", "test-skill"},
		{"  spaces  ", "spaces"},
	}

	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.expected {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseStrictValidation(t *testing.T) {
	// Skill sem version — deve falhar em strict
	raw := `---
name: test-skill
description: A test skill without version
---
Test content`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse (loose) deveria passar: %v", err)
	}

	// Strict deve falhar (sem version)
	if err := validateSpec(meta); err == nil {
		t.Error("validateSpec (strict) deveria falhar sem version")
	}
}

func TestStrictNameMax64(t *testing.T) {
	longName := "a-" + strings.Repeat("b", 63) // 65 chars
	raw := `---
name: ` + longName + `
version: 1.0.0
description: A skill with a very long name for testing purposes
---
Content`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse (loose) deveria passar: %v", err)
	}

	if err := validateSpec(meta); err == nil {
		t.Error("validateSpec (strict) deveria falhar com name > 64 chars")
	} else {
		t.Logf("Corretamente rejeitou name longo: %v", err)
	}
}

func TestStrictDescriptionMin10(t *testing.T) {
	raw := `---
name: short-desc
version: 1.0.0
description: Too short
---
Content`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse (loose) deveria passar: %v", err)
	}

	if err := validateSpec(meta); err == nil {
		t.Error("validateSpec (strict) deveria falhar com description < 10 chars")
	} else {
		t.Logf("Corretamente rejeitou description curta: %v", err)
	}
}

func TestMinMaxVersion(t *testing.T) {
	// minVersion válido
	raw := `---
name: versioned-skill
version: 1.0.0
description: A skill with version constraints for testing
minVersion: "1.0.0"
maxVersion: "2.0.0"
---
Content`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}
	if meta.MinVersion != "1.0.0" {
		t.Errorf("MinVersion esperado '1.0.0', got %q", meta.MinVersion)
	}
	if meta.MaxVersion != "2.0.0" {
		t.Errorf("MaxVersion esperado '2.0.0', got %q", meta.MaxVersion)
	}

	// minVersion inválido deve falhar
	rawBad := `---
name: bad-version
version: 1.0.0
description: A skill with invalid minVersion for testing
minVersion: "abc"
---
Content`

	_, _, err = Parse(rawBad)
	if err == nil {
		t.Error("Parse deveria falhar com minVersion inválido")
	} else {
		t.Logf("Corretamente rejeitou minVersion inválido: %v", err)
	}
}

func TestExpandTemplateVars(t *testing.T) {
	meta := &SkillMetadata{
		Filesystem: &FilesystemPermissions{
			Read:  []string{"${HOME}/.config/test/*", "${PROJECT_ROOT}/src/**"},
			Write: []string{"${TEMP}/output/*"},
			Deny:  []string{"${HOME}/.ssh/*"},
		},
	}

	meta.ExpandTemplateVars("/my/project")

	// ${HOME} deve ter sido expandido (não deve mais conter ${HOME})
	for _, p := range meta.Filesystem.Read {
		if strings.Contains(p, "${HOME}") || strings.Contains(p, "${PROJECT_ROOT}") {
			t.Errorf("Template var não expandida em Read: %q", p)
		}
	}
	for _, p := range meta.Filesystem.Write {
		if strings.Contains(p, "${TEMP}") {
			t.Errorf("Template var não expandida em Write: %q", p)
		}
	}
	for _, p := range meta.Filesystem.Deny {
		if strings.Contains(p, "${HOME}") {
			t.Errorf("Template var não expandida em Deny: %q", p)
		}
	}

	t.Logf("Read paths: %v", meta.Filesystem.Read)
	t.Logf("Write paths: %v", meta.Filesystem.Write)
	t.Logf("Deny paths: %v", meta.Filesystem.Deny)
}

func TestParseToolsLegacy(t *testing.T) {
	// Formato legado: tools como lista simples
	raw := `---
name: legacy-skill
version: 1.0.0
description: A legacy skill with simple tools list
tools:
  - read_file
  - write_file
---
Legacy content`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}

	if meta.Tools == nil {
		t.Fatal("Tools não deve ser nil após parse de lista legada")
	}
	if len(meta.Tools.Allowed) != 2 {
		t.Errorf("Tools.Allowed esperado 2, got %d", len(meta.Tools.Allowed))
	}
}

func TestAllowedToolsCommaSeparated(t *testing.T) {
	// Formato Claude Code oficial: allowed-tools como string comma-separated
	raw := `---
name: safe-reader
description: Read files without making any changes to them
allowed-tools: Read, Grep, Glob
---
Read-only skill content`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}

	if meta.Tools == nil {
		t.Fatal("Tools não deve ser nil após parse de allowed-tools string")
	}
	if len(meta.Tools.Allowed) != 3 {
		t.Errorf("Tools.Allowed esperado 3, got %d: %v", len(meta.Tools.Allowed), meta.Tools.Allowed)
	}
	expected := []string{"Read", "Grep", "Glob"}
	for i, exp := range expected {
		if i < len(meta.Tools.Allowed) && meta.Tools.Allowed[i] != exp {
			t.Errorf("Tools.Allowed[%d] esperado %q, got %q", i, exp, meta.Tools.Allowed[i])
		}
	}
	t.Logf("allowed-tools parsed: %v", meta.Tools.Allowed)
}

func TestDisableModelInvocation(t *testing.T) {
	raw := `---
name: deploy-skill
description: Deploy the application to production environment
disable-model-invocation: true
---
Deploy instructions`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}

	if !meta.DisableModelInvocation {
		t.Error("DisableModelInvocation deveria ser true")
	}
	if meta.IsModelInvocable() {
		t.Error("IsModelInvocable() deveria retornar false")
	}
	// Mesmo com auto_load, disable-model-invocation bloqueia
	meta.AutoLoad = true
	if meta.IsAutoLoad() {
		t.Error("IsAutoLoad() deveria retornar false quando disable-model-invocation=true")
	}
}

func TestUserInvocable(t *testing.T) {
	// user-invocable: false (background knowledge)
	raw := `---
name: legacy-context
description: Background knowledge about the legacy system architecture
user-invocable: false
---
Legacy system docs`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}

	if meta.IsUserInvocable() {
		t.Error("IsUserInvocable() deveria retornar false")
	}

	// Default (sem campo) = true
	rawDefault := `---
name: normal-skill
description: A normal skill without user-invocable field set
---
Normal content`

	metaDefault, _, err := Parse(rawDefault)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}
	if !metaDefault.IsUserInvocable() {
		t.Error("IsUserInvocable() deveria retornar true por default")
	}
}

func TestContextFork(t *testing.T) {
	raw := `---
name: research-skill
description: Research a topic thoroughly using explore subagent
context: fork
agent: Explore
---
Research instructions`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}

	if !meta.IsForkContext() {
		t.Error("IsForkContext() deveria retornar true")
	}
	if meta.Agent != "Explore" {
		t.Errorf("Agent esperado 'Explore', got %q", meta.Agent)
	}
	if meta.SkillContext != "fork" {
		t.Errorf("SkillContext esperado 'fork', got %q", meta.SkillContext)
	}
}

func TestContextForkValidation(t *testing.T) {
	// context inválido
	raw := `---
name: bad-context
description: A skill with invalid context value for testing
context: invalid
---
Content`

	_, _, err := Parse(raw)
	if err == nil {
		t.Error("Parse deveria falhar com context inválido")
	} else {
		t.Logf("Corretamente rejeitou context inválido: %v", err)
	}

	// agent sem context=fork
	rawAgent := `---
name: bad-agent
description: A skill with agent but no fork context value
agent: Explore
---
Content`

	_, _, err = Parse(rawAgent)
	if err == nil {
		t.Error("Parse deveria falhar com agent sem context=fork")
	} else {
		t.Logf("Corretamente rejeitou agent sem fork: %v", err)
	}
}

func TestArgumentHintAndModel(t *testing.T) {
	raw := `---
name: fix-issue
description: Fix a GitHub issue by number using standard coding practices
argument-hint: "[issue-number]"
model: claude-sonnet-4-20250514
disable-model-invocation: true
---
Fix GitHub issue $ARGUMENTS`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}

	if meta.ArgumentHint != "[issue-number]" {
		t.Errorf("ArgumentHint esperado '[issue-number]', got %q", meta.ArgumentHint)
	}
	if meta.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model esperado 'claude-sonnet-4-20250514', got %q", meta.Model)
	}
}

func TestHooksField(t *testing.T) {
	raw := `---
name: hooked-skill
description: A skill with hooks scoped to its lifecycle period
hooks:
  pre: "echo starting"
  post: "echo done"
---
Skill with hooks`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse falhou: %v", err)
	}

	if meta.Hooks == nil {
		t.Error("Hooks não deve ser nil")
	}
	t.Logf("Hooks: %v", meta.Hooks)
}

func TestGetUserInvocableSkills(t *testing.T) {
	// Cria skills no diretório de trabalho (workdir) que o resolver monitora
	testSkillsBase := filepath.Join(".assistente", "skills")

	// Skill 1: user-invocable (default true)
	skill1Dir := filepath.Join(testSkillsBase, "test-invocable-skill")
	if err := os.MkdirAll(skill1Dir, 0755); err != nil {
		t.Fatalf("Falha ao criar diretório: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(".assistente") })

	os.WriteFile(filepath.Join(skill1Dir, "SKILL.md"), []byte(`---
name: test-invocable-skill
version: "1.0.0"
description: A user invocable skill for testing
argument-hint: "[issue-number]"
---
Do something with $ARGUMENTS`), 0644)

	// Skill 2: user-invocable=false
	skill2Dir := filepath.Join(testSkillsBase, "test-hidden-skill")
	os.MkdirAll(skill2Dir, 0755)
	os.WriteFile(filepath.Join(skill2Dir, "SKILL.md"), []byte(`---
name: test-hidden-skill
version: "1.0.0"
description: A hidden skill for testing invocable
user-invocable: false
---
This is hidden from slash menu`), 0644)

	mgr := NewManager()
	invocable, err := mgr.GetUserInvocableSkills()
	if err != nil {
		t.Fatalf("GetUserInvocableSkills failed: %v", err)
	}

	// Deve encontrar test-invocable-skill mas NÃO test-hidden-skill
	foundInvocable := false
	for _, s := range invocable {
		if s.Slug == "test-hidden-skill" {
			t.Error("test-hidden-skill (user-invocable=false) should not appear")
		}
		if s.Slug == "test-invocable-skill" {
			foundInvocable = true
			if s.ArgumentHint != "[issue-number]" {
				t.Errorf("expected argument-hint '[issue-number]', got %q", s.ArgumentHint)
			}
		}
	}
	if !foundInvocable {
		t.Error("test-invocable-skill should appear in user invocable list")
	}
}

func TestGetSkillFiles(t *testing.T) {
	testSkillsBase := filepath.Join(".assistente", "skills")
	skillDir := filepath.Join(testSkillsBase, "test-files-skill")
	os.MkdirAll(filepath.Join(skillDir, "examples"), 0755)
	t.Cleanup(func() { os.RemoveAll(".assistente") })

	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test-files-skill
version: "1.0.0"
description: A skill with supporting files for testing
---
Main skill content`), 0644)
	os.WriteFile(filepath.Join(skillDir, "README.md"), []byte("# Readme"), 0644)
	os.WriteFile(filepath.Join(skillDir, "examples", "basic.md"), []byte("# Basic example"), 0644)

	mgr := NewManager()
	files, err := mgr.GetSkillFiles("test-files-skill")
	if err != nil {
		t.Fatalf("GetSkillFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 supporting files, got %d: %v", len(files), files)
	}

	// Deve conter README.md e examples/basic.md, mas NÃO SKILL.md
	for _, f := range files {
		if filepath.Base(f) == "SKILL.md" {
			t.Error("SKILL.md should be excluded from supporting files")
		}
	}
	t.Logf("Supporting files: %v", files)
}
