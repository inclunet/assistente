package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestExternalIdentityCheckIssuerReadinessRejectsInvalidInput(t *testing.T) {
	db := setupAuthTestDB(t)
	repo := NewExternalIdentityRepository(db)

	tests := []struct {
		name   string
		repo   *ExternalIdentityRepository
		ctx    context.Context
		issuer string
	}{
		{name: "nil repository", repo: nil, ctx: context.Background(), issuer: "issuer"},
		{name: "nil database", repo: &ExternalIdentityRepository{}, ctx: context.Background(), issuer: "issuer"},
		{name: "nil context", repo: repo, ctx: nil, issuer: "issuer"},
		{name: "empty issuer", repo: repo, ctx: context.Background(), issuer: ""},
		{name: "issuer whitespace", repo: repo, ctx: context.Background(), issuer: " issuer"},
		{name: "issuer NUL", repo: repo, ctx: context.Background(), issuer: "issuer\x00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.repo.CheckIssuerReadiness(tc.ctx, tc.issuer); !errors.Is(err, ErrExternalIdentityNotReady) {
				t.Fatalf("CheckIssuerReadiness() error = %v, want %v", err, ErrExternalIdentityNotReady)
			}
		})
	}
}

func TestExternalIdentityCheckIssuerReadinessRequiresBothTablesWithoutCreatingThem(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mappingTable bool
	}{
		{name: "mapping and audit tables absent"},
		{name: "audit table absent", mappingTable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAuthTestDB(t)
			if tc.mappingTable {
				if err := db.AutoMigrate(&ExternalIdentityMapping{}); err != nil {
					t.Fatal(err)
				}
			}
			var tablesBefore int64
			if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tablesBefore).Error; err != nil {
				t.Fatal(err)
			}

			err := NewExternalIdentityRepository(db).CheckIssuerReadiness(context.Background(), "issuer")
			if !errors.Is(err, ErrExternalIdentityNotReady) {
				t.Fatalf("CheckIssuerReadiness() error = %v, want %v", err, ErrExternalIdentityNotReady)
			}
			var tablesAfter int64
			if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tablesAfter).Error; err != nil {
				t.Fatal(err)
			}
			if tablesAfter != tablesBefore {
				t.Fatalf("readiness criou tabela: antes=%d depois=%d", tablesBefore, tablesAfter)
			}
		})
	}
}

func TestExternalIdentityCheckIssuerReadinessRequiresExactBootstrapIssuer(t *testing.T) {
	for _, tc := range []struct {
		name        string
		auditIssuer string
		auditAction string
	}{
		{name: "bootstrap ausente"},
		{name: "issuer errado", auditIssuer: "other-issuer", auditAction: "bootstrap"},
		{name: "ação não é bootstrap", auditIssuer: "issuer", auditAction: "create"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := externalIdentityReadinessDB(t)
			if tc.auditAction != "" {
				insertExternalIdentityAudit(t, db, tc.auditIssuer, tc.auditAction)
			}
			var auditRowsBefore int64
			if err := db.Model(&database.ExternalIdentityAdminAudit{}).Count(&auditRowsBefore).Error; err != nil {
				t.Fatal(err)
			}

			err := NewExternalIdentityRepository(db).CheckIssuerReadiness(context.Background(), "issuer")
			if !errors.Is(err, ErrExternalIdentityNotReady) {
				t.Fatalf("CheckIssuerReadiness() error = %v, want %v", err, ErrExternalIdentityNotReady)
			}
			var auditRowsAfter int64
			if err := db.Model(&database.ExternalIdentityAdminAudit{}).Count(&auditRowsAfter).Error; err != nil {
				t.Fatal(err)
			}
			if auditRowsAfter != auditRowsBefore {
				t.Fatalf("readiness alterou auditoria: antes=%d depois=%d", auditRowsBefore, auditRowsAfter)
			}
		})
	}
}

func TestExternalIdentityCheckIssuerReadinessAcceptsBootstrapEvenIfUserInactive(t *testing.T) {
	db := externalIdentityReadinessDB(t)
	user, err := NewIdentityService(db).CreateLocalUser(context.Background(), CreateUserParams{
		Username: "bootstrap-admin", Password: "unused-password", Admin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	insertExternalIdentityBootstrapAuditForUser(t, db, "issuer", user.ID)
	if err := db.Model(&database.User{}).Where("id = ?", user.ID).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}

	var auditRowsBefore int64
	if err := db.Model(&database.ExternalIdentityAdminAudit{}).Count(&auditRowsBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewExternalIdentityRepository(db).CheckIssuerReadiness(context.Background(), "issuer"); err != nil {
		t.Fatalf("bootstrap registrado deve habilitar readiness mesmo com ator inativo: %v", err)
	}
	var auditRowsAfter int64
	if err := db.Model(&database.ExternalIdentityAdminAudit{}).Count(&auditRowsAfter).Error; err != nil {
		t.Fatal(err)
	}
	if auditRowsAfter != auditRowsBefore {
		t.Fatalf("readiness alterou auditoria: antes=%d depois=%d", auditRowsBefore, auditRowsAfter)
	}
}

func TestExternalIdentityCheckIssuerReadinessReturnsCanceledContext(t *testing.T) {
	db := externalIdentityReadinessDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewExternalIdentityRepository(db).CheckIssuerReadiness(ctx, "issuer"); !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckIssuerReadiness() error = %v, want %v", err, context.Canceled)
	}
}

func externalIdentityReadinessDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupAuthTestDB(t)
	if err := db.AutoMigrate(&ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertExternalIdentityAudit(t *testing.T, db *gorm.DB, issuer, action string) {
	t.Helper()
	user, err := NewIdentityService(db).CreateLocalUser(context.Background(), CreateUserParams{
		Username: "bootstrap-" + issuer, Password: "unused-password", Admin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	insertExternalIdentityAuditForUser(t, db, issuer, action, user.ID)
}

func insertExternalIdentityBootstrapAuditForUser(t *testing.T, db *gorm.DB, issuer, userID string) {
	t.Helper()
	insertExternalIdentityAuditForUser(t, db, issuer, "bootstrap", userID)
}

func insertExternalIdentityAuditForUser(t *testing.T, db *gorm.DB, issuer, action, userID string) {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	row := database.ExternalIdentityAdminAudit{
		ID: id.String(), Issuer: issuer, ActorSubject: userID, ActorUserID: userID,
		Action: action, Subject: userID, UserID: userID, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
}
