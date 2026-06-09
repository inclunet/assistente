package database

// ==================== Skills (AEP-0051) ====================

// Skill é a persistência de um skill (instruções Markdown + metadados YAML).
// Substitui o armazenamento em filesystem (`skills/{slug}/SKILL.md`).
//
// Modelagem "colunas-completas": todos os campos escalares do SkillMetadata viram
// colunas (queryáveis), e cada struct aninhada/opcional é serializada como JSON em
// uma coluna TEXT. Isso garante roundtrip fiel (Skill → DB → Skill deep equal).
//
// Skills são instância-wide (sem user_id): o catálogo é compartilhado, como os
// builtins do tool_catalog. Slug é o identificador humano e é globalmente único.
type Skill struct {
	UUIDModel

	// Identificação
	Slug        string `json:"slug" gorm:"not null;index;uniqueIndex:ux_skills_slug"`
	Name        string `json:"name" gorm:"not null"`
	Version     string `json:"version" gorm:"not null"`
	Description string `json:"description" gorm:"not null;type:text"`

	// Identidade / apresentação
	DisplayName string `json:"displayName,omitempty"`
	Author      string `json:"author,omitempty"`
	AuthorEmail string `json:"authorEmail,omitempty"`
	AuthorURL   string `json:"authorUrl,omitempty" gorm:"column:author_url"`
	License     string `json:"license,omitempty"`
	Repository  string `json:"repository,omitempty" gorm:"type:text"`
	Homepage    string `json:"homepage,omitempty" gorm:"type:text"`
	Keywords    string `json:"keywords,omitempty" gorm:"type:text"` // JSON array

	// Categorização
	Category    string `json:"category,omitempty" gorm:"index"`
	Subcategory string `json:"subcategory,omitempty"`
	Type        string `json:"type,omitempty" gorm:"index"`
	Difficulty  string `json:"difficulty,omitempty"`
	Audience    string `json:"audience,omitempty" gorm:"type:text"` // JSON array

	// Compatibilidade
	MinVersion string `json:"minVersion,omitempty"`
	MaxVersion string `json:"maxVersion,omitempty"`
	Platforms  string `json:"platforms,omitempty" gorm:"type:text"`  // JSON array
	Languages  string `json:"languages,omitempty" gorm:"type:text"`  // JSON array
	Frameworks string `json:"frameworks,omitempty" gorm:"type:text"` // JSON array

	// Controle de invocação
	AutoLoad               bool   `json:"autoLoad" gorm:"not null;default:false;index"`
	DisableModelInvocation bool   `json:"disableModelInvocation" gorm:"not null;default:false"`
	UserInvocable          *bool  `json:"userInvocable,omitempty"` // NULL = default(true)
	ArgumentHint           string `json:"argumentHint,omitempty"`
	SkillContext           string `json:"skillContext,omitempty"` // "fork" para subagent isolado
	Agent                  string `json:"agent,omitempty"`        // subagent type quando context=fork
	Model                  string `json:"model,omitempty"`        // modelo preferido

	// Permissões / tools
	AllowedTools     string `json:"allowedTools,omitempty" gorm:"type:text"`     // string bruta "Read, Grep, Glob"
	ToolsConfig      string `json:"toolsConfig,omitempty" gorm:"type:text"`      // JSON: *ToolPermissions resolvida
	FilesystemConfig string `json:"filesystemConfig,omitempty" gorm:"type:text"` // JSON: *FilesystemPermissions
	NetworkConfig    string `json:"networkConfig,omitempty" gorm:"type:text"`    // JSON: *NetworkPermissions

	// Input/Output/Behavior/Triggers/Hooks/Dependencies/MCP (JSON)
	InputSpec          string `json:"inputSpec,omitempty" gorm:"type:text"`
	OutputSpec         string `json:"outputSpec,omitempty" gorm:"type:text"`
	BehaviorConfig     string `json:"behaviorConfig,omitempty" gorm:"type:text"`
	TriggersConfig     string `json:"triggersConfig,omitempty" gorm:"type:text"`
	HooksConfig        string `json:"hooksConfig,omitempty" gorm:"type:text"`
	DependenciesConfig string `json:"dependenciesConfig,omitempty" gorm:"type:text"`
	MCPConfig          string `json:"mcpConfig,omitempty" gorm:"type:text"`

	// Conteúdo Markdown (após frontmatter)
	Content string `json:"content" gorm:"not null;type:text"`

	// Gerenciamento de builtins / customização
	IsBuiltin      bool   `json:"isBuiltin" gorm:"not null;default:false;index"`
	BuiltinVersion string `json:"builtinVersion,omitempty"`
	IsCustomized   bool   `json:"isCustomized" gorm:"not null;default:false"`

	// Junction (allowed/denied) populada a partir de ToolsConfig para consultas
	Tools []SkillTool `json:"-" gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE"`
}

// TableName fixa o nome da tabela.
func (Skill) TableName() string { return "skills" }

// SkillTool é a junction de permissões de tool por skill (allowed/denied).
// Derivada de Skill.ToolsConfig (ToolPermissions) para permitir queries eficientes.
type SkillTool struct {
	UUIDModel
	SkillID  string `json:"skillId" gorm:"not null;index;uniqueIndex:ux_skill_tools_identity,priority:1"`
	ToolName string `json:"toolName" gorm:"not null;uniqueIndex:ux_skill_tools_identity,priority:2"`
	Relation string `json:"relation" gorm:"not null;uniqueIndex:ux_skill_tools_identity,priority:3"` // "allowed" | "denied"

	Skill *Skill `json:"-" gorm:"foreignKey:SkillID"`
}

// TableName fixa o nome da tabela.
func (SkillTool) TableName() string { return "skill_tools" }

// Constantes de relação para skill_tools.
const (
	SkillToolAllowed = "allowed"
	SkillToolDenied  = "denied"
)
