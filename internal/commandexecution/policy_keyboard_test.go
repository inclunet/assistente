package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/database"
)

func TestLocalReadPolicyKeyboardRequiresCurrentLocalSession(t *testing.T) {
	db := newPolicyDB(t)
	principal, _, session := newPolicyIdentity(t, db)
	policy, err := NewLocalReadAuthorizer(db, map[string][]string{"workspace.list": {database.UserRoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	if err := policy(context.Background(), principal, "workspace.list", commandcatalog.KeyboardLocal); err != nil {
		t.Fatal(err)
	}
	for _, source := range []commandcatalog.Source{commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck, commandcatalog.Event} {
		if err := policy(context.Background(), principal, "workspace.list", source); !errors.Is(err, ErrDenied) {
			t.Fatalf("origem não montada aceita: %s %v", source, err)
		}
	}
	if err := db.Model(session).Update("revoked_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := policy(context.Background(), principal, "workspace.list", commandcatalog.KeyboardLocal); !errors.Is(err, ErrDenied) {
		t.Fatalf("sessão revogada aceita: %v", err)
	}
}

func TestLocalStreamDeckReadPolicyOnlyAllowsStreamDeckAndHonorsRevocation(t *testing.T) {
	db := newPolicyDB(t)
	principal, _, session := newPolicyIdentity(t, db)
	policy, err := NewLocalStreamDeckReadAuthorizer(db, map[string][]string{"workspace.list": {database.UserRoleUser}})
	if err != nil {
		t.Fatal(err)
	}
	if err := policy(context.Background(), principal, "workspace.list", commandcatalog.StreamDeck); err != nil {
		t.Fatal(err)
	}
	for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.UI, commandcatalog.CLI, commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.Event} {
		if err := policy(context.Background(), principal, "workspace.list", source); !errors.Is(err, ErrDenied) {
			t.Fatalf("origem não Stream Deck aceita: %s %v", source, err)
		}
	}
	if err := db.Model(session).Update("revoked_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := policy(context.Background(), principal, "workspace.list", commandcatalog.StreamDeck); !errors.Is(err, ErrDenied) {
		t.Fatalf("sessão revogada aceita: %v", err)
	}
}
