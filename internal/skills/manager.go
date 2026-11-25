package skills

import (
	"assistente/internal/configdir"
	"assistente/internal/logging"
	"assistente/internal/slug"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const skillFile = "SKILL.md" // Cada skill fica em skills/{slug}/SKILL.md
const fullSkillCacheTTL = 30 * time.Second

// discoveredSkill representa um skill encontrado no filesystem (antes do parse).
type discoveredSkill struct {
	slug   string
	path   string // caminho absoluto do SKILL.md
	source configdir.Source
}

// Manager gerencia skills no formato: .assistente/skills/{slug}/SKILL.md
// Usa configdir.Resolver para resolução multi-diretório.
type Manager struct {
	installedSkills func(context.Context) ([]Skill, error)
	resolver        *configdir.Resolver
	mu              sync.Mutex
	cache           cachedFullSkills
}

type cachedFullSkills struct {
	skills []Skill
	at     time.Time
}

// NewManager cria um novo gerenciador de skills
func NewManager() *Manager {
	return &Manager{
		resolver: configdir.NewResolver("skills"),
	}
}

// discoverAll escaneia todos os diretórios de busca e retorna skills encontrados.
// Formato: skills/{slug}/SKILL.md
// Skills com o mesmo slug: o de maior prioridade (workdir > home > exe) prevalece.
func (m *Manager) discoverAll() []discoveredSkill {
	items, _ := m.discoverNative()
	return items
}

// Legacy consumers use available definitions even when one root fails. The
// builder reports incomplete discovery before offering its selection.
func (m *Manager) discoverNative() ([]discoveredSkill, error) {
	var discoveryErr error
	searchPaths := m.resolver.GetSearchPaths()
	resolved := map[string]discoveredSkill{} // slug -> discoveredSkill

	// Itera na ordem de prioridade crescente — o último encontrado sobrescreve
	for _, dir := range searchPaths {
		info, err := os.Stat(dir)
		if err == nil && !info.IsDir() {
			discoveryErr = fmt.Errorf("skill root is not a directory")
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				discoveryErr = err
			}
			continue
		}

		source := configdir.SourceForPath(dir)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			slug := entry.Name()
			skillPath := filepath.Join(dir, slug, skillFile)
			if _, err := os.Stat(skillPath); err == nil {
				resolved[slug] = discoveredSkill{
					slug:   slug,
					path:   skillPath,
					source: source,
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				discoveryErr = err
			}
		}
	}

	result := make([]discoveredSkill, 0, len(resolved))
	for _, ds := range resolved {
		result = append(result, ds)
	}
	return result, discoveryErr
}

// loadSkill carrega e faz parse de um skill descoberto.
func loadSkill(ds discoveredSkill) (*Skill, error) {
	data, err := os.ReadFile(ds.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill %s: %w", ds.slug, err)
	}

	meta, content, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse skill %s: %w", ds.slug, err)
	}

	// Se name está vazio, usa o slug (nome do diretório) como fallback
	// Spec: "If omitted, uses the directory name"
	if meta.Name == "" {
		meta.Name = ds.slug
	}

	// Expande variáveis de template nos paths de filesystem (${HOME}, ${PROJECT_ROOT}, ${TEMP})
	projectRoot, _ := os.Getwd()
	meta.ExpandTemplateVars(projectRoot)

	return &Skill{
		SkillMetadata: *meta,
		Slug:          ds.slug,
		Source:        string(ds.source),
		Content:       content,
		Path:          ds.path,
	}, nil
}

