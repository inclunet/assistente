package allowlist

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"assistente/internal/configdir"
)

const (
	// resolverSubdir é o subdiretório dentro de .assistente/ para allowlists
	resolverSubdir = "allowlists"

	// defaultSlug é o slug da allowlist padrão
	defaultSlug = "padrao"
)

// Manager gerencia o CRUD de allowlists usando o configdir.Resolver.
// Allowlists são armazenadas em .assistente/allowlists/<slug>.json.
type Manager struct {
	resolver *configdir.Resolver
}

// NewManager cria um novo gerenciador de allowlists.
func NewManager() *Manager {
	return &Manager{
		resolver: configdir.NewResolver(resolverSubdir),
	}
}

// List retorna informações resumidas de todas as allowlists disponíveis.
func (m *Manager) List() ([]AllowlistInfo, error) {
	files, err := m.resolver.List()
	if err != nil {
		return nil, fmt.Errorf("erro ao listar allowlists: %w", err)
	}

	result := make([]AllowlistInfo, 0, len(files))
	for _, f := range files {
		al, err := m.loadFromFile(f.Filename)
		if err != nil {
			log.Printf("[Allowlist] Erro ao carregar %s: %v", f.Filename, err)
			continue
		}

		result = append(result, AllowlistInfo{
			Slug:        f.Name,
			Name:        al.Name,
			Description: al.Description,
			RuleCount:   len(al.AutoApprove) + len(al.AlwaysDeny) + len(al.CommandRules),
		})
	}

	return result, nil
}

// Get carrega uma allowlist pelo slug.
func (m *Manager) Get(slug string) (*Allowlist, error) {
	filename := slug + ".json"
	al, err := m.loadFromFile(filename)
	if err != nil {
		return nil, fmt.Errorf("allowlist '%s' não encontrada: %w", slug, err)
	}
	return al, nil
}

// Create cria uma nova allowlist.
func (m *Manager) Create(al *Allowlist) (string, error) {
	if al.Name == "" {
		return "", fmt.Errorf("nome da allowlist é obrigatório")
	}

	slug := slugify(al.Name)
	if m.resolver.Exists(slug + ".json") {
		return "", fmt.Errorf("allowlist '%s' já existe", slug)
	}

	if err := m.save(slug, al); err != nil {
		return "", err
	}

	log.Printf("[Allowlist] Criada: %s (%s)", al.Name, slug)
	return slug, nil
}

// Update atualiza uma allowlist existente.
func (m *Manager) Update(slug string, al *Allowlist) error {
	filename := slug + ".json"
	if !m.resolver.Exists(filename) {
		return fmt.Errorf("allowlist '%s' não encontrada", slug)
	}

	if err := m.save(slug, al); err != nil {
		return err
	}

	log.Printf("[Allowlist] Atualizada: %s", slug)
	return nil
}

// Delete remove uma allowlist.
func (m *Manager) Delete(slug string) error {
	filename := slug + ".json"
	if err := m.resolver.Delete(filename); err != nil {
		return fmt.Errorf("erro ao excluir allowlist '%s': %w", slug, err)
	}

	log.Printf("[Allowlist] Excluída: %s", slug)
	return nil
}

// EnsureDefaults cria a allowlist padrão se nenhuma existir.
func (m *Manager) EnsureDefaults() error {
	files, err := m.resolver.List()
	if err != nil {
		// Diretório pode não existir ainda — tenta criar
		if err := m.resolver.EnsureHomeDir(); err != nil {
			return fmt.Errorf("erro ao criar diretório de allowlists: %w", err)
		}
		files, err = m.resolver.List()
		if err != nil {
			return fmt.Errorf("erro ao listar allowlists: %w", err)
		}
	}

	if len(files) > 0 {
		return nil // já existem allowlists
	}

	al := DefaultAllowlist()
	if err := m.save(defaultSlug, al); err != nil {
		return fmt.Errorf("erro ao criar allowlist padrão: %w", err)
	}

	log.Printf("[Allowlist] Allowlist padrão criada: %s", defaultSlug)
	return nil
}

