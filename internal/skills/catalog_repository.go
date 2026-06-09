package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"assistente/internal/database"

	"gorm.io/gorm"
)

// contentHashHex calcula um hash estável do corpo da skill, usado para detectar
// defasagem entre o catálogo persistido e a fonte canônica (AEP-0072 D2).
func contentHashHex(content string) string {
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// catalogEntryToModel projeta um SkillCatalogEntry de domínio na row persistida.
func catalogEntryToModel(entry SkillCatalogEntry, contentHash string) *database.SkillCatalog {
	return &database.SkillCatalog{
		Slug:               entry.Slug,
		Name:               entry.Name,
		DisplayName:        entry.DisplayName,
		Description:        entry.Description,
		Type:               entry.Type,
		Path:               entry.Path,
		ContextBudget:      entry.ContextBudget,
		RequiresTools:      entry.RequiresTools,
		RequiresFilesystem: entry.RequiresFilesystem,
		RequiresNetwork:    entry.RequiresNetwork,
		RequiresMCP:        entry.RequiresMCP,
		AutoLoad:           entry.AutoLoad,
		AutoloadReason:     entry.AutoloadReason,
		ModelInvocable:     entry.ModelInvocable,
		UserInvocable:      entry.UserInvocable,
		IsBuiltin:          entry.IsBuiltin,
		ContentHash:        contentHash,
	}
}

// catalogModelToEntry reidrata um SkillCatalogEntry a partir da row persistida.
func catalogModelToEntry(m database.SkillCatalog) SkillCatalogEntry {
	return SkillCatalogEntry{
		Slug:               m.Slug,
		Name:               m.Name,
		DisplayName:        m.DisplayName,
		Description:        m.Description,
		Type:               m.Type,
		Path:               m.Path,
		ContextBudget:      m.ContextBudget,
		RequiresTools:      m.RequiresTools,
		RequiresFilesystem: m.RequiresFilesystem,
		RequiresNetwork:    m.RequiresNetwork,
		RequiresMCP:        m.RequiresMCP,
		AutoLoad:           m.AutoLoad,
		AutoloadReason:     m.AutoloadReason,
		ModelInvocable:     m.ModelInvocable,
		UserInvocable:      m.UserInvocable,
		IsBuiltin:          m.IsBuiltin,
	}
}

// ListCatalog devolve o catálogo compacto persistido (AEP-0072 D1, Nível 1).
func (r *DBRepository) ListCatalog(ctx context.Context) ([]SkillCatalogEntry, error) {
	var rows []database.SkillCatalog
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	entries := make([]SkillCatalogEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, catalogModelToEntry(row))
	}
	return entries, nil
}

// CatalogMaterializer escreve o corpo da skill em disco e devolve o caminho
// legível usado na ativação por leitura (AEP-0072 D2). Injetado pelo Manager
// (que conhece o cache em disco); nil = mantém apenas o Path já presente na skill.
type CatalogMaterializer func(s Skill) (string, error)

// RebuildCatalog reconstrói o skill_catalog a partir das skills persistidas
// (fonte canônica). Idempotente: substitui completamente o catálogo dentro de
// uma transação. Chamado após seed/import e a cada CRUD de skill.
//
// Quando `materialize` é fornecido, o corpo de cada skill é pré-materializado em
// disco e o caminho resultante é persistido no catálogo, tornando o Nível 1
// (descoberta) servível diretamente do catálogo, sem recarregar o corpo.
func (r *DBRepository) RebuildCatalog(ctx context.Context, materialize CatalogMaterializer) error {
	var rows []database.Skill
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&rows).Error; err != nil {
		return err
	}

	models := make([]*database.SkillCatalog, 0, len(rows))
	for i := range rows {
		s, err := skillFromModel(&rows[i])
		if err != nil {
			return err
		}
		entry := CatalogEntryFromSkill(s)
		entry.Slug = rows[i].Slug
		// IsBuiltin vem da row persistida (a projeção de domínio não conhece a
		// origem builtin/custom do DB).
		entry.IsBuiltin = rows[i].IsBuiltin
		if materialize != nil {
			if path, err := materialize(*s); err == nil && path != "" {
				entry.Path = path
			}
		}
		models = append(models, catalogEntryToModel(entry, contentHashHex(rows[i].Content)))
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&database.SkillCatalog{}).Error; err != nil {
			return err
		}
		for _, m := range models {
			if err := tx.Create(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