// List retorna todos os skills resolvidos (sem duplicatas, maior prioridade ganha).
func (m *Manager) List() ([]SkillInfo, error) {
	discovered := m.discoverAll()

	infos := make([]SkillInfo, 0, len(discovered))
	for _, ds := range discovered {
		skill, err := loadSkill(ds)
		if err != nil {
			logging.Infof(context.Background(), "skills.manager", "[Skills] Ignorando skill %s: %v", ds.slug, err)
			continue
		}

		infos = append(infos, SkillInfo{
			SkillMetadata: skill.SkillMetadata,
			Slug:          skill.Slug,
			Source:        skill.Source,
			AutoLoad:      skill.IsAutoLoad(),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos, nil
}

// Get carrega um skill completo pelo slug.
func (m *Manager) Get(slug string) (*Skill, error) {
	discovered := m.discoverAll()
	for _, ds := range discovered {
		if ds.slug == slug {
			return loadSkill(ds)
		}
	}
	return nil, fmt.Errorf("skill not found: %s", slug)
}

// Create cria um novo skill: ~/.assistente/skills/{slug}/SKILL.md
func (m *Manager) Create(meta *SkillMetadata, content string) (string, error) {
	if err := validateMetadata(meta); err != nil {
		return "", err
	}

	slug := Slugify(meta.Name)

	// Verifica se já existe
	discovered := m.discoverAll()
	for _, ds := range discovered {
		if ds.slug == slug {
			return "", fmt.Errorf("skill already exists: %s", slug)
		}
	}

	raw, err := Compose(meta, content)
	if err != nil {
		return "", err
	}

	homeDir := m.resolver.GetHomeDir()
	if homeDir == "" {
		return "", fmt.Errorf("home directory not available")
	}

	skillDir := filepath.Join(homeDir, slug)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create skill directory: %w", err)
	}

	skillPath := filepath.Join(skillDir, skillFile)
	if err := os.WriteFile(skillPath, []byte(raw), 0644); err != nil {
		return "", fmt.Errorf("failed to write skill file: %w", err)
	}

	m.invalidateFullSkillsCache()
	return slug, nil
}

// Duplicate cria uma copia de um skill existente no diretorio home.
func (m *Manager) Duplicate(slug string) (string, error) {
	skill, err := m.Get(slug)
	if err != nil {
		return "", err
	}

	existing := map[string]bool{}
	for _, ds := range m.discoverAll() {
		existing[ds.slug] = true
	}

	baseSlug := Slugify(skill.Name)
	newName := nextCopyName(baseSlug, existing)

	meta := skill.SkillMetadata
	meta.Name = newName
	if meta.DisplayName == "" || meta.DisplayName == skill.Name {
		meta.DisplayName = newName
	}

	return m.Create(&meta, skill.Content)
}

// Update atualiza um skill existente.
func (m *Manager) Update(slug string, meta *SkillMetadata, content string) error {
	if err := validateMetadata(meta); err != nil {
		return err
	}

	discovered := m.discoverAll()
	for _, ds := range discovered {
		if ds.slug == slug {
			raw, err := Compose(meta, content)
			if err != nil {
				return err
			}
			if err := os.WriteFile(ds.path, []byte(raw), 0644); err != nil {
				return err
			}
			m.invalidateFullSkillsCache()
			return nil
		}
	}

	return fmt.Errorf("skill not found: %s", slug)
}

// Delete remove o skill (diretório inteiro).
func (m *Manager) Delete(slug string) error {
	discovered := m.discoverAll()
	for _, ds := range discovered {
		if ds.slug == slug {
			if err := os.RemoveAll(filepath.Dir(ds.path)); err != nil {
				return err
			}
			m.invalidateFullSkillsCache()
			return nil
		}
	}
	return fmt.Errorf("skill not found: %s", slug)
}

// GetSearchPaths retorna os caminhos de busca do resolver
func (m *Manager) GetSearchPaths() []string {
	return m.resolver.GetSearchPaths()
}

// EnsureDir garante que o diretório de skills existe no home
func (m *Manager) EnsureDir() error {
	return m.resolver.EnsureHomeDir()
}

// GetAllSkillsFull retorna todos os skills com conteúdo completo.
func (m *Manager) GetAllSkillsFull() ([]Skill, error) {
	if cached, ok := m.fullSkillsCache(); ok {
		return cached, nil
	}
	discovered := m.discoverAll()

	var result []Skill
	for _, ds := range discovered {
		skill, err := loadSkill(ds)
		if err != nil {
			continue
		}
		result = append(result, *skill)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	m.storeFullSkillsCache(result)
	return cloneSkills(result), nil
}

func (m *Manager) fullSkillsCache() ([]Skill, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cache.skills) == 0 || time.Since(m.cache.at) > fullSkillCacheTTL {
		return nil, false
	}
	return cloneSkills(m.cache.skills), true
}

func (m *Manager) storeFullSkillsCache(skills []Skill) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = cachedFullSkills{skills: cloneSkills(skills), at: time.Now()}
}

func (m *Manager) invalidateFullSkillsCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = cachedFullSkills{}
}

func cloneSkills(input []Skill) []Skill {
	if len(input) == 0 {
		return nil
	}
	out := make([]Skill, len(input))
	copy(out, input)
	return out
}

