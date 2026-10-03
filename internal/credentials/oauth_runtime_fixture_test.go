package credentials

import (
	"context"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

type sharedOperationFixture struct {
	a, b          *Manager
	db            *gorm.DB
	ctx           context.Context
	r             oauthflow.Record
	first, second *oauthStore
}

func newSharedOperationFixture(t *testing.T, endpoint, grant string) sharedOperationFixture {
	t.Helper()
	a, b, db, ctx, id := legacyOperationFixture(t)
	if err := a.ClearLegacyOAuth(ctx, "legacy", id, ""); err != nil {
		t.Fatal(err)
	}
	r := oauthflow.Record{Version: 1, Revision: 1, ID: "shared-grant", UserID: "owner", ConsumerID: id, Integration: "mcp", GrantType: grant, State: "connected", Resource: endpoint, Audience: endpoint,
		Client: oauthflow.ClientRegistration{ID: "client", Method: "manual", AuthMethod: "none"}, Endpoints: oauthflow.Endpoints{Token: endpoint + "/token"},
		Tokens: oauthflow.Tokens{Access: "old", Refresh: "old-refresh", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Hour)}}
	if grant == "client_credentials" {
		r.State, r.Tokens = "pending", oauthflow.Tokens{}
		r.Client.AuthMethod, r.Client.Secret = "client_secret_post", "secret"
	}
	one, err := a.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	two, err := b.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, second := one.(*oauthStore), two.(*oauthStore)
	if err := first.CreateWithConsumer(ctx, r, func(tx *gorm.DB) error {
		return tx.Model(&database.MCPServer{}).Where("id = ?", id).Updates(map[string]any{"oauth_managed": true, "oauth_authorization_id": r.ID, "url": endpoint}).Error
	}); err != nil {
		t.Fatal(err)
	}
	return sharedOperationFixture{a: a, b: b, db: db, ctx: ctx, r: r, first: first, second: second}
}

// Independent service gates model distinct processes; only the real encrypted
// vault lease can coordinate them. Protocol and CAS are production oauthflow.
func independentOAuthService(r oauthflow.Record) *oauthflow.Service {
	return oauthflow.New(oauthflow.Integration{ID: r.Integration, Resource: r.Resource, Endpoints: r.Endpoints, GrantType: r.GrantType, ConsumerID: r.ConsumerID, IdentityOptional: true, ClientCredentials: r.GrantType == "client_credentials"})
}
