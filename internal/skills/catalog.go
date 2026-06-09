package skills

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

// approxTokensPerChar é uma heurística simples (≈4 chars/token) para estimar o
// custo de contexto do corpo quando context_budget não foi declarado.
const approxCharsPerToken = 4

// EstimateContextBudget estima o custo em tokens de um corpo Markdown a partir
// do número de caracteres (heurística ~4 chars/token, mínimo 1 quando não vazio).
func EstimateContextBudget(content string) int {
	n := len(content)
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
