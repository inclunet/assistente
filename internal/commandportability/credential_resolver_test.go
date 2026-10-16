package commandportability

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newCredentialResolverTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:credential-resolver?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestCredentialPatternResolverUsesExactDestinationOwnerMetadata(t *testing.T) {
	db := newCredentialResolverTestDB(t)
	owner := uuid.Must(uuid.NewV7()).String()
	foreign := uuid.Must(uuid.NewV7()).String()
	otherForeign := uuid.Must(uuid.NewV7()).String()
	if err := db.Create([]database.CredentialEntry{
		{UUIDModel: database.UUIDModel{ID: "owned-id"}, UserID: owner, Pattern: "api.example", TokenEnc: "not-read"},
		{UUIDModel: database.UUIDModel{ID: "foreign-id"}, UserID: foreign, Pattern: "foreign.example", TokenEnc: "not-read"},
		{UUIDModel: database.UUIDModel{ID: "ambiguous-a"}, UserID: foreign, Pattern: "shared.example", TokenEnc: "not-read"},
		{UUIDModel: database.UUIDModel{ID: "ambiguous-b"}, UserID: otherForeign, Pattern: "shared.example", TokenEnc: "not-read"},
		{UUIDModel: database.UUIDModel{ID: "same-pattern-foreign"}, UserID: foreign, Pattern: "api.example", TokenEnc: "not-read"},
		{UUIDModel: database.UUIDModel{ID: "literal-wildcard"}, UserID: owner, Pattern: "*.github.com", TokenEnc: "not-read"},
		{UUIDModel: database.UUIDModel{ID: "literal-underscore"}, UserID: owner, Pattern: "foo_bar", TokenEnc: "not-read"},
		{UUIDModel: database.UUIDModel{ID: "wildcard-lookalike"}, UserID: foreign, Pattern: "api.github.com", TokenEnc: "not-read"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	resolve := NewCredentialPatternResolver(db)
	ctx := database.WithUserID(context.Background(), owner)

	tests := []struct {
		name    string
		pattern string
		want    CredentialStatus
	}{
		{name: "available exact owner wins over foreign same pattern", pattern: "api.example", want: CredentialAvailable},
		{name: "missing", pattern: "missing.example", want: CredentialMissing},
		{name: "foreign", pattern: "foreign.example", want: CredentialForeign},
		{name: "foreign candidates are not destination ambiguity", pattern: "shared.example", want: CredentialForeign},
		{name: "id is not a fallback", pattern: "owned-id", want: CredentialMissing},
		{name: "literal wildcard pattern", pattern: "*.github.com", want: CredentialAvailable},
		{name: "literal underscore pattern", pattern: "foo_bar", want: CredentialAvailable},
		{name: "wildcard-like other pattern is isolated", pattern: "api.github.com", want: CredentialForeign},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolve(ctx, tc.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("status para %q: got %q, want %q", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestCredentialPatternResolverRequiresOwnerAndRejectsApproximateOrManagedPatterns(t *testing.T) {
	db := newCredentialResolverTestDB(t)
	resolve := NewCredentialPatternResolver(db)
	owner := database.WithUserID(context.Background(), uuid.Must(uuid.NewV7()).String())

	if _, err := resolve(context.Background(), "api.example"); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("sem owner: got %v, want ErrUserScopeRequired", err)
	}
	for _, pattern := range []string{"", " api.example", "internal-auth:refresh-token"} {
		t.Run(pattern, func(t *testing.T) {
			_, err := resolve(owner, pattern)
			if pattern == "internal-auth:refresh-token" {
				if !errors.Is(err, ErrCredentialPatternManaged) {
					t.Fatalf("managed pattern: got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrCredentialPatternInvalid) {
				t.Fatalf("pattern inválido %q: got %v", pattern, err)
			}
		})
	}
}

func TestCredentialPatternResolverReadsMetadataWithoutDecryptingSecrets(t *testing.T) {
	db := newCredentialResolverTestDB(t)
	owner := uuid.Must(uuid.NewV7()).String()
	if err := db.Create(&database.CredentialEntry{
		UUIDModel: database.UUIDModel{ID: "opaque"}, UserID: owner, Pattern: "opaque.example", TokenEnc: "malformed-ciphertext",
	}).Error; err != nil {
		t.Fatal(err)
	}
	status, err := NewCredentialPatternResolver(db)(database.WithUserID(context.Background(), owner), "opaque.example")
	if err != nil {
		t.Fatal(err)
	}
	if status != CredentialAvailable {
		t.Fatalf("metadata disponível: got %q", status)
	}
}

func TestCredentialPatternResolverReportsAmbiguousLegacyDuplicateForDestinationOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:credential-resolver-legacy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Este schema representa uma base legada/corrompida antes do índice
	// (user_id, pattern); a duplicidade é intencional para testar fail-closed.
	if err := db.Exec(`CREATE TABLE credential_entries (id TEXT PRIMARY KEY, user_id TEXT, pattern TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	owner := uuid.Must(uuid.NewV7()).String()
	if err := db.Exec(`INSERT INTO credential_entries (id, user_id, pattern) VALUES (?, ?, ?), (?, ?, ?)`,
		"duplicate-a", owner, "api.example", "duplicate-b", owner, "api.example").Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), owner)
	status, err := NewCredentialPatternResolver(db)(ctx, "api.example")
	if err != nil {
		t.Fatal(err)
	}
	if status != CredentialAmbiguous {
		t.Fatalf("duplicidade do owner: got %q, want %q", status, CredentialAmbiguous)
	}
}

func TestCredentialPatternResolverPropagatesCancellationAndDatabaseFailure(t *testing.T) {
	db := newCredentialResolverTestDB(t)
	owner := uuid.Must(uuid.NewV7()).String()
	resolve := NewCredentialPatternResolver(db)
	ctx, cancel := context.WithCancel(database.WithUserID(context.Background(), owner))
	cancel()
	if _, err := resolve(ctx, "api.example"); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado: got %v, want context.Canceled", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(database.WithUserID(context.Background(), owner), "api.example"); err == nil {
		t.Fatal("DB fechado foi tratado como missing sem erro")
	}
}
