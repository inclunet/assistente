package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"assistente/internal/database"

	"gorm.io/gorm"
)

// canonicalSkillHash calcula um hash estável da projeção canônica de uma skill
// (frontmatter + corpo, via Compose, mais a origem builtin/custom). É a base da
// detecção de defasagem entre o catálogo persistido e a fonte canônica das
// skills (AEP-0072 D2): qualquer mudança relevante para o catálogo altera o hash.
func canonicalSkillHash(s *Skill, isBuiltin bool) string {
	raw, err := Compose(&s.SkillMetadata, s.Content)
	if err != nil {
		// Fallback defensivo: ao menos o corpo entra no hash.
		raw = s.Content
	}
	sum := sha256.Sum256([]byte(raw + "\x00builtin=" + strconv.FormatBool(isBuiltin)))
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

	// Projeta cada skill e calcula o hash canônico ANTES de materializar, para
	// poder pular o trabalho pesado quando o catálogo já está em sincronia.
	type pending struct {
		skill     *Skill
		slug      string
		skillType string
		isBuiltin bool
		hash      string
	}
	items := make([]pending, 0, len(rows))
	desired := make(map[string]string, len(rows))
	for i := range rows {
		s, err := skillFromModel(&rows[i])
		if err != nil {
			return err
		}
		h := canonicalSkillHash(s, rows[i].IsBuiltin)
		items = append(items, pending{skill: s, slug: rows[i].Slug, skillType: rows[i].Type, isBuiltin: rows[i].IsBuiltin, hash: h})
		desired[rows[i].Slug] = h
	}

	// AEP-0072 D2: detecção de defasagem via ContentHash. Se o conjunto de slugs
	// e todos os hashes batem com o catálogo persistido, não há nada a fazer —
	// evita re-materializar e reescrever o catálogo a cada chamada (no-op).
	fresh, err := r.catalogMatchesHashes(ctx, desired)
	if err != nil {
		return err
	}
	if fresh {
		return nil
	}

	models := make([]*database.SkillCatalog, 0, len(items))
	for _, it := range items {
		entry := CatalogEntryFromSkill(it.skill)
		entry.Slug = it.slug
		// IsBuiltin vem da row persistida (a projeção de domínio não conhece a
		// origem builtin/custom do DB).
		entry.IsBuiltin = it.isBuiltin
		if materialize != nil {
			// Falha-rápido: um path vazio quebraria a ativação via read_file no
			// Nível 1 (o builder omitiria a skill silenciosamente). Tratamos tanto
			// erro quanto path vazio como falha. Como a materialização ocorre antes
			// da transação, o catálogo anterior permanece íntegro (sem estado parcial).
			path, err := materialize(*it.skill)
			if err != nil {
				return fmt.Errorf("materializar skill %q para o catálogo: %w", it.slug, err)
			}
			if path == "" {
				return fmt.Errorf("materializar skill %q para o catálogo: path vazio", it.slug)
			}
			entry.Path = path
		}
		models = append(models, catalogEntryToModel(entry, it.hash))
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

// catalogMatchesHashes informa se o catálogo persistido está em sincronia com o
// conjunto desejado (slug → hash canônico). Defasagem = contagem diferente, slug
// ausente/extra, ou hash divergente. Base da detecção de drift (AEP-0072 D2).
func (r *DBRepository) catalogMatchesHashes(ctx context.Context, desired map[string]string) (bool, error) {
	var rows []database.SkillCatalog
	if err := r.db.WithContext(ctx).Select("slug", "content_hash", "path").Find(&rows).Error; err != nil {
		return false, err
	}
	if len(rows) != len(desired) {
		return false, nil
	}
	for _, row := range rows {
		want, ok := desired[row.Slug]
		if !ok || row.ContentHash != want {
			return false, nil
		}
		// Catálogo sem path materializado é considerado defasado (precisa
		// re-materializar para servir o Nível 1 via read_file).
		if row.Path == "" {
			return false, nil
		}
	}
	return true, nil
}
