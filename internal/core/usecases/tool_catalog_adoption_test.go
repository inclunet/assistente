package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/llm"
	"assistente/internal/tools"
)

type adoptionCatalogStore struct {
	entries []tools.ToolCatalogEntry
}

func (s adoptionCatalogStore) ListTools(context.Context, tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error) {
	return append([]tools.ToolCatalogEntry(nil), s.entries...), nil
}

type adoptionTool struct {
	name string
	meta tools.CatalogMetadata
}

func (t adoptionTool) Name() string                           { return t.name }
func (t adoptionTool) Description() string                    { return t.name }
func (t adoptionTool) Parameters() json.RawMessage            { return json.RawMessage(`{"type":"object"}`) }
func (t adoptionTool) CatalogMetadata() tools.CatalogMetadata { return t.meta }
func (t adoptionTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}

func TestAutoDiscoverReadOnlyToolsNeverElevatesWriteOrOptIn(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(adoptionTool{name: tools.ToolCatalogName, meta: tools.CatalogMetadata{Risk: "read"}})
	registry.MustRegister(adoptionTool{name: "read_file", meta: tools.CatalogMetadata{Package: "coding_readonly", Risk: "read"}})
	registry.MustRegister(adoptionTool{name: "write_file", meta: tools.CatalogMetadata{Package: "coding_edit", Risk: "write"}})
	registry.MustRegisterOptIn(adoptionTool{name: "secret_reader", meta: tools.CatalogMetadata{Package: "secrets", Risk: "read"}})
	policy := chat.NewToolSelectionPolicy(registry).ResolveEffectiveToolPolicy(chat.ProfileToolConfig{
		ToolPolicyDefault: string(chat.ToolPolicyOnDemand),
	})
	store := adoptionCatalogStore{entries: []tools.ToolCatalogEntry{
		{Name: "write_file", Description: "write files", Package: "coding_edit", Risk: "write", AvailabilityStatus: tools.ToolAvailabilityAvailable},
		{Name: "secret_reader", Description: "read secret files", Package: "secrets", Risk: "read", AvailabilityStatus: tools.ToolAvailabilityAvailable},
		{Name: "read_file", Description: "read files", Package: "coding_readonly", Risk: "read", AvailabilityStatus: tools.ToolAvailabilityAvailable},
	}}

	got, err := autoDiscoverReadOnlyTools(context.Background(), store, policy, "read files", []string{"coding_edit"}, nil)
	if err != nil {
		t.Fatalf("autoDiscoverReadOnlyTools: %v", err)
	}
	if len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("auto-search elevou escrita ou opt-in: %#v", got)
	}
}

func TestCanonicalToolSelectorMatcherReusesPolicyGrammar(t *testing.T) {
	entry := tools.ToolCatalogEntry{Name: "mcp_atlassian__search_issues", Package: "mcp:atlassian"}
	if match, wildcard := canonicalToolSelectorMatcher("mcp/atlassian/*", entry); !match || !wildcard {
		t.Fatalf("seletor MCP canônico não casou: match=%v wildcard=%v", match, wildcard)
	}
	if match, wildcard := canonicalToolSelectorMatcher("mcp/slack/*", entry); match || !wildcard {
		t.Fatalf("seletor de outro servidor: match=%v wildcard=%v", match, wildcard)
	}
}

func TestCatalogLoadWildcardUsesCanonicalMatcher(t *testing.T) {
	store := adoptionCatalogStore{entries: []tools.ToolCatalogEntry{
		{Name: "mcp_atlassian__search_issues", Package: "mcp:atlassian", Risk: "network", AvailabilityStatus: tools.ToolAvailabilityAvailable},
		{Name: "mcp_atlassian__get_issue", Package: "mcp:atlassian", Risk: "network", AvailabilityStatus: tools.ToolAvailabilityAvailable},
		{Name: "mcp_slack__search", Package: "mcp:slack", Risk: "network", AvailabilityStatus: tools.ToolAvailabilityAvailable},
	}}
	loaded := tools.NewLoadedToolStore()
	ctx := tools.WithToolCatalogRuntime(context.Background(), tools.ToolCatalogRuntime{
		Store:          loaded,
		ConversationID: "conv-1",
		ProfileSlug:    "padrao",
		VisibleNames: []string{
			"mcp_atlassian__search_issues",
			"mcp_atlassian__get_issue",
			"mcp_slack__search",
		},
		MatchSelector: canonicalToolSelectorMatcher,
	})
	result, err := tools.NewCatalogTool(store).Execute(ctx, json.RawMessage(`{"action":"load","tools":["mcp/atlassian/*"]}`))
	if err != nil || result.IsError {
		t.Fatalf("load wildcard: result=%#v err=%v", result, err)
	}
	var payload struct {
		LoadedTools []string `json:"loaded_tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.LoadedTools) != 2 ||
		payload.LoadedTools[0] != "mcp_atlassian__get_issue" ||
		payload.LoadedTools[1] != "mcp_atlassian__search_issues" {
		t.Fatalf("wildcard carregou conjunto inesperado: %#v", payload.LoadedTools)
	}
}

func TestIsFirstConversationTurn(t *testing.T) {
	if !isFirstConversationTurn([]llm.Message{{Role: "system"}, {Role: "user"}}) {
		t.Fatal("primeiro turno não reconhecido")
	}
	if isFirstConversationTurn([]llm.Message{{Role: "user"}, {Role: "assistant"}, {Role: "user"}}) {
		t.Fatal("segundo turno reconhecido como primeiro")
	}
}
