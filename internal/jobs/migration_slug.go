package jobs

import (
	"fmt"
	"log"

	"assistente/internal/database"

	"gorm.io/gorm"
)

// slugRenormalizationTarget descreve uma tabela cujo slug é gerado por
// normalizeSlug e que pode conter registros legados (slug com acentos ou
// símbolos) gravados antes da unificação do algoritmo em internal/slug
// (issue #255 / PR #281).
type slugRenormalizationTarget struct {
	table string
	model any
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
//   - Não destrutiva: se a forma canônica colidiria com um slug já existente
//     para o mesmo usuário, a linha legada é PRESERVADA sem alteração e um aviso
//     é logado, deixando a resolução para o operador. Nenhum dado é apagado.
//   - Slugs que normalizam para vazio (ex.: só símbolos) são preservados como
//     estão, pois não há forma canônica segura para reescrevê-los.
func RenormalizeLegacySlugs(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	targets := []slugRenormalizationTarget{
		{table: "tags", model: &database.Tag{}},
		{table: "job_pipelines", model: &database.JobPipeline{}},
		{table: "jobs", model: &database.Job{}},
	}
	var totalUpdated, totalSkipped int
	for _, target := range targets {
		updated, skipped, err := renormalizeSlugsForTable(db, target)
		if err != nil {
			return fmt.Errorf("re-normaliza slugs de %s: %w", target.table, err)
		}
		totalUpdated += updated
		totalSkipped += skipped
	}
	if totalUpdated > 0 || totalSkipped > 0 {
		log.Printf("[Jobs] Re-normalização de slugs legados concluída: %d atualizados, %d preservados por conflito", totalUpdated, totalSkipped)
	}
	return nil
}

// renormalizeSlugsForTable re-normaliza os slugs de uma única tabela. Retorna a
// contagem de linhas atualizadas e de linhas preservadas por colisão.
func renormalizeSlugsForTable(db *gorm.DB, target slugRenormalizationTarget) (updated, skipped int, err error) {
	if !db.Migrator().HasTable(target.model) {
		return 0, 0, nil
	}

	type slugRow struct {
		ID     string `gorm:"column:id"`
		UserID string `gorm:"column:user_id"`
		Slug   string `gorm:"column:slug"`
	}
	var rows []slugRow
	if err := db.Table(target.table).
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

	// 2ª passada: aplica as mudanças, pulando colisões.
	for _, change := range toChange {
		if isTaken(change.userID, change.canonical) {
			log.Printf("[Jobs] AVISO: slug legado %q (tabela %s, user %s) colide com %q já existente; preservado sem alteração para resolução manual",
				change.oldSlug, target.table, change.userID, change.canonical)
			// A linha legada continua ocupando seu slug atual.
			occupy(change.userID, change.oldSlug)
			skipped++
			continue
		}
		res := db.Table(target.table).Where("id = ?", change.id).Update("slug", change.canonical)
		if res.Error != nil {
			return updated, skipped, res.Error
		}
		occupy(change.userID, change.canonical)
		updated++
	}

	return updated, skipped, nil
}
