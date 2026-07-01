package jobs

import (
	"assistente/internal/logging"
	"context"
	"fmt"
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
// Escopo auditado (issue #288):
//
//	Esta migração re-normaliza APENAS a coluna `slug` das tabelas `jobs`,
//	`job_pipelines` e `tags`.
//
//	A auditoria confirmou que os vínculos estruturais entre jobs, pipelines e tags
//	não dependem de slugs textuais em JSON: jobs referenciam pipelines por
//	`pipeline_id`, tags usam `tag_assignments.tag_id` e runs/events persistem
//	`job_id`/`job_run_id`. As colunas `Inputs`, `OutputConfig`, `EventsConfig`,
//	payloads de eventos e mensagens de log são payloads/templates opacos do
//	usuário ou dados derivados de execução; podem conter strings iguais a slugs,
//	mas não há contrato estrutural que permita reescrevê-las por regex/JSONPath
//	sem risco de corromper conteúdo do usuário.
//
//	No caso raro de colisão com sufixo (ex.: a forma canônica "cafe-job" já
//	existia e a linha legada vira "cafe-job-2"), cada renomeação é registrada em
//	log no nível WARN com o mapeamento completo (user, tabela, slug antigo → slug
//	novo). Se algum payload opaco depender semanticamente de um slug literal, a
//	correção deve ser manual e guiada por esse log, não automática nesta migração.
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
		logging.Infof(context.Background(), "jobs.migration-slug", "[Jobs] Re-normalização de slugs legados concluída: %d atualizados (%d deles com sufixo por colisão de slug canônico)", totalUpdated, totalSuffixed)
	}
	return nil
}

// slugRenormalizationPageSize é o tamanho da página (keyset) usada ao varrer as
// tabelas em renormalizeSlugsForTable. É uma variável (e não constante) apenas
// para permitir que os testes a reduzam e exercitem o caminho de múltiplas
// páginas sem precisar inserir milhares de linhas.
var slugRenormalizationPageSize = 1000

// slugRow é uma linha mínima (id, user_id, slug) lida das tabelas alvo.
type slugRow struct {
	ID     string `gorm:"column:id"`
	UserID string `gorm:"column:user_id"`
	Slug   string `gorm:"column:slug"`
}

