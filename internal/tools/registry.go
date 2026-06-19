package tools

import (
	"fmt"
	"sort"
	"sync"
)

// Registry é o registro central de ferramentas disponíveis.
// Thread-safe para registro e consulta simultâneos.
type Registry struct {
	mu                sync.RWMutex
	tools             map[string]Tool
	optIn             map[string]bool // tools que só entram no payload quando explicitamente listadas em enabled_tools
	discoverableOptIn map[string]bool // opt-in que aparece em UI/catalogo para seleção explícita
}

// NewRegistry cria um novo registro de ferramentas vazio.
func NewRegistry() *Registry {
	return &Registry{
		tools:             make(map[string]Tool),
		optIn:             make(map[string]bool),
		discoverableOptIn: make(map[string]bool),
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

// RegisterOptIn registra uma ferramenta que só fica disponível quando
// explicitamente listada no enabled_tools do perfil. Não aparece quando
// enabled_tools é nil (todas). Útil para tools de contexto específico
// como text_edit (editor).
func (r *Registry) RegisterOptIn(tool Tool) error {
	if err := r.Register(tool); err != nil {
		return err
	}
	r.mu.Lock()
	r.optIn[tool.Name()] = true
	r.mu.Unlock()
	return nil
}

// RegisterDiscoverableOptIn registra uma ferramenta opt-in que deve aparecer
// em catálogos/listas de seleção para enabled_tools explícito, mas continua
// fora do payload padrão quando enabled_tools é nil.
func (r *Registry) RegisterDiscoverableOptIn(tool Tool) error {
	if err := r.RegisterOptIn(tool); err != nil {
		return err
	}
	r.mu.Lock()
	r.discoverableOptIn[tool.Name()] = true
	r.mu.Unlock()
	return nil
}

// MustRegisterOptIn é como RegisterOptIn mas faz panic em caso de erro.
func (r *Registry) MustRegisterOptIn(tool Tool) {
	if err := r.RegisterOptIn(tool); err != nil {
		panic(err)
	}
}

// MustRegisterDiscoverableOptIn é como RegisterDiscoverableOptIn mas faz panic em caso de erro.
func (r *Registry) MustRegisterDiscoverableOptIn(tool Tool) {
	if err := r.RegisterDiscoverableOptIn(tool); err != nil {
		panic(err)
	}
}

// IsOptIn retorna true se a ferramenta é opt-in (requer enabled_tools explícito).
func (r *Registry) IsOptIn(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.optIn[name]
}

// Get retorna uma ferramenta pelo nome.
// Retorna nil e false se não encontrada.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// All retorna todas as ferramentas registradas (exceto opt-in), ordenadas por nome.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for name, tool := range r.tools {
		if r.optIn[name] {
			continue
		}
		tools = append(tools, tool)
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})

	return tools
}

// Discoverable retorna tools não opt-in e opt-in marcadas como descobríveis,
// ordenadas por nome. Use para UI/catálogo de seleção explícita.
func (r *Registry) Discoverable() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for name, tool := range r.tools {
		if r.optIn[name] && !r.discoverableOptIn[name] {
			continue
		}
		tools = append(tools, tool)
	}

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

// FilterOutOptInNames remove tools opt-in de uma lista descoberta dinamicamente.
// Perfis com enabled_tools explícito continuam podendo selecioná-las via FilterByNames.
func (r *Registry) FilterOutOptInNames(names []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if r.optIn[name] {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
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
	delete(r.optIn, name)
	delete(r.discoverableOptIn, name)
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

	sortToolDefinitions(defs)
	return defs
}

// FilterByNames retorna definições apenas das ferramentas cujos nomes estão na lista.
// Ferramentas não encontradas são silenciosamente ignoradas.
// Útil para filtrar tools por perfil.
func (r *Registry) FilterByNames(names []string) []ToolDefinition {
	r.mu.RLock()
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
	r.mu.RUnlock()

	sortToolDefinitions(defs)
	return defs
}

func sortToolDefinitions(defs []ToolDefinition) {
	sort.SliceStable(defs, func(i, j int) bool {
		left := defs[i].Function.Name
		right := defs[j].Function.Name
		if toolDefinitionRank(left) != toolDefinitionRank(right) {
			return toolDefinitionRank(left) < toolDefinitionRank(right)
		}
		return left < right
	})
}

func toolDefinitionRank(name string) int {
	switch name {
	case ToolCatalogName:
		return 0
	case LoadSkillName:
		return 1
	default:
		return 10
	}
}
