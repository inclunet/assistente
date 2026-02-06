package skills

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Registry gerencia todas as skills carregadas
type Registry struct {
	skills map[string]*Skill
	mu     sync.RWMutex
	dir    string // Diretório base das skills (~/.assistente/skills/)
}

// NewRegistry cria um novo registry de skills
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
	}
}

// GetSkillsDir retorna o diretório de skills (~/.assistente/skills/)
func GetSkillsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(homeDir, ".assistente", "skills")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadFromDir carrega todas as skills de um diretório
func (r *Registry) LoadFromDir(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.dir = dir
	r.skills = make(map[string]*Skill) // Reset

	skills, err := LoadSkillsFromDir(dir)
	if err != nil {
		return err
	}

	for _, skill := range skills {
		r.skills[skill.Name] = skill
		log.Printf("[Skills] Carregada: %s (%s) auto_load=%v", skill.Name, skill.Description, skill.AutoLoad)
	}

	log.Printf("[Skills] Total: %d skills carregadas de %s", len(r.skills), dir)
	return nil
}

// Reload recarrega todas as skills do diretório configurado
func (r *Registry) Reload() error {
	if r.dir == "" {
		return fmt.Errorf("diretório de skills não configurado")
	}
	return r.LoadFromDir(r.dir)
}

// Get retorna uma skill pelo nome
func (r *Registry) Get(name string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills[name]
}

// GetAll retorna todas as skills
func (r *Registry) GetAll() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		result = append(result, skill)
	}
	return result
}

// GetAutoLoad retorna as skills com auto_load=true
func (r *Registry) GetAutoLoad() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Skill
	for _, skill := range r.skills {
		if skill.AutoLoad {
			result = append(result, skill)
		}
	}
	return result
}

// GetCatalogPrompt retorna o texto do catálogo de skills formatado para injeção no system prompt.
// Inclui a lista de skills disponíveis e o conteúdo das skills auto-load.
func (r *Registry) GetCatalogPrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.skills) == 0 {
		return ""
	}

	var sb strings.Builder

	// Lista de skills disponíveis (catálogo)
	sb.WriteString("\n\n## Available Skills\n")
	sb.WriteString("You can read a skill's full instructions using the `skill_read` tool.\n\n")

	for _, skill := range r.skills {
		autoTag := ""
		if skill.AutoLoad {
			autoTag = " [auto-loaded]"
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s%s\n", skill.Name, skill.Description, autoTag))
	}

	// Conteúdo das skills auto-load
	autoLoadSkills := make([]*Skill, 0)
	for _, skill := range r.skills {
		if skill.AutoLoad {
			autoLoadSkills = append(autoLoadSkills, skill)
		}
	}

	if len(autoLoadSkills) > 0 {
		sb.WriteString("\n## Auto-loaded Skill Instructions\n")
		for _, skill := range autoLoadSkills {
			sb.WriteString(fmt.Sprintf("\n### Skill: %s\n", skill.DisplayName))
			sb.WriteString(skill.Content)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Count retorna o número de skills carregadas
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}
