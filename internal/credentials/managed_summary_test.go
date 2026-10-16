package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func TestManagedSummaryIsScopedRedactedAndNonResolving(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	m := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	store, err := m.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := oauthflow.Record{Version: 1, ID: "oauth-id", UserID: "owner", Integration: "mcp", ConsumerID: "server", State: "connected", Revision: 1,
		Client: oauthflow.ClientRegistration{Secret: "secret-client"}, Tokens: oauthflow.Tokens{Access: "secret-access", Refresh: "secret-refresh"}}
	if err := store.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	bot := "secret-bot"
	if err := m.UpdateStaticConnection(ctx, func(*gorm.DB) (StaticConnectionUpdate, error) {
		return StaticConnectionUpdate{Integration: "slack", ConsumerID: "channel", Changes: map[SecretRole]*string{RoleBotToken: &bot}, Commit: func(*gorm.DB, string, map[SecretRole]bool) error { return nil }}, nil
	}); err != nil {
		t.Fatal(err)
	}
	// A corrupt unrelated/legacy entry must never be decrypted by this listing.
	m.credentials = append(m.credentials, &DomainCredential{UserID: "owner", Pattern: "mcp-client:legacy", Auth: &AuthConfig{Token: "not-ciphertext"}})
	rows, err := m.ListManagedCredentials(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	body, _ := json.Marshal(rows)
	for _, secret := range []string{"secret-", "tokens", "client", "components"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("secret field in projection: %s", body)
		}
	}
	if rows[0].Unreadable || rows[0].ConsumerID != "server" || rows[1].Unreadable || rows[1].ConsumerID != "channel" {
		t.Fatalf("metadata: %+v", rows)
	}
	if other, err := m.ListManagedCredentials(database.WithUserID(context.Background(), "other")); err != nil || len(other) != 0 {
		t.Fatalf("cross-user listing: %+v %v", other, err)
	}
	if _, err := m.ListManagedCredentials(context.Background()); err == nil {
		t.Fatal("anonymous listing accepted")
	}
	// Damage only this ciphertext: a safe placeholder remains, other rows survive.
	m.credentials[0].Auth.OAuthEnc = "broken"
	rows, err = m.ListManagedCredentials(ctx)
	if err != nil || len(rows) != 2 || !rows[0].Unreadable || rows[0].ConsumerID != "" || rows[1].Unreadable {
		t.Fatalf("unreadable handling: %+v %v", rows, err)
	}
}
