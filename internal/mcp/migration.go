package mcp

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) migrateFilesystemConfigsToRepository(ctx context.Context, repo Repository) error {
	existing, err := repo.ListServers(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
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
	for _, filename := range jsonFiles {
		data, _, err := m.resolver.Read(filename)
		if err != nil {
			return fmt.Errorf("erro ao ler config MCP legado %s: %w", filename, err)
		}
		slug := strings.TrimSuffix(filename, filepath.Ext(filename))
		cfg, err := ParseServerConfig(data, slug)
		if err != nil {
			return fmt.Errorf("erro ao parsear config MCP legado %s: %w", filename, err)
		}
		cfg.Slug = slug
		m.applyInlineAuthFromConfig(slug, &cfg, data)
		if err := repo.SaveServer(ctx, &cfg); err != nil {
			return fmt.Errorf("erro ao migrar config MCP legado %s: %w", filename, err)
		}
		imported++
	}

	if imported == 0 {
		return nil
	}
	if err := m.backupMigratedConfigDir(); err != nil {
		return err
	}
	log.Printf("[MCP] Migração filesystem → DB concluída: %d servidores importados", imported)
	return nil
}

func (m *Manager) backupMigratedConfigDir() error {
	homeDir := m.resolver.GetHomeDir()
	if strings.TrimSpace(homeDir) == "" {
		return nil
	}
	if _, err := os.Stat(homeDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("erro ao verificar diretório MCP legado: %w", err)
	}
	backupDir := homeDir + ".migrated"
	if _, err := os.Stat(backupDir); err == nil {
		log.Printf("[MCP] Backup de configs legado já existe: %s", backupDir)
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao verificar backup MCP legado: %w", err)
	}
	if err := os.Rename(homeDir, backupDir); err != nil {
		return fmt.Errorf("erro ao renomear diretório MCP legado para backup: %w", err)
	}
	log.Printf("[MCP] Diretório MCP legado renomeado para backup: %s", backupDir)
	return nil
}
