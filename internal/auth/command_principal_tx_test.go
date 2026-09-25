package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRevalidateLocalSessionTxUsesTransactionAndRechecksState(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*gorm.DB, *SessionService, *database.User, *TokenPair, time.Time) error
		wantOK bool
	}{
		{name: "valid", wantOK: true},
		{
			name: "revoked",
			mutate: func(tx *gorm.DB, _ *SessionService, _ *database.User, issued *TokenPair, _ time.Time) error {
				return tx.Model(&database.Session{}).Where("id = ?", issued.SessionID).Update("revoked_at", time.Now().UTC()).Error
			},
		},
		{
			name: "expired",
			mutate: func(tx *gorm.DB, _ *SessionService, _ *database.User, issued *TokenPair, now time.Time) error {
				return tx.Model(&database.Session{}).Where("id = ?", issued.SessionID).Update("expires_at", now.Add(-time.Second)).Error
			},
		},
		{
			name: "inactive user",
			mutate: func(tx *gorm.DB, _ *SessionService, user *database.User, _ *TokenPair, _ time.Time) error {
				return tx.Model(&database.User{}).Where("id = ?", user.ID).Update("is_active", false).Error
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service, user, now := newCommandPrincipalFixture(t)
			sqlDB, err := service.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			issued, err := service.IssueSession(context.Background(), user, "tx-test")
			if err != nil {
				t.Fatal(err)
			}
			principal := LocalSessionPrincipal{UserID: user.ID, SessionID: issued.SessionID}

			err = service.db.Transaction(func(tx *gorm.DB) error {
				if test.mutate != nil {
					if err := test.mutate(tx, service, user, issued, now); err != nil {
						return err
					}
				}
				got, err := service.RevalidateLocalSessionTx(context.Background(), tx, principal)
				if test.wantOK {
					if err != nil || got != principal {
						t.Fatalf("sessão válida: principal=%+v err=%v", got, err)
					}
					return nil
				}
				if !errors.Is(err, ErrUnauthenticatedLocalSession) || got != (LocalSessionPrincipal{}) {
					t.Fatalf("sessão inválida aceita: principal=%+v err=%v", got, err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRevalidateLocalSessionTxPreservesCancellationAndInfrastructureError(t *testing.T) {
	service, user, _ := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "tx-test")
	if err != nil {
		t.Fatal(err)
	}
	principal := LocalSessionPrincipal{UserID: user.ID, SessionID: issued.SessionID}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.db.Transaction(func(tx *gorm.DB) error {
		_, gotErr := service.RevalidateLocalSessionTx(canceled, tx, principal)
		if !errors.Is(gotErr, context.Canceled) {
			t.Fatalf("cancelamento não preservado: %v", gotErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	want := errors.New("injected tx query failure")
	const callbackName = "command_principal_tx_test_query_failure"
	if err := service.db.Callback().Query().Before("gorm:query").Register(callbackName, func(db *gorm.DB) {
		db.Error = want
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.db.Callback().Query().Remove(callbackName) }()
	if err := service.db.Transaction(func(tx *gorm.DB) error {
		_, gotErr := service.RevalidateLocalSessionTx(context.Background(), tx, principal)
		if !errors.Is(gotErr, want) {
			t.Fatalf("erro de infraestrutura não preservado: %v", gotErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRevalidateLocalSessionPreservesCancellationDuringQuery(t *testing.T) {
	service, user, _ := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "tx-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const callbackName = "command_principal_cancel_during_query"
	if err := service.db.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		cancel()
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.db.Callback().Query().Remove(callbackName) }()
	principal, gotErr := service.RevalidateLocalSession(ctx, LocalSessionPrincipal{UserID: user.ID, SessionID: issued.SessionID})
	if !errors.Is(gotErr, context.Canceled) || principal != (LocalSessionPrincipal{}) {
		t.Fatalf("cancelamento durante consulta foi achatado: principal=%+v err=%v", principal, gotErr)
	}
}

func TestRevalidateLocalSessionTxRejectsForeignRoot(t *testing.T) {
	service, user, _ := newCommandPrincipalFixture(t)
	issued, err := service.IssueSession(context.Background(), user, "tx-test")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	foreignSQL, err := foreign.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := foreignSQL.Close(); err != nil {
			t.Error(err)
		}
	}()
	principal := LocalSessionPrincipal{UserID: user.ID, SessionID: issued.SessionID}
	if err := foreign.Transaction(func(tx *gorm.DB) error {
		got, gotErr := service.RevalidateLocalSessionTx(context.Background(), tx, principal)
		if !errors.Is(gotErr, ErrUnauthenticatedLocalSession) || got != (LocalSessionPrincipal{}) {
			t.Fatalf("foreign root aceito: principal=%+v err=%v", got, gotErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
