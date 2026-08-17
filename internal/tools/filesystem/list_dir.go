package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"assistente/internal/tools"
)

// ListDirectory lista o conteúdo de um diretório com metadados.
type ListDirectory struct {
	workDir string
}

// NewListDirectory cria uma nova instância de ListDirectory.
func NewListDirectory(workDir string) *ListDirectory {
	return &ListDirectory{workDir: workDir}
}

func (t *ListDirectory) Name() string { return "list_directory" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *ListDirectory) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"}
}

func (t *ListDirectory) Description() string {
	return "Lists files and directories for a path (default: working directory). Returns name, type, and size. Not recursive by default; use recursive=true and max_depth."
}

func (t *ListDirectory) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do diretório (absoluto ou relativo ao diretório de trabalho). Padrão: diretório de trabalho."
			},
			"recursive": {
				"type": "boolean",
				"description": "Se true, lista recursivamente todos os subdiretórios. Padrão: false."
			},
			"max_depth": {
				"type": "integer",
				"description": "Profundidade máxima da recursão (somente com recursive=true). Padrão: 3."
			}
		},
		"additionalProperties": false
	}`)
}

type listDirArgs struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	MaxDepth  *int   `json:"max_depth,omitempty"`
}

// Limites para evitar saídas muito grandes
const (
	defaultMaxDepth = 3
	maxEntries      = 1000
)

func (t *ListDirectory) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a listDirArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	// Padrão: diretório de trabalho
	dirPath := a.Path
	if dirPath == "" {
		dirPath = "."
	}

	// Resolve caminho
	fullPath, err := t.resolvePath(dirPath)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Valida segurança
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "list"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se é diretório
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Content: fmt.Sprintf("Diretório não encontrado: %s", dirPath), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar diretório: %v", err), IsError: true}, nil
	}
	if !info.IsDir() {
		return tools.ToolResult{Content: fmt.Sprintf("'%s' é um arquivo, não um diretório. Use read_file.", dirPath), IsError: true}, nil
	}

	maxDepth := defaultMaxDepth
	if a.MaxDepth != nil && *a.MaxDepth > 0 {
		maxDepth = *a.MaxDepth
	}

	if a.Recursive {
		return t.listRecursive(ctx, fullPath, dirPath, maxDepth)
	}

	return t.listFlat(ctx, fullPath, dirPath)
}

// listFlat lista apenas o nível imediato do diretório
func (t *ListDirectory) listFlat(ctx context.Context, fullPath, displayPath string) (tools.ToolResult, error) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler diretório: %v", err), IsError: true}, nil
	}

	var lines []string
	dirCount, fileCount := 0, 0
	skippedSensitive := 0
	skippedBySkill := 0

	for _, entry := range entries {
		entryPath := filepath.Join(fullPath, entry.Name())
		if err := validateSkillFilesystemAllowlist(ctx, entryPath, t.workDir, "list"); err != nil {
			skippedBySkill++
			continue
		}
		if ToolPolicy().BlockSensitive && isSensitiveEntry(entryPath, entry.Type()) {
			skippedSensitive++
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if entry.IsDir() {
			lines = append(lines, fmt.Sprintf("  [DIR]  %s/", entry.Name()))
			dirCount++
		} else {
			lines = append(lines, fmt.Sprintf("  [FILE] %s (%s)", entry.Name(), formatSize(info.Size())))
			fileCount++
		}
	}

	// Ordena: diretórios primeiro, depois arquivos
	sort.SliceStable(lines, func(i, j int) bool {
		iIsDir := strings.Contains(lines[i], "[DIR]")
		jIsDir := strings.Contains(lines[j], "[DIR]")
		if iIsDir != jIsDir {
			return iIsDir
		}
		return lines[i] < lines[j]
	})

	header := fmt.Sprintf("Diretório: %s (%d diretórios, %d arquivos)\n", displayPath, dirCount, fileCount)
	if skippedSensitive > 0 {
		header += fmt.Sprintf("(%d arquivo(s) sensível(is) omitido(s))\n", skippedSensitive)
	}
	if skippedBySkill > 0 {
		header += fmt.Sprintf("(%d entrada(s) omitida(s) por permissões do skill)\n", skippedBySkill)
	}
	return tools.ToolResult{
		Content: header + strings.Join(lines, "\n"),
		Metadata: map[string]any{
			"directories":       dirCount,
			"files":             fileCount,
			"skipped":           skippedSensitive + skippedBySkill,
			"skipped_sensitive": skippedSensitive,
			"skipped_by_skill":  skippedBySkill,
		},
	}, nil
}

// listRecursive lista recursivamente com indentação em árvore
func (t *ListDirectory) listRecursive(ctx context.Context, fullPath, displayPath string, maxDepth int) (tools.ToolResult, error) {
	var lines []string
	totalFiles, totalDirs := 0, 0
	truncated := false
	skippedSensitive := 0
	skippedBySkill := 0

	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if depth > maxDepth {
			return nil
		}
		if totalFiles+totalDirs >= maxEntries {
			truncated = true
			return nil
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil // Ignora diretórios sem permissão
		}

		indent := strings.Repeat("  ", depth)
		for _, entry := range entries {
			if totalFiles+totalDirs >= maxEntries {
				truncated = true
				return nil
			}

			entryPath := filepath.Join(dir, entry.Name())
			if err := validateSkillFilesystemAllowlist(ctx, entryPath, t.workDir, "list"); err != nil {
				if entry.IsDir() {
					// Não desce em diretórios fora do escopo do skill.
					skippedBySkill++
					continue
				}
				skippedBySkill++
				continue
			}

			// Ignora diretórios comuns que poluem a listagem
			name := entry.Name()
			if entry.IsDir() && shouldSkipDir(name) {
				lines = append(lines, fmt.Sprintf("%s%s/ (ignorado)", indent, name))
				continue
			}

			if entry.IsDir() {
				lines = append(lines, fmt.Sprintf("%s%s/", indent, name))
				totalDirs++
				if err := walk(entryPath, depth+1); err != nil {
					return err
				}
			} else {
				if ToolPolicy().BlockSensitive && isSensitiveEntry(entryPath, entry.Type()) {
					skippedSensitive++
					continue
				}

				info, _ := entry.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				lines = append(lines, fmt.Sprintf("%s%s (%s)", indent, name, formatSize(size)))
				totalFiles++
			}
		}
		return nil
	}

	if err := walk(fullPath, 0); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao listar: %v", err), IsError: true}, nil
	}

	header := fmt.Sprintf("Diretório: %s (recursivo, profundidade máx: %d)\n%d diretórios, %d arquivos\n",
		displayPath, maxDepth, totalDirs, totalFiles)
	if skippedSensitive > 0 {
		header += fmt.Sprintf("(%d arquivo(s) sensível(is) omitido(s))\n", skippedSensitive)
	}
	if skippedBySkill > 0 {
		header += fmt.Sprintf("(%d entrada(s) omitida(s) por permissões do skill)\n", skippedBySkill)
	}

	if truncated {
		header += fmt.Sprintf("(TRUNCADO: limite de %d entradas atingido)\n", maxEntries)
	}

	return tools.ToolResult{
		Content: header + strings.Join(lines, "\n"),
		Metadata: map[string]any{
			"directories":       totalDirs,
			"files":             totalFiles,
			"truncated":         truncated,
			"skipped":           skippedSensitive + skippedBySkill,
			"skipped_sensitive": skippedSensitive,
			"skipped_by_skill":  skippedBySkill,
		},
	}, nil
}

// resolvePath converte caminho relativo para absoluto
func (t *ListDirectory) resolvePath(path string) (string, error) {
	return resolveFilePath(path, t.workDir)
}

// shouldSkipDir retorna true para diretórios que devem ser ignorados na recursão
func shouldSkipDir(name string) bool {
	skip := map[string]bool{
		"node_modules": true,
		".git":         true,
		".svn":         true,
		"__pycache__":  true,
		".next":        true,
		".nuxt":        true,
		"dist":         true,
		"vendor":       true,
		".cache":       true,
		".vscode":      true,
		".idea":        true,
	}
	return skip[name]
}

// formatSize formata bytes em formato legível
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
