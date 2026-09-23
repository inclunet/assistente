package auth

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	enrollmentIssuer = "https://idp.example"
	enrollmentScope  = "assistente:identity:admin"
)

func enrollmentDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.User{}, &ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func enrollmentService(t *testing.T, db *gorm.DB, claims map[string]*ExternalClaims) *ExternalIdentityAdminService {
	t.Helper()
	service, err := NewExternalIdentityAdminService(commandExternalVerifierStub{claims: claims}, NewExternalIdentityRepository(db), ExternalIdentityAdminConfig{
		Issuer: enrollmentIssuer, AdminScopes: []string{enrollmentScope}, AdminRoles: []string{"admin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func auditRows(t *testing.T, db *gorm.DB) []database.ExternalIdentityAdminAudit {
	t.Helper()
	var rows []database.ExternalIdentityAdminAudit
	if err := db.Order("created_at, id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestExternalIdentityBootstrapRequiresIssuerScopeAndLegacyUUIDSubject(t *testing.T) {
	db := enrollmentDB(t, filepath.Join(t.TempDir(), "enrollment.db"))
	user, err := NewIdentityService(db).CreateLocalUser(context.Background(), CreateUserParams{Username: "legacy", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]*ExternalClaims{
		"valid":        {Issuer: enrollmentIssuer, Subject: user.ID, Scope: enrollmentScope},
		"wrong-issuer": {Issuer: "https://other.example", Subject: user.ID, Scope: enrollmentScope},
		"no-scope":     {Issuer: enrollmentIssuer, Subject: user.ID, Scope: "read", Roles: []string{"admin"}},
		"bad-subject":  {Issuer: enrollmentIssuer, Subject: "not-a-local-id", Scope: enrollmentScope},
	}
	service := enrollmentService(t, db, claims)
	for _, tc := range []struct {
		token string
		want  error
	}{{"invalid", ErrExternalAdministratorRequired}, {"wrong-issuer", ErrExternalAdministratorRequired}, {"no-scope", ErrExternalAdminScopeRequired}, {"bad-subject", ErrExternalTargetUserRequired}} {
		if _, err := service.BootstrapLegacy(context.Background(), tc.token); !errors.Is(err, tc.want) {
			t.Errorf("BootstrapLegacy(%q) error=%v want=%v", tc.token, err, tc.want)
		}
	}
	if rows := auditRows(t, db); len(rows) != 0 {
		t.Fatalf("rejected bootstrap left audit rows: %+v", rows)
	}
	mapping, err := service.BootstrapLegacy(context.Background(), "valid")
	if err != nil || mapping.UserID != user.ID || mapping.Subject != user.ID || mapping.Issuer != enrollmentIssuer {
		t.Fatalf("bootstrap mapping=%+v err=%v", mapping, err)
	}
	if _, err := service.BootstrapLegacy(context.Background(), "valid"); !errors.Is(err, ErrExternalIdentityAlreadyMapped) {
		t.Fatalf("second bootstrap error=%v want already mapped", err)
	}
	rows := auditRows(t, db)
	if len(rows) != 1 || rows[0].Action != "bootstrap" || rows[0].ActorSubject != user.ID || rows[0].ActorUserID != user.ID || rows[0].Subject != user.ID || rows[0].UserID != user.ID {
		t.Fatalf("bootstrap audit mismatch: %+v", rows)
	}
}

func TestExternalIdentityCreateMappedRequiresBootstrapAndActiveMappedAdmin(t *testing.T) {
	db := enrollmentDB(t, filepath.Join(t.TempDir(), "enrollment.db"))
	ids := NewIdentityService(db)
	admin, err := ids.CreateLocalUser(context.Background(), CreateUserParams{Username: "legacy-admin", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ids.CreateLocalUser(context.Background(), CreateUserParams{Username: "target", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	const rawToken = "signed-admin-token"
	service := enrollmentService(t, db, map[string]*ExternalClaims{
		rawToken: {Issuer: enrollmentIssuer, Subject: admin.ID, Scope: enrollmentScope},
	})
	params := ExternalIdentityMappingParams{Issuer: enrollmentIssuer, Subject: "remote-target", UserID: target.ID}
	if _, err := service.CreateMapped(context.Background(), rawToken, params); !errors.Is(err, ErrExternalAdministratorRequired) {
		t.Fatalf("create without mapped admin error=%v want admin required", err)
	}
	if rows := auditRows(t, db); len(rows) != 0 {
		t.Fatalf("pre-bootstrap create left audit rows: %+v", rows)
	}
	if _, err := service.BootstrapLegacy(context.Background(), rawToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMapped(context.Background(), rawToken, params); err != nil {
		t.Fatalf("CreateMapped: %v", err)
	}
	if _, err := service.CreateMapped(context.Background(), rawToken, params); !errors.Is(err, ErrExternalIdentityAlreadyMapped) {
		t.Fatalf("duplicate create error=%v want already mapped", err)
	}
	if _, err := service.CreateMapped(context.Background(), rawToken, ExternalIdentityMappingParams{Issuer: "https://other.example", Subject: "x", UserID: target.ID}); !errors.Is(err, ErrExternalIdentityNotMapped) {
		t.Fatalf("wrong target issuer error=%v", err)
	}
	if _, err := service.CreateMapped(context.Background(), rawToken, ExternalIdentityMappingParams{Issuer: enrollmentIssuer, Subject: "other", UserID: "invalid"}); !errors.Is(err, ErrExternalTargetUserRequired) {
		t.Fatalf("invalid target error=%v", err)
	}
	rows := auditRows(t, db)
	if len(rows) != 2 || rows[0].Action != "bootstrap" || rows[1].Action != "create" {
		t.Fatalf("audit rows=%+v want bootstrap + create only", rows)
	}
	for _, row := range rows {
		for _, value := range []string{row.Issuer, row.ActorSubject, row.ActorUserID, row.Action, row.Subject, row.UserID} {
			if strings.Contains(value, rawToken) {
				t.Fatalf("raw token persisted in audit row: %+v", row)
			}
		}
	}
	var rawTokenRows int64
	if err := db.Raw(`SELECT count(*) FROM external_identity_admin_audits WHERE issuer LIKE ? OR actor_subject LIKE ? OR actor_user_id LIKE ? OR action LIKE ? OR subject LIKE ? OR user_id LIKE ?`, "%"+rawToken+"%", "%"+rawToken+"%", "%"+rawToken+"%", "%"+rawToken+"%", "%"+rawToken+"%", "%"+rawToken+"%").Scan(&rawTokenRows).Error; err != nil {
		t.Fatal(err)
	}
	if rawTokenRows != 0 {
		t.Fatalf("raw token appears in %d audit rows", rawTokenRows)
	}
	if err := NewExternalIdentityRepository(db).Revoke(context.Background(), enrollmentIssuer, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMapped(context.Background(), rawToken, ExternalIdentityMappingParams{Issuer: enrollmentIssuer, Subject: "after-revoke", UserID: target.ID}); !errors.Is(err, ErrExternalAdministratorRequired) {
		t.Fatalf("revoked admin create error=%v", err)
	}
	if rows := auditRows(t, db); len(rows) != 2 {
		t.Fatalf("revoked admin added audit: %+v", rows)
	}
}

func TestExternalIdentityCreateMappedWithoutBootstrapAuditReturnsNotReady(t *testing.T) {
	db := enrollmentDB(t, filepath.Join(t.TempDir(), "enrollment.db"))
	ids := NewIdentityService(db)
	admin, err := ids.CreateLocalUser(context.Background(), CreateUserParams{Username: "legacy-admin", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ids.CreateLocalUser(context.Background(), CreateUserParams{Username: "target", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	repo := NewExternalIdentityRepository(db)
	if _, err := repo.Create(context.Background(), ExternalIdentityMappingParams{Issuer: enrollmentIssuer, Subject: admin.ID, UserID: admin.ID}); err != nil {
		t.Fatal(err)
	}
	service := enrollmentService(t, db, map[string]*ExternalClaims{
		"admin": {Issuer: enrollmentIssuer, Subject: admin.ID, Scope: enrollmentScope},
	})
	_, err = service.CreateMapped(context.Background(), "admin", ExternalIdentityMappingParams{Issuer: enrollmentIssuer, Subject: "remote-target", UserID: target.ID})
	if !errors.Is(err, ErrExternalIdentityNotReady) {
		t.Fatalf("create without bootstrap audit error=%v want not ready", err)
	}
	if rows := auditRows(t, db); len(rows) != 0 {
		t.Fatalf("missing-bootstrap refusal left audit rows: %+v", rows)
	}
	if _, err := repo.Resolve(context.Background(), enrollmentIssuer, "remote-target"); !errors.Is(err, ErrExternalIdentityNotMapped) {
		t.Fatalf("missing-bootstrap refusal created target mapping: %v", err)
	}
}

func TestExternalIdentityCreateMappedRollsBackAuditWhenMappingFails(t *testing.T) {
	db := enrollmentDB(t, filepath.Join(t.TempDir(), "enrollment.db"))
	ids := NewIdentityService(db)
	admin, err := ids.CreateLocalUser(context.Background(), CreateUserParams{Username: "legacy-admin", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ids.CreateLocalUser(context.Background(), CreateUserParams{Username: "target", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	service := enrollmentService(t, db, map[string]*ExternalClaims{
		"admin": {Issuer: enrollmentIssuer, Subject: admin.ID, Scope: enrollmentScope},
	})
	if _, err := service.BootstrapLegacy(context.Background(), "admin"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_enrollment_mapping BEFORE INSERT ON external_identity_mappings WHEN NEW.subject = 'rollback-target' BEGIN SELECT RAISE(ABORT, 'forced mapping failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateMapped(context.Background(), "admin", ExternalIdentityMappingParams{Issuer: enrollmentIssuer, Subject: "rollback-target", UserID: target.ID})
	if err == nil {
		t.Fatal("expected mapping trigger to fail")
	}
	rows := auditRows(t, db)
	if len(rows) != 1 || rows[0].Action != "bootstrap" {
		t.Fatalf("audit insert was not rolled back: %+v", rows)
	}
	if _, err := NewExternalIdentityRepository(db).Resolve(context.Background(), enrollmentIssuer, "rollback-target"); !errors.Is(err, ErrExternalIdentityNotMapped) {
		t.Fatalf("failed mapping persisted: %v", err)
	}
}

func TestExternalIdentityBootstrapConcurrentConnectionsHasSingleWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	db1 := enrollmentDB(t, path)
	user, err := NewIdentityService(db1).CreateLocalUser(context.Background(), CreateUserParams{Username: "legacy", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	db2, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB2, err := db2.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB2.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB2.Close() })
	if err := db2.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db2.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
		t.Fatal(err)
	}
	claims := map[string]*ExternalClaims{"admin": {Issuer: enrollmentIssuer, Subject: user.ID, Scope: enrollmentScope}}
	services := []*ExternalIdentityAdminService{enrollmentService(t, db1, claims), enrollmentService(t, db2, claims)}
	start := make(chan struct{})
	results := make(chan error, len(services))
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(service *ExternalIdentityAdminService) {
			defer wg.Done()
			<-start
			_, err := service.BootstrapLegacy(context.Background(), "admin")
			results <- err
		}(service)
	}
	close(start)
	wg.Wait()
	close(results)
	winners, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrExternalIdentityAlreadyMapped):
			conflicts++
		default:
			t.Errorf("concurrent bootstrap error=%v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d; want one each", winners, conflicts)
	}
	if rows := auditRows(t, db1); len(rows) != 1 {
		t.Fatalf("concurrent bootstrap audit rows=%d want 1", len(rows))
	}
}

func TestExternalIdentityEnrollmentWithoutStorageIsNotReady(t *testing.T) {
	service, err := NewExternalIdentityAdminService(commandExternalVerifierStub{}, nil, ExternalIdentityAdminConfig{Issuer: enrollmentIssuer, AdminScopes: []string{enrollmentScope}})
	if err == nil {
		t.Fatal("constructor should reject nil repository")
	}
	if service != nil || !errors.Is(err, ErrExternalIdentityNotReady) {
		t.Fatalf("service=%v err=%v", service, err)
	}
	// A partially initialized service must fail closed without dereferencing storage.
	partial := &ExternalIdentityAdminService{cfg: ExternalIdentityAdminConfig{Issuer: enrollmentIssuer, AdminScopes: []string{enrollmentScope}}}
	if _, err := partial.BootstrapLegacy(context.Background(), "token"); !errors.Is(err, ErrExternalIdentityNotReady) {
		t.Fatalf("nil-storage bootstrap error=%v", err)
	}
	if _, err := partial.CreateMapped(context.Background(), "token", ExternalIdentityMappingParams{}); !errors.Is(err, ErrExternalIdentityNotReady) {
		t.Fatalf("nil-storage create error=%v", err)
	}
}
