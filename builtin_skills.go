package main

import (
	"bytes"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"assistente/internal/configdir"
	"assistente/internal/skills"
)

//go:embed all:.assistente/skills
var builtinSkillsFS embed.FS

// installBuiltinSkills copia skills embutidos no binário para ~/.assistente/skills/.
// Instala skills novos e atualiza os que têm versão mais antiga que a embutida.
// Skills que o usuário já atualizou (versão >= embutida) não são sobrescritos.
func (a *App) installBuiltinSkills() {
	resolver := configdir.NewResolver("skills")
	homeDir := resolver.GetHomeDir()
	if homeDir == "" {
		log.Printf("[Skills] Home dir not available, skipping builtin skill install")
		return
	}

	if err := os.MkdirAll(homeDir, 0755); err != nil {
		log.Printf("[Skills] Error creating skills home dir: %v", err)
		return
	}

	skillEntries, err := fs.ReadDir(builtinSkillsFS, ".assistente/skills")
	if err != nil {
		log.Printf("[Skills] Error reading embedded skills: %v", err)
		return
	}

	for _, entry := range skillEntries {
		if !entry.IsDir() {
			continue
		}

		slug := entry.Name()
		embeddedBase := ".assistente/skills/" + slug

		embeddedSkillData, err := fs.ReadFile(builtinSkillsFS, embeddedBase+"/SKILL.md")
		if err != nil {
			continue
		}

		embeddedMeta, _, err := skills.Parse(string(embeddedSkillData))
		if err != nil {
			log.Printf("[Skills] Error parsing embedded skill %s: %v", slug, err)
			continue
		}

		targetDir := filepath.Join(homeDir, slug)
		targetSkillFile := filepath.Join(targetDir, "SKILL.md")

		if existingData, err := os.ReadFile(targetSkillFile); err == nil {
			existingMeta, _, err := skills.Parse(string(existingData))
			if err == nil && !isVersionNewer(embeddedMeta.Version, existingMeta.Version) {
				if bytes.Equal(embeddedSkillData, existingData) {
					log.Printf("[Skills] Builtin %s v%s up to date (installed: v%s)", slug, embeddedMeta.Version, existingMeta.Version)
					continue
				}
				log.Printf("[Skills] Updating builtin skill %s v%s (content changed)", slug, embeddedMeta.Version)
			} else {
				log.Printf("[Skills] Updating builtin skill %s: v%s → v%s", slug, existingMeta.Version, embeddedMeta.Version)
			}
		} else {
			log.Printf("[Skills] Installing builtin skill %s v%s", slug, embeddedMeta.Version)
		}

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			log.Printf("[Skills] Error creating dir for %s: %v", slug, err)
			continue
		}

		a.copyEmbeddedSkillDir(embeddedBase, targetDir)
	}
}

// copyEmbeddedSkillDir copia todos os arquivos de um skill embutido para o diretório destino.
func (a *App) copyEmbeddedSkillDir(embeddedBase, targetDir string) {
	entries, err := fs.ReadDir(builtinSkillsFS, embeddedBase)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := fs.ReadFile(builtinSkillsFS, embeddedBase+"/"+entry.Name())
		if err != nil {
			log.Printf("[Skills] Error reading embedded file %s: %v", entry.Name(), err)
			continue
		}

		targetFile := filepath.Join(targetDir, entry.Name())
		if err := os.WriteFile(targetFile, data, 0644); err != nil {
			log.Printf("[Skills] Error writing %s: %v", targetFile, err)
		}
	}
}

// isVersionNewer retorna true se newVer é mais recente que oldVer (semver X.Y.Z).
func isVersionNewer(newVer, oldVer string) bool {
	newParts := parseVersion(newVer)
	oldParts := parseVersion(oldVer)

	for i := 0; i < 3; i++ {
		if newParts[i] > oldParts[i] {
			return true
		}
		if newParts[i] < oldParts[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	var parts [3]int
	segments := strings.SplitN(v, ".", 3)
	for i, s := range segments {
		if i >= 3 {
			break
		}
		parts[i], _ = strconv.Atoi(s)
	}
	return parts
}
