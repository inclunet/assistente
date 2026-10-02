package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestOAuthInventoryClassifiesWithoutInferringMigrationSafety(t *testing.T) {
	configs := []ServerConfig{
		{ID: "pkce", Slug: "pkce", AuthType: AuthOAuth2PKCE, URL: "https://mcp.example.com", OAuth2ClientID: "configured"},
		{ID: "cc", Slug: "cc", AuthType: AuthOAuth2ClientCredentials, OAuth2ClientID: "configured", OAuth2TokenURL: "https://issuer.example/token"},
		{ID: "managed", Slug: "managed", AuthType: AuthOAuth2PKCE, OAuthManaged: true},
	}
	entries := []credentials.LegacyOAuthEntry{
		{Pattern: "mcp-client:pkce", Source: "static", AuthType: "oauth2", Readable: true, ClientID: "different"},
		{Pattern: "mcp-tokens:pkce", Source: "static", AuthType: "oauth2", Readable: false},
		{Pattern: "*.example.com", Source: "static", AuthType: "oauth2", Readable: true},
		{Pattern: "mcp-client:removed", Source: "command", AuthType: "oauth2"},
		{Pattern: "mcp-tokens:managed", Source: "static", AuthType: "oauth2", Readable: true},
	}
	items := classifyOAuthInventory(configs, entries)
	byID := map[string]OAuthInventoryItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if len(items) != 5 {
		t.Fatalf("unexpected inventory: %+v", items)
	}
	for _, issue := range []string{"conflicting_client", "unreadable", "missing_tokens", "missing_endpoint", "unknown_registration", "hostname_credential"} {
		if !slices.Contains(byID["pkce"].Issues, issue) {
			t.Fatalf("missing issue %s", issue)
		}
	}
	if !reflect.DeepEqual(byID["cc"].Issues, []string{"missing_secret"}) || byID["cc"].Kind != "client_credentials" {
		t.Fatal("CC classified as PKCE")
	}
	if !reflect.DeepEqual(byID["managed"].Issues, []string{"legacy_residue"}) {
		t.Fatal("legacy residue hidden")
	}
	if byID["credential:mcp-client:removed"].Kind != "unassociated" {
		t.Fatal("orphan hidden")
	}
	if byID["credential:*.example.com"].Kind != "hostname" {
		t.Fatal("shared credential hidden")
	}
}

func TestOAuthInventoryReportsHostnameSnapshotEligibility(t *testing.T) {
	items := classifyOAuthInventory(nil, []credentials.LegacyOAuthEntry{
		{Pattern: "shared.example", Readable: true, HostnameSnapshotEligible: true},
		{Pattern: "shared.example/private", Readable: true},
	})
	if len(items) != 2 || len(items[0].Issues) != 0 || !reflect.DeepEqual(items[1].Issues, []string{"snapshot_ineligible"}) {
		t.Fatalf("incorrect snapshot eligibility: %+v", items)
	}
}

func TestOAuthInventoryReportsProblemsInManagedResidueAndUnassociatedEntries(t *testing.T) {
	configs := []ServerConfig{{ID: "managed", Slug: "managed", AuthType: AuthOAuth2PKCE, OAuthManaged: true}}
	entries := []credentials.LegacyOAuthEntry{
		{Pattern: "mcp-client:managed", Source: "command", AuthType: "bearer"},
		{Pattern: "mcp-tokens:managed", Source: "static", AuthType: "oauth2", Readable: false},
		{Pattern: "mcp-client:orphan", Source: "static", AuthType: "basic", Readable: false},
	}
	items := classifyOAuthInventory(configs, entries)
	if len(items) != 2 {
		t.Fatalf("unexpected inventory: %+v", items)
	}
	for _, item := range items {
		if item.ID == "managed" {
			if !reflect.DeepEqual(item.Issues, []string{"external_source", "legacy_residue", "unexpected_type", "unreadable"}) {
				t.Fatalf("residue details hidden: %+v", item)
			}
		} else if !slices.Contains(item.Issues, "unexpected_type") || !slices.Contains(item.Issues, "unreadable") {
			t.Fatalf("orphan details hidden: %+v", item)
		}
	}
}

