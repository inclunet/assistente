package profiles

import (
	"assistente/internal/config"
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

// GetActive retorna o perfil ativo global (lido do config.json)
func (m *Manager) GetActive() (*Profile, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	activeSlug := cfg.ActiveProfile
	if activeSlug == "" {
		activeSlug = "padrao"
	}

	profile, err := m.Get(activeSlug)
	if err != nil {
		// Perfil ativo não encontrado — tenta o padrão
		profile, err = m.Get("padrao")
		if err != nil {
			// Nenhum perfil encontrado — retorna o default em memória
			return DefaultProfile(), nil
		}
	}

	return profile, nil
}

// SetActive define o perfil ativo global (salva no config.json)
func (m *Manager) SetActive(slug string) error {
	// Verifica se o perfil existe
	filename := slug + ".json"
	if !m.resolver.Exists(filename) {
		return fmt.Errorf("profile not found: %s", slug)
	}

	return config.Update(func(cfg *config.Config) *config.Config {
		cfg.ActiveProfile = slug
		return cfg
	})
}

// GetActiveSlug retorna o slug do perfil ativo
func (m *Manager) GetActiveSlug() string {
	cfg, err := config.Load()
	if err != nil {
		return "padrao"
	}
	if cfg.ActiveProfile == "" {
		return "padrao"
	}
	return cfg.ActiveProfile
}

// GetSearchPaths retorna os caminhos de busca do resolver
func (m *Manager) GetSearchPaths() []string {
	return m.resolver.GetSearchPaths()
}

// EnsureDefaults garante que o perfil padrão existe
func (m *Manager) EnsureDefaults() error {
	if err := m.resolver.EnsureHomeDir(); err != nil {
		return err
	}

	// Verifica se existe pelo menos um perfil
	files, err := m.resolver.List()
	if err != nil {
		return err
	}

	hasProfiles := false
	for _, f := range files {
		if strings.HasSuffix(f.Filename, ".json") {
			hasProfiles = true
			break
		}
	}

	if !hasProfiles {
		// Cria o perfil padrão
		defaultProfile := DefaultProfile()
		if _, err := m.Create(defaultProfile); err != nil {
			return fmt.Errorf("failed to create default profile: %w", err)
		}

		// Cria perfil "Modelo Local"
		localProfile := &Profile{
			Name:        "Modelo Local",
			Description: "Para modelos locais (Ollama, LM Studio, etc.).",
			Icon:        "desktop-outline",
			Chat: ChatConfig{
				Temperature:          0.7,
				MaxTokens:            4096,
				TopP:                 1.0,
				ResponseTimeout:      300,
				SystemPromptPosition: "after",
			},
			Voice: VoiceConfig{
				Provider: "disabled",
				Rate:     1.0,
				Pitch:    1.0,
				Volume:   1.0,
			},
			Interaction: InteractionConfig{
				STTProvider:    "webspeech",
				Language:       "pt-BR",
				FeedbackSounds: true,
			},
		}
		if _, err := m.Create(localProfile); err != nil {
			fmt.Printf("Aviso: erro ao criar perfil 'Modelo Local': %v\n", err)
		}

		// Cria perfil "Programação"
		codeProfile := &Profile{
			Name:        "Programação",
			Description: "Otimizado para programação e código.",
			Icon:        "code-slash-outline",
			Chat: ChatConfig{
				Temperature:          0.3,
				MaxTokens:            8192,
				TopP:                 0.95,
				ResponseTimeout:      180,
				SystemPromptPosition: "after",
			},
			Voice: VoiceConfig{
				Provider: "disabled",
				Rate:     1.0,
				Pitch:    1.0,
				Volume:   1.0,
			},
			Interaction: InteractionConfig{
				STTProvider:    "webspeech",
				Language:       "pt-BR",
				FeedbackSounds: true,
			},
		}
		if _, err := m.Create(codeProfile); err != nil {
			fmt.Printf("Aviso: erro ao criar perfil 'Programação': %v\n", err)
		}
	}

	return nil
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
