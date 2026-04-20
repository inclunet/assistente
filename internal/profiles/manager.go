package profiles

import (
	"assistente/internal/configdir"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Manager gerencia perfis de conversa armazenados em arquivos JSON.
// Usa configdir.Resolver para resolução multi-diretório.
type Manager struct {
	resolver *configdir.Resolver
}

// NewManager cria um novo gerenciador de perfis
func NewManager() *Manager {
	return &Manager{
		resolver: configdir.NewResolver("profiles"),
	}
}

// List retorna todos os perfis resolvidos (sem duplicatas, maior prioridade ganha)
func (m *Manager) List() ([]ProfileInfo, error) {
	files, err := m.resolver.List()
	if err != nil {
		return nil, err
	}

	infos := make([]ProfileInfo, 0, len(files))
	for _, f := range files {
		// Só arquivos .json
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}

		// Tenta ler o perfil para extrair metadados
		data, _, err := m.resolver.Read(f.Filename)
		if err != nil {
			continue
		}

		var profile Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			continue
		}

		infos = append(infos, ProfileInfo{
			Name:        profile.Name,
			Slug:        f.Name,
			Description: profile.Description,
			Icon:        profile.Icon,
			Source:      string(f.Source),
		})
	}

	// Ordena por nome
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos, nil
}

// Get carrega um perfil pelo slug (nome do arquivo sem extensão)
func (m *Manager) Get(slug string) (*Profile, error) {
	filename := slug + ".json"

	data, _, err := m.resolver.Read(filename)
	if err != nil {
		return nil, fmt.Errorf("profile not found: %s", slug)
	}

	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse profile %s: %w", slug, err)
	}

	return &profile, nil
}

// Create cria um novo perfil no diretório home (~/.assistente/profiles/)
func (m *Manager) Create(profile *Profile) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}

	slug := Slugify(profile.Name)
	filename := slug + ".json"

	// Verifica se já existe
	if m.resolver.Exists(filename) {
		return "", fmt.Errorf("profile already exists: %s", slug)
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", err
	}

	if err := m.resolver.Create(filename, data); err != nil {
		return "", err
	}

	return slug, nil
}

// Duplicate cria uma copia de um perfil existente no diretorio home.
func (m *Manager) Duplicate(slug string) (string, error) {
	profile, err := m.Get(slug)
	if err != nil {
		return "", err
	}

	newProfile := *profile
	newProfile.Name = m.nextCopyName(profile.Name)
	newProfile.Active = false
	newProfile.BuiltinVersion = ""

	return m.Create(&newProfile)
}

// Update atualiza o perfil no arquivo válido (maior prioridade)
func (m *Manager) Update(slug string, profile *Profile) error {
	if err := profile.Validate(); err != nil {
		return err
	}

	filename := slug + ".json"

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}

	return m.resolver.Write(filename, data)
}

// Delete remove o perfil válido (maior prioridade)
func (m *Manager) Delete(slug string) error {
	filename := slug + ".json"
	return m.resolver.Delete(filename)
}

// GetActive retorna o perfil marcado como Active: true em seu JSON
// NOTA: Migrado para usar Profile.Active em vez de config.json
// Se nenhum estiver marcado, retorna o primeiro disponível (fallback)
func (m *Manager) GetActive() (*Profile, error) {
	files, err := m.resolver.List()
	if err != nil {
		return nil, fmt.Errorf("erro ao listar perfis: %w", err)
	}

	var firstProfile *Profile
	var padraoProfile *Profile

	for _, f := range files {
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}

		slug := strings.TrimSuffix(f.Filename, ".json")
		profile, err := m.Get(slug)
		if err != nil {
			continue
		}

		if firstProfile == nil {
			firstProfile = profile
		}

		if profile.Active {
			return profile, nil
		}

		if slug == "padrao" {
			padraoProfile = profile
		}
	}

	// Fallback: prefer "padrao" over arbitrary first profile (map iteration is non-deterministic)
	if padraoProfile != nil {
		return padraoProfile, nil
	}
	if firstProfile != nil {
		return firstProfile, nil
	}

	return DefaultProfile(), nil
}

// SetActive marca um perfil como Active: true e desativa os outros
// NOTA: Migrado para usar Profile.Active em vez de config.json
func (m *Manager) SetActive(slug string) error {
	// Verifica se o perfil existe
	profile, err := m.Get(slug)
	if err != nil {
		return fmt.Errorf("profile not found: %s", slug)
	}

	// Marca como ativo
	profile.Active = true
	if err := m.Update(slug, profile); err != nil {
		return err
	}

	// Desativa os outros
	files, err := m.resolver.List()
	if err != nil {
		return nil // Não é erro crítico
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}
		otherSlug := strings.TrimSuffix(f.Filename, ".json")
		if otherSlug == slug {
			continue
		}

		other, err := m.Get(otherSlug)
		if err != nil {
			continue
		}

		if other.Active {
			other.Active = false
			if updateErr := m.Update(otherSlug, other); updateErr != nil {
				return fmt.Errorf("failed to deactivate profile %s: %w", otherSlug, updateErr)
			}
		}
	}

	return nil
}

// GetActiveSlug retorna o slug do perfil ativo
func (m *Manager) GetActiveSlug() string {
	files, err := m.resolver.List()
	if err != nil {
		return "padrao"
	}

	var firstSlug string
	hasPadrao := false

	for _, f := range files {
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}

		slug := strings.TrimSuffix(f.Filename, ".json")
		if firstSlug == "" {
			firstSlug = slug
		}
		if slug == "padrao" {
			hasPadrao = true
		}

		profile, err := m.Get(slug)
		if err != nil {
			continue
		}
		if profile.Active {
			return slug
		}
	}

	if hasPadrao {
		return "padrao"
	}
	if firstSlug != "" {
		return firstSlug
	}

	return "padrao"
}

// GetSearchPaths retorna os caminhos de busca do resolver
func (m *Manager) GetSearchPaths() []string {
	return m.resolver.GetSearchPaths()
}

// EnsureDefaults ensures the profiles home directory exists.
// Builtin profiles are now installed by App.installBuiltinProfiles() from embedded JSON files.
func (m *Manager) EnsureDefaults() error {
	return m.resolver.EnsureHomeDir()
}

// Slugify converte um nome em slug seguro para nome de arquivo.
// Ex: "Padrão" -> "padrao", "Modelo Local" -> "modelo-local"
func Slugify(name string) string {
	// Normaliza Unicode (NFD) para separar caracteres base de acentos
	normalized := norm.NFD.String(name)

	// Remove marcas diacríticas (acentos)
	var builder strings.Builder
	for _, r := range normalized {
		if unicode.Is(unicode.Mn, r) {
			continue // Pula combining marks (acentos)
		}
		builder.WriteRune(r)
	}

	result := builder.String()

	// Converte para minúsculas
	result = strings.ToLower(result)

	// Substitui espaços e caracteres não-alfanuméricos por hífens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	result = reg.ReplaceAllString(result, "-")

	// Remove hífens do início e fim
	result = strings.Trim(result, "-")

	if result == "" {
		result = "perfil"
	}

	return result
}

func (m *Manager) nextCopyName(baseName string) string {
	if baseName == "" {
		baseName = "Perfil"
	}

	if candidate := baseName + " (Copia)"; !m.resolver.Exists(Slugify(candidate)+".json") {
		return candidate
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (Copia %d)", baseName, i)
		if !m.resolver.Exists(Slugify(candidate) + ".json") {
			return candidate
		}
	}
}
