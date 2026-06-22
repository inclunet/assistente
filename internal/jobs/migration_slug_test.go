package jobs

import (
	"fmt"
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSlugMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, sErr := db.DB(); sErr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(
		&database.Tag{},
		&database.JobPipeline{},
		&database.Job{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// insertJob grava um job diretamente com o slug informado (sem passar por
// normalizeSlug), simulando registros legados persistidos.
func insertJob(t *testing.T, db *gorm.DB, userID, slug string) string {
	t.Helper()
	row := database.Job{
		UserID:        userID,
		Slug:          slug,
		Name:          slug,
		ToolCatalogID: "tool",
		ToolName:      "tool",
		Enabled:       true,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("insert job %q: %v", slug, err)
	}
	return row.ID
}

// insertJobWithID insere um job com ID explícito. Como o BeforeCreate só gera
// UUIDv7 quando o ID está vazio, isso torna a ordenação por (user_id, id)
// determinística nos testes de paginação — sem depender de o UUIDv7 ser
// estritamente monotônico entre inserções no mesmo milissegundo.
func insertJobWithID(t *testing.T, db *gorm.DB, id, userID, slug string) string {
	t.Helper()
	row := database.Job{
		UUIDModel:     database.UUIDModel{ID: id},
		UserID:        userID,
		Slug:          slug,
		Name:          slug,
		ToolCatalogID: "tool",
		ToolName:      "tool",
		Enabled:       true,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("insert job %q (id=%s): %v", slug, id, err)
	}
	return row.ID
}

func jobSlugByID(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var slug string
	if err := db.Table("jobs").Select("slug").Where("id = ?", id).Scan(&slug).Error; err != nil {
		t.Fatalf("read slug %s: %v", id, err)
	}
	return slug
}

func TestRenormalizeLegacySlugs_RenormalizesAcrossTables(t *testing.T) {
	db := setupSlugMigrationDB(t)

	tag := database.Tag{UserID: "user-a", Slug: "café-tag", Name: "Café"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	pipeline := database.JobPipeline{UserID: "user-a", Slug: "My Pipeline!", Name: "My Pipeline"}
	if err := db.Create(&pipeline).Error; err != nil {
		t.Fatalf("insert pipeline: %v", err)
	}
	legacyJobID := insertJob(t, db, "user-a", "café-job")
	canonicalJobID := insertJob(t, db, "user-a", "daily-report")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	var gotTag string
	if err := db.Table("tags").Select("slug").Where("id = ?", tag.ID).Scan(&gotTag).Error; err != nil {
		t.Fatalf("read tag slug: %v", err)
	}
	if gotTag != "cafe-tag" {
		t.Errorf("tag slug = %q, quero %q", gotTag, "cafe-tag")
	}
	var gotPipeline string
	if err := db.Table("job_pipelines").Select("slug").Where("id = ?", pipeline.ID).Scan(&gotPipeline).Error; err != nil {
		t.Fatalf("read pipeline slug: %v", err)
	}
	if gotPipeline != "my-pipeline" {
		t.Errorf("pipeline slug = %q, quero %q", gotPipeline, "my-pipeline")
	}
	if got := jobSlugByID(t, db, legacyJobID); got != "cafe-job" {
		t.Errorf("job legado slug = %q, quero %q", got, "cafe-job")
	}
	if got := jobSlugByID(t, db, canonicalJobID); got != "daily-report" {
		t.Errorf("job canônico slug = %q, quero %q (não deveria mudar)", got, "daily-report")
	}
}

func TestRenormalizeLegacySlugs_RenamesOnCollision(t *testing.T) {
	db := setupSlugMigrationDB(t)

	// Já-canônico tem precedência: ocupa "cafe-report".
	canonicalID := insertJob(t, db, "user-a", "cafe-report")
	// Legado normaliza para o mesmo "cafe-report" → como colide, deve ser
	// re-normalizado para um slug canônico único ("cafe-report-2"), permanecendo
	// endereçável (antes ficava preservado com o slug não-canônico, inalcançável
	// pelos lookups que normalizam a busca para a forma canônica).
	legacyID := insertJob(t, db, "user-a", "Café Report")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	if got := jobSlugByID(t, db, canonicalID); got != "cafe-report" {
		t.Errorf("job canônico slug = %q, quero %q", got, "cafe-report")
	}
	if got := jobSlugByID(t, db, legacyID); got != "cafe-report-2" {
		t.Errorf("job legado slug = %q, quero %q (canônico único por colisão)", got, "cafe-report-2")
	}
	// Nenhum dado apagado: as duas linhas continuam existindo.
	var count int64
	db.Table("jobs").Where("user_id = ?", "user-a").Count(&count)
	if count != 2 {
		t.Errorf("contagem de jobs = %d, quero 2 (nenhuma linha apagada)", count)
	}
	// Ambos os slugs finais são canônicos e estáveis: normalizar de novo não muda.
	for _, s := range []string{"cafe-report", "cafe-report-2"} {
		if got := normalizeSlug(s); got != s {
			t.Errorf("slug final %q não é canônico (normalizeSlug = %q)", s, got)
		}
	}
}

// TestRenormalizeLegacySlugs_CollisionRowsRemainAddressable é a reprodução direta
// do bug do Bugbot: duas linhas do mesmo usuário cujos slugs normalizam para o
// mesmo canônico. Após a migração, AMBAS devem ser encontráveis por um WHERE
// slug = normalizeSlug(?) — simulando GetJob/DeleteJob/jobRowBySlug — e cada
// lookup deve resolver exatamente a linha esperada (sem ambiguidade).
func TestRenormalizeLegacySlugs_CollisionRowsRemainAddressable(t *testing.T) {
	db := setupSlugMigrationDB(t)

	canonicalID := insertJob(t, db, "user-a", "daily-report")
	legacyID := insertJob(t, db, "user-a", "Dáily Report")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	canonicalSlug := jobSlugByID(t, db, canonicalID)
	legacySlug := jobSlugByID(t, db, legacyID)

	if canonicalSlug == legacySlug {
		t.Fatalf("slugs colidiram após migração: ambos = %q", canonicalSlug)
	}

	// jobIDBySlug emula o lookup do repositório: WHERE slug = normalizeSlug(?).
	jobIDBySlug := func(slug string) (string, error) {
		var id string
		err := db.Table("jobs").
			Select("id").
			Where("user_id = ? AND slug = ?", "user-a", normalizeSlug(slug)).
			Scan(&id).Error
		return id, err
	}

	gotCanonical, err := jobIDBySlug(canonicalSlug)
	if err != nil {
		t.Fatalf("lookup canônico: %v", err)
	}
	if gotCanonical != canonicalID {
		t.Errorf("lookup por %q resolveu id %q, quero %q", canonicalSlug, gotCanonical, canonicalID)
	}

	gotLegacy, err := jobIDBySlug(legacySlug)
	if err != nil {
		t.Fatalf("lookup legado: %v", err)
	}
	if gotLegacy != legacyID {
		t.Errorf("lookup por %q resolveu id %q, quero %q", legacySlug, gotLegacy, legacyID)
	}
}

// TestRenormalizeLegacySlugs_MultipleCollisions garante sufixos incrementais
// estáveis quando três linhas normalizam para o mesmo canônico.
func TestRenormalizeLegacySlugs_MultipleCollisions(t *testing.T) {
	db := setupSlugMigrationDB(t)

	idCanonical := insertJob(t, db, "user-a", "relatorio")
	idLegacy1 := insertJob(t, db, "user-a", "Relatório")
	idLegacy2 := insertJob(t, db, "user-a", "RELATORIO!!!")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	got := map[string]string{
		idCanonical: jobSlugByID(t, db, idCanonical),
		idLegacy1:   jobSlugByID(t, db, idLegacy1),
		idLegacy2:   jobSlugByID(t, db, idLegacy2),
	}
	if got[idCanonical] != "relatorio" {
		t.Errorf("canônico = %q, quero %q", got[idCanonical], "relatorio")
	}
	// As duas linhas legadas recebem "relatorio-2" e "relatorio-3" (a ordem entre
	// elas depende da ordenação por id; o que importa é unicidade e canonicidade).
	legacySlugs := map[string]bool{got[idLegacy1]: true, got[idLegacy2]: true}
	for _, want := range []string{"relatorio-2", "relatorio-3"} {
		if !legacySlugs[want] {
			t.Errorf("esperava que alguma linha legada tivesse slug %q; obtido %v", want, got)
		}
	}

	// Todos distintos → índice único (user_id, slug) respeitado.
	seen := map[string]bool{}
	for _, s := range got {
		if seen[s] {
			t.Errorf("slug duplicado após migração: %q", s)
		}
		seen[s] = true
	}
}

// TestRenormalizeLegacySlugs_SuffixAvoidsExistingCanonical garante que o sufixo
// pula um slug canônico já ocupado (ex.: "relatorio-2" já existe).
func TestRenormalizeLegacySlugs_SuffixAvoidsExistingCanonical(t *testing.T) {
	db := setupSlugMigrationDB(t)

	insertJob(t, db, "user-a", "relatorio")
	insertJob(t, db, "user-a", "relatorio-2")
	legacyID := insertJob(t, db, "user-a", "Relatório")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	if got := jobSlugByID(t, db, legacyID); got != "relatorio-3" {
		t.Errorf("legado = %q, quero %q (deve pular relatorio-2 já existente)", got, "relatorio-3")
	}
}

func TestRenormalizeLegacySlugs_ScopedByUser(t *testing.T) {
	db := setupSlugMigrationDB(t)

	// Mesmo slug legado em usuários distintos: cada um é re-normalizado de forma
	// independente, sem colisão entre usuários.
	idA := insertJob(t, db, "user-a", "café-job")
	idB := insertJob(t, db, "user-b", "café-job")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	if got := jobSlugByID(t, db, idA); got != "cafe-job" {
		t.Errorf("job user-a slug = %q, quero %q", got, "cafe-job")
	}
	if got := jobSlugByID(t, db, idB); got != "cafe-job" {
		t.Errorf("job user-b slug = %q, quero %q", got, "cafe-job")
	}
}

func TestRenormalizeLegacySlugs_Idempotent(t *testing.T) {
	db := setupSlugMigrationDB(t)

	legacyID := insertJob(t, db, "user-a", "café-job")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("primeira execução: %v", err)
	}
	first := jobSlugByID(t, db, legacyID)
	if first != "cafe-job" {
		t.Fatalf("após 1ª execução slug = %q, quero %q", first, "cafe-job")
	}

	// 2ª execução não deve alterar nada (já canônico) nem falhar.
	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("segunda execução: %v", err)
	}
	if got := jobSlugByID(t, db, legacyID); got != "cafe-job" {
		t.Errorf("após 2ª execução slug = %q, quero %q", got, "cafe-job")
	}
}

// TestRenormalizeLegacySlugs_PaginatesAcrossPages exercita o caminho de múltiplas
// páginas (keyset): reduz o tamanho de página para um valor pequeno e insere mais
// linhas do que cabem em uma página, garantindo que todas as linhas legadas são
// re-normalizadas mesmo varrendo a tabela em vários lotes.
func TestRenormalizeLegacySlugs_PaginatesAcrossPages(t *testing.T) {
	db := setupSlugMigrationDB(t)

	orig := slugRenormalizationPageSize
	slugRenormalizationPageSize = 2
	t.Cleanup(func() { slugRenormalizationPageSize = orig })

	const total = 7 // > 3 páginas com page size 2
	ids := make([]string, 0, total)
	for i := 0; i < total; i++ {
		// Cada slug legado normaliza para "café-job-N" -> "cafe-job-N", todos distintos.
		ids = append(ids, insertJob(t, db, "user-a", fmt.Sprintf("Café Job %d", i)))
	}

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	for i, id := range ids {
		want := fmt.Sprintf("cafe-job-%d", i)
		if got := jobSlugByID(t, db, id); got != want {
			t.Errorf("linha %d: slug = %q, quero %q", i, got, want)
		}
	}
}

// TestRenormalizeLegacySlugs_CollisionSpanningPages garante a correção das duas
// passadas paginadas: um slug legado em uma página inicial colide com um slug
// já-canônico que só aparece em uma página posterior. O canônico tem precedência
// e o legado deve receber sufixo, mesmo estando em páginas diferentes.
func TestRenormalizeLegacySlugs_CollisionSpanningPages(t *testing.T) {
	db := setupSlugMigrationDB(t)

	orig := slugRenormalizationPageSize
	slugRenormalizationPageSize = 2
	t.Cleanup(func() { slugRenormalizationPageSize = orig })

	// IDs explícitos e lexicograficamente ordenáveis tornam a ordenação por
	// (user_id, id) determinística: o legado fica na 1ª página e o canônico
	// numa posterior, independentemente de o UUIDv7 ser monotônico entre
	// inserções no mesmo milissegundo (evita flakiness). Com page size 2:
	// página 1 = [0001 legado, 0002 filler-1]; canônico (0005) cai depois.
	const idBase = "00000000-0000-7000-8000-00000000000"
	legacyID := insertJobWithID(t, db, idBase+"1", "user-a", "Café Report")
	insertJobWithID(t, db, idBase+"2", "user-a", "filler-1")
	insertJobWithID(t, db, idBase+"3", "user-a", "filler-2")
	insertJobWithID(t, db, idBase+"4", "user-a", "filler-3")
	canonicalID := insertJobWithID(t, db, idBase+"5", "user-a", "cafe-report")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	if got := jobSlugByID(t, db, canonicalID); got != "cafe-report" {
		t.Errorf("canônico = %q, quero %q (precedência mesmo em página posterior)", got, "cafe-report")
	}
	if got := jobSlugByID(t, db, legacyID); got != "cafe-report-2" {
		t.Errorf("legado = %q, quero %q (sufixo por colisão entre páginas)", got, "cafe-report-2")
	}
}

func TestRenormalizeLegacySlugs_NoopWhenTablesMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Sem AutoMigrate: tabelas não existem. A migração deve ser noop sem erro.
	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize sem tabelas: %v", err)
	}
}
