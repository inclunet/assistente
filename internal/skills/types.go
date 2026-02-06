package skills

// Skill representa uma skill carregada de um arquivo SKILL.md
type Skill struct {
	// Name é o identificador único da skill (nome do diretório)
	Name string `json:"name"`

	// DisplayName é o nome amigável para exibição
	DisplayName string `json:"display_name"`

	// Description é uma descrição curta para o catálogo
	Description string `json:"description"`

	// AutoLoad indica se o conteúdo deve ser injetado automaticamente no system prompt
	AutoLoad bool `json:"auto_load"`

	// Tools lista as tools genéricas que a skill utiliza
	Tools []string `json:"tools,omitempty"`

	// Content é o conteúdo completo do SKILL.md (instruções para o LLM)
	Content string `json:"content"`

	// Path é o caminho absoluto do arquivo SKILL.md
	Path string `json:"path"`
}

// SkillCatalogEntry é uma versão resumida para o catálogo injetado no system prompt
type SkillCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AutoLoad    bool   `json:"auto_load"`
}
