package credentials

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
)

func TestLegacyClientGrantExpiredLeaseCanRetryWithoutReauthorization(t *testing.T) {
	a, b, db, ctx, id := legacyOperationFixture(t)
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Update("auth_type", "oauth2_client_credentials").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DeletePattern(ctx, "mcp-tokens:legacy"); err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	op, auth, err := a.BeginLegacyClientGrant(ctx, "legacy", id, nil)
	if err != nil || auth.ClientSecret != "secret" {
		t.Fatal(err)
	}
	if _, _, err := b.BeginLegacyClientGrant(ctx, "legacy", id, nil); !errors.Is(err, oauthflow.ErrTransient) {
		t.Fatal("parallel operation allowed", err)
	}
	if err := op.Commit(ctx, &AuthConfig{Token: "must-not-persist"}); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("client grant persisted rotating state", err)
	}
	plain, err := a.decrypt(op.control)
	if err != nil {
		t.Fatal(err)
	}
	var control legacyOAuthControl
	if err := json.Unmarshal([]byte(plain), &control); err != nil {
		t.Fatal(err)
	}
	if control.Pending {
		t.Fatal("client grant marked as rotating refresh")
	}
	control.Until = time.Now().Add(-time.Minute)
	raw, _ := json.Marshal(control)
	enc, err := a.encrypt(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.CredentialEntry{}).Where("id = ?", op.rowID).Update("legacy_oauth_control_enc", enc).Error; err != nil {
		t.Fatal(err)
	}
	next, _, err := b.BeginLegacyClientGrant(ctx, "legacy", id, nil)
	if err != nil {
		t.Fatal("crash required reauthorization", err)
	}
	defer next.End()
	if err := op.FinishClientGrant(ctx); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("stale completion accepted", err)
	}
	if err := next.FinishClientGrant(ctx); err != nil {
		t.Fatal(err)
	}
	var row database.CredentialEntry
	if err := db.Where("id = ?", op.rowID).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.LegacyOAuthControlEnc != "" || row.TokenEnc != "" || row.RefreshTokenEnc != "" {
		t.Fatal("grant left secret material or control")
	}
}

func TestLegacyClientGrantAcquisitionRollsBackAndRefusesPKCEResidue(t *testing.T) {
	a, _, db, ctx, id := legacyOperationFixture(t)
	if err := db.Model(&database.MCPServer{}).Where("id = ?", id).Update("auth_type", "oauth2_client_credentials").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.RegisterPatternWithContext(ctx, "mcp-client:legacy", &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.BeginLegacyClientGrant(ctx, "legacy", id, nil); err == nil {
		t.Fatal("rotating grant repurposed")
	}
	if err := a.DeletePattern(ctx, "mcp-tokens:legacy"); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("deny_cc_lease", func(tx *gorm.DB) { _ = tx.AddError(errors.New("lease write failure")) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("deny_cc_lease") })
	if _, _, err := a.BeginLegacyClientGrant(ctx, "legacy", id, nil); err == nil {
		t.Fatal("failed lease acquired")
	}
	var count int64
	if err := db.Model(&database.CredentialEntry{}).Where("pattern = ?", "mcp-tokens:legacy").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial token row persisted", err)
	}
}
