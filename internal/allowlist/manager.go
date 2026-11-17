package allowlist

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"assistente/internal/authorization"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"
)

const resolverSubdir = "allowlists"
const defaultSlug = "padrao"

type Manager struct {
	mu        sync.Mutex
	repo      *authorization.Repository
	paths     func() []string
	home      func() string
	mayImport func(context.Context) bool
}
type Options struct {
	Repository *authorization.Repository
	Paths      func() []string
	HomeDir    func() string
	MayImport  func(context.Context) bool
}

func NewManager(options Options) *Manager {
	if options.Repository == nil {
		options.Repository = authorization.NewRepository(database.DB)
	}
	if options.Paths == nil {
		options.Paths = configdir.NewResolver(resolverSubdir).GetSearchPaths
	}
	if options.HomeDir == nil {
		options.HomeDir = configdir.NewResolver(resolverSubdir).GetHomeDir
	}
	return &Manager{repo: options.Repository, paths: options.Paths, home: options.HomeDir, mayImport: options.MayImport}
}

type policySelector struct {
	Kind    string       `json:"kind"`
	Pattern string       `json:"pattern,omitempty"`
	Rule    *CommandRule `json:"rule,omitempty"`
}

func policyRules(al *Allowlist) ([]database.AuthorizationRule, error) {
	if err := al.Validate(); err != nil {
		return nil, err
	}
	var rows []database.AuthorizationRule
	add := func(selector policySelector, effect string) error {
		data, err := json.Marshal(selector)
		if err != nil {
			return err
		}
		rows = append(rows, database.AuthorizationRule{Selector: string(data), SelectorVersion: 1, Effect: effect})
		return nil
	}
	for _, pattern := range al.AutoApprove {
		if err := add(policySelector{Kind: "pattern", Pattern: pattern}, "allow"); err != nil {
			return nil, err
		}
	}
	for _, pattern := range al.AlwaysDeny {
		if err := add(policySelector{Kind: "pattern", Pattern: pattern}, "deny"); err != nil {
			return nil, err
		}
	}
	for _, rule := range al.CommandRules {
		effect := strings.ToLower(strings.TrimSpace(rule.Decision))
		if effect == "approve" {
			effect = "allow"
		}
		if effect == "confirm" {
			effect = "ask"
		}
		if err := add(policySelector{Kind: "structured", Rule: &rule}, effect); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
func policyFor(slug, source string, al *Allowlist) database.AuthorizationPolicy {
	effect := "ask"
	if strings.EqualFold(strings.TrimSpace(al.DefaultAction), "deny") {
		effect = "deny"
	}
	return database.AuthorizationPolicy{Domain: "shell", Slug: slug, Source: source, Name: al.Name, Description: al.Description, DefaultEffect: effect}
}
func (m *Manager) prepare(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, dir := range m.paths() {
		marker := "directory:" + filepath.Clean(dir)
		done, err := m.repo.Imported(ctx, "shell", marker)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		var importedNames []string
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			importedNames = append(importedNames, entry.Name())
			source := filepath.Join(dir, entry.Name())
			done, err := m.repo.Imported(ctx, "shell", source)
			if err != nil {
				return err
			}
			if done {
				continue
			}
			allowed, err := m.repo.AuthorizeLegacyImport(ctx, "shell", source, m.mayImport != nil && m.mayImport(ctx))
			if err != nil {
				return err
			}
			if !allowed {
				continue
			}
			data, err := os.ReadFile(source)
			if err != nil {
				return err
			}
			var al Allowlist
			if err := json.Unmarshal(data, &al); err != nil {
				return err
			}
			if entry.Name() == defaultSlug+".json" {
				mergeDefaultRules(&al)
			}
			rows, err := policyRules(&al)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(data)
			policy := policyFor(strings.TrimSuffix(entry.Name(), ".json"), dir, &al)
			if err := m.repo.ImportPolicy(ctx, source, fmt.Sprintf("%x", digest), policy, rows, func() error {
				current, err := os.ReadFile(source)
				if err != nil {
					return err
				}
				if sha256.Sum256(current) != digest {
					return errors.New("política alterada durante importação")
				}
				return nil
			}); err != nil {
				return err
			}
		}
		if err := m.repo.Import(ctx, "shell", marker, "directory-v1", nil, func() error {
			current, err := os.ReadDir(dir)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			var names []string
			for _, entry := range current {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
					names = append(names, entry.Name())
				}
			}
			if strings.Join(names, "\x00") != strings.Join(importedNames, "\x00") {
				return errors.New("origens de políticas alteradas durante importação")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// Preserva a evolução de defaults já aplicada pelo boot anterior à migração,
// sem substituir programas que o usuário já personalizou (AEP-0060).
func mergeDefaultRules(al *Allowlist) {
	existing := map[string]bool{}
	for _, rule := range al.CommandRules {
		existing[strings.ToLower(strings.TrimSpace(rule.Program))] = true
	}
	for _, rule := range DefaultAllowlist().CommandRules {
		if !existing[strings.ToLower(strings.TrimSpace(rule.Program))] {
			al.CommandRules = append(al.CommandRules, rule)
		}
	}
}
func (m *Manager) policies(ctx context.Context) ([]database.AuthorizationPolicy, error) {
	if err := m.prepare(ctx); err != nil {
		return nil, err
	}
	rows, err := m.repo.Policies(ctx, "shell")
	if err != nil {
		return nil, err
	}
	bySlug := map[string]database.AuthorizationPolicy{}
	// Preserva a prioridade exe -> home -> workspace sem fundir homônimos.
	for _, path := range m.paths() {
		for _, row := range rows {
			if filepath.Clean(row.Source) == filepath.Clean(path) {
				bySlug[row.Slug] = row
			}
		}
	}
	out := make([]database.AuthorizationPolicy, 0, len(bySlug))
	for _, row := range bySlug {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}
func (m *Manager) List(ctx context.Context) ([]AllowlistInfo, error) {
	if err := m.EnsureDefaults(ctx); err != nil {
		return nil, err
	}
	rows, err := m.policies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AllowlistInfo, 0, len(rows))
	for _, row := range rows {
		current, rules, err := m.repo.PolicySnapshot(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, AllowlistInfo{Slug: current.Slug, Name: current.Name, Description: current.Description, RuleCount: len(rules)})
	}
	return out, nil
}
func (m *Manager) selected(ctx context.Context, slug string) (database.AuthorizationPolicy, error) {
	if err := configdir.ValidateFilename(slug + ".json"); err != nil {
		return database.AuthorizationPolicy{}, err
	}
	rows, err := m.policies(ctx)
	if err != nil {
		return database.AuthorizationPolicy{}, err
	}
	for _, row := range rows {
		if row.Slug == slug {
			return row, nil
		}
	}
	return database.AuthorizationPolicy{}, authorization.ErrNotFound
}
func (m *Manager) Get(ctx context.Context, slug string) (*Allowlist, error) {
	if err := m.EnsureDefaults(ctx); err != nil {
		return nil, err
	}
	row, err := m.selected(ctx, slug)
	if err != nil {
		return nil, err
	}
	row, rules, err := m.repo.PolicySnapshot(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	al := &Allowlist{Name: row.Name, Description: row.Description, DefaultAction: "confirm"}
	if row.DefaultEffect == "deny" {
		al.DefaultAction = "deny"
	}
	for _, rule := range rules {
		var selector policySelector
		if rule.SelectorVersion != 1 {
			return nil, errors.New("versão de política desconhecida")
		}
		if err := json.Unmarshal([]byte(rule.Selector), &selector); err != nil {
			return nil, err
		}
		switch selector.Kind {
		case "pattern":
			switch rule.Effect {
			case "allow":
				al.AutoApprove = append(al.AutoApprove, selector.Pattern)
			case "deny":
				al.AlwaysDeny = append(al.AlwaysDeny, selector.Pattern)
			default:
				return nil, errors.New("efeito de padrão inválido")
			}
		case "structured":
			if selector.Rule == nil {
				return nil, errors.New("regra estruturada vazia")
			}
			al.CommandRules = append(al.CommandRules, *selector.Rule)
		default:
			return nil, errors.New("seletor de política desconhecido")
		}
	}
	if err := al.Validate(); err != nil {
		return nil, err
	}
	if inv, ok := invocationctx.Get(ctx); ok && inv.ProfileSlug != "" {
		if err := m.repo.BindPolicy(ctx, row.ID, inv.ProfileSlug); err != nil {
			return nil, err
		}
	}
	return al, nil
}
func (m *Manager) Create(ctx context.Context, al *Allowlist) (string, error) {
	if strings.TrimSpace(al.Name) == "" {
		return "", errors.New("nome da política obrigatório")
	}
	slug := slugify(al.Name)
	if _, err := m.selected(ctx, slug); err == nil {
		return "", errors.New("política já existe")
	} else if !errors.Is(err, authorization.ErrNotFound) {
		return "", err
	}
	source := m.home()
	if source == "" {
		return "", errors.New("origem da política indisponível")
	}
	// Novas políticas pertencem à configuração do usuário, como o Create anterior.
	rules, err := policyRules(al)
	if err != nil {
		return "", err
	}
	return slug, m.repo.CreatePolicy(ctx, policyFor(slug, source, al), rules)
}
func (m *Manager) Update(ctx context.Context, slug string, al *Allowlist) error {
	row, err := m.selected(ctx, slug)
	if err != nil {
		return err
	}
	rules, err := policyRules(al)
	if err != nil {
		return err
	}
	return m.repo.SavePolicy(ctx, policyFor(slug, row.Source, al), rules)
}
func (m *Manager) Delete(ctx context.Context, slug string) error {
	row, err := m.selected(ctx, slug)
	if err != nil {
		return err
	}
	return m.repo.DeletePolicy(ctx, row.ID)
}
func (m *Manager) EnsureDefaults(ctx context.Context) error {
	rows, err := m.policies(ctx)
	if err != nil {
		return err
	}
	done, err := m.repo.Imported(ctx, "shell", "builtin:padrao")
	if err != nil || done {
		return err
	}
	for _, row := range rows {
		if row.Slug == defaultSlug {
			return m.repo.Import(ctx, "shell", "builtin:padrao", "v1", nil, func() error { return nil })
		}
	}
	source := m.home()
	if source == "" {
		return errors.New("origem da política indisponível")
	}
	al := DefaultAllowlist()
	rules, err := policyRules(al)
	if err != nil {
		return err
	}
	return m.repo.ImportPolicy(ctx, "builtin:padrao", "v1", policyFor(defaultSlug, source, al), rules, func() error { return nil })
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