func TestOAuthInventoryDiscardsResultWhenVaultSessionChanges(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	if err := repo.db.Callback().Query().After("gorm:query").Register("reset_inventory_session", func(tx *gorm.DB) {
		if tx.Statement.Table == "mcp_servers" {
			m.credMgr.Reset([]byte("different-key-exactly-32-bytes!!"), true)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repo.db.Callback().Query().Remove("reset_inventory_session") }()
	items, err := m.InspectOAuthInventory(ctx)
	if err == nil || items != nil {
		t.Fatal("published inventory across a vault reset")
	}
}

func TestOAuthInventoryIncludesOnlyBearerHostsMatchingOAuthConsumers(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	cfg := ServerConfig{Slug: "legacy", Name: "Legacy", Transport: TransportStreamable, URL: "https://MCP.EXAMPLE.COM:8443/api", AuthType: AuthOAuth2PKCE}
	if err := repo.SaveServer(ctx, &cfg); err != nil {
		t.Fatal(err)
	}
	static := ServerConfig{Slug: "static", Name: "Static", Transport: TransportStreamable, URL: "https://static.example.net", AuthType: AuthBearer}
	if err := repo.SaveServer(ctx, &static); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"*.example.com", "MCP.EXAMPLE.COM", "unrelated.example.net", "static.example.net"} {
		if err := m.credMgr.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "IMPORTED-SECRET"}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := m.InspectOAuthInventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("unexpected inventory: %+v", items)
	}
	var consumer, hostname, exactHostname bool
	for _, item := range items {
		if item.ID == cfg.ID {
			consumer = slices.Contains(item.Issues, "hostname_credential")
		}
		if item.ID == "credential:*.example.com" {
			hostname = item.Kind == "hostname"
		}
		if item.ID == "credential:MCP.EXAMPLE.COM" {
			exactHostname = item.Kind == "hostname"
		}
	}
	if !consumer || !hostname || !exactHostname {
		t.Fatal("imported hostname token not associated with uppercase OAuth resource")
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "IMPORTED-SECRET") {
		t.Fatal("imported token exposed")
	}
}

func TestOAuthInventoryNoNetworkNoMutationAndUserIsolation(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	legacy := managedConfig(server.URL)
	legacy.OAuthManaged = false
	if err := m.SaveConfig("legacy", legacy); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveConfig("managed", managedConfig(server.URL)); err != nil {
		t.Fatal(err)
	}
	if err := m.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("legacy"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "ACCESS-SECRET", RefreshURL: "REFRESH-SECRET"}); err != nil {
		t.Fatal(err)
	}
	var before, after []database.CredentialEntry
	if err := repo.db.Order("id").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	items, err := m.InspectOAuthInventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two consumers, got %d", len(items))
	}
	for _, item := range items {
		if item.Kind == "managed" && len(item.Issues) != 0 {
			t.Fatalf("invalid managed diagnostics: %+v", item)
		}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), server.URL) {
		t.Fatal("sensitive fields exposed")
	}
	if err := repo.db.Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || requests.Load() != 0 {
		t.Fatal("inventory had side effects")
	}
	other, err := m.InspectOAuthInventory(database.WithUserID(t.Context(), "other-user"))
	if err != nil || len(other) != 0 {
		t.Fatalf("user data leaked: %v", err)
	}
	if _, err := m.InspectOAuthInventory(context.Background()); err == nil {
		t.Fatal("unscoped inventory allowed")
	}
	if err := repo.db.Where("source = ?", "oauth").Delete(&database.CredentialEntry{}).Error; err != nil {
		t.Fatal(err)
	}
	items, err = m.InspectOAuthInventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Kind == "managed" && !slices.Contains(item.Issues, "invalid_reference") {
			t.Fatal("dangling reference hidden")
		}
	}
}
