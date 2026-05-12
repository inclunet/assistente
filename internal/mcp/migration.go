package mcp

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"assistente/internal/portability"
)

func (m *Manager) migrateFilesystemConfigsToRepository(ctx context.Context, repo Repository) error {
	existing, err := repo.ListServers(ctx)
	if err != nil {
		return err
	}
	existingBySlug := make(map[string]struct{}, len(existing))
	for _, cfg := range existing {
		existingBySlug[cfg.Slug] = struct{}{}
	}

	files, err := m.resolver.List()
	if err != nil {
		log.Printf("[MCP] Migração filesystem → DB ignorada: não foi possível listar configs legadas: %v", err)
		return nil
	}
	jsonFiles := make([]string, 0, len(files))
	for _, f := range files {
		if strings.HasSuffix(f.Filename, configExt) {
			jsonFiles = append(jsonFiles, f.Filename)
		}
	}
	if len(jsonFiles) == 0 {
		return nil
	}

	imported := 0
	skipped := 0
	for _, filename := range jsonFiles {
		slug := strings.TrimSuffix(filename, filepath.Ext(filename))
		if _, exists := existingBySlug[slug]; exists {
			skipped++
			continue
		}
		data, _, err := m.resolver.Read(filename)
		if err != nil {
			return fmt.Errorf("erro ao ler config MCP legado %s: %w", filename, err)
		}
		cfg, err := ParseServerConfig(data, slug)
		if err != nil {
			return fmt.Errorf("erro ao parsear config MCP legado %s: %w", filename, err)
		}
		cfg.Slug = slug
		m.applyInlineAuthFromConfig(slug, &cfg, data)
		if _, err := portability.ImportMCPServerWithContext(ctx, mcpServerExportFromConfig(cfg)); err != nil {
			return fmt.Errorf("erro ao migrar config MCP legado %s: %w", filename, err)
		}
		existingBySlug[slug] = struct{}{}
		imported++
	}

	if imported > 0 || skipped > 0 {
		log.Printf("[MCP] Importação de configs MCP legadas concluída: %d importados, %d já existentes", imported, skipped)
	}
	return nil
}
