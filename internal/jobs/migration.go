package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/portability"
)

// LegacyDefinitionSource retorna uma fonte read-only para definições YAML
// legadas. Logs em runs/**/*.json e events/*.jsonl não são listados.
func (m *Manager) LegacyDefinitionSource() portability.LegacyImportSource {
	return legacyDefinitionSource{baseDir: m.cfg.BaseDir}
}

// ImportLegacyDefinitions importa apenas definições/configurações de jobs.
// Runs e eventos legados são descartados por desenho.
func (m *Manager) ImportLegacyDefinitions(ctx context.Context) (portability.LegacyImportResult, error) {
	return portability.ImportLegacyResourcesWithContext(ctx, portability.LegacyImportRequest[*Job]{
		ResourceType: "jobs",
		Source:       m.LegacyDefinitionSource(),
		Parse: func(_ portability.LegacyImportFile, data []byte) (*Job, error) {
			return Parse(data)
		},
		Import: func(ctx context.Context, job *Job) (bool, error) {
			if _, err := m.cfg.Repository.GetJob(ctx, job.ID); err == nil {
				return false, nil
			}
			if err := m.cfg.Repository.SaveJob(ctx, job); err != nil {
				return false, err
			}
			return true, nil
		},
	})
}

type legacyDefinitionSource struct {
	baseDir string
}

func (s legacyDefinitionSource) ListLegacyImportFiles(context.Context) ([]portability.LegacyImportFile, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]portability.LegacyImportFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if name == "catalog.yaml" || name == "catalog.yml" {
			continue
		}
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			continue
		}
		files = append(files, portability.LegacyImportFile{
			Name:     strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml"),
			Filename: name,
			Path:     filepath.Join(s.baseDir, name),
			Source:   "jobs",
		})
	}
	return files, nil
}

func (s legacyDefinitionSource) ReadLegacyImportFile(_ context.Context, filename string) ([]byte, error) {
	clean := filepath.Base(filename)
	return os.ReadFile(filepath.Join(s.baseDir, clean))
}
