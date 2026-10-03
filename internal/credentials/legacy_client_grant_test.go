package credentials

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func TestLegacyClientGrantExpiredLeaseCanRetryWithoutReauthorization(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_secret") != "secret" || r.Form.Get("refresh_token") != "" {
			t.Error("client grant reused rotating refresh or lost secret")
		}
		if call == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		access := "current"
		if call == 1 {
			access = "late"
		}
		_, _ = io.WriteString(w, `{"access_token":"`+access+`","refresh_token":"must-not-persist","token_type":"Bearer","expires_in":3600}`)
	}))
	defer func() { unblock(); server.Close() }()
	f := newSharedOperationFixture(t, server.URL, "client_credentials")
	first, second := independentOAuthService(f.r), independentOAuthService(f.r)
	done := make(chan error, 1)
	go func() { _, err := first.Resolve(f.ctx, f.first, f.r.ID, f.r.Resource, ""); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("client grant not started")
	}
	if _, err := second.Resolve(f.ctx, f.second, f.r.ID, f.r.Resource, ""); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatal("parallel operation allowed", err)
	}
	active, err := f.second.Load(f.ctx, f.r.ID)
	if err != nil || !active.RefreshActive() || active.Tokens.Refresh != "" {
		t.Fatal("missing client-grant lease or rotating state", err)
	}
	// Simulate an expired process lease while its late response is still in flight.
	active.RefreshUntil = time.Now().Add(-time.Minute)
	active.Revision++
	if err := f.second.CompareAndSwap(f.ctx, active, active.Revision-1); err != nil {
		t.Fatal(err)
	}
	current, err := second.Resolve(f.ctx, f.second, f.r.ID, f.r.Resource, "")
	if err != nil || current.Tokens.Access != "current" || current.Tokens.Refresh != "" || current.RefreshPending {
		t.Fatal("crash required reauthorization or persisted rotating state", err)
	}
	unblock()
	if err := <-done; !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("stale completion accepted", err)
	}
	persisted, err := f.first.Load(f.ctx, f.r.ID)
	if err != nil || persisted.Tokens.Access != current.Tokens.Access || persisted.Tokens.Refresh != current.Tokens.Refresh || persisted.Tokens.Type != current.Tokens.Type || persisted.Tokens.ID != current.Tokens.ID || !persisted.Tokens.ExpiresAt.Equal(current.Tokens.ExpiresAt) || !persisted.Tokens.EarliestRefreshAt.Equal(current.Tokens.EarliestRefreshAt) || persisted.RefreshPending || calls.Load() != 2 {
		t.Fatal("late completion replaced current grant", err)
	}
	var row database.CredentialEntry
	if err := f.db.First(&row, "id = ?", f.r.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.OAuthEnc == "" || strings.Contains(row.OAuthEnc, "secret") || row.LegacyOAuthControlEnc != "" || row.TokenEnc != "" || row.RefreshTokenEnc != "" {
		t.Fatal("grant left split secret material or legacy control")
	}
}

func TestLegacyClientGrantAcquisitionRollsBackAndRefusesPKCEResidue(t *testing.T) {
	a, _, db, ctx, id := legacyOperationFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"recovered","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Updates(database.MCPServer{AuthType: "oauth2_client_credentials", URL: server.URL, OAuth2TokenURL: server.URL + "/token"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prepare := func(row database.MCPServer, auth *AuthConfig, credentialID string) (oauthflow.Record, database.MCPServer, error) {
		record := oauthflow.Record{Version: 1, Revision: 1, ID: credentialID, UserID: row.UserID, ConsumerID: row.ID, Integration: "mcp", GrantType: "client_credentials", State: "pending", Resource: row.URL, Endpoints: oauthflow.Endpoints{Token: row.OAuth2TokenURL}, Client: oauthflow.ClientRegistration{ID: auth.ClientID, Secret: auth.ClientSecret, Method: "manual", AuthMethod: "client_secret_post"}}
		row.OAuthManaged, row.OAuthAuthorizationID = true, credentialID
		row.OAuth2ClientID, row.OAuth2TokenURL = "", ""
		return record, row, nil
	}
	for _, residue := range []string{"refresh", "access_only"} {
		if residue == "access_only" {
			if err := a.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &AuthConfig{Source: "static", Type: "oauth2", Token: "access-only-residue"}); err != nil {
				t.Fatal(err)
			}
		}
		var before, after database.CredentialEntry
		if err := db.Where("pattern = ?", "mcp-tokens:legacy").First(&before).Error; err != nil {
			t.Fatal(err)
		}
		snapshot, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.ConvertLegacyClientCredentials(ctx, dir, snapshot.ID, "client_secret_post", prepare, nil); err == nil {
			t.Fatalf("%s PKCE residue repurposed", residue)
		}
		if err := db.First(&after, "id = ?", before.ID).Error; err != nil {
			t.Fatal(err)
		}
		if before.TokenEnc == "" || before.TokenEnc != after.TokenEnc || before.RefreshTokenEnc != after.RefreshTokenEnc || after.LegacyOAuthControlEnc != "" {
			t.Fatal("rejected residue altered or acquired a lease")
		}
	}
	if err := a.DeletePattern(ctx, "mcp-tokens:legacy"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.CreateLegacyOAuthSnapshot(ctx, dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ConvertLegacyClientCredentials(ctx, dir, snapshot.ID, "client_secret_post", prepare, nil); err != nil {
		t.Fatal(err)
	}
	var consumer database.MCPServer
	if err := db.First(&consumer, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	store, err := a.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load(ctx, consumer.OAuthAuthorizationID)
	if err != nil {
		t.Fatal(err)
	}
	service := independentOAuthService(before)
	if err := db.Callback().Update().Before("gorm:update").Register("deny_cc_lease", func(tx *gorm.DB) {
		if fields, ok := tx.Statement.Dest.(map[string]any); ok && fields["oauth_enc"] != nil {
			_ = tx.AddError(errors.New("lease write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("deny_cc_lease") })
	if _, err := service.Resolve(ctx, store, before.ID, before.Resource, ""); err == nil {
		t.Fatal("failed lease acquired")
	}
	after, err := store.Load(ctx, before.ID)
	if err != nil || after.Revision != before.Revision || after.Tokens != before.Tokens || after.RefreshPending || calls.Load() != 0 {
		t.Fatal("failed lease changed grant or reached endpoint", err)
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "mcp-tokens:legacy").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial legacy token row persisted", err)
	}
	if err := db.Callback().Update().Remove("deny_cc_lease"); err != nil {
		t.Fatal(err)
	}
	if r, err := service.Resolve(ctx, store, before.ID, before.Resource, ""); err != nil || r.Tokens.Access != "recovered" || r.Tokens.Refresh != "" || calls.Load() != 1 {
		t.Fatal("lease could not recover", err)
	}
}
