package skills

import "unicode/utf8"

// Catálogo compacto de skills (AEP-0072 D1/D2 — Nível 1, descoberta).
//
// O SkillCatalogEntry é a projeção leve usada na descoberta: nome, descrição
// (com gatilhos), custo estimado e pré-condições de capability. Sem corpo. É o
// análogo do tool_catalog (AEP-0049): o banco de skills (AEP-0051) é a fonte
// canônica; o catálogo é o índice compacto para orçar o bloco do Nível 1.

// SkillCatalogEntry descreve uma skill no catálogo de descoberta (sem corpo).
type SkillCatalogEntry struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description"`
	Type        string `json:"type,omitempty"`

	// Path é o caminho em disco legível do corpo da skill (pré-materializado no
	// rebuild do catálogo em modo DB; SKILL.md original em modo filesystem). É o
	// alvo do read_file na ativação por leitura (AEP-0072 D2, Nível 2).
	Path string `json:"path,omitempty"`

	// Custo aproximado do corpo (tokens) para o planner de budget do Nível 1.
	ContextBudget int `json:"contextBudget"`

	// Pré-condições de capability (explícitas OU inferidas das permissões).
	RequiresTools      bool `json:"requiresTools"`
	RequiresFilesystem bool `json:"requiresFilesystem"`
	RequiresNetwork    bool `json:"requiresNetwork"`
	RequiresMCP        bool `json:"requiresMcp"`

	// Controle de carregamento.
	AutoLoad       bool   `json:"autoLoad"`
	AutoloadReason string `json:"autoloadReason,omitempty"`
	ModelInvocable bool   `json:"modelInvocable"`
	UserInvocable  bool   `json:"userInvocable"`

	IsBuiltin bool `json:"isBuiltin"`
}

// CatalogByNamesOrdered devolve as entradas cujo slug/nome está em names, na ordem
// de names (modo lista-explícita do perfil). nil = todas; vazio = nenhuma.
func CatalogByNamesOrdered(all []SkillCatalogEntry, names []string) []SkillCatalogEntry {
	if names == nil {
		return all
	}
	if len(names) == 0 {
		return nil
	}
	// Mapas separados para slug e nome: misturá-los no mesmo namespace permitiria
	// colisões (o slug de uma entrada igual ao nome de outra) e resultado não
	// determinístico. Resolve-se cada identificador pedido por slug primeiro,
	// depois por nome.
	bySlug := make(map[string]SkillCatalogEntry, len(all))
	byName := make(map[string]SkillCatalogEntry, len(all))
	for _, e := range all {
		bySlug[e.Slug] = e
		if e.Name != "" {
			byName[e.Name] = e
		}
	}
	var out []SkillCatalogEntry
	for _, n := range names {
		if e, ok := bySlug[n]; ok {
			out = append(out, e)
			continue
		}
		if e, ok := byName[n]; ok {
			out = append(out, e)
		}
	}
	return out
}

// GetDisplayName devolve o rótulo de exibição (DisplayName ou Name como fallback).
func (e SkillCatalogEntry) GetDisplayName() string {
	if e.DisplayName != "" {
		return e.DisplayName
	}
	return e.Name
}

// approxCharsPerToken é uma heurística simples (≈4 chars/token) para estimar o
// custo de contexto do corpo quando context_budget não foi declarado.
const approxCharsPerToken = 4

// EstimateContextBudget estima o custo em tokens de um corpo Markdown a partir
// do número de caracteres (heurística ~4 chars/token, mínimo 1 quando não vazio).
func EstimateContextBudget(content string) int {
	// Conta caracteres (runes), não bytes: textos com runes multibyte (acentos,
	// emojis, CJK) subestimariam o tamanho com len() e distorceriam o budget.
	n := utf8.RuneCountInString(content)
	if n == 0 {
		return 0
	}
	tokens := n / approxCharsPerToken
	if tokens < 1 {
		return 1
	}
	return tokens
}

// CatalogEntryFromSkill projeta um Skill completo no entry compacto do catálogo.
// Quando context_budget não foi declarado (0), estima a partir do corpo.
func CatalogEntryFromSkill(s *Skill) SkillCatalogEntry {
	if s == nil {
		return SkillCatalogEntry{}
	}
	entry := catalogEntryFromMetadata(&s.SkillMetadata, s.Slug)
	if entry.ContextBudget == 0 {
		entry.ContextBudget = EstimateContextBudget(s.Content)
	}
	entry.IsBuiltin = s.Source == sourceExe
	entry.Path = s.Path
	return entry
}

// CatalogEntryFromInfo projeta um SkillInfo (resumo, sem corpo) no entry. Sem o
// corpo não há como estimar o budget; usa context_budget declarado (0 se ausente).
func CatalogEntryFromInfo(info SkillInfo) SkillCatalogEntry {
	entry := catalogEntryFromMetadata(&info.SkillMetadata, info.Slug)
	entry.IsBuiltin = info.IsBuiltin
	return entry
}

// sourceExe espelha configdir.SourceExe sem importar o pacote aqui.
const sourceExe = "exe"

func catalogEntryFromMetadata(m *SkillMetadata, slug string) SkillCatalogEntry {
	return SkillCatalogEntry{
		Slug:               slug,
		Name:               m.Name,
		DisplayName:        m.DisplayName,
		Description:        m.Description,
		Type:               m.Type,
		ContextBudget:      m.ContextBudget,
		RequiresTools:      m.EffectiveRequiresTools(),
		RequiresFilesystem: m.EffectiveRequiresFilesystem(),
		RequiresNetwork:    m.EffectiveRequiresNetwork(),
		RequiresMCP:        m.EffectiveRequiresMCP(),
		AutoLoad:           m.IsAutoLoad(),
		AutoloadReason:     m.AutoloadReason,
		ModelInvocable:     m.IsModelInvocable(),
		UserInvocable:      m.IsUserInvocable(),
	}
}
