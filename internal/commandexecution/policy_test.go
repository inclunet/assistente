package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newPolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func newPolicyIdentity(t *testing.T, db *gorm.DB) (auth.LocalSessionPrincipal, *database.User, *database.Session) {
	t.Helper()
	userID, _ := uuid.NewV7()
	sessionID, _ := uuid.NewV7()
	user := &database.User{UUIDModel: database.UUIDModel{ID: userID.String()}, Username: "ana", PasswordHash: "hash", Role: database.UserRoleUser, IsActive: true}
	session := &database.Session{UUIDModel: database.UUIDModel{ID: sessionID.String()}, UserID: user.ID, RefreshTokenHash: "refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(session).Error; err != nil {
		t.Fatal(err)
	}
	return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: session.ID}, user, session
}

func TestNewLocalReadAuthorizerValidatesAndClonesPolicy(t *testing.T) {
	db := newPolicyDB(t)
	principal, _, _ := newPolicyIdentity(t, db)
	roles := []string{database.UserRoleUser}
	allowed := map[string][]string{"files.read": roles}
	authorize, err := NewLocalReadAuthorizer(db, allowed)
	if err != nil {
		t.Fatal(err)
	}
	allowed["files.read"][0] = database.UserRoleAdmin
	allowed["other.read"] = []string{database.UserRoleAdmin}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.UI); err != nil {
		t.Fatalf("política deveria ser cloneada: %v", err)
	}

	invalid := []map[string][]string{
		nil,
		{},
		{"filesread": {database.UserRoleUser}},
		{"files.read": {}},
		{"files.read": {database.UserRoleUser, database.UserRoleUser}},
		{"files.read": {"owner"}},
	}
	for _, policy := range invalid {
		if _, err := NewLocalReadAuthorizer(db, policy); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("política inválida aceita: %#v, err=%v", policy, err)
		}
	}
	if _, err := NewLocalReadAuthorizer(nil, map[string][]string{"files.read": {database.UserRoleUser}}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("db nil deveria falhar: %v", err)
	}
}

func TestLocalReadAuthorizerReadsCurrentRoleAndSessionState(t *testing.T) {
	db := newPolicyDB(t)
	principal, user, session := newPolicyIdentity(t, db)
	authorize, err := NewLocalReadAuthorizer(db, map[string][]string{"files.read": {database.UserRoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.Palette); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(user).Update("role", database.UserRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.CLI); !errors.Is(err, ErrDenied) {
		t.Fatalf("troca para admin não deveria conceder acesso sem regra explícita: %v", err)
	}
	// Restaurar uma role permitida: os próximos casos precisam provar a
	// recusa por inatividade/revogação, não repetir a recusa pela role anterior.
	if err := db.Model(user).Update("role", database.UserRoleUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.UI); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(user).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.UI); !errors.Is(err, ErrDenied) {
		t.Fatalf("usuário inativo não deveria autorizar: %v", err)
	}
	if err := db.Model(user).Update("is_active", true).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Model(session).Update("revoked_at", &now).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.UI); !errors.Is(err, ErrDenied) {
		t.Fatalf("sessão revogada não deveria autorizar: %v", err)
	}
	if err := db.Model(session).Update("revoked_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	otherID, _ := uuid.NewV7()
	wrong := principal
	wrong.SessionID = otherID.String()
	if err := authorize(context.Background(), wrong, "files.read", commandcatalog.UI); !errors.Is(err, ErrDenied) {
		t.Fatalf("session mismatch não deveria autorizar: %v", err)
	}
	if err := authorize(context.Background(), principal, "unknown.read", commandcatalog.UI); !errors.Is(err, ErrDenied) {
		t.Fatalf("comando fora da allowlist não deveria autorizar: %v", err)
	}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.Chat); !errors.Is(err, ErrDenied) {
		t.Fatalf("origem não suportada não deveria autorizar: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := authorize(ctx, principal, "files.read", commandcatalog.UI); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento deveria ser preservado: %v", err)
	}
}

func TestLocalReadAuthorizerRejectsSessionExpiredBeforeQuery(t *testing.T) {
	db := newPolicyDB(t)
	principal, _, session := newPolicyIdentity(t, db)
	authorize, err := NewLocalReadAuthorizer(db, map[string][]string{"files.read": {database.UserRoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Second)
	if err := db.Model(session).Update("expires_at", expired).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorize(context.Background(), principal, "files.read", commandcatalog.UI); !errors.Is(err, ErrDenied) {
		t.Fatalf("sessão expirada deveria falhar fechado: %v", err)
	}
}
