package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadSkillsFromDir carrega todas as skills de um diretório.
// Cada subdiretório que contém um SKILL.md é tratado como uma skill.
func LoadSkillsFromDir(dir string) ([]*Skill, error) {
	var skills []*Skill

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return skills, nil // Diretório não existe ainda, retorna vazio
		}
		return nil, fmt.Errorf("erro ao ler diretório de skills %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			continue // Subdiretório sem SKILL.md, ignora
		}

		skill, err := LoadSkill(entry.Name(), skillPath)
		if err != nil {
			fmt.Printf("⚠️ [Skills] Erro ao carregar skill '%s': %v\n", entry.Name(), err)
			continue
		}

		skills = append(skills, skill)
	}

	return skills, nil
}

// LoadSkill carrega uma skill individual de um arquivo SKILL.md.
// O formato esperado é:
//
//	---
//	name: Nome da Skill
//	description: Descrição curta
//	auto_load: true
//	tools: [file_read, file_write]
//	---
//	# Conteúdo Markdown com instruções para o LLM
func LoadSkill(dirName, path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler %s: %w", path, err)
	}

	content := string(data)
	skill := &Skill{
		Name: dirName,
		Path: path,
	}

	// Tenta extrair frontmatter YAML-like (entre --- e ---)
	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		frontmatter, body, err := parseFrontmatter(content)
		if err == nil {
			skill.Content = strings.TrimSpace(body)
			applyFrontmatter(skill, frontmatter)
		} else {
			// Se falhar o parse do frontmatter, usa conteúdo inteiro
			skill.Content = strings.TrimSpace(content)
		}
	} else {
		skill.Content = strings.TrimSpace(content)
	}

	// Defaults
	if skill.DisplayName == "" {
		skill.DisplayName = dirName
	}
	if skill.Description == "" {
		// Usa primeira linha do conteúdo como descrição
		if idx := strings.Index(skill.Content, "\n"); idx > 0 {
			firstLine := strings.TrimSpace(skill.Content[:idx])
			firstLine = strings.TrimPrefix(firstLine, "#")
			firstLine = strings.TrimSpace(firstLine)
			if len(firstLine) > 100 {
				firstLine = firstLine[:100] + "..."
			}
			skill.Description = firstLine
		}
	}

	return skill, nil
}

// parseFrontmatter extrai o frontmatter YAML-like do conteúdo Markdown.
// Retorna um map de chave-valor e o body restante.
func parseFrontmatter(content string) (map[string]string, string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, content, fmt.Errorf("sem frontmatter")
	}

	// Remove o primeiro ---
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return nil, content, fmt.Errorf("frontmatter não fechado")
	}

	frontmatterRaw := rest[:endIdx]
	body := rest[endIdx+4:] // +4 para pular \n---

	fm := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(frontmatterRaw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			fm[key] = value
		}
	}

	return fm, body, nil
}

// applyFrontmatter aplica os valores do frontmatter na skill.
func applyFrontmatter(skill *Skill, fm map[string]string) {
	if v, ok := fm["name"]; ok {
		skill.DisplayName = v
	}
	if v, ok := fm["description"]; ok {
		skill.Description = v
	}
	if v, ok := fm["auto_load"]; ok {
		skill.AutoLoad = strings.EqualFold(v, "true") || v == "1"
	}
	if v, ok := fm["tools"]; ok {
		// Parse simples de lista: [tool1, tool2] ou tool1, tool2
		v = strings.Trim(v, "[]")
		parts := strings.Split(v, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				skill.Tools = append(skill.Tools, p)
			}
		}
	}
}
