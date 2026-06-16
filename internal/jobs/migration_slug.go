package jobs

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// slugRenormalizationTarget descreve uma tabela cujo slug é gerado por
// normalizeSlug e que pode conter registros legados (slug com acentos ou
// símbolos) gravados antes da unificação do algoritmo em internal/slug
// (issue #255).
type slugRenormalizationTarget struct {
	table string
}

// RenormalizeLegacySlugs re-normaliza para a forma canônica de internal/slug os
// slugs já persistidos de jobs, pipelines e tags — as três tabelas cujo slug é
// produzido por normalizeSlug.
//
// Motivação: normalizeSlug passou a delegar para slug.Slugify (NFD + remoção de
// acentos + colapso de não-alfanuméricos). Antes ela só fazia lowercase/trim e
// trocava espaço por hífen. Como normalizeSlug é usada TANTO na escrita
// (CreateJob/CreatePipeline/UpsertTag) QUANTO nos lookups (WHERE slug = ? em
// GetJob/GetPipeline/jobRowBySlug/etc.), registros legados com slug fora da
// forma canônica (ex.: "café-job") deixam de ser encontráveis após o deploy
// (o lookup passa a normalizar a busca para "cafe-job"). Esta migração alinha o
// dado persistido ao algoritmo canônico, mantendo o slug como identificador
// estável (AEP-0048, D5).
//
// Garantias:
//   - Idempotente: só atualiza linhas cujo slug difere da forma canônica;
//     reexecuções não fazem nada além do necessário. Se a tabela não existe, é
//     noop.
//   - Escopada por usuário: o índice único é (user_id, slug); a detecção de
//     colisão e a re-normalização são feitas por usuário.
//   - Não destrutiva e sempre endereçável: se a forma canônica colidiria com um
//     slug já existente para o mesmo usuário, a linha legada é re-normalizada para
//     um slug canônico ÚNICO derivado da forma canônica com sufixo numérico
//     (`-2`, `-3`, …), garantindo unicidade (user_id, slug) e que toda linha
//     permaneça endereçável por um slug canônico. O rename é logado. Nenhum dado
//     é apagado. (Antes a linha legada era preservada com seu slug não-canônico,
//     o que a tornava inalcançável pelos lookups — que normalizam a busca para a
//     forma canônica — e fazia toggle/delete/persist/log atingirem o job errado.)
//   - Slugs que normalizam para vazio (ex.: só símbolos) são preservados como
//     estão, pois não há forma canônica segura para reescrevê-los.
//   - Atomicidade: a re-normalização de CADA tabela roda dentro de uma transação
//     (db.Transaction). Os contadores só são acumulados após o commit da
//     transação daquela tabela; se a transação falhar, o banco não fica com
//     slugs parcialmente migrados naquela tabela.
//
// Limitação conhecida (IMPORTANTE):
//
//	Esta migração re-normaliza APENAS a coluna `slug` das tabelas `jobs`,
//	`job_pipelines` e `tags`. Slugs também aparecem como TEXTO em outros lugares
//	que NÃO são tocados aqui — por exemplo: `Inputs`, `OutputConfig` e
//	`EventsConfig` de jobs/pipelines, payloads de eventos e mensagens de log.
//	Essas referências textuais ao slug NÃO são re-normalizadas.
//
//	Isso é especialmente perigoso no caso de COLISÃO com sufixo: quando um slug
//	legado é re-normalizado para uma forma canônica com sufixo (ex.: a forma
//	canônica "cafe-job" já existia e a linha legada vira "cafe-job-2"), qualquer
//	referência textual antiga que ainda aponte para "cafe-job" passa a resolver
//	SILENCIOSAMENTE para o outro job (o que ocupava a forma canônica base), e
//	não para a linha originalmente pretendida. Para auditoria/rollback manual,
//	cada renomeação com sufixo é registrada em log no nível WARN com o
//	mapeamento completo (user, tabela, slug antigo → slug novo); ver loop abaixo.
func RenormalizeLegacySlugs(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	targets := []slugRenormalizationTarget{
		{table: "tags"},
		{table: "job_pipelines"},
		{table: "jobs"},
	}
	var totalUpdated, totalSuffixed int
	for _, target := range targets {
		// A re-normalização de cada tabela é atômica: ou todas as linhas daquela
		// tabela são atualizadas, ou nenhuma. Os contadores só são acumulados
		// após o commit, evitando estado parcialmente migrado em caso de falha.
		var updated, suffixed int
		err := db.Transaction(func(tx *gorm.DB) error {
			u, s, err := renormalizeSlugsForTable(tx, target)
			if err != nil {
				return err
			}
			updated, suffixed = u, s
			return nil
		})
		if err != nil {
			return fmt.Errorf("re-normaliza slugs de %s: %w", target.table, err)
		}
		totalUpdated += updated
		totalSuffixed += suffixed
	}
	if totalUpdated > 0 {
		log.Printf("[Jobs] Re-normalização de slugs legados concluída: %d atualizados (%d deles com sufixo por colisão de slug canônico)", totalUpdated, totalSuffixed)
	}
	return nil
}

