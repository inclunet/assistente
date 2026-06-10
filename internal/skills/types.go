package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// SkillMetadata contém todos os campos do frontmatter YAML conforme a especificação SKILL.md.
// Ref: https://aiskill.market/blog/claude-code-skill-md-format
type SkillMetadata struct {
	// === Required Fields ===
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description" json:"description"` // max 160 chars

	// === Identity Fields ===
	DisplayName string   `yaml:"displayName,omitempty" json:"displayName,omitempty"`
	Author      string   `yaml:"author,omitempty" json:"author,omitempty"`
	AuthorEmail string   `yaml:"authorEmail,omitempty" json:"authorEmail,omitempty"`
	AuthorURL   string   `yaml:"authorUrl,omitempty" json:"authorUrl,omitempty"`
	License     string   `yaml:"license,omitempty" json:"license,omitempty"` // SPDX identifier
	Repository  string   `yaml:"repository,omitempty" json:"repository,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	Keywords    []string `yaml:"keywords,omitempty" json:"keywords,omitempty"` // max 10

	// === Categorization Fields ===
	Category    string   `yaml:"category,omitempty" json:"category,omitempty"`
	Subcategory string   `yaml:"subcategory,omitempty" json:"subcategory,omitempty"`
	Type        string   `yaml:"type,omitempty" json:"type,omitempty"`             // command, agent, hook, mcp
	Difficulty  string   `yaml:"difficulty,omitempty" json:"difficulty,omitempty"` // beginner, intermediate, advanced
	Audience    []string `yaml:"audience,omitempty" json:"audience,omitempty"`

	// === Compatibility Fields ===
	MinVersion string   `yaml:"minVersion,omitempty" json:"minVersion,omitempty"` // versão mínima do host (spec: minClaudeVersion)
	MaxVersion string   `yaml:"maxVersion,omitempty" json:"maxVersion,omitempty"` // versão máxima do host (spec: maxClaudeVersion)
	Platforms  []string `yaml:"platforms,omitempty" json:"platforms,omitempty"`   // macos, linux, windows
	Languages  []string `yaml:"languages,omitempty" json:"languages,omitempty"`
	Frameworks []string `yaml:"frameworks,omitempty" json:"frameworks,omitempty"`

	// === Invocation Control Fields (Claude Code official) ===
	DisableModelInvocation bool   `yaml:"disable-model-invocation,omitempty" json:"disableModelInvocation,omitempty"` // true impede auto-invocação pelo modelo
	UserInvocable          *bool  `yaml:"user-invocable,omitempty" json:"userInvocable,omitempty"`                    // false esconde do menu /slash. Default: true
	ArgumentHint           string `yaml:"argument-hint,omitempty" json:"argumentHint,omitempty"`                      // dica de args, ex: "[issue-number]"
	SkillContext           string `yaml:"context,omitempty" json:"context,omitempty"`                                 // "fork" para subagent isolado
	Agent                  string `yaml:"agent,omitempty" json:"agent,omitempty"`                                     // subagent type quando context=fork (Explore, Plan, etc.)
	Model                  string `yaml:"model,omitempty" json:"model,omitempty"`                                     // modelo a usar quando skill ativo

	// === Permission Fields ===
	Filesystem   *FilesystemPermissions `yaml:"filesystem,omitempty" json:"filesystem,omitempty"`
	Network      *NetworkPermissions    `yaml:"network,omitempty" json:"network,omitempty"`
	Tools        *ToolPermissions       `yaml:"-" json:"tools,omitempty"`         // parsed via ResolveToolsRaw
	ToolsRaw     any                    `yaml:"tools,omitempty" json:"-"`         // captura o valor bruto do YAML
	AllowedTools string                 `yaml:"allowed-tools,omitempty" json:"-"` // formato simples: "Read, Grep, Glob"

	// === Input/Output Fields ===
	Input  *InputConfig  `yaml:"input,omitempty" json:"input,omitempty"`
	Output *OutputConfig `yaml:"output,omitempty" json:"output,omitempty"`

	// === Behavior Fields ===
	Behavior *BehaviorConfig `yaml:"behavior,omitempty" json:"behavior,omitempty"`

	// === Trigger Fields (hooks) ===
	Triggers *TriggerConfig `yaml:"triggers,omitempty" json:"triggers,omitempty"`

	// === Skill-scoped Hooks (Claude Code official) ===
	Hooks any `yaml:"hooks,omitempty" json:"hooks,omitempty"` // hooks no ciclo de vida do skill

	// === Dependencies ===
	Dependencies *DependenciesConfig `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`

	// === MCP Integration ===
	MCP *MCPConfig `yaml:"mcp,omitempty" json:"mcp,omitempty"`

	// === Catálogo / Gating (AEP-0072 D4) ===
	// ContextBudget é o custo aproximado do corpo da skill (em tokens), usado
	// pelo planner do Nível 1 para orçar o bloco de descoberta. 0 = desconhecido
	// (o planner estima a partir do conteúdo).
	ContextBudget int `yaml:"context_budget,omitempty" json:"contextBudget,omitempty"`
	// AutoloadReason é a justificativa textual obrigatória quando auto_load=true
	// (D5: autoload é exceção, não regra).
	AutoloadReason string `yaml:"autoload_reason,omitempty" json:"autoloadReason,omitempty"`
	// RequiresTools/Filesystem/Network/MCP são pré-condições de capability
	// declaradas explicitamente. Quando o contexto/perfil não oferece a
	// capacidade, a skill é omitida ou degradada (não injetada).
	RequiresTools      bool `yaml:"requires_tools,omitempty" json:"requiresTools,omitempty"`
	RequiresFilesystem bool `yaml:"requires_filesystem,omitempty" json:"requiresFilesystem,omitempty"`
	RequiresNetwork    bool `yaml:"requires_network,omitempty" json:"requiresNetwork,omitempty"`
	RequiresMCP        bool `yaml:"requires_mcp,omitempty" json:"requiresMcp,omitempty"`

	// === Compat: campos legados/custom ===
	// AutoLoad indica que a skill deve ser injetada automaticamente no system
	// prompt (auto_load). Exposto no JSON para que a UI possa criar/editar skills
	// autoload (exige autoload_reason em modo estrito — AEP-0072 D5).
	AutoLoad bool `yaml:"auto_load,omitempty" json:"autoLoad,omitempty"`
}

// FilesystemPermissions define permissões de acesso ao filesystem.
type FilesystemPermissions struct {
	Read  []string `yaml:"read,omitempty" json:"read,omitempty"`
	Write []string `yaml:"write,omitempty" json:"write,omitempty"`
	Deny  []string `yaml:"deny,omitempty" json:"deny,omitempty"`
}

// NetworkPermissions define permissões de acesso à rede.
type NetworkPermissions struct {
	AllowedHosts []string `yaml:"allowedHosts,omitempty" json:"allowedHosts,omitempty"`
	DeniedHosts  []string `yaml:"deniedHosts,omitempty" json:"deniedHosts,omitempty"`
}

// ToolPermissions define quais tools o skill pode usar.
type ToolPermissions struct {
	Allowed      []string      `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	Denied       []string      `yaml:"denied,omitempty" json:"denied,omitempty"`
	BashCommands *BashCommands `yaml:"bashCommands,omitempty" json:"bashCommands,omitempty"`
}

