package skills

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const frontmatterDelimiter = "---"

var (
	// name: lowercase com hífens (kebab-case)
	namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	// version: semver X.Y.Z
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// Skill types válidos
var validTypes = map[string]bool{
	SkillTypeCommand: true,
	SkillTypeAgent:   true,
	SkillTypeHook:    true,
	SkillTypeMCP:     true,
}

// Difficulty levels válidos
var validDifficulties = map[string]bool{
	DifficultyBeginner:     true,
	DifficultyIntermediate: true,
	DifficultyAdvanced:     true,
}

// Parse extrai metadados (frontmatter YAML) e conteúdo (Markdown) de um SKILL.md.
// Valida campos obrigatórios conforme a especificação SKILL.md.
func Parse(raw string) (*SkillMetadata, string, error) {
	raw = strings.TrimSpace(raw)

	if !strings.HasPrefix(raw, frontmatterDelimiter) {
		return nil, "", fmt.Errorf("skill must start with YAML frontmatter (---)")
	}

	rest := raw[len(frontmatterDelimiter):]

	closingIndex := strings.Index(rest, "\n"+frontmatterDelimiter)
	if closingIndex == -1 {
		return nil, "", fmt.Errorf("closing frontmatter delimiter (---) not found")
	}

	frontmatterRaw := strings.TrimSpace(rest[:closingIndex])
	content := strings.TrimSpace(rest[closingIndex+len("\n"+frontmatterDelimiter):])

	var meta SkillMetadata
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &meta); err != nil {
		return nil, "", fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}

	// Resolve campos polimórficos (tools: lista vs objeto)
	meta.ResolveToolsRaw()

	// Validação tolerante na leitura (Postel's Law)
	if err := validateSpecLoose(&meta); err != nil {
		return nil, "", err
	}

	return &meta, content, nil
}

// validateSpec valida campos conforme a especificação SKILL.md.
// strict=true exige todos os campos obrigatórios (criação via UI).
// strict=false é tolerante para leitura de skills existentes (Postel's Law).
func validateSpec(meta *SkillMetadata) error {
	return validateSpecStrict(meta, true)
}

// validateSpecLoose valida de forma tolerante (leitura de skills existentes).
func validateSpecLoose(meta *SkillMetadata) error {
	return validateSpecStrict(meta, false)
}

func validateSpecStrict(meta *SkillMetadata, strict bool) error {
	// Name é sempre obrigatório
	if meta.Name == "" {
		return fmt.Errorf("'name' is required in frontmatter")
	}
	if strict {
		// Exige kebab-case
		if !namePattern.MatchString(meta.Name) {
			return fmt.Errorf("'name' must be kebab-case (lowercase, hyphens): got %q", meta.Name)
		}
		// Max 64 chars (spec)
		if len(meta.Name) > 64 {
			return fmt.Errorf("'name' must be at most 64 characters: got %d", len(meta.Name))
		}
	}

	// Version obrigatório em modo estrito
	if strict {
		if meta.Version == "" {
			return fmt.Errorf("'version' is required in frontmatter")
		}
		if !versionPattern.MatchString(meta.Version) {
			return fmt.Errorf("'version' must be semver (X.Y.Z): got %q", meta.Version)
		}
	}

	// Description obrigatório
	if meta.Description == "" {
		return fmt.Errorf("'description' is required in frontmatter")
	}
	if strict {
		if len(meta.Description) < 10 {
			return fmt.Errorf("'description' must be at least 10 characters: got %d", len(meta.Description))
		}
		if len(meta.Description) > 160 {
			return fmt.Errorf("'description' must be at most 160 characters: got %d", len(meta.Description))
		}
	}

	// === Optional field validation (aplicam sempre) ===

	// minVersion/maxVersion devem ser semver se presentes
	if meta.MinVersion != "" && !versionPattern.MatchString(meta.MinVersion) {
		return fmt.Errorf("'minVersion' must be semver (X.Y.Z): got %q", meta.MinVersion)
	}
	if meta.MaxVersion != "" && !versionPattern.MatchString(meta.MaxVersion) {
		return fmt.Errorf("'maxVersion' must be semver (X.Y.Z): got %q", meta.MaxVersion)
	}

	if meta.Type != "" && !validTypes[meta.Type] {
		return fmt.Errorf("'type' must be one of: command, agent, hook, mcp — got %q", meta.Type)
	}

	if meta.Difficulty != "" && !validDifficulties[meta.Difficulty] {
		return fmt.Errorf("'difficulty' must be one of: beginner, intermediate, advanced — got %q", meta.Difficulty)
	}

	if len(meta.Keywords) > 10 {
		return fmt.Errorf("'keywords' must have at most 10 entries: got %d", len(meta.Keywords))
	}

	if meta.Behavior != nil {
		if meta.Behavior.Timeout < 0 || meta.Behavior.Timeout > 3600 {
			return fmt.Errorf("'behavior.timeout' must be between 0 and 3600: got %d", meta.Behavior.Timeout)
		}
	}

	// context deve ser "fork" se presente
	if meta.SkillContext != "" && meta.SkillContext != "fork" {
		return fmt.Errorf("'context' must be 'fork' if set — got %q", meta.SkillContext)
	}

	// agent requer context=fork
	if meta.Agent != "" && meta.SkillContext != "fork" {
		return fmt.Errorf("'agent' requires 'context: fork'")
	}

	// Hook skills devem ter triggers
	if meta.Type == SkillTypeHook && meta.Triggers == nil {
		return fmt.Errorf("hook skills must have 'triggers' configuration")
	}

	// MCP skills devem ter mcp config
	if meta.Type == SkillTypeMCP && meta.MCP == nil {
		return fmt.Errorf("mcp skills must have 'mcp' configuration")
	}

	return nil
}

// Compose gera o conteúdo completo do SKILL.md (frontmatter + corpo Markdown).
func Compose(meta *SkillMetadata, content string) (string, error) {
	yamlBytes, err := yaml.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(frontmatterDelimiter)
	sb.WriteString("\n")
	sb.WriteString(strings.TrimSpace(string(yamlBytes)))
	sb.WriteString("\n")
	sb.WriteString(frontmatterDelimiter)
	sb.WriteString("\n")
	if content != "" {
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
