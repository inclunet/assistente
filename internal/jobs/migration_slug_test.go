package jobs

import (
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

func TestRenormalizeLegacySlugs_SkipsOnCollision(t *testing.T) {
	db := setupSlugMigrationDB(t)

	// Já-canônico tem precedência: ocupa "cafe-report".
	canonicalID := insertJob(t, db, "user-a", "cafe-report")
	// Legado normaliza para o mesmo "cafe-report" → deve ser preservado.
	legacyID := insertJob(t, db, "user-a", "Café Report")

	if err := RenormalizeLegacySlugs(db); err != nil {
		t.Fatalf("renormalize: %v", err)
	}

	if got := jobSlugByID(t, db, canonicalID); got != "cafe-report" {
		t.Errorf("job canônico slug = %q, quero %q", got, "cafe-report")
	}
	if got := jobSlugByID(t, db, legacyID); got != "Café Report" {
		t.Errorf("job legado slug = %q, quero %q (preservado por conflito)", got, "Café Report")
	}
	// Nenhum dado apagado: as duas linhas continuam existindo.
	var count int64
	db.Table("jobs").Where("user_id = ?", "user-a").Count(&count)
	if count != 2 {
		t.Errorf("contagem de jobs = %d, quero 2 (nenhuma linha apagada)", count)
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