// BashCommands define comandos shell permitidos/bloqueados.
type BashCommands struct {
	Allowed []string `yaml:"allowed,omitempty" json:"allowed,omitempty"`
	Denied  []string `yaml:"denied,omitempty" json:"denied,omitempty"`
}

// InputConfig define os argumentos que o skill aceita.
type InputConfig struct {
	Arguments []ArgumentDef `yaml:"arguments,omitempty" json:"arguments,omitempty"`
	Context   *ContextReq   `yaml:"context,omitempty" json:"context,omitempty"`
}

// ArgumentDef define um argumento individual.
type ArgumentDef struct {
	Name        string   `yaml:"name" json:"name"`
	Type        string   `yaml:"type" json:"type"` // string, boolean, number, string[]
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool     `yaml:"required,omitempty" json:"required,omitempty"`
	Default     any      `yaml:"default,omitempty" json:"default,omitempty"`
	Enum        []string `yaml:"enum,omitempty" json:"enum,omitempty"`
}

// ContextReq define requisitos de contexto do skill.
type ContextReq struct {
	RequiresProject     bool `yaml:"requiresProject,omitempty" json:"requiresProject,omitempty"`
	RequiresGit         bool `yaml:"requiresGit,omitempty" json:"requiresGit,omitempty"`
	RequiresPackageJSON bool `yaml:"requiresPackageJson,omitempty" json:"requiresPackageJson,omitempty"`
}

