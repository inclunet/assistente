package skills

import (
	"encoding/json"
	"reflect"

	"assistente/internal/database"
)

// Conversões entre o domínio (Skill/SkillInfo) e o model GORM (database.Skill).
//
// Estratégia "colunas-completas" (AEP-0051): escalares viram colunas; structs
// aninhadas/opcionais e slices viram JSON em colunas TEXT. ToolsConfig é a fonte
// de verdade das permissões; a junction skill_tools é derivada apenas para query.

// marshalJSONField serializa um valor opcional em JSON, retornando "" quando o
// valor é nil (ponteiro/slice/map nil) ou um slice vazio — preservando a
// semântica omitempty para roundtrip fiel.
func marshalJSONField(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map:
		if rv.IsNil() {
			return "", nil
		}
	}
	if rv.Kind() == reflect.Slice && rv.Len() == 0 {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalJSONField desserializa um campo JSON em dst (ponteiro), tratando ""
// como ausência de valor (no-op, mantendo o zero value de dst).
func unmarshalJSONField(s string, dst any) error {
	if s == "" {
		return nil
	}
	return json.Unmarshal([]byte(s), dst)
}

// skillToolRows deriva as rows de junction (allowed/denied) a partir das
// permissões de tools resolvidas.
func skillToolRows(tp *ToolPermissions) []database.SkillTool {
	if tp == nil {
		return nil
	}
	rows := make([]database.SkillTool, 0, len(tp.Allowed)+len(tp.Denied))
	for _, t := range tp.Allowed {
		rows = append(rows, database.SkillTool{ToolName: t, Relation: database.SkillToolAllowed})
	}
	for _, t := range tp.Denied {
		rows = append(rows, database.SkillTool{ToolName: t, Relation: database.SkillToolDenied})
	}
	if len(rows) == 0 {
		return nil
	}
	return rows
}

// skillToModel converte o domínio Skill para o model GORM (com junction Tools
// derivada). Campos de gerenciamento (IsBuiltin/BuiltinVersion/IsCustomized) NÃO
// são derivados do domínio — são responsabilidade do seed/importador/repository.
func skillToModel(s *Skill) (*database.Skill, error) {
	m := &database.Skill{
		Slug:        s.Slug,
		Name:        s.Name,
		Version:     s.Version,
		Description: s.Description,
		DisplayName: s.DisplayName,
		Author:      s.Author,
		AuthorEmail: s.AuthorEmail,
		AuthorURL:   s.AuthorURL,
		License:     s.License,
		Repository:  s.Repository,
		Homepage:    s.Homepage,
		Category:    s.Category,
		Subcategory: s.Subcategory,
		Type:        s.Type,
		Difficulty:  s.Difficulty,
		MinVersion:  s.MinVersion,
		MaxVersion:  s.MaxVersion,

		AutoLoad:               s.AutoLoad,
		DisableModelInvocation: s.DisableModelInvocation,
		UserInvocable:          s.UserInvocable,
		ArgumentHint:           s.ArgumentHint,
		SkillContext:           s.SkillContext,
		Agent:                  s.Agent,
		Model:                  s.Model,

		AllowedTools: s.AllowedTools,
		Content:      s.Content,
	}

	type jsonField struct {
		src any
		dst *string
	}
	fields := []jsonField{
		{s.Keywords, &m.Keywords},
		{s.Audience, &m.Audience},
		{s.Platforms, &m.Platforms},
		{s.Languages, &m.Languages},
		{s.Frameworks, &m.Frameworks},
		{s.Tools, &m.ToolsConfig},
		{s.Filesystem, &m.FilesystemConfig},
		{s.Network, &m.NetworkConfig},
		{s.Input, &m.InputSpec},
		{s.Output, &m.OutputSpec},
		{s.Behavior, &m.BehaviorConfig},
		{s.Triggers, &m.TriggersConfig},
		{s.Hooks, &m.HooksConfig},
		{s.Dependencies, &m.DependenciesConfig},
		{s.MCP, &m.MCPConfig},
	}
	for _, f := range fields {
		v, err := marshalJSONField(f.src)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}

	m.Tools = skillToolRows(s.Tools)
	return m, nil
}

// skillFromModel reconstrói o domínio Skill a partir do model GORM. As rows de
// junction (skill_tools) são ignoradas: ToolsConfig é a fonte de verdade.
func skillFromModel(m *database.Skill) (*Skill, error) {
	s := &Skill{
		Slug:    m.Slug,
		Content: m.Content,
	}
	meta := &s.SkillMetadata
	meta.Name = m.Name
	meta.Version = m.Version
	meta.Description = m.Description
	meta.DisplayName = m.DisplayName
	meta.Author = m.Author
	meta.AuthorEmail = m.AuthorEmail
	meta.AuthorURL = m.AuthorURL
	meta.License = m.License
	meta.Repository = m.Repository
	meta.Homepage = m.Homepage
	meta.Category = m.Category
	meta.Subcategory = m.Subcategory
	meta.Type = m.Type
	meta.Difficulty = m.Difficulty
	meta.MinVersion = m.MinVersion
	meta.MaxVersion = m.MaxVersion

	meta.AutoLoad = m.AutoLoad
	meta.DisableModelInvocation = m.DisableModelInvocation
	meta.UserInvocable = m.UserInvocable
	meta.ArgumentHint = m.ArgumentHint
	meta.SkillContext = m.SkillContext
	meta.Agent = m.Agent
	meta.Model = m.Model
	meta.AllowedTools = m.AllowedTools

	type jsonField struct {
		src string
		dst any
	}
	fields := []jsonField{
		{m.Keywords, &meta.Keywords},
		{m.Audience, &meta.Audience},
		{m.Platforms, &meta.Platforms},
		{m.Languages, &meta.Languages},
		{m.Frameworks, &meta.Frameworks},
		{m.ToolsConfig, &meta.Tools},
		{m.FilesystemConfig, &meta.Filesystem},
		{m.NetworkConfig, &meta.Network},
		{m.InputSpec, &meta.Input},
		{m.OutputSpec, &meta.Output},
		{m.BehaviorConfig, &meta.Behavior},
		{m.TriggersConfig, &meta.Triggers},
		{m.HooksConfig, &meta.Hooks},
		{m.DependenciesConfig, &meta.Dependencies},
		{m.MCPConfig, &meta.MCP},
	}
	for _, f := range fields {
		if err := unmarshalJSONField(f.src, f.dst); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// skillInfoFromModel reconstrói um SkillInfo (resumo leve) a partir do model.
func skillInfoFromModel(m *database.Skill) (SkillInfo, error) {
	s, err := skillFromModel(m)
	if err != nil {
		return SkillInfo{}, err
	}
	return SkillInfo{
		SkillMetadata: s.SkillMetadata,
		Slug:          s.Slug,
		IsBuiltin:     m.IsBuiltin,
	}, nil
}
