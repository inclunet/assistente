package tools

import "testing"

func TestRankCatalogEntriesUsesPreferredRecentAndStableFallback(t *testing.T) {
	entries := []ToolCatalogEntry{
		{Name: "zeta", Package: "other", Origin: ToolOriginMCPBridge, Risk: "read"},
		{Name: "alpha", Package: "preferred", Origin: ToolOriginBuiltin, Risk: "read"},
		{Name: "beta", Package: "preferred", Origin: ToolOriginBuiltin, Risk: "read"},
	}
	got := RankCatalogEntries(entries, CatalogDiscoveryOptions{
		PreferredPackages: []string{"preferred"},
		RecentNames:       []string{"beta", "alpha"},
	})
	if len(got) != 3 || got[0].Name != "beta" || got[1].Name != "alpha" || got[2].Name != "zeta" {
		t.Fatalf("ranking inesperado: %#v", got)
	}

	fallback := RankCatalogEntries(entries, CatalogDiscoveryOptions{})
	if fallback[0].Name != "alpha" || fallback[1].Name != "beta" || fallback[2].Name != "zeta" {
		t.Fatalf("fallback não é estável por origem/nome: %#v", fallback)
	}
}

func TestRankCatalogEntriesFindsRelevantToolsInOneSearch(t *testing.T) {
	entries := []ToolCatalogEntry{
		{Name: "write_file", Description: "Write a file", Risk: "write"},
		{Name: "grep_search", Description: "Search file contents", Category: "filesystem", Risk: "read"},
		{Name: "read_file", Description: "Read a file", Category: "filesystem", Risk: "read"},
	}
	got := RankCatalogEntries(entries, CatalogDiscoveryOptions{
		Query:        "buscar arquivos",
		ReadOnlyOnly: true,
		Limit:        2,
	})
	if len(got) != 2 || got[0].Name != "grep_search" || got[1].Name != "read_file" {
		t.Fatalf("descoberta inesperada: %#v", got)
	}
	for _, entry := range got {
		if entry.Risk != "read" {
			t.Fatalf("auto-search elevou risco %q: %#v", entry.Risk, got)
		}
	}
}

func TestLoadedToolStoreRecentUsageIsConversationScoped(t *testing.T) {
	store := NewLoadedToolStore()
	store.RecordUsage("conv-a", "padrao", "read_file", "grep_search", "read_file")
	store.RecordUsage("conv-b", "padrao", "search_conversations")

	got := store.RecentNames("conv-a", "padrao")
	if len(got) != 2 || got[0] != "read_file" || got[1] != "grep_search" {
		t.Fatalf("recência da conversa A = %#v", got)
	}
	if other := store.RecentNames("conv-b", "padrao"); len(other) != 1 || other[0] != "search_conversations" {
		t.Fatalf("recência vazou entre conversas: %#v", other)
	}
	store.RecentNames("conv-a", "outro")
	if got := store.RecentNames("conv-a", "padrao"); len(got) != 0 {
		t.Fatalf("mudança de perfil deveria invalidar recência: %#v", got)
	}
}

func TestLoadedToolStoreClaimsAutoSearchOncePerConversationProfile(t *testing.T) {
	store := NewLoadedToolStore()
	if !store.ClaimAutoSearch("conv-a", "padrao") {
		t.Fatal("primeira tentativa deveria ser autorizada")
	}
	if store.ClaimAutoSearch("conv-a", "padrao") {
		t.Fatal("segunda tentativa não deveria ser autorizada")
	}
	if !store.ClaimAutoSearch("conv-b", "padrao") {
		t.Fatal("outra conversa deveria ter tentativa própria")
	}
	if !store.ClaimAutoSearch("conv-a", "programacao") {
		t.Fatal("mudança de perfil deveria iniciar novo estado runtime")
	}
}