// GetSearchPaths retorna os caminhos onde allowlists são buscadas.
func (m *Manager) GetSearchPaths() []string {
	return m.resolver.GetSearchPaths()
}

// loadFromFile carrega uma allowlist de um arquivo.
func (m *Manager) loadFromFile(filename string) (*Allowlist, error) {
	data, _, err := m.resolver.Read(filename)
	if err != nil {
		return nil, err
	}

	var al Allowlist
	if err := json.Unmarshal(data, &al); err != nil {
		return nil, fmt.Errorf("erro ao parsear %s: %w", filename, err)
	}

	return &al, nil
}

// save serializa e salva uma allowlist.
func (m *Manager) save(slug string, al *Allowlist) error {
	data, err := json.MarshalIndent(al, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar allowlist: %w", err)
	}

	filename := slug + ".json"

	// Tenta Write (atualizar arquivo existente) primeiro, senão Create
	if m.resolver.Exists(filename) {
		return m.resolver.Write(filename, data)
	}

	return m.resolver.Create(filename, data)
}

// DefaultAllowlist retorna a allowlist padrão com comandos comuns de desenvolvimento.
func DefaultAllowlist() *Allowlist {
	return &Allowlist{
		Name:        "Padrão",
		Description: "Comandos comuns de desenvolvimento — leitura e consulta liberados, ações destrutivas bloqueadas",
		AutoApprove: []string{
			// Navegação e listagem
			"ls", "dir", "pwd", "cd", "tree",
			// Leitura de arquivos
			"cat", "type", "head", "tail", "less", "more", "wc",
			// Busca
			"grep", "find", "which", "where", "rg",
			// Git (somente leitura)
			"git status", "git diff", "git log", "git branch", "git remote",
			"git show", "git blame", "git stash list", "git tag",
			// Informações do sistema
			"echo", "env", "printenv", "whoami", "hostname", "uname",
			// Linguagens (versões e info)
			"go version", "go env", "go list",
			"node --version", "npm --version", "npm list",
			"python --version", "pip --version", "pip list",
			// Build e testes (leitura)
			"go test *", "npm test", "npm run *",
			"go build *", "go vet *",
		},
		AlwaysDeny: []string{
			// Comandos destrutivos do sistema
			"rm -rf /", "rm -rf /*",
			"del /s /q C:\\",
			"format",
			"mkfs",
			":(){ :|:& };:",
			// Desligamento
			"shutdown", "reboot", "halt", "poweroff",
			// Elevação de privilégio
			"sudo rm *",
			"sudo shutdown",
			"sudo reboot",
		},
		CommandRules: []CommandRule{
			{
				Program:     "kubectl",
				Subcommands: []string{"get"},
				Args:        []string{"*"},
				Decision:    "approve",
				Description: "Leitura de recursos Kubernetes",
			},
			{
				Program:     "kubectl",
				Subcommands: []string{"describe"},
				Args:        []string{"*"},
				Decision:    "approve",
				Description: "Inspecao de recursos Kubernetes",
			},
			{
				Program:     "kubectl",
				Subcommands: []string{"logs"},
				Args:        []string{"*"},
				Decision:    "approve",
				Description: "Leitura de logs Kubernetes",
			},
			{
				Program:     "kubectl",
				Subcommands: []string{"delete"},
				Args:        []string{"*"},
				Decision:    "confirm",
				Description: "Exclusao de recursos Kubernetes exige confirmacao",
			},
			{
				Program:     "kubectl",
				Subcommands: []string{"patch"},
				Args:        []string{"*"},
				Decision:    "confirm",
				Description: "Alteracao parcial de recursos Kubernetes exige confirmacao",
			},
		},
		DefaultAction: "confirm",
	}
}

// slugify converte um nome em slug (lowercase, sem espaços, sem acentos).
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))

	// Substitui acentos comuns
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u", "ü", "u",
		"ç", "c",
		" ", "-",
	)
	s = replacer.Replace(s)

	// Remove caracteres não alfanuméricos (exceto hífen)
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	return result.String()
}