// GetUserInvocableSkills retorna skills que podem ser invocados pelo usuário via /slash.
// Filtra por IsUserInvocable() == true.
func (m *Manager) GetUserInvocableSkills() ([]SkillInfo, error) {
	discovered := m.discoverAll()

	var infos []SkillInfo
	for _, ds := range discovered {
		skill, err := loadSkill(ds)
		if err != nil {
			logging.Infof(context.Background(), "skills.manager", "[Skills] Ignorando skill %s: %v", ds.slug, err)
			continue
		}
		if !skill.IsUserInvocable() {
			continue
		}
		infos = append(infos, SkillInfo{
			SkillMetadata: skill.SkillMetadata,
			Slug:          skill.Slug,
			Source:        skill.Source,
			AutoLoad:      skill.IsAutoLoad(),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos, nil
}

// GetSkillFiles retorna arquivos complementares do diretório de um skill (excluindo SKILL.md).
// Usado para progressive file loading: o modelo pode ler esses arquivos via read_file.
func (m *Manager) GetSkillFiles(slug string) ([]string, error) {
	discovered := m.discoverAll()
	for _, ds := range discovered {
		if ds.slug != slug {
			continue
		}

		return m.listSupportingFiles(ds.path)
	}
	return nil, fmt.Errorf("skill %q: %w", slug, os.ErrNotExist)
}

// Os gerenciadores de manifestos locais podem listar os arquivos da origem
// já selecionada sem resolver novamente a precedência do slug.
type supportingFileLister interface {
	listSupportingFiles(skillPath string) ([]string, error)
}

func (*Manager) listSupportingFiles(skillPath string) ([]string, error) {
	return collectSupportingFiles(filepath.Dir(skillPath), filepath.Walk)
}

func collectSupportingFiles(skillDir string, walk func(string, filepath.WalkFunc) error) ([]string, error) {
	var files []string
	err := walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Ignora o próprio SKILL.md
		if filepath.Base(path) == skillFile {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list skill files: %w", err)
	}

	sort.Strings(files)
	return files, nil
}

// FilterByNames filtra uma lista de skills pelos identificadores fornecidos.
// Aceita tanto slug (nome do diretório) quanto name (campo do frontmatter).
// Se names for nil, retorna todos. Se for vazio, retorna nenhum.
func FilterByNames(allSkills []Skill, names []string) []Skill {
	if names == nil {
		return allSkills
	}
	if len(names) == 0 {
		return nil
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	var result []Skill
	for _, s := range allSkills {
		if nameSet[s.Slug] || nameSet[s.Name] {
			result = append(result, s)
		}
	}
	return result
}

// FilterByNamesOrdered retorna skills na mesma ordem do slice names.
func FilterByNamesOrdered(allSkills []Skill, names []string) []Skill {
	if names == nil {
		return allSkills
	}
	if len(names) == 0 {
		return nil
	}

	skillMap := make(map[string]Skill, len(allSkills))
	for _, s := range allSkills {
		skillMap[s.Slug] = s
		skillMap[s.Name] = s
	}

	var result []Skill
	for _, name := range names {
		if s, ok := skillMap[name]; ok {
			result = append(result, s)
		}
	}
	return result
}

// FilterExcludeNames retorna skills cujo slug NÃO está na lista.
func FilterExcludeNames(allSkills []Skill, names []string) []Skill {
	if len(names) == 0 {
		return allSkills
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	var result []Skill
	for _, s := range allSkills {
		if !nameSet[s.Slug] && !nameSet[s.Name] {
			result = append(result, s)
		}
	}
	return result
}

// validateMetadata valida campos obrigatórios para criação/atualização.
func validateMetadata(meta *SkillMetadata) error {
	return validateSpec(meta)
}

// Slugify converte um nome em slug seguro para nome de diretório.
// Delega ao pacote canônico internal/slug, usando "skill" como fallback.
func Slugify(name string) string {
	return slug.Slugify(name, "skill")
}

func nextCopyName(baseSlug string, existing map[string]bool) string {
	if baseSlug == "" {
		baseSlug = "skill"
	}

	if !existing[baseSlug+"-copia"] {
		return baseSlug + "-copia"
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-copia-%d", baseSlug, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

// NewManagerWithInstalledSkills includes account-scoped plugin skills only on
// reads that have an authenticated caller. The filesystem cache stays local.
func NewManagerWithInstalledSkills(installed func(context.Context) ([]Skill, error)) *Manager {
	m := NewManager()
	m.installedSkills = installed
	return m
}

type FullSkillSource interface{ GetAllSkillsFull() ([]Skill, error) }

func ListForContext(ctx context.Context, source FullSkillSource) ([]Skill, error) {
	if scoped, ok := source.(interface {
		GetAllSkillsForContext(context.Context) ([]Skill, error)
	}); ok {
		return scoped.GetAllSkillsForContext(ctx)
	}
	return source.GetAllSkillsFull()
}

func (m *Manager) GetAllSkillsForContext(ctx context.Context) ([]Skill, error) {
	local, err := m.GetAllSkillsFull()
	if err != nil || m.installedSkills == nil {
		return local, err
	}
	installed, err := m.installedSkills(ctx)
	if err != nil {
		return nil, err
	}
	visible := local[:0]
	for _, skill := range local {
		if strings.HasPrefix(skill.Slug, PluginPrefix) {
			continue
		}
		if strings.HasPrefix(skill.Name, PluginPrefix) {
			skill.DisplayName = skill.GetDisplayName()
			skill.Name = skill.Slug
		}
		visible = append(visible, skill)
	}
	return append(visible, installed...), nil
}

const PluginPrefix = "plugin:"
