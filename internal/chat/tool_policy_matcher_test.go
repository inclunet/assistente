package chat

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/tools"
)

type policyMetadataTool struct {
	name  string
	pkg   string
	optIn bool
}

func (t policyMetadataTool) Name() string                { return t.name }
func (t policyMetadataTool) Description() string         { return t.name }
func (t policyMetadataTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t policyMetadataTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Package: t.pkg}
}
func (t policyMetadataTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}

func TestParseToolPolicySelectorNormalizaAliases(t *testing.T) {
	tests := map[string]string{
		"mcp/atlassian/*":       "mcp/atlassian/*",
		"mcp:atlassian/*":       "mcp/atlassian/*",
		"mcp_atlassian__*":      "mcp/atlassian/*",
		"mcp__atlassian__*":     "mcp/atlassian/*",
		"package/history/*":     "package/history/*",
		"history/*":             "package/history/*",
		"mcp/*":                 "mcp/*",
		"mcp_*__*":              "mcp/*",
		"package/*":             "package/*",
		"*":                     "*",
		"mcp_atlassian__search": "mcp_atlassian__search",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, ok := ParseToolPolicySelector(raw)
			if !ok || got.Canonical != want {
				t.Fatalf("ParseToolPolicySelector(%q) = %#v, %v; esperado %q", raw, got, ok, want)
			}
		})
	}
}

func TestToolPolicyMatcherPrecedenciaEDesempateRestritivo(t *testing.T) {
	matcher := NewToolPolicyMatcher(map[string]string{
		"mcp/*":                       string(ToolPolicyPreloaded),
		"mcp/atlassian/*":             string(ToolPolicyOnDemand),
		"mcp:atlassian/*":             string(ToolPolicyPreloaded),
		"mcp_atlassian__create_issue": string(ToolPolicyDisabled),
		"package/history/*":           string(ToolPolicyPreloaded),
		"history/*":                   string(ToolPolicyOnDemand),
		"search_conversations":        string(ToolPolicyDisabled),
	}, string(ToolPolicyDisabled))

	tests := []struct {
		name   string
		target ToolPolicyTarget
		want   ToolPolicyState
	}{
		{"literal disabled vence wildcard específico", ToolPolicyTarget{Name: "mcp_atlassian__create_issue"}, ToolPolicyDisabled},
		{"wildcard específico vence geral", ToolPolicyTarget{Name: "mcp_atlassian__search"}, ToolPolicyOnDemand},
		{"wildcard geral cobre outro MCP", ToolPolicyTarget{Name: "mcp_slack__send"}, ToolPolicyPreloaded},
		{"literal builtin vence pacote", ToolPolicyTarget{Name: "search_conversations", Package: "history"}, ToolPolicyDisabled},
		{"aliases empatados escolhem restritivo", ToolPolicyTarget{Name: "history_other", Package: "history"}, ToolPolicyOnDemand},
		{"default rege sem correspondência", ToolPolicyTarget{Name: "read_file", Package: "filesystem"}, ToolPolicyDisabled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := matcher.Resolve(tc.target).State; got != tc.want {
				t.Fatalf("estado = %s, esperado %s", got, tc.want)
			}
		})
	}
}

func TestToolPolicyMatcherWildcardPermissivoNaoElevaOptIn(t *testing.T) {
	matcher := NewToolPolicyMatcher(map[string]string{
		"*":     string(ToolPolicyPreloaded),
		"job":   string(ToolPolicyOnDemand),
		"mcp/*": string(ToolPolicyPreloaded),
	}, string(ToolPolicyOnDemand))

	if got := matcher.Resolve(ToolPolicyTarget{Name: "text_edit", Package: "filesystem", OptIn: true}).State; got != ToolPolicyDisabled {
		t.Fatalf("wildcard não deveria elevar opt-in, got %s", got)
	}
	if got := matcher.Resolve(ToolPolicyTarget{Name: "job", Package: "job", OptIn: true}).State; got != ToolPolicyOnDemand {
		t.Fatalf("literal deveria poder autorizar opt-in, got %s", got)
	}
}

func TestToolPolicySelectorMatchesNamespaces(t *testing.T) {
	mcpAll, _ := ParseToolPolicySelector("mcp/*")
	mcpServer, _ := ParseToolPolicySelector("mcp/atlassian/*")
	pkg, _ := ParseToolPolicySelector("package/history/*")
	native, _ := ParseToolPolicySelector("*")

	mcpTarget := ToolPolicyTarget{Name: "mcp_atlassian__search"}
	nativeTarget := ToolPolicyTarget{Name: "search_conversations", Package: "history"}
	if !mcpAll.Matches(mcpTarget) || !mcpServer.Matches(mcpTarget) || native.Matches(mcpTarget) {
		t.Fatal("seletores MCP/native não reconheceram namespaces")
	}
	if !pkg.Matches(nativeTarget) || !native.Matches(nativeTarget) || mcpAll.Matches(nativeTarget) {
		t.Fatal("seletores de package/native não reconheceram metadata")
	}
}

func TestEffectiveToolPolicyExpandePackagePelosMetadadosDoRegistry(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(policyMetadataTool{name: tools.ToolCatalogName, pkg: "control"})
	registry.MustRegister(policyMetadataTool{name: "search_conversations", pkg: "history"})
	registry.MustRegister(policyMetadataTool{name: "read_file", pkg: "filesystem"})
	registry.MustRegisterOptIn(policyMetadataTool{name: "text_edit", pkg: "filesystem", optIn: true})

	effective := NewToolSelectionPolicy(registry).ResolveEffectiveToolPolicy(ProfileToolConfig{
		ToolPolicyDefault: string(ToolPolicyDisabled),
		ToolPolicy: map[string]string{
			"package/history/*":    string(ToolPolicyPreloaded),
			"package/filesystem/*": string(ToolPolicyOnDemand),
		},
	})

	if got := effective.State("search_conversations"); got != ToolPolicyPreloaded {
		t.Fatalf("package history = %s, esperado preloaded", got)
	}
	if got := effective.State("read_file"); got != ToolPolicyOnDemand {
		t.Fatalf("package filesystem = %s, esperado on_demand", got)
	}
	if got := effective.State("text_edit"); got != ToolPolicyDisabled {
		t.Fatalf("wildcard de package não deve elevar opt-in, got %s", got)
	}
}