// OutputConfig define o formato de saída do skill.
type OutputConfig struct {
	Format string         `yaml:"format,omitempty" json:"format,omitempty"` // text, markdown, json, code
	Schema map[string]any `yaml:"schema,omitempty" json:"schema,omitempty"`
}

// BehaviorConfig define configurações de execução do skill.
type BehaviorConfig struct {
	Timeout     int              `yaml:"timeout,omitempty" json:"timeout,omitempty"` // segundos
	Retry       *RetryConfig     `yaml:"retry,omitempty" json:"retry,omitempty"`
	Cache       *CacheConfig     `yaml:"cache,omitempty" json:"cache,omitempty"`
	Interactive *InteractiveConf `yaml:"interactive,omitempty" json:"interactive,omitempty"`
	Model       *ModelConfig     `yaml:"model,omitempty" json:"model,omitempty"`
}

// RetryConfig define configuração de retry.
type RetryConfig struct {
	MaxAttempts int `yaml:"maxAttempts,omitempty" json:"maxAttempts,omitempty"`
	BackoffMs   int `yaml:"backoffMs,omitempty" json:"backoffMs,omitempty"`
}

// CacheConfig define configuração de cache.
type CacheConfig struct {
	Enabled    bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	TTLSeconds int      `yaml:"ttlSeconds,omitempty" json:"ttlSeconds,omitempty"`
	KeyFields  []string `yaml:"keyFields,omitempty" json:"keyFields,omitempty"`
}

// InteractiveConf define configurações de modo interativo.
type InteractiveConf struct {
	ConfirmDestructive bool `yaml:"confirmDestructive,omitempty" json:"confirmDestructive,omitempty"`
	ShowProgress       bool `yaml:"showProgress,omitempty" json:"showProgress,omitempty"`
}

