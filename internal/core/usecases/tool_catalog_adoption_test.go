package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/llm"
	"assistente/internal/tools"
)

type adoptionCatalogStore struct {
	entries []tools.ToolCatalogEntry
}

func (s adoptionCatalogStore) ListTools(_ context.Context, filter tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error) {
	allowed := make(map[string]struct{}, len(filter.NameIn))
	for _, name := range filter.NameIn {
		allowed[name] = struct{}{}
	}
	entries := make([]tools.ToolCatalogEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if filter.NameIn != nil {
			if _, ok := allowed[entry.Name]; !ok {
				continue
			}
		}
		if filter.AvailabilityStatus != "" && entry.AvailabilityStatus != filter.AvailabilityStatus {
			continue
		}
		entries = append(entries, entry)
	}
	if filter.Limit > 0 && len(entries) > filter.Limit {
		entries = entries[:filter.Limit]
	}
	return entries, nil
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

func TestCatalogLoadWildcardEvaluatesAllVisibleEntriesAndCapsMatches(t *testing.T) {
	entries := make([]tools.ToolCatalogEntry, 0, 230)
	visible := make([]string, 0, 230)
	for i := 0; i < 205; i++ {
		name := fmt.Sprintf("mcp_slack__tool_%03d", i)
		entries = append(entries, tools.ToolCatalogEntry{Name: name, Package: "mcp:slack", AvailabilityStatus: tools.ToolAvailabilityAvailable})
		visible = append(visible, name)
	}
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("mcp_atlassian__tool_%03d", i)
		entries = append(entries, tools.ToolCatalogEntry{Name: name, Package: "mcp:atlassian", AvailabilityStatus: tools.ToolAvailabilityAvailable})
		visible = append(visible, name)
	}
	ctx := tools.WithToolCatalogRuntime(context.Background(), tools.ToolCatalogRuntime{
		Store:          tools.NewLoadedToolStore(),
		ConversationID: "conv-large",
		ProfileSlug:    "padrao",
		VisibleNames:   visible,
		MatchSelector:  canonicalToolSelectorMatcher,
	})
	result, err := tools.NewCatalogTool(adoptionCatalogStore{entries: entries}).Execute(
		ctx,
		json.RawMessage(`{"action":"load","tools":["mcp/atlassian/*"]}`),
	)
	if err != nil || result.IsError {
		t.Fatalf("load wildcard amplo: result=%#v err=%v", result, err)
	}
	var payload struct {
		LoadedTools   []string `json:"loaded_tools"`
		RejectedTools []struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		} `json:"rejected_tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.LoadedTools) != tools.MaxCatalogWildcardMatches {
		t.Fatalf("loaded=%d, esperado limite %d", len(payload.LoadedTools), tools.MaxCatalogWildcardMatches)
	}
	if len(payload.RejectedTools) != 1 ||
		payload.RejectedTools[0].Name != "mcp/atlassian/*" ||
		payload.RejectedTools[0].Reason != tools.LoadedToolRejectWildcardLimit {
		t.Fatalf("rejeição de limite inesperada: %#v", payload.RejectedTools)
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
