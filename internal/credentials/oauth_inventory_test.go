package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"assistente/internal/database"
)

func TestInspectLegacyOAuthPersistedScopedAndReadOnly(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	ctx := database.WithUserID(t.Context(), "ana")
	m := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), NewDBStore(), true)
	secret := "NEVER-EXPOSE-THIS-SECRET"
	auth := &AuthConfig{Type: "oauth2", Source: "static", Token: secret, RefreshURL: secret, ClientID: "client", ClientSecret: secret}
	if err := m.RegisterPatternWithContext(ctx, "mcp-client:one", auth); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterPatternWithContext(database.WithUserID(t.Context(), "other"), "other.example", auth); err != nil {
		t.Fatal(err)
	}
	// Deliberately absent from the in-memory cache. Corrupt unrelated headers must
	// not hide the inventory, and external source configuration is never resolved.
	rows := []database.CredentialEntry{
		{UserID: "ana", Pattern: "mcp-tokens:broken", AuthType: "oauth2", Source: "static", RefreshTokenEnc: "old-plaintext"},
		{UserID: "ana", Pattern: "mcp-client:command", AuthType: "oauth2", Source: "command", SourceConfigEnc: "invalid-executable-config"},
		{UserID: "ana", Pattern: "unrelated", AuthType: "custom", HeadersEnc: "invalid-json"},
		{UserID: "", Pattern: "mcp-client:instance", AuthType: "oauth2"},
	}
	for i := range rows {
		if err := database.DB().Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	var before, after []database.CredentialEntry
	if err := database.DB().Order("id").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	items, err := m.InspectLegacyOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("unexpected count: %d", len(items))
	}
	byPattern := map[string]LegacyOAuthEntry{}
	for _, item := range items {
		byPattern[item.Pattern] = item
	}
	got := byPattern["mcp-client:one"]
	if !got.Readable || !got.HasAccess || !got.HasRefresh || !got.HasSecret || got.ClientID != "client" {
		t.Fatal("missing persisted metadata")
	}
	if byPattern["mcp-tokens:broken"].Readable || byPattern["mcp-client:command"].Readable {
		t.Fatal("accepted plaintext or resolved external source")
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte(`"ClientID"`)) {
		t.Fatal("secret in diagnostic serialization")
	}
	if err := database.DB().Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("inventory mutated the vault")
	}
	if _, err := m.InspectLegacyOAuth(context.Background()); err == nil {
		t.Fatal("unscoped inventory allowed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := m.InspectLegacyOAuth(canceled); err == nil {
		t.Fatal("canceled inventory allowed")
	}
	wrongKey := NewManagerWithStore(bytes.Repeat([]byte{8}, 32), NewDBStore(), true)
	items, err = wrongKey.InspectLegacyOAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Readable {
			t.Fatal("wrong key accepted")
		}
	}
}

func TestLegacyOAuthHostMatchingUsesResolverSemantics(t *testing.T) {
	entry := LegacyOAuthEntry{Pattern: "*.example.com"}
	if !entry.MatchesHost("mcp.example.com") || !entry.MatchesHost("MCP.EXAMPLE.COM") || entry.MatchesHost("a.b.example.com") || entry.MatchesHost("example.com") {
		t.Fatal("wildcard matching changed")
	}
	entry.Pattern = "MCP.EXAMPLE.COM"
	if !entry.MatchesHost("MCP.EXAMPLE.COM") {
		t.Fatal("exact native fallback hidden")
	}
}

func TestInspectPublishedLegacyOAuthPreservesPlaintextForInvestigation(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	data, err := os.ReadFile(filepath.Join("testdata", "published", "0.1.9-0.5.0-plaintext-refresh.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture publishedCredentialFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	m := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), NewDBStore(), true)
	access, err := m.encrypt(fixture.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	insertLegacyCredentialEntry(t, fixture.ID, fixture.UserID, fixture.Pattern, access, fixture.RefreshTokenLegacyPlaintext)
	items, err := m.InspectLegacyOAuth(database.WithUserID(t.Context(), fixture.UserID))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Readable {
		t.Fatal("published plaintext treated as a decryptable grant")
	}
	if got := readRefreshTokenEnc(t, fixture.ID); got != fixture.RefreshTokenLegacyPlaintext {
		t.Fatal("published data changed during inventory")
	}
}
