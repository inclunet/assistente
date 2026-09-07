package mcp

import (
	"context"

	"assistente/internal/configdir"
	"assistente/internal/portability"
)

// LegacyConfigSource returns a read-only source for legacy MCP JSON files.
// The import orchestration lives in internal/portability; the manager only
// adapts its historical resolver to the shared source interface.
func (m *Manager) LegacyConfigSource() portability.LegacyImportSource {
	return legacyConfigSource{resolver: m.resolver}
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