// renormalizeSlugsForTable re-normaliza os slugs de uma única tabela. Retorna a
// contagem de linhas atualizadas e quantas delas precisaram de sufixo numérico
// por colisão de slug canônico. Todas as queries usam o tx recebido para que a
// operação seja atômica: o chamador a invoca dentro de db.Transaction(...).
func renormalizeSlugsForTable(tx *gorm.DB, target slugRenormalizationTarget) (updated, suffixed int, err error) {
	if !tx.Migrator().HasTable(target.table) {
		return 0, 0, nil
	}

	type slugRow struct {
		ID     string `gorm:"column:id"`
		UserID string `gorm:"column:user_id"`
		Slug   string `gorm:"column:slug"`
	}
	var rows []slugRow
	if err := tx.Table(target.table).
		Select("id", "user_id", "slug").
		Order("user_id, id").
		Scan(&rows).Error; err != nil {
		return 0, 0, err
	}

	// taken modela os slugs que estarão ocupados por usuário ao fim da migração
	// (forma final no banco), garantindo o respeito ao índice único
	// (user_id, slug) sem precisar consultar o banco a cada linha.
	taken := make(map[string]map[string]bool)
	occupy := func(userID, s string) {
		if taken[userID] == nil {
			taken[userID] = make(map[string]bool)
		}
		taken[userID][s] = true
	}
	isTaken := func(userID, s string) bool {
		return taken[userID] != nil && taken[userID][s]
	}

	type pendingChange struct {
		id        string
		userID    string
		oldSlug   string
		canonical string
	}
	var toChange []pendingChange

	// 1ª passada: linhas já canônicas (ou que normalizam para vazio) ocupam seu
	// slug atual; as demais entram na fila de mudança.
	for _, row := range rows {
		canonical := normalizeSlug(row.Slug)
		if canonical == "" || canonical == row.Slug {
			occupy(row.UserID, row.Slug)
			continue
		}
		toChange = append(toChange, pendingChange{
			id:        row.ID,
			userID:    row.UserID,
			oldSlug:   row.Slug,
			canonical: canonical,
		})
	}

	// 2ª passada: aplica as mudanças. Em colisão, deriva um slug canônico único
	// com sufixo numérico para manter a linha endereçável (ver doc da função).
	for _, change := range toChange {
		finalSlug := change.canonical
		if isTaken(change.userID, finalSlug) {
			finalSlug = uniqueCanonicalSlug(change.canonical, func(s string) bool {
				return isTaken(change.userID, s)
			})
			// WARN: renomeação com sufixo é o caso de risco descrito na
			// "Limitação conhecida" da docstring (referências textuais antigas ao
			// slug podem resolver para o job errado). Logamos o mapeamento
			// completo (user, tabela, slug antigo → slug novo) para auditoria e
			// rollback manual.
			log.Printf("[WARN][Jobs] slug legado renomeado com sufixo por colisão (auditar referências textuais): user=%s tabela=%s slug antigo=%q -> slug novo=%q (forma canônica %q já estava ocupada)",
				change.userID, target.table, change.oldSlug, finalSlug, change.canonical)
			suffixed++
		}
		res := tx.Table(target.table).Where("id = ?", change.id).Update("slug", finalSlug)
		if res.Error != nil {
			return updated, suffixed, res.Error
		}
		occupy(change.userID, finalSlug)
		updated++
	}

	return updated, suffixed, nil
}

// uniqueCanonicalSlug retorna a forma canônica base quando livre; caso contrário
// anexa sufixos numéricos crescentes (`-2`, `-3`, …) até encontrar um slug ainda
// não ocupado, conforme reportado por taken. O resultado continua sendo um slug
// canônico válido (somente minúsculas, dígitos e hífens) e endereçável pelos
// lookups, que normalizam a busca para a mesma forma.
func uniqueCanonicalSlug(canonical string, taken func(string) bool) string {
	if !taken(canonical) {
		return canonical
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", canonical, i)
		if !taken(candidate) {
			return candidate
		}
	}
}
