package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// ==================== Mock Tool ====================

// registryMockTool é uma ferramenta de teste para os testes do Registry
type registryMockTool struct {
	name        string
	description string
	params      json.RawMessage
	executeFn   func(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

func (m *registryMockTool) Name() string                { return m.name }
func (m *registryMockTool) Description() string         { return m.description }
func (m *registryMockTool) Parameters() json.RawMessage { return m.params }
func (m *registryMockTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, args)
	}
	return ToolResult{Content: "ok"}, nil
}

func newRegistryMockTool(name string) *registryMockTool {
	return &registryMockTool{
		name:        name,
		description: "Mock tool: " + name,
		params:      json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

func toolNamesForTest(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

// ==================== Registry Tests ====================

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()

	tool := newRegistryMockTool("test_tool")
	err := r.Register(tool)
	if err != nil {
		t.Fatalf("Register falhou: %v", err)
	}

	if !r.Has("test_tool") {
		t.Error("Has retornou false após Register")
	}

	if r.Count() != 1 {
		t.Errorf("Count esperado 1, obtido %d", r.Count())
	}
}

func TestRegistryDuplicateRegister(t *testing.T) {
	r := NewRegistry()

	r.MustRegister(newRegistryMockTool("dup"))

	err := r.Register(newRegistryMockTool("dup"))
	if err == nil {
		t.Error("Esperava erro ao registrar ferramenta duplicada")
	}
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("find_me"))

	tool, ok := r.Get("find_me")
	if !ok {
		t.Fatal("Get retornou false para tool registrada")
	}
	if tool.Name() != "find_me" {
		t.Errorf("Nome esperado 'find_me', obtido '%s'", tool.Name())
	}

	_, ok = r.Get("not_found")
	if ok {
		t.Error("Get retornou true para tool não registrada")
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("beta"))
	r.MustRegister(newRegistryMockTool("alpha"))
	r.MustRegister(newRegistryMockTool("gamma"))

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All esperado 3 tools, obtido %d", len(all))
	}

	// Deve estar ordenado por nome
	if all[0].Name() != "alpha" || all[1].Name() != "beta" || all[2].Name() != "gamma" {
		t.Errorf("All não está ordenado: %s, %s, %s", all[0].Name(), all[1].Name(), all[2].Name())
	}
}

func TestRegistryNames(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("zeta"))
	r.MustRegister(newRegistryMockTool("alpha"))

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("Names esperado 2, obtido %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "zeta" {
		t.Errorf("Names não está ordenado: %v", names)
	}
}

func TestRegistryToDefinitions(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegister(newRegistryMockTool("list_dir"))

	defs := r.ToDefinitions()
	if len(defs) != 2 {
		t.Fatalf("ToDefinitions esperado 2, obtido %d", len(defs))
	}

	for _, def := range defs {
		if def.Type != "function" {
			t.Errorf("Type esperado 'function', obtido '%s'", def.Type)
		}
		if def.Function.Name == "" {
			t.Error("Function.Name está vazio")
		}
		if def.Function.Description == "" {
			t.Error("Function.Description está vazio")
		}
	}
}

func TestRegistryFilterByNames(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegister(newRegistryMockTool("write_file"))
	r.MustRegister(newRegistryMockTool("grep_search"))

	// Filtra apenas 2 das 3
	defs := r.FilterByNames([]string{"read_file", "grep_search"})
	if len(defs) != 2 {
		t.Fatalf("FilterByNames esperado 2, obtido %d", len(defs))
	}
	if defs[0].Function.Name != "grep_search" || defs[1].Function.Name != "read_file" {
		t.Fatalf("FilterByNames deve ordenar por nome estavel, got %#v", defs)
	}

	// Tool inexistente é ignorada silenciosamente
	defs = r.FilterByNames([]string{"read_file", "inexistente"})
	if len(defs) != 1 {
		t.Fatalf("FilterByNames com inexistente esperado 1, obtido %d", len(defs))
	}
}

