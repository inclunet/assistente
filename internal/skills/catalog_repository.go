package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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
	// Tie-breaker por slug: name não é único, então sem ele a ordem relativa de
	// skills homônimas seria indefinida (afeta budget/omissão e a saída do prompt).
	if err := r.db.WithContext(ctx).Order("name ASC, slug ASC").Find(&rows).Error; err != nil {
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

// catalogSnapshotItem é a projeção de uma skill usada na reconstrução do catálogo.
type catalogSnapshotItem struct {
	skill     *Skill
	slug      string
	isBuiltin bool
	hash      string
}

// loadCatalogSnapshot lê as skills persistidas (via db, que pode ser a conexão ou
// uma transação) e devolve a projeção de cada uma + o mapa slug→hash canônico.
func loadCatalogSnapshot(db *gorm.DB) ([]catalogSnapshotItem, map[string]string, error) {
	var rows []database.Skill
	if err := db.Order("name ASC, slug ASC").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	items := make([]catalogSnapshotItem, 0, len(rows))
	desired := make(map[string]string, len(rows))
	for i := range rows {
		s, err := skillFromModel(&rows[i])
		if err != nil {
			return nil, nil, err
		}
		h := canonicalSkillHash(s, rows[i].IsBuiltin)
		items = append(items, catalogSnapshotItem{skill: s, slug: rows[i].Slug, isBuiltin: rows[i].IsBuiltin, hash: h})
		desired[rows[i].Slug] = h
	}
	return items, desired, nil
}

func equalHashMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// catalogInsertBatchSize controla quantas entries são inseridas por roundtrip ao
// regravar o catálogo (batch insert), limitando o tempo de lock da transação.
const catalogInsertBatchSize = 100

// RebuildCatalog reconstrói o skill_catalog a partir das skills persistidas
// (fonte canônica). Idempotente: substitui completamente o catálogo. Chamado
// após seed/import e a cada CRUD de skill.
//
// Quando `materialize` é fornecido, o corpo de cada skill é pré-materializado em
// disco e o caminho resultante é persistido no catálogo, tornando o Nível 1
// (descoberta) servível diretamente do catálogo, sem recarregar o corpo.
//
// Estratégia em duas fases para NÃO fazer I/O de disco (materialização) dentro da
// transação — o que prenderia o lock por mais tempo e deixaria arquivos de cache
// órfãos num rollback:
//  1. Lê o snapshot de `skills` e calcula os hashes canônicos.
//  2. Materializa os corpos em disco (fora da transação; o materializer é
//     idempotente — mesmo path/conteúdo para a mesma skill).
//  3. Abre a transação de escrita e só grava se o snapshot ainda for válido
//     (re-lê `skills` no tx e confere os hashes). Mutação concorrente entre as
//     fases dispara retry, garantindo que o catálogo reflita um estado real do DB.
func (r *DBRepository) RebuildCatalog(ctx context.Context, materialize CatalogMaterializer) error {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		desired, models, fresh, err := r.prepareCatalog(ctx, materialize)
		if err != nil {
			return err
		}
		if fresh {
			return nil
		}
		committed, err := r.commitCatalogIfFresh(ctx, desired, models)
		if err != nil {
			return err
		}
		if committed {
			return nil
		}
		// Defasagem concorrente entre as fases: recomeça com snapshot novo.
	}
	return fmt.Errorf("reconstruir catálogo: defasagem concorrente persistente após %d tentativas", maxAttempts)
}