// ModelConfig define preferências de modelo AI.
type ModelConfig struct {
	Preferred   string  `yaml:"preferred,omitempty" json:"preferred,omitempty"`
	Fallback    string  `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	Temperature float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens   int     `yaml:"maxTokens,omitempty" json:"maxTokens,omitempty"`
}

// TriggerConfig define triggers para hooks.
type TriggerConfig struct {
	Events   []string       `yaml:"events,omitempty" json:"events,omitempty"` // PreToolUse, PostToolUse
	Filters  *TriggerFilter `yaml:"filters,omitempty" json:"filters,omitempty"`
	Priority int            `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// TriggerFilter define filtros para triggers.
type TriggerFilter struct {
	Tools []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	Files []string `yaml:"files,omitempty" json:"files,omitempty"`
}

// DependenciesConfig define dependências externas do skill.
type DependenciesConfig struct {
	NPM      []string `yaml:"npm,omitempty" json:"npm,omitempty"`
	Pip      []string `yaml:"pip,omitempty" json:"pip,omitempty"`
	Commands []string `yaml:"commands,omitempty" json:"commands,omitempty"`
	Skills   []string `yaml:"skills,omitempty" json:"skills,omitempty"`
}

// MCPConfig define integração com servidor MCP.
type MCPConfig struct {
	Server *MCPServerConfig `yaml:"server,omitempty" json:"server,omitempty"`
	Tools  []MCPToolDef     `yaml:"tools,omitempty" json:"tools,omitempty"`
}

// MCPServerConfig define configuração do servidor MCP.
type MCPServerConfig struct {
	Command string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// MCPToolDef define uma tool MCP exposta pelo skill.
type MCPToolDef struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Skill representa um skill completo com metadados, conteúdo e localização.
type Skill struct {
	SkillMetadata
	Slug    string `json:"slug"`    // nome do diretório
	Source  string `json:"source"`  // "exe", "home", "workdir"
	Content string `json:"content"` // corpo Markdown (sem frontmatter)
	Path    string `json:"path"`    // caminho absoluto do SKILL.md
}

// SkillInfo é um resumo leve de um skill para listagem (sem conteúdo).
type SkillInfo struct {
	SkillMetadata
	Slug      string `json:"slug"`
	IsBuiltin bool   `json:"isBuiltin"` // AEP-0051: substitui o antigo Source (exe/home/workdir)
}

// ResolveToolsRaw converte os campos de tools em ToolPermissions.
// Suporta três formatos (em ordem de prioridade):
//  1. allowed-tools: "Read, Grep, Glob"                → ToolPermissions{Allowed: [...]} (Claude Code oficial)
//  2. tools: [read_file, write_file]                    → ToolPermissions{Allowed: [...]} (legado lista)
//  3. tools: {allowed: [...], denied: [...]}             → ToolPermissions{...}           (Agent Skills spec)
func (m *SkillMetadata) ResolveToolsRaw() {
	// 1. allowed-tools (string comma-separated) — formato Claude Code oficial
	if m.AllowedTools != "" && m.Tools == nil {
		parts := strings.Split(m.AllowedTools, ",")
		allowed := make([]string, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				allowed = append(allowed, t)
			}
		}
		if len(allowed) > 0 {
			m.Tools = &ToolPermissions{Allowed: allowed}
		}
	}

	// 2/3. tools (YAML polimórfico)
	if m.ToolsRaw != nil && m.Tools == nil {
		switch v := m.ToolsRaw.(type) {
		case []any:
			// Formato legado: lista simples → interpreta como allowed
			allowed := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					allowed = append(allowed, s)
				}
			}
			if len(allowed) > 0 {
				m.Tools = &ToolPermissions{Allowed: allowed}
			}
		case map[string]any:
			// Formato spec: objeto com allowed/denied
			tp := &ToolPermissions{}
			if a, ok := v["allowed"]; ok {
				if arr, ok := a.([]any); ok {
					for _, item := range arr {
						if s, ok := item.(string); ok {
							tp.Allowed = append(tp.Allowed, s)
						}
					}
				}
			}
			if d, ok := v["denied"]; ok {
				if arr, ok := d.([]any); ok {
					for _, item := range arr {
						if s, ok := item.(string); ok {
							tp.Denied = append(tp.Denied, s)
						}
					}
				}
			}
			if bc, ok := v["bashCommands"]; ok {
				if bcMap, ok := bc.(map[string]any); ok {
					tp.BashCommands = &BashCommands{}
					if a, ok := bcMap["allowed"]; ok {
						if arr, ok := a.([]any); ok {
							for _, item := range arr {
								if s, ok := item.(string); ok {
									tp.BashCommands.Allowed = append(tp.BashCommands.Allowed, s)
								}
							}
						}
					}
					if d, ok := bcMap["denied"]; ok {
						if arr, ok := d.([]any); ok {
							for _, item := range arr {
								if s, ok := item.(string); ok {
									tp.BashCommands.Denied = append(tp.BashCommands.Denied, s)
								}
							}
						}
					}
				}
			}
			m.Tools = tp
		}
	}

	m.ToolsRaw = nil // limpa após resolver
}

// ExpandTemplateVars expande variáveis de template nos paths de filesystem.
// Suporta: ${HOME}, ${PROJECT_ROOT}, ${TEMP}
// Compatível com a spec SKILL.md (ex: "${HOME}/.config/skill/*").
func (m *SkillMetadata) ExpandTemplateVars(projectRoot string) {
	if m.Filesystem == nil {
		return
	}

	home, _ := os.UserHomeDir()
	tempDir := os.TempDir()

	expand := func(paths []string) []string {
		result := make([]string, len(paths))
		for i, p := range paths {
			p = strings.ReplaceAll(p, "${HOME}", home)
			p = strings.ReplaceAll(p, "${PROJECT_ROOT}", projectRoot)
			p = strings.ReplaceAll(p, "${TEMP}", tempDir)
			result[i] = filepath.FromSlash(p)
		}
		return result
	}

	if len(m.Filesystem.Read) > 0 {
		m.Filesystem.Read = expand(m.Filesystem.Read)
	}
	if len(m.Filesystem.Write) > 0 {
		m.Filesystem.Write = expand(m.Filesystem.Write)
	}
	if len(m.Filesystem.Deny) > 0 {
		m.Filesystem.Deny = expand(m.Filesystem.Deny)
	}
}