// renormalizeSlugsForTable re-normaliza os slugs de uma única tabela. Retorna a
// contagem de linhas atualizadas e quantas delas precisaram de sufixo numérico
// por colisão de slug canônico. Todas as queries usam o tx recebido para que a
// operação seja atômica: o chamador a invoca dentro de db.Transaction(...).
//
// Estratégia de leitura (issue #287): a tabela é varrida em PÁGINAS por keyset
// (cursor por (user_id, id)), nunca materializando todas as linhas de uma vez —
// cada página carrega no máximo slugRenormalizationPageSize linhas. São feitas
// duas passadas paginadas sobre a mesma ordenação:
//
//   - 1ª passada: registra em `taken` os slugs já-canônicos (ou que normalizam
//     para vazio), que têm precedência e permanecem inalterados.
//   - 2ª passada: aplica as mudanças nas linhas legadas, resolvendo colisões.
//
// Duas passadas são necessárias (em vez de uma só) porque um slug legado em uma
// página inicial pode colidir com um slug já-canônico em uma página posterior;
// só após conhecer TODOS os slugs canônicos do usuário podemos atribuir com
// segurança a forma final (e o eventual sufixo) das linhas legadas.
//
// Trade-off explícito: `taken` continua sendo um set de slugs por usuário em
// memória. Isso é inerente à correção da detecção de colisão escopada por
// (user_id, slug) — não dá para decidir o slug final de uma linha sem saber
// quais formas canônicas daquele usuário já estão ocupadas. O que esta versão
// elimina é a materialização de TODAS as linhas das três tabelas de uma vez no
// boot: agora só uma página de linhas e o set de slugs vivem na memória. Uma
// alternativa (consultar o banco slug-a-slug com WHERE user_id = ? AND slug = ?)
// foi descartada por trocar memória por N round-trips e tender a ser MAIS lenta
// na base pequena típica (single-user local); ver issue #287.
func renormalizeSlugsForTable(tx *gorm.DB, target slugRenormalizationTarget) (updated, suffixed int, err error) {
	if !tx.Migrator().HasTable(target.table) {
		return 0, 0, nil
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

	// 1ª passada (paginada): linhas já canônicas (ou que normalizam para vazio)
	// ocupam seu slug atual; as demais são ignoradas aqui e reprocessadas na 2ª.
	if err := forEachSlugRowPaged(tx, target.table, func(row slugRow) error {
		canonical := normalizeSlug(row.Slug)
		if canonical == "" || canonical == row.Slug {
			occupy(row.UserID, row.Slug)
		}
		return nil
	}); err != nil {
		return 0, 0, err
	}

	// 2ª passada (paginada): aplica as mudanças. Em colisão, deriva um slug
	// canônico único com sufixo numérico para manter a linha endereçável (ver
	// doc da função). A atualização só muda `slug`, então o cursor de keyset por
	// (user_id, id) permanece estável durante a varredura.
	if err := forEachSlugRowPaged(tx, target.table, func(row slugRow) error {
		canonical := normalizeSlug(row.Slug)
		if canonical == "" || canonical == row.Slug {
			return nil
		}
		finalSlug := canonical
		if isTaken(row.UserID, finalSlug) {
			finalSlug = uniqueCanonicalSlug(canonical, func(s string) bool {
				return isTaken(row.UserID, s)
			})
			// WARN: renomeação com sufixo é o caso de risco descrito na
			// "Limitação conhecida" da docstring (referências textuais antigas ao
			// slug podem resolver para o job errado). Logamos o mapeamento
			// completo (user, tabela, slug antigo → slug novo) para auditoria e
			// rollback manual.
			logging.Warnf(context.Background(), "jobs.migration-slug", "[WARN][Jobs] slug legado renomeado com sufixo por colisão (auditar referências textuais): user=%s tabela=%s slug antigo=%q -> slug novo=%q (forma canônica %q já estava ocupada)",
				row.UserID, target.table, row.Slug, finalSlug, canonical)
			suffixed++
		}
		res := tx.Table(target.table).Where("id = ?", row.ID).Update("slug", finalSlug)
		if res.Error != nil {
			return res.Error
		}
		occupy(row.UserID, finalSlug)
		updated++
		return nil
	}); err != nil {
		return updated, suffixed, err
	}

	return updated, suffixed, nil
}

// forEachSlugRowPaged itera todas as linhas (id, user_id, slug) de table em
// páginas ordenadas por (user_id, id), usando paginação por KEYSET (cursor) em
// vez de OFFSET. Com keyset o custo não cresce com o número de páginas já lidas
// e a leitura nunca materializa a tabela inteira: no máximo
// slugRenormalizationPageSize linhas ficam na memória por vez. fn é chamada para
// cada linha em ordem; se retornar erro, a iteração para e o erro é propagado.
//
// A página é totalmente lida (Scan) antes de fn ser chamada, de modo que não há
// cursor aberto enquanto fn eventualmente escreve no mesmo tx — importante para
// SQLite, que serializa leitura/escrita na conexão.
func forEachSlugRowPaged(tx *gorm.DB, table string, fn func(slugRow) error) error {
	// Falha cedo se o tamanho de página for inválido: com LIMIT <= 0 a varredura
	// viraria um noop silencioso (LIMIT 0) ou leria a tabela inteira numa única
	// página (dependendo do dialeto), mascarando bugs em vez de paginar.
	if slugRenormalizationPageSize <= 0 {
		return fmt.Errorf("slugRenormalizationPageSize inválido (%d): deve ser > 0", slugRenormalizationPageSize)
	}
	var lastUser, lastID string
	first := true
	for {
		var page []slugRow
		q := tx.Table(table).
			Select("id", "user_id", "slug").
			Order("user_id, id").
			Limit(slugRenormalizationPageSize)
		if !first {
			// Keyset sobre a chave composta (user_id, id), coerente com o ORDER BY.
			q = q.Where("user_id > ? OR (user_id = ? AND id > ?)", lastUser, lastUser, lastID)
		}
		if err := q.Scan(&page).Error; err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		for i := range page {
			if err := fn(page[i]); err != nil {
				return err
			}
		}
		if len(page) < slugRenormalizationPageSize {
			return nil
		}
		last := page[len(page)-1]
		lastUser, lastID = last.UserID, last.ID
		first = false
	}
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
