package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveSkill seleciona um caminho explícito ou uma skill de diretórios conhecidos.
// Nomes duplicados são erro: não se deve escolher instruções silenciosamente.
func resolveSkill(root, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("nome da skill vazio")
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) || strings.HasSuffix(strings.ToLower(name), ".md") {
		return name, nil
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("nome de skill inválido")
	}
	dirs := []string{".assistente/skills", ".github/skills", ".agents/skills", ".claude/skills", ".cursor/skills"}
	var matches []string
	for _, dir := range dirs {
		base := filepath.Join(root, filepath.FromSlash(dir))
		path := filepath.Join(base, name, "SKILL.md")
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("verificar skill: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("skill não é arquivo regular: %s", path)
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(realRoot, realPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", fmt.Errorf("skill fora do repositório: %s", path)
		}
		matches = append(matches, path)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("skill %q não encontrada no repositório", name)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("skill %q ambígua; selecione um caminho: %s", name, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

// readSkill mantém a abertura da skill descoberta dentro da raiz mesmo se um
// symlink for substituído entre a descoberta e a leitura.
func readSkill(root, name string) ([]byte, error) {
	path, err := resolveSkill(root, name)
	if err != nil {
		return nil, err
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) || strings.HasSuffix(strings.ToLower(name), ".md") {
		return os.ReadFile(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil, err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	info, err := dir.Stat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("skill não é arquivo regular")
	}
	return dir.ReadFile(rel)
}
