package profiles

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Manager gerencia profiles unificados carregados do filesystem
type Manager struct {
	profiles      map[string]*UnifiedProfile
	activeProfile string // nome do profile ativo
	dir           string // diretório base (~/.assistente/profiles/)
	mu            sync.RWMutex
}

// NewManager cria um novo manager de profiles
func NewManager() *Manager {
	return &Manager{
		profiles: make(map[string]*UnifiedProfile),
	}
}

// GetProfilesDir retorna o diretório de profiles (~/.assistente/profiles/)
func GetProfilesDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(homeDir, ".assistente", "profiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadFromDir carrega todos os profiles YAML de um diretório
func (m *Manager) LoadFromDir(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dir = dir
	m.profiles = make(map[string]*UnifiedProfile)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("erro ao ler diretório de profiles: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		profile, err := LoadProfile(filePath)
		if err != nil {
			log.Printf("[Profiles] Erro ao carregar %s: %v", entry.Name(), err)
			continue
		}

		m.profiles[profile.Name] = profile
		log.Printf("[Profiles] Carregado: %s (%s)", profile.Name, profile.Description)
	}

	// Se não há profiles, cria um default
	if len(m.profiles) == 0 {
		defaultProfile := DefaultProfile()
		defaultProfile.FilePath = filepath.Join(dir, "default.yaml")
		if err := SaveProfile(defaultProfile); err != nil {
			log.Printf("[Profiles] Erro ao criar profile default: %v", err)
		} else {
			m.profiles["default"] = defaultProfile
			log.Println("[Profiles] Profile default criado")
		}
	}

	// Se não tem profile ativo, usa o primeiro disponível
	if m.activeProfile == "" {
		// Tenta "default" primeiro
		if _, ok := m.profiles["default"]; ok {
			m.activeProfile = "default"
		} else {
			// Usa qualquer um
			for name := range m.profiles {
				m.activeProfile = name
				break
			}
		}
	}

	log.Printf("[Profiles] Total: %d profiles carregados, ativo: %s", len(m.profiles), m.activeProfile)
	return nil
}

// LoadProfile carrega um profile individual de um arquivo YAML
func LoadProfile(path string) (*UnifiedProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler %s: %w", path, err)
	}

	var profile UnifiedProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("erro ao parsear YAML %s: %w", path, err)
	}

	profile.FilePath = path

	// Se o nome está vazio, usa o nome do arquivo
	if profile.Name == "" {
		base := filepath.Base(path)
		profile.Name = strings.TrimSuffix(base, ".yaml")
	}

	return &profile, nil
}

// SaveProfile salva um profile em arquivo YAML
func SaveProfile(profile *UnifiedProfile) error {
	if profile.FilePath == "" {
		return fmt.Errorf("caminho do arquivo não definido")
	}

	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("erro ao serializar profile: %w", err)
	}

	return os.WriteFile(profile.FilePath, data, 0644)
}

// GetAll retorna todos os profiles
func (m *Manager) GetAll() []*UnifiedProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*UnifiedProfile, 0, len(m.profiles))
	for _, p := range m.profiles {
		result = append(result, p)
	}
	return result
}

// Get retorna um profile pelo nome
func (m *Manager) Get(name string) *UnifiedProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profiles[name]
}

// GetActive retorna o profile ativo
func (m *Manager) GetActive() *UnifiedProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if p, ok := m.profiles[m.activeProfile]; ok {
		return p
	}

	// Fallback para default
	if p, ok := m.profiles["default"]; ok {
		return p
	}

	// Fallback para qualquer um
	for _, p := range m.profiles {
		return p
	}

	return DefaultProfile()
}

// GetActiveName retorna o nome do profile ativo
func (m *Manager) GetActiveName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeProfile
}

// SetActive define o profile ativo
func (m *Manager) SetActive(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.profiles[name]; !ok {
		return fmt.Errorf("profile não encontrado: %s", name)
	}

	m.activeProfile = name
	log.Printf("[Profiles] Profile ativo: %s", name)
	return nil
}

// Save salva um profile (cria ou atualiza)
func (m *Manager) Save(profile *UnifiedProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if profile.FilePath == "" && m.dir != "" {
		slug := strings.ToLower(strings.ReplaceAll(profile.Name, " ", "-"))
		profile.FilePath = filepath.Join(m.dir, slug+".yaml")
	}

	if err := SaveProfile(profile); err != nil {
		return err
	}

	m.profiles[profile.Name] = profile
	return nil
}

// Delete remove um profile
func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile não encontrado: %s", name)
	}

	// Não permite deletar o profile ativo
	if name == m.activeProfile {
		return fmt.Errorf("não é possível deletar o profile ativo")
	}

	// Remove o arquivo
	if profile.FilePath != "" {
		os.Remove(profile.FilePath)
	}

	delete(m.profiles, name)
	return nil
}

// Reload recarrega profiles do diretório
func (m *Manager) Reload() error {
	if m.dir == "" {
		return fmt.Errorf("diretório não configurado")
	}
	active := m.GetActiveName()
	err := m.LoadFromDir(m.dir)
	if err == nil && active != "" {
		m.SetActive(active)
	}
	return err
}

// Count retorna o número de profiles
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.profiles)
}
