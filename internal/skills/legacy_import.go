package skills

import (
	"context"
	"fmt"
	"log"
	"os"

	"assistente/internal/portability"
)

// LegacySkillSource retorna uma fonte read-only para os SKILL.md em filesystem.
//
// Reaproveita a descoberta multi-diretório (workdir > home > exe) e NÃO renomeia,
// apaga ou reescreve os arquivos originais (AEP-0051 D9).
func (m *Manager) LegacySkillSource() portability.LegacyImportSource {
	return legacySkillSource{mgr: m}
}

type legacySkillSource struct {
	mgr *Manager
}

func (s legacySkillSource) ListLegacyImportFiles(context.Context) ([]portability.LegacyImportFile, error) {
	discovered := s.mgr.discoverAll()
	files := make([]portability.LegacyImportFile, 0, len(discovered))
	for _, ds := range discovered {
		files = append(files, portability.LegacyImportFile{
			Name:     ds.slug,
			Filename: ds.path, // caminho absoluto do SKILL.md (lido por ReadLegacyImportFile)
			Path:     ds.path,
			Source:   string(ds.source),
		})
	}
	return files, nil
}

func (s legacySkillSource) ReadLegacyImportFile(_ context.Context, filename string) ([]byte, error) {
	// filename é o caminho absoluto descoberto por ListLegacyImportFiles — read-only.
	return os.ReadFile(filename)
}

// ImportLegacySkills importa os SKILL.md do filesystem para o banco de forma
// idempotente e não-destrutiva (AEP-0051 D9). Skill já existente no DB = skipped;
// os arquivos originais permanecem intactos no disco.
func (m *Manager) ImportLegacySkills(ctx context.Context) (portability.LegacyImportResult, error) {
	return m.importLegacySkillsFrom(ctx, m.LegacySkillSource())
}

// importLegacySkillsFrom executa a importação a partir de uma Source arbitrária
// (testável com fontes em memória).
func (m *Manager) importLegacySkillsFrom(ctx context.Context, source portability.LegacyImportSource) (portability.LegacyImportResult, error) {
	if m.repo == nil {
		return portability.LegacyImportResult{ResourceType: "skills"}, fmt.Errorf("repository de skills não configurado")
	}
	result, err := portability.ImportLegacyResourcesWithContext(ctx, portability.LegacyImportRequest[*Skill]{
		ResourceType: "skills",
		Source:       source,
		Parse: func(file portability.LegacyImportFile, data []byte) (*Skill, error) {
			meta, content, err := Parse(string(data))
			if err != nil {
				return nil, err
			}
			if meta.Name == "" {
				meta.Name = file.Name
			}
			return &Skill{SkillMetadata: *meta, Slug: file.Name, Content: content}, nil
		},
		Import: func(ctx context.Context, skill *Skill) (bool, error) {
			exists, err := m.repo.ExistsBySlug(ctx, skill.Slug)
			if err != nil {
				return false, err
			}
			if exists {
				return false, nil // já no DB — não duplica nem altera
			}
			if _, err := m.repo.Create(ctx, skill); err != nil {
				return false, err
			}
			return true, nil
		},
		// Observabilidade (AEP-0072 Fase 5 / #123): avisos de qualidade de
		// descrição nas skills recém-importadas, sem falhar a importação.
		Inspect: func(_ portability.LegacyImportFile, skill *Skill) []string {
			warnings := ValidateDescriptionQuality(skill.Description)
			if len(warnings) == 0 {
				return nil
			}
			msgs := make([]string, 0, len(warnings))
			for _, w := range warnings {
				msgs = append(msgs, w.Message)
			}
			return msgs
		},
	})
	// AEP-0072 D1: ressincroniza o catálogo após a importação em massa, mesmo
	// que parcial — só faz sentido se algo foi importado.
	if result.Imported > 0 {
		if rebuildErr := m.repo.RebuildCatalog(ctx, m.MaterializeSkill); rebuildErr != nil {
			log.Printf("[Skills] Erro ao reconstruir catálogo pós-importação: %v", rebuildErr)
		}
	}
	return result, err
}
