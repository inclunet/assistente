package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
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
func (m *registryMockTool) Description() string          { return m.description }
func (m *registryMockTool) Parameters() json.RawMessage  { return m.params }
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

	// Tool inexistente é ignorada silenciosamente
	defs = r.FilterByNames([]string{"read_file", "inexistente"})
	if len(defs) != 1 {
		t.Fatalf("FilterByNames com inexistente esperado 1, obtido %d", len(defs))
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

// ==================== Executor Tests ====================

func TestExecutorSingleSuccess(t *testing.T) {
	r := NewRegistry()
	tool := newRegistryMockTool("echo")
	tool.executeFn = func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
		return ToolResult{Content: "hello world"}, nil
	}
	r.MustRegister(tool)

	exec := NewExecutor(r, DefaultExecutorConfig())
	result := exec.ExecuteOne(context.Background(), ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: FunctionCall{
			Name:      "echo",
			Arguments: `{}`,
		},
	})

	if result.Error != nil {
		t.Fatalf("Erro inesperado: %v", result.Error)
	}
	if result.Result.Content != "hello world" {
		t.Errorf("Content esperado 'hello world', obtido '%s'", result.Result.Content)
	}
	if result.CallID != "call_1" {
		t.Errorf("CallID esperado 'call_1', obtido '%s'", result.CallID)
	}
	if result.ToolName != "echo" {
		t.Errorf("ToolName esperado 'echo', obtido '%s'", result.ToolName)
	}
}

func TestExecutorToolNotFound(t *testing.T) {
	r := NewRegistry()
	exec := NewExecutor(r, DefaultExecutorConfig())

	result := exec.ExecuteOne(context.Background(), ToolCall{
		ID:       "call_1",
		Type:     "function",
		Function: FunctionCall{Name: "nao_existe", Arguments: `{}`},
	})

	if !result.Result.IsError {
		t.Error("Esperava IsError=true para tool não encontrada")
	}
}

func TestExecutorInvalidJSON(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(newRegistryMockTool("test"))

	exec := NewExecutor(r, DefaultExecutorConfig())
	result := exec.ExecuteOne(context.Background(), ToolCall{
		ID:       "call_1",
		Type:     "function",
		Function: FunctionCall{Name: "test", Arguments: `{invalid json`},
	})

	if !result.Result.IsError {
		t.Error("Esperava IsError=true para JSON inválido")
	}
}

func TestExecutorTimeout(t *testing.T) {
	r := NewRegistry()
	tool := newRegistryMockTool("slow")
	tool.executeFn = func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
		select {
		case <-time.After(5 * time.Second):
			return ToolResult{Content: "done"}, nil
		case <-ctx.Done():
			return ToolResult{}, ctx.Err()
		}
	}
	r.MustRegister(tool)

	cfg := DefaultExecutorConfig()
	cfg.ToolTimeout = 100 * time.Millisecond
	exec := NewExecutor(r, cfg)

	result := exec.ExecuteOne(context.Background(), ToolCall{
		ID:       "call_1",
		Type:     "function",
		Function: FunctionCall{Name: "slow", Arguments: `{}`},
	})

	if !result.Result.IsError {
		t.Error("Esperava IsError=true para timeout")
	}
}

func TestExecutorTruncation(t *testing.T) {
	r := NewRegistry()
	tool := newRegistryMockTool("big_output")
	tool.executeFn = func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
		// Gera resultado maior que o limite
		bigContent := make([]byte, 1024)
		for i := range bigContent {
			bigContent[i] = 'x'
		}
		return ToolResult{Content: string(bigContent)}, nil
	}
	r.MustRegister(tool)

	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 100 // Limite baixo para teste
	exec := NewExecutor(r, cfg)

	result := exec.ExecuteOne(context.Background(), ToolCall{
		ID:       "call_1",
		Type:     "function",
		Function: FunctionCall{Name: "big_output", Arguments: `{}`},
	})

	if result.Result.IsError {
		t.Errorf("Não deveria ser erro, apenas truncado: %s", result.Result.Content)
	}
	if result.Result.Metadata["truncated"] != true {
		t.Error("Metadata 'truncated' deveria ser true")
	}
}

func TestExecutorParallel(t *testing.T) {
	r := NewRegistry()

	// Cria 3 tools que demoram 100ms cada
	for i := 0; i < 3; i++ {
		tool := newRegistryMockTool(fmt.Sprintf("tool_%d", i))
		idx := i
		tool.executeFn = func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
			time.Sleep(100 * time.Millisecond)
			return ToolResult{Content: fmt.Sprintf("result_%d", idx)}, nil
		}
		r.MustRegister(tool)
	}

	exec := NewExecutor(r, DefaultExecutorConfig())

	calls := []ToolCall{
		{ID: "call_0", Type: "function", Function: FunctionCall{Name: "tool_0", Arguments: `{}`}},
		{ID: "call_1", Type: "function", Function: FunctionCall{Name: "tool_1", Arguments: `{}`}},
		{ID: "call_2", Type: "function", Function: FunctionCall{Name: "tool_2", Arguments: `{}`}},
	}

	start := time.Now()
	results := exec.ExecuteAll(context.Background(), calls)
	elapsed := time.Since(start)

	// Se executou em paralelo, deve levar ~100ms, não ~300ms
	if elapsed > 250*time.Millisecond {
		t.Errorf("ExecuteAll demorou %v — provavelmente não executou em paralelo", elapsed)
	}

	// Verifica resultados na ordem correta
	for i, result := range results {
		expected := fmt.Sprintf("result_%d", i)
		if result.Result.Content != expected {
			t.Errorf("Resultado[%d]: esperado '%s', obtido '%s'", i, expected, result.Result.Content)
		}
		if result.CallID != fmt.Sprintf("call_%d", i) {
			t.Errorf("CallID[%d]: esperado 'call_%d', obtido '%s'", i, i, result.CallID)
		}
	}
}

func TestExecutorPanicRecovery(t *testing.T) {
	r := NewRegistry()
	tool := newRegistryMockTool("panicker")
	tool.executeFn = func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
		panic("ferramenta explodiu!")
	}
	r.MustRegister(tool)

	exec := NewExecutor(r, DefaultExecutorConfig())
	result := exec.ExecuteOne(context.Background(), ToolCall{
		ID:       "call_1",
		Type:     "function",
		Function: FunctionCall{Name: "panicker", Arguments: `{}`},
	})

	if !result.Result.IsError {
		t.Error("Esperava IsError=true após panic")
	}
}

func TestExecutorContextCancellation(t *testing.T) {
	r := NewRegistry()
	tool := newRegistryMockTool("waiting")
	tool.executeFn = func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
		<-ctx.Done()
		return ToolResult{}, ctx.Err()
	}
	r.MustRegister(tool)

	exec := NewExecutor(r, DefaultExecutorConfig())

	ctx, cancel := context.WithCancel(context.Background())
	// Cancela após 50ms (simula o usuário cancelando)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result := exec.ExecuteOne(ctx, ToolCall{
		ID:       "call_1",
		Type:     "function",
		Function: FunctionCall{Name: "waiting", Arguments: `{}`},
	})

	if !result.Result.IsError {
		t.Error("Esperava IsError=true após cancelamento")
	}
}
