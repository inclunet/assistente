package mcp

import (
	"context"
	"log"

	"assistente/internal/configdir"
	"assistente/internal/portability"
)

func (m *Manager) importLegacyFilesystemConfigs(ctx context.Context) error {
	result, err := portability.ImportLegacyMCPServersWithContext(ctx, legacyConfigSource{resolver: m.resolver}, m.credMgr)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		log.Printf("[MCP] Importação legada: %s", warning)
	}
	if result.Imported > 0 || result.Skipped > 0 {
		log.Printf("[MCP] Importação de configs MCP legadas concluída: %d importados, %d já existentes", result.Imported, result.Skipped)
	}
	return nil
}

type legacyConfigSource struct {
	resolver *configdir.Resolver
}

func (s legacyConfigSource) ListLegacyImportFiles(context.Context) ([]portability.LegacyImportFile, error) {
	files, err := s.resolver.List()
	if err != nil {
		return nil, err
	}
	result := make([]portability.LegacyImportFile, 0, len(files))
	for _, file := range files {
		result = append(result, portability.LegacyImportFile{
			Name:     file.Name,
			Filename: file.Filename,
			Path:     file.Path,
			Source:   string(file.Source),
		})
	}
	return result, nil
}

func (s legacyConfigSource) ReadLegacyImportFile(_ context.Context, filename string) ([]byte, error) {
	data, _, err := s.resolver.Read(filename)
	return data, err
}
