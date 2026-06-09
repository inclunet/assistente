package skills

import "testing"

func TestParseCatalogMetadata(t *testing.T) {
	raw := `---
name: deploy-helper
version: 1.0.0
description: Use when the user asks to deploy the application to production
auto_load: true
autoload_reason: required for every deploy-related conversation
context_budget: 800
requires_tools: true
requires_filesystem: true
requires_network: true
requires_mcp: true
---
Body`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if meta.ContextBudget != 800 {
		t.Errorf("ContextBudget: got %d want 800", meta.ContextBudget)
	}
	if meta.AutoloadReason != "required for every deploy-related conversation" {
		t.Errorf("AutoloadReason: got %q", meta.AutoloadReason)
	}
	if !meta.RequiresTools || !meta.RequiresFilesystem || !meta.RequiresNetwork || !meta.RequiresMCP {
		t.Errorf("requires_* flags não parseadas: %+v", meta)
	}
	if err := validateSpec(meta); err != nil {
		t.Errorf("validateSpec (strict) deveria passar: %v", err)
	}
}

func TestValidateAutoloadRequiresReason(t *testing.T) {
	raw := `---
name: noisy-skill
version: 1.0.0
description: Use when the user needs the noisy autoload skill behavior
auto_load: true
---
Body`

	meta, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse (loose) deveria passar: %v", err)
	}
	// Loose não exige reason (backward-compat).
	if err := validateSpecLoose(meta); err != nil {
		t.Errorf("validateSpecLoose não deveria falhar: %v", err)
	}
	// Strict exige autoload_reason.
	if err := validateSpec(meta); err == nil {
		t.Error("validateSpec (strict) deveria falhar: auto_load sem autoload_reason")
	}
}

func TestValidateContextBudgetNegative(t *testing.T) {
	meta := &SkillMetadata{
		Name:          "x",
		Version:       "1.0.0",
		Description:   "Use when testing negative budget validation here",
		ContextBudget: -5,
	}
	if err := validateSpecLoose(meta); err == nil {
		t.Error("context_budget negativo deveria falhar mesmo em loose")
	}
}

func TestEffectiveRequiresInferredFromConfig(t *testing.T) {
	meta := &SkillMetadata{
		Tools:      &ToolPermissions{Allowed: []string{"Read"}},
		Filesystem: &FilesystemPermissions{Read: []string{"/a"}},
		Network:    &NetworkPermissions{AllowedHosts: []string{"x.com"}},
		MCP:        &MCPConfig{Server: &MCPServerConfig{Command: "node"}},
	}
	if !meta.EffectiveRequiresTools() {
		t.Error("EffectiveRequiresTools deveria inferir de Tools config")
	}
	if !meta.EffectiveRequiresFilesystem() {
		t.Error("EffectiveRequiresFilesystem deveria inferir de Filesystem config")
	}
	if !meta.EffectiveRequiresNetwork() {
		t.Error("EffectiveRequiresNetwork deveria inferir de Network config")
	}
	if !meta.EffectiveRequiresMCP() {
		t.Error("EffectiveRequiresMCP deveria inferir de MCP config")
	}
	if !meta.RequiresAnyCapability() {
		t.Error("RequiresAnyCapability deveria ser true")
	}
}

func TestEffectiveRequiresExplicitFlag(t *testing.T) {
	// Sem config, mas com flag explícita.
	meta := &SkillMetadata{RequiresNetwork: true}
	if meta.EffectiveRequiresTools() {
		t.Error("EffectiveRequiresTools deveria ser false sem config nem flag")
	}
	if !meta.EffectiveRequiresNetwork() {
		t.Error("EffectiveRequiresNetwork deveria honrar a flag explícita")
	}
	if !meta.RequiresAnyCapability() {
		t.Error("RequiresAnyCapability deveria ser true com flag explícita")
	}
}

func TestEffectiveRequiresNoneWhenEmpty(t *testing.T) {
	meta := &SkillMetadata{Name: "plain", Version: "1.0.0"}
	if meta.RequiresAnyCapability() {
		t.Error("skill sem capacidades não deveria requerer nada")
	}
}

func TestValidateDescriptionQuality(t *testing.T) {
	// Boa descrição: 3ª pessoa + frase-gatilho.
	if w := ValidateDescriptionQuality("Use when the user asks to format Go code"); len(w) != 0 {
		t.Errorf("descrição boa não deveria gerar warnings: %+v", w)
	}

	// Sem frase-gatilho.
	w := ValidateDescriptionQuality("Formats Go source files according to gofmt")
	if !hasWarning(w, DescriptionWarnNoTrigger) {
		t.Errorf("esperava warning de no_trigger: %+v", w)
	}

	// Primeira pessoa.
	w = ValidateDescriptionQuality("I will format your Go code when you ask")
	if !hasWarning(w, DescriptionWarnFirstPerson) {
		t.Errorf("esperava warning de first_person: %+v", w)
	}

	// Muito curta.
	w = ValidateDescriptionQuality("fmt")
	if !hasWarning(w, DescriptionWarnTooShort) {
		t.Errorf("esperava warning de too_short: %+v", w)
	}

	// Vazia: sem warnings (presença é tratada por validateSpec).
	if w := ValidateDescriptionQuality("   "); len(w) != 0 {
		t.Errorf("descrição vazia não deveria gerar warnings: %+v", w)
	}
}

func hasWarning(ws []DescriptionWarning, code string) bool {
	for _, w := range ws {
		if w.Code == code {
			return true
		}
	}
	return false
}
