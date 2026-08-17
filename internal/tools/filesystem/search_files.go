package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/tools"
)

// SearchFiles busca arquivos por padrão glob no disco.
type SearchFiles struct {
	workDir string
}

// NewSearchFiles cria uma nova instância de SearchFiles.
func NewSearchFiles(workDir string) *SearchFiles {
	return &SearchFiles{workDir: workDir}
}

func (t *SearchFiles) Name() string { return "search_files" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *SearchFiles) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"}
}

func (t *SearchFiles) Description() string {
	return "Finds files by glob pattern. Use **/ for recursive search (e.g., '**/test_*.py'). Without **/, it searches only the base directory."
}

func (t *SearchFiles) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Padrão glob para busca. Exemplos: '*.go', '**/*.ts', 'internal/**/*.go', 'README*'"
			},
			"path": {
				"type": "string",
				"description": "Diretório base para busca (padrão: diretório de trabalho)"
			},
			"max_results": {
				"type": "integer",
				"description": "Número máximo de resultados (padrão: 100)"
			}
		},
		"required": ["pattern"],
		"additionalProperties": false
	}`)
}

type searchFilesArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	MaxResults *int   `json:"max_results,omitempty"`
}

const defaultMaxResults = 100

func (t *SearchFiles) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a searchFilesArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Pattern == "" {
		return tools.ToolResult{Content: "Parâmetro 'pattern' é obrigatório", IsError: true}, nil
	}

	// Diretório base
	basePath := a.Path
	if basePath == "" {
		basePath = "."
	}

	fullBase, err := t.resolvePath(basePath)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Valida segurança
	if err := validatePathWithPolicy(ctx, fullBase, t.workDir, ToolPolicy(), "search"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se diretório existe
	if info, err := os.Stat(fullBase); err != nil || !info.IsDir() {
		return tools.ToolResult{Content: fmt.Sprintf("Diretório não encontrado: %s", basePath), IsError: true}, nil
	}

	maxResults := defaultMaxResults
	if a.MaxResults != nil && *a.MaxResults > 0 {
		maxResults = *a.MaxResults
	}

	// Detecta se é um padrão recursivo (contém **/)
	isRecursive := strings.Contains(a.Pattern, "**/") || strings.Contains(a.Pattern, "**\\")

	var matches []string
	truncated := false
	skippedBySkill := 0

	if isRecursive {
		// Busca recursiva: percorre a árvore e aplica glob no nome relativo
		// Extrai o padrão após **/ para match
		matchPattern := a.Pattern
		// Normaliza separadores
		matchPattern = strings.ReplaceAll(matchPattern, "\\", "/")

		err = filepath.WalkDir(fullBase, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Ignora erros de permissão
			}

			// Verifica contexto para permitir cancelamento
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Pula diretórios que devem ser ignorados
			if d.IsDir() && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}

			// Enforcement por skill: não vazar nomes fora do escopo
			if err := validateSkillFilesystemAllowlist(ctx, path, t.workDir, "search"); err != nil {
				skippedBySkill++
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Link apontando para fora do sandbox: não vazar nomes externos
			if walkEntryEscapesSandbox(path, d.Type(), t.workDir) {
				return nil
			}

			if d.IsDir() {
				return nil
			}

			// Toolcalling: não vazar nomes de arquivos sensíveis
			if ToolPolicy().BlockSensitive && isSensitiveEntry(path, d.Type()) {
				return nil
			}

			// Calcula caminho relativo
			relPath, err := filepath.Rel(fullBase, path)
			if err != nil {
				return nil
			}
			// Normaliza para / para match consistente
			relPath = filepath.ToSlash(relPath)

			// Tenta match com o padrão
			matched, err := filepath.Match(extractGlobPattern(matchPattern), filepath.Base(relPath))
			if err != nil {
				return nil
			}

			// Se não casou com o nome simples, tenta o caminho completo
			if !matched {
				matched = matchPath(relPath, matchPattern)
			}

			if matched {
				if len(matches) >= maxResults {
					truncated = true
					return filepath.SkipAll
				}

				info, _ := d.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				matches = append(matches, fmt.Sprintf("  %s (%s)", relPath, formatSize(size)))
			}

			return nil
		})
	} else {
		// Busca não-recursiva: usa filepath.Glob no diretório base
		globPattern := filepath.Join(fullBase, a.Pattern)
		globMatches, err := filepath.Glob(globPattern)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Padrão glob inválido: %v", err), IsError: true}, nil
		}

		for _, match := range globMatches {
			if len(matches) >= maxResults {
				truncated = true
				break
			}

			// Link apontando para fora do sandbox: não vazar nomes externos
			if pathEscapesSandbox(match, t.workDir) {
				continue
			}

			// Toolcalling: não vazar nomes de arquivos sensíveis
			if ToolPolicy().BlockSensitive && isSensitiveFileResolved(match) {
				continue
			}
			if err := validateSkillFilesystemAllowlist(ctx, match, t.workDir, "search"); err != nil {
				skippedBySkill++
				continue
			}

			relPath, _ := filepath.Rel(fullBase, match)
			if relPath == "" {
				relPath = match
			}

			info, err := os.Stat(match)
			if err != nil {
				continue
			}

			prefix := "[FILE]"
			sizeStr := formatSize(info.Size())
			if info.IsDir() {
				prefix = "[DIR] "
				sizeStr = ""
			}

			if sizeStr != "" {
				matches = append(matches, fmt.Sprintf("  %s %s (%s)", prefix, filepath.ToSlash(relPath), sizeStr))
			} else {
				matches = append(matches, fmt.Sprintf("  %s %s/", prefix, filepath.ToSlash(relPath)))
			}
		}
	}

	if err != nil && ctx.Err() != nil {
		return tools.ToolResult{Content: "Busca cancelada pelo usuário", IsError: true}, nil
	}

	if len(matches) == 0 {
		return tools.ToolResult{
			Content: fmt.Sprintf("Nenhum arquivo encontrado com o padrão '%s' em '%s'", a.Pattern, basePath),
			Metadata: map[string]any{
				"results": 0,
			},
		}, nil
	}

	header := fmt.Sprintf("Busca: '%s' em '%s' — %d resultado(s)\n", a.Pattern, basePath, len(matches))
	if truncated {
		header += fmt.Sprintf("(TRUNCADO: limite de %d resultados atingido)\n", maxResults)
	}
	if skippedBySkill > 0 {
		header += fmt.Sprintf("(%d caminho(s) omitido(s) por permissões do skill)\n", skippedBySkill)
	}

	return tools.ToolResult{
		Content: header + strings.Join(matches, "\n"),
		Metadata: map[string]any{
			"results":          len(matches),
			"truncated":        truncated,
			"skipped_by_skill": skippedBySkill,
		},
	}, nil
}

func (t *SearchFiles) resolvePath(path string) (string, error) {
	return resolveFilePath(path, t.workDir)
}

// extractGlobPattern extrai o padrão simples de um padrão recursivo
// Ex: "**/*.go" -> "*.go", "**/test_*.py" -> "test_*.py"
func extractGlobPattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	parts := strings.Split(pattern, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pattern
}

// matchPath tenta fazer match de um caminho relativo com um padrão que pode conter **/
func matchPath(relPath, pattern string) bool {
	// Normaliza separadores
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	pattern = strings.ReplaceAll(pattern, "\\", "/")

	// Caso simples: **/*.ext → match qualquer profundidade com a extensão
	if strings.HasPrefix(pattern, "**/") {
		subPattern := strings.TrimPrefix(pattern, "**/")
		// Tenta match no nome do arquivo
		matched, _ := filepath.Match(subPattern, filepath.Base(relPath))
		if matched {
			return true
		}
		// Tenta match no caminho relativo completo
		parts := strings.Split(relPath, "/")
		for i := range parts {
			subPath := strings.Join(parts[i:], "/")
			if matched, _ := filepath.Match(subPattern, subPath); matched {
				return true
			}
		}
	}

	// Tenta match direto
	matched, _ := filepath.Match(pattern, relPath)
	return matched
}
