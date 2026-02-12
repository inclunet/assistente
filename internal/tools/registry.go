package tools

import (
	"fmt"
	"sort"
	"sync"
)

// Registry é o registro central de ferramentas disponíveis.
// Thread-safe para registro e consulta simultâneos.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry cria um novo registro de ferramentas vazio.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register registra uma ferramenta no registro.
// Retorna erro se já existir uma ferramenta com o mesmo nome.
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("ferramenta '%s' já registrada", name)
	}

	r.tools[name] = tool
	return nil
}

// MustRegister registra uma ferramenta e faz panic se houver erro.
// Útil para registro na inicialização do app.
func (r *Registry) MustRegister(tool Tool) {
	if err := r.Register(tool); err != nil {
		panic(err)
	}
}

// Get retorna uma ferramenta pelo nome.
// Retorna nil e false se não encontrada.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// All retorna todas as ferramentas registradas, ordenadas por nome.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	// Ordena por nome para resultado determinístico
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})

	return tools
}

// Names retorna os nomes de todas as ferramentas registradas, ordenados.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// Count retorna o número de ferramentas registradas.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}

// Unregister remove uma ferramenta do registro pelo nome.
// Retorna true se a ferramenta existia e foi removida, false se não existia.
func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return false
	}
	delete(r.tools, name)
	return true
}

// Has verifica se uma ferramenta com o nome dado está registrada.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.tools[name]
	return ok
}

// ToDefinitions converte todas as ferramentas registradas para o formato
// ToolDefinition usado no campo "tools" do ChatRequest (protocolo OpenAI).
func (r *Registry) ToDefinitions() []ToolDefinition {
	tools := r.All()
	defs := make([]ToolDefinition, len(tools))

	for i, tool := range tools {
		defs[i] = ToolDefinition{
			Type: "function",
			Function: FunctionDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		}
	}

	return defs
}

// FilterByNames retorna definições apenas das ferramentas cujos nomes estão na lista.
// Ferramentas não encontradas são silenciosamente ignoradas.
// Útil para filtrar tools por perfil.
func (r *Registry) FilterByNames(names []string) []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]ToolDefinition, 0, len(names))
	for _, name := range names {
		tool, ok := r.tools[name]
		if !ok {
			continue
		}
		defs = append(defs, ToolDefinition{
			Type: "function",
			Function: FunctionDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		})
	}

	return defs
}
