package jobs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/tools"

	"gopkg.in/yaml.v3"
)

// CatalogFile e a estrutura do catalog.yaml gerado automaticamente.
type CatalogFile struct {
	GeneratedAt string         `yaml:"generated_at"`
	ToolCount   int            `yaml:"tool_count"`
	Tools       []CatalogEntry `yaml:"tools"`
}

// GenerateCatalog gera o arquivo catalog.yaml com todas as tools disponiveis.
// O arquivo e read-only e regenerado quando MCP servers conectam/desconectam.
func GenerateCatalog(toolRegistry *tools.Registry, outputDir string) error {
	allTools := toolRegistry.All()

	entries := make([]CatalogEntry, 0, len(allTools))
	for _, t := range allTools {
		source := "internal"
		if strings.HasPrefix(t.Name(), "mcp_") {
			source = "mcp"
		}

		entries = append(entries, CatalogEntry{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Parameters(),
			Source:      source,
		})
	}

	catalog := CatalogFile{
		GeneratedAt: time.Now().Format("2006-01-02T15:04:05Z"),
		ToolCount:   len(entries),
		Tools:       entries,
	}

	data, err := yaml.Marshal(&catalog)
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create catalog dir: %w", err)
	}

	path := filepath.Join(outputDir, "catalog.yaml")
	header := []byte("# Auto-generated tool catalog — DO NOT EDIT\n# Regenerated when MCP servers connect/disconnect\n\n")
	content := append(header, data...)

	if err := os.WriteFile(path, content, 0444); err != nil {
		// Se o arquivo ja existe como read-only, remover e reescrever
		_ = os.Chmod(path, 0644)
		if err := os.WriteFile(path, content, 0444); err != nil {
			return fmt.Errorf("write catalog: %w", err)
		}
	}

	log.Printf("[Jobs] Catalog generated: %d tools in %s", len(entries), path)
	return nil
}

// ReadCatalog le o catalogo de tools do disco.
func ReadCatalog(baseDir string) (*CatalogFile, error) {
	path := filepath.Join(baseDir, "catalog.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var catalog CatalogFile
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	return &catalog, nil
}

// GetCatalogEntries retorna as entradas do catalogo, lendo do disco.
func GetCatalogEntries(baseDir string) ([]CatalogEntry, error) {
	catalog, err := ReadCatalog(baseDir)
	if err != nil {
		return nil, err
	}
	return catalog.Tools, nil
}