// IsAutoLoad retorna true se o skill deve ser injetado automaticamente no system prompt.
// Um skill é auto_load se: auto_load=true E disable-model-invocation NÃO é true.
func (m *SkillMetadata) IsAutoLoad() bool {
	if m.DisableModelInvocation {
		return false
	}
	return m.AutoLoad
}

// IsModelInvocable retorna true se o modelo pode invocar este skill automaticamente.
// Oposto de disable-model-invocation. Default: true.
func (m *SkillMetadata) IsModelInvocable() bool {
	return !m.DisableModelInvocation
}

// IsUserInvocable retorna true se o usuário pode invocar via /slash.
// Default: true (nil = true).
func (m *SkillMetadata) IsUserInvocable() bool {
	if m.UserInvocable == nil {
		return true
	}
	return *m.UserInvocable
}

// IsForkContext retorna true se o skill roda em subagent isolado.
func (m *SkillMetadata) IsForkContext() bool {
	return m.SkillContext == "fork"
}

// GetDisplayName retorna o nome de exibição (displayName ou name).
func (m *SkillMetadata) GetDisplayName() string {
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.Name
}

// GetToolsAllowed retorna a lista de tools permitidas (do campo estruturado).
func (m *SkillMetadata) GetToolsAllowed() []string {
	if m.Tools != nil {
		return m.Tools.Allowed
	}
	return nil
}

// dependsOnToolsConfig retorna true se a skill declara permissões de tools
// (allowed/denied/bashCommands) — capability inferida do schema de permissão.
func (m *SkillMetadata) dependsOnToolsConfig() bool {
	return m.Tools != nil &&
		(len(m.Tools.Allowed) > 0 || len(m.Tools.Denied) > 0 || m.Tools.BashCommands != nil)
}

// dependsOnFilesystemConfig retorna true se a skill declara permissões de filesystem.
func (m *SkillMetadata) dependsOnFilesystemConfig() bool {
	return m.Filesystem != nil &&
		(len(m.Filesystem.Read) > 0 || len(m.Filesystem.Write) > 0 || len(m.Filesystem.Deny) > 0)
}

// dependsOnNetworkConfig retorna true se a skill declara permissões de rede.
func (m *SkillMetadata) dependsOnNetworkConfig() bool {
	return m.Network != nil &&
		(len(m.Network.AllowedHosts) > 0 || len(m.Network.DeniedHosts) > 0)
}

// EffectiveRequiresTools combina a flag explícita requires_tools (D4) com a
// capability inferida das permissões declaradas. Usado pelo gating (AEP-0072).
func (m *SkillMetadata) EffectiveRequiresTools() bool {
	return m.RequiresTools || m.dependsOnToolsConfig()
}

// EffectiveRequiresFilesystem combina requires_filesystem com a capability inferida.
func (m *SkillMetadata) EffectiveRequiresFilesystem() bool {
	return m.RequiresFilesystem || m.dependsOnFilesystemConfig()
}

// EffectiveRequiresNetwork combina requires_network com a capability inferida.
func (m *SkillMetadata) EffectiveRequiresNetwork() bool {
	return m.RequiresNetwork || m.dependsOnNetworkConfig()
}

// EffectiveRequiresMCP combina requires_mcp com a capability inferida (config MCP presente).
func (m *SkillMetadata) EffectiveRequiresMCP() bool {
	return m.RequiresMCP || m.MCP != nil
}

// RequiresAnyCapability retorna true se a skill depende de qualquer capacidade
// (tools, filesystem, network ou MCP), explícita ou inferida.
func (m *SkillMetadata) RequiresAnyCapability() bool {
	return m.EffectiveRequiresTools() ||
		m.EffectiveRequiresFilesystem() ||
		m.EffectiveRequiresNetwork() ||
		m.EffectiveRequiresMCP()
}

// Skill types
const (
	SkillTypeCommand = "command"
	SkillTypeAgent   = "agent"
	SkillTypeHook    = "hook"
	SkillTypeMCP     = "mcp"
)

// Valid difficulty levels
const (
	DifficultyBeginner     = "beginner"
	DifficultyIntermediate = "intermediate"
	DifficultyAdvanced     = "advanced"
)