func TestRegistryDefinitionsKeepRuntimeControlToolsFirst(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("write_file"))
	r.MustRegister(newRegistryMockTool(LoadSkillName))
	r.MustRegister(newRegistryMockTool(ToolCatalogName))
	r.MustRegister(newRegistryMockTool("read_file"))

	defs := r.FilterByNames([]string{"write_file", LoadSkillName, "read_file", ToolCatalogName})
	got := []string{}
	for _, def := range defs {
		got = append(got, def.Function.Name)
	}
	want := []string{ToolCatalogName, LoadSkillName, "read_file", "write_file"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

// ==================== Opt-In Tests ====================

func TestRegistryOptIn_ExcludedFromAll(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegister(newRegistryMockTool("write_file"))
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("All esperado 2 (sem opt-in), obtido %d", len(all))
	}
	for _, tool := range all {
		if tool.Name() == "text_edit" {
			t.Error("text_edit (opt-in) não deveria aparecer em All()")
		}
	}
}

func TestRegistryOptIn_ExcludedFromToDefinitions(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))

	defs := r.ToDefinitions()
	if len(defs) != 1 {
		t.Fatalf("ToDefinitions esperado 1 (sem opt-in), obtido %d", len(defs))
	}
	if defs[0].Function.Name != "read_file" {
		t.Errorf("esperado read_file, obtido %s", defs[0].Function.Name)
	}
}

func TestRegistryOptIn_IncludedInFilterByNames(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))

	defs := r.FilterByNames([]string{"text_edit"})
	if len(defs) != 1 {
		t.Fatalf("FilterByNames esperado 1 (text_edit explícito), obtido %d", len(defs))
	}
	if defs[0].Function.Name != "text_edit" {
		t.Errorf("esperado text_edit, obtido %s", defs[0].Function.Name)
	}
}

func TestRegistryOptIn_FilterOutOptInNames(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegisterOptIn(newRegistryMockTool("job"))

	got := r.FilterOutOptInNames([]string{"job", "read_file", "missing"})
	want := []string{"read_file", "missing"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func TestRegistryDiscoverableOptIn_AppearsOnlyInDiscovery(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))
	r.MustRegisterDiscoverableOptIn(newRegistryMockTool("job"))

	all := r.All()
	if len(all) != 1 || all[0].Name() != "read_file" {
		t.Fatalf("All should exclude opt-in tools, got %#v", toolNamesForTest(all))
	}
	discoverable := r.Discoverable()
	want := []string{"job", "read_file"}
	if got := toolNamesForTest(discoverable); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Discoverable got %#v, want %#v", got, want)
	}
	filtered := r.FilterOutOptInNames([]string{"job", "read_file"})
	if len(filtered) != 1 || filtered[0] != "read_file" {
		t.Fatalf("discoverable opt-in should still be filtered from dynamic expansion: %#v", filtered)
	}
}

func TestRegistryOptIn_IncludedInCountAndNames(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))

	if r.Count() != 2 {
		t.Errorf("Count esperado 2 (inclui opt-in), obtido %d", r.Count())
	}
	names := r.Names()
	if len(names) != 2 {
		t.Errorf("Names esperado 2, obtido %d", len(names))
	}
}

func TestRegistryOptIn_GetStillWorks(t *testing.T) {
	r := NewRegistry()
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))

	tool, ok := r.Get("text_edit")
	if !ok {
		t.Fatal("Get retornou false para tool opt-in")
	}
	if tool.Name() != "text_edit" {
		t.Errorf("nome esperado text_edit, obtido %s", tool.Name())
	}
}

func TestRegistryOptIn_IsOptIn(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("read_file"))
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))

	if r.IsOptIn("read_file") {
		t.Error("read_file não deveria ser opt-in")
	}
	if !r.IsOptIn("text_edit") {
		t.Error("text_edit deveria ser opt-in")
	}
}

func TestRegistryOptIn_UnregisterCleansUp(t *testing.T) {
	r := NewRegistry()
	r.MustRegisterOptIn(newRegistryMockTool("text_edit"))

	if !r.IsOptIn("text_edit") {
		t.Fatal("text_edit deveria ser opt-in antes de unregister")
	}

	r.Unregister("text_edit")

	if r.IsOptIn("text_edit") {
		t.Error("opt-in deveria ser limpo após unregister")
	}
	if r.Has("text_edit") {
		t.Error("tool deveria ser removida após unregister")
	}
}
