package allowlist

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
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
			logging.Errorf(context.Background(), "allowlist.manager", "[Allowlist] Erro ao carregar %s: %v", f.Filename, err)
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

	logging.Infof(context.Background(), "allowlist.manager", "[Allowlist] Criada: %s (%s)", al.Name, slug)
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

	logging.Infof(context.Background(), "allowlist.manager", "[Allowlist] Atualizada: %s", slug)
	return nil
}

// Delete remove uma allowlist.
func (m *Manager) Delete(slug string) error {
	filename := slug + ".json"
	if err := m.resolver.Delete(filename); err != nil {
		return fmt.Errorf("erro ao excluir allowlist '%s': %w", slug, err)
	}

	logging.Infof(context.Background(), "allowlist.manager", "[Allowlist] Excluída: %s", slug)
	return nil
}

// EnsureDefaults garante a allowlist padrao no boot.
//
// Comportamento:
//   - Se o diretorio de allowlists esta vazio, cria padrao.json a partir de
//     DefaultAllowlist().
//   - Se ja ha allowlists no disco, nao sobrescreve nada — apenas executa o
//     migrador idempotente para mesclar novas CommandRules em padrao.json
//     quando o usuario ainda nao tem regras estruturadas para um determinado
//     programa (ex.: usuarios pre-AEP-0060 nao perdem o beneficio das regras
//     default de kubectl ao atualizar).
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
		return m.migrateDefaultRules()
	}

	al := DefaultAllowlist()
	if err := m.save(defaultSlug, al); err != nil {
		return fmt.Errorf("erro ao criar allowlist padrão: %w", err)
	}

	logging.Infof(context.Background(), "allowlist.manager", "[Allowlist] Allowlist padrão criada: %s", defaultSlug)
	return nil
}

// migrateDefaultRules mescla CommandRules de DefaultAllowlist() em padrao.json
// para programas que o usuario ainda nao customizou.
//
// E idempotente: se padrao.json ja contem qualquer regra para "kubectl", nao
// tocamos em nenhuma regra de "kubectl" — respeitamos a customizacao mesmo
// que ela cubra so um subcomando. So adicionamos regras de programas
// totalmente ausentes em al.CommandRules.
//
// Custo: a funcao sempre executa Exists + loadFromFile (leitura + unmarshal)
// para descobrir se ha programas ausentes; o que evitamos quando nada precisa
// ser feito e a escrita no disco (e a re-validacao via save). Em outras
// palavras, runs subsequentes sao "no-write" mas nao "no-I/O".
//
// Falhas aqui sao logadas mas nao bloqueiam o boot do app: a feature nova fica
// indisponivel para o usuario (igual a hoje), mas o sistema sobe normal.
func (m *Manager) migrateDefaultRules() error {
	filename := defaultSlug + ".json"
	if !m.resolver.Exists(filename) {
		return nil
	}

	al, err := m.loadFromFile(filename)
	if err != nil {
		logging.Errorf(context.Background(), "allowlist.manager", "[Allowlist] Migracao pulada: erro ao ler %s: %v", filename, err)
		return nil
	}

	existingPrograms := make(map[string]struct{}, len(al.CommandRules))
	for _, rule := range al.CommandRules {
		key := strings.ToLower(strings.TrimSpace(rule.Program))
		if key == "" {
			continue
		}
		existingPrograms[key] = struct{}{}
	}

	defaults := DefaultAllowlist().CommandRules
	var added []CommandRule
	addedPrograms := make(map[string]struct{})
	for _, rule := range defaults {
		key := strings.ToLower(strings.TrimSpace(rule.Program))
		if key == "" {
			continue
		}
		if _, ok := existingPrograms[key]; ok {
			continue
		}
		added = append(added, rule)
		addedPrograms[key] = struct{}{}
	}

	if len(added) == 0 {
		return nil
	}

	al.CommandRules = append(al.CommandRules, added...)
	if err := m.save(defaultSlug, al); err != nil {
		// Nao falhamos o boot: logamos e seguimos como antes.
		logging.Errorf(context.Background(), "allowlist.manager", "[Allowlist] Migracao falhou ao salvar %s: %v", filename, err)
		return nil
	}

	programs := make([]string, 0, len(addedPrograms))
	for p := range addedPrograms {
		programs = append(programs, p)
	}
	logging.Infof(context.Background(), "allowlist.manager", "[Allowlist] Migracao mesclou %d regra(s) estruturada(s) para programa(s): %s", len(added), strings.Join(programs, ", "))
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
//
// Antes de serializar, valida a allowlist com Allowlist.Validate(). Isso
// rejeita decisoes desconhecidas, regras sem programa e wildcards "*" fora
// da ultima posicao em Subcommands/Args. O fail-fast aqui complementa o
// fail-closed em runtime (parseRuleDecision/matchSequence) — o problema
// e detectado na origem (UI ou edicao manual) ao inves de virar uma
// confirmacao silenciosa que engana o autor da regra.
func (m *Manager) save(slug string, al *Allowlist) error {
	if err := al.Validate(); err != nil {
		return err
	}

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
			"git rev-parse", "git remote get-url", "git --no-pager diff",
			"git --no-pager log", "git --no-pager show",
			// GitHub CLI (review de PR / leitura)
			"gh pr view", "gh pr list", "gh api",
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
