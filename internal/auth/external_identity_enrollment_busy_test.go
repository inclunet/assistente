package auth

import (
	"context"
	"path/filepath"
	"testing"

	"assistente/internal/database"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestExternalIdentityEnrollmentWaitsForWriterWithoutRepeatingAudit(t *testing.T) {
	for _, operation := range []string{"bootstrap", "create"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			dsn := database.SQLiteDSN(filepath.Join(t.TempDir(), "enrollment.db"))
			db := enrollmentDB(t, dsn)
			if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
				t.Fatal(err)
			}
			user, err := NewIdentityService(db).CreateLocalUser(ctx, CreateUserParams{Username: "legacy", Password: "test-password"})
			if err != nil {
				t.Fatal(err)
			}
			service := enrollmentService(t, db, map[string]*ExternalClaims{"admin": {Issuer: enrollmentIssuer, Subject: user.ID, Scope: enrollmentScope}})
			if operation == "create" {
				if _, err := service.BootstrapLegacy(ctx, "admin"); err != nil {
					t.Fatal(err)
				}
			}
			writerDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			writerPool, err := writerDB.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = writerPool.Close() }()
			writer, err := writerPool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = writer.Close() }()
			if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			defer func() { _, _ = writer.ExecContext(ctx, "ROLLBACK") }()
			busyObserved := false
			releaseWriter := func(tx *gorm.DB) {
				if database.IsSQLiteBusyError(tx.Error) && !busyObserved {
					busyObserved = true
					if _, err := writer.ExecContext(ctx, "ROLLBACK"); err != nil {
						t.Error(err)
					}
				}
			}
			// Observe o erro real: não há sleep nem liberação antecipada do writer.
			// Raw cobre BEGIN IMMEDIATE; Create também prova a falha da implementação anterior.
			if err := db.Callback().Raw().After("gorm:raw").Register("test:release_writer", releaseWriter); err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Create().After("gorm:create").Register("test:release_writer", releaseWriter); err != nil {
				t.Fatal(err)
			}
			auditAttempts := 0
			if err := db.Callback().Create().Before("gorm:create").Register("test:count_audit", func(tx *gorm.DB) {
				if tx.Statement.Table == "external_identity_admin_audits" {
					auditAttempts++
				}
			}); err != nil {
				t.Fatal(err)
			}
			var mapping *ExternalIdentityMapping
			subject := user.ID
			if operation == "bootstrap" {
				mapping, err = service.BootstrapLegacy(ctx, "admin")
			} else {
				subject = "new-subject"
				mapping, err = service.CreateMapped(ctx, "admin", ExternalIdentityMappingParams{Issuer: enrollmentIssuer, Subject: subject, UserID: user.ID})
			}
			if !busyObserved {
				t.Fatal("writer contention was not exercised")
			}
			if err != nil {
				t.Fatalf("enrollment after transient contention: %v", err)
			}
			if mapping == nil || mapping.Subject != subject || mapping.UserID != user.ID {
				t.Fatalf("unexpected mapping: %+v", mapping)
			}
			if auditAttempts != 1 {
				t.Fatalf("audit attempts=%d want 1", auditAttempts)
			}
			wantRows := 1
			if operation == "create" {
				wantRows = 2
			}
			if rows := auditRows(t, db); len(rows) != wantRows {
				t.Fatalf("audit rows=%d want %d", len(rows), wantRows)
			}
			resolved, err := service.repo.Resolve(ctx, enrollmentIssuer, subject)
			if err != nil || resolved.UserID != user.ID {
				t.Fatalf("persisted mapping=%+v err=%v", resolved, err)
			}
		})
	}
}