// prepareCatalog (Fase 1+2) lê o snapshot, decide se há defasagem e, em caso
// afirmativo, materializa os corpos (fora de transação) montando os modelos a
// gravar. fresh=true quando o catálogo já está em sincronia (no-op).
func (r *DBRepository) prepareCatalog(ctx context.Context, materialize CatalogMaterializer) (desired map[string]string, models []*database.SkillCatalog, fresh bool, err error) {
	db := r.db.WithContext(ctx)
	items, desired, err := loadCatalogSnapshot(db)
	if err != nil {
		return nil, nil, false, err
	}

	// AEP-0072 D2: detecção de defasagem via ContentHash. Se o conjunto de slugs
	// e todos os hashes batem com o catálogo persistido, não há nada a fazer —
	// evita re-materializar e reescrever o catálogo a cada chamada (no-op).
	fresh, err = catalogMatchesHashes(db, desired, materialize != nil)
	if err != nil {
		return nil, nil, false, err
	}
	if fresh {
		return nil, nil, true, nil
	}

	models = make([]*database.SkillCatalog, 0, len(items))
	for _, it := range items {
		entry := CatalogEntryFromSkill(it.skill)
		entry.Slug = it.slug
		// IsBuiltin vem da row persistida (a projeção de domínio não conhece a
		// origem builtin/custom do DB).
		entry.IsBuiltin = it.isBuiltin
		if materialize != nil {
			// Falha-rápido: um path vazio quebraria a ativação via read_file no
			// Nível 1 (o builder omitiria a skill silenciosamente). Tratamos tanto
			// erro quanto path vazio como falha — sem gravar nada no catálogo.
			path, err := materialize(*it.skill)
			if err != nil {
				return nil, nil, false, fmt.Errorf("materializar skill %q para o catálogo: %w", it.slug, err)
			}
			if path == "" {
				return nil, nil, false, fmt.Errorf("materializar skill %q para o catálogo: path vazio", it.slug)
			}
			entry.Path = path
		}
		models = append(models, catalogEntryToModel(entry, it.hash))
	}
	return desired, models, false, nil
}

// commitCatalogIfFresh (Fase 3) grava o catálogo numa transação, mas só se o
// snapshot de `skills` ainda corresponder ao `desired` capturado na Fase 1. Se
// houve mutação concorrente, não grava e devolve committed=false (o caller faz
// retry com snapshot novo) — evitando persistir um estado inconsistente.
func (r *DBRepository) commitCatalogIfFresh(ctx context.Context, desired map[string]string, models []*database.SkillCatalog) (committed bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, current, err := loadCatalogSnapshot(tx)
		if err != nil {
			return err
		}
		if !equalHashMaps(current, desired) {
			return nil // drift entre as fases → não grava; caller faz retry
		}
		// AllowGlobalUpdate torna explícita a intenção de apagar TODAS as linhas do
		// catálogo (o rebuild substitui o snapshot inteiro), em vez do workaround
		// pouco idiomático `Where("1 = 1")`.
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&database.SkillCatalog{}).Error; err != nil {
			return err
		}
		// Batch insert: reduz roundtrips e o tempo de lock da transação conforme o
		// catálogo cresce (GORM rejeita slice vazio, então só insere quando há itens).
		if len(models) > 0 {
			if err := tx.CreateInBatches(models, catalogInsertBatchSize).Error; err != nil {
				return err
			}
		}
		committed = true
		return nil
	})
	return committed, err
}

// catalogMatchesHashes informa se o catálogo persistido está em sincronia com o
// conjunto desejado (slug → hash canônico). Defasagem = contagem diferente, slug
// ausente/extra, ou hash divergente. Base da detecção de drift (AEP-0072 D2).
//
// requirePath só vale quando o rebuild recebe um materializador: nesse caso um
// path vazio na entrada significa que ela ainda não foi materializada e o catálogo
// precisa ser reconstruído. Sem materializador (ex.: testes/callers que não usam
// ativação por leitura) o path não é gerenciado e não deve forçar rebuild.
func catalogMatchesHashes(tx *gorm.DB, desired map[string]string, requirePath bool) (bool, error) {
	var rows []database.SkillCatalog
	if err := tx.Select("slug", "content_hash", "path").Find(&rows).Error; err != nil {
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
		// Com materializador, a entrada só está "fresh" se o corpo materializado
		// ainda existe em disco. Path vazio (nunca materializado) OU arquivo
		// ausente (ex.: cache limpo) força rebuild + re-materialização — caso
		// contrário o catálogo manteria um path quebrado e o read_file do Nível 1
		// falharia. Sem materializador, o path não é gerenciado e não força rebuild.
		if requirePath {
			if row.Path == "" {
				return false, nil
			}
			if _, err := os.Stat(row.Path); err != nil {
				// Arquivo ausente (cache limpo) = defasagem → rebuild. Outros erros
				// (permissão, I/O transitório) são propagados para não mascarar
				// problemas operacionais como simples "stale".
				if os.IsNotExist(err) {
					return false, nil
				}
				return false, fmt.Errorf("stat do corpo materializado %q: %w", row.Path, err)
			}
		}
	}
	return true, nil
}
