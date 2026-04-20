package filesystem

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"assistente/internal/tools"
)

// GrepSearch busca por padrão (regex ou literal) dentro do conteúdo de arquivos.
// Similar ao ripgrep/grep, retorna linhas correspondentes com contexto.
type GrepSearch struct {
	workDir string
}

// NewGrepSearch cria uma nova instância de GrepSearch.
func NewGrepSearch(workDir string) *GrepSearch {
	return &GrepSearch{workDir: workDir}
}

func (t *GrepSearch) Name() string { return "grep_search" }

func (t *GrepSearch) Description() string {
	return "Searches file contents by pattern (Go regex or literal). Returns matching lines with line numbers and context. Use include to filter files and case_sensitive=false for case-insensitive search."
}

func (t *GrepSearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Padrão de busca. Texto literal ou expressão regular (Go regex syntax)."
			},
			"path": {
				"type": "string",
				"description": "Diretório ou arquivo para buscar (padrão: diretório de trabalho)."
			},
			"include": {
				"type": "string",
				"description": "Glob para filtrar arquivos. Exemplos: '*.go', '*.{ts,tsx}', '*.py'."
			},
			"case_sensitive": {
				"type": "boolean",
				"description": "Se false, busca é case-insensitive. Padrão: true."
			},
			"max_results": {
				"type": "integer",
				"description": "Número máximo de correspondências totais. Padrão: 100."
			},
			"context_lines": {
				"type": "integer",
				"description": "Linhas de contexto antes e depois de cada match. Padrão: 2."
			}
		},
		"required": ["pattern"],
		"additionalProperties": false
	}`)
}

type grepSearchArgs struct {
	Pattern       string `json:"pattern"`
	Path          string `json:"path"`
	Include       string `json:"include"`
	CaseSensitive *bool  `json:"case_sensitive,omitempty"`
	MaxResults    *int   `json:"max_results,omitempty"`
	ContextLines  *int   `json:"context_lines,omitempty"`
}

// grepMatch representa um match individual
type grepMatch struct {
	File       string
	LineNumber int
	LineText   string
}

// Limites de segurança
const (
	grepDefaultMaxResults   = 100
	grepDefaultContextLines = 2
	grepMaxFileSize         = 5 * 1024 * 1024 // 5MB — pula arquivos maiores
	grepMaxFilesScanned     = 10000           // Limite de arquivos escaneados
)

func (t *GrepSearch) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a grepSearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Pattern == "" {
		return tools.ToolResult{Content: "Parâmetro 'pattern' é obrigatório", IsError: true}, nil
	}

	// Padrões de busca
	caseSensitive := true
	if a.CaseSensitive != nil {
		caseSensitive = *a.CaseSensitive
	}

	maxResults := grepDefaultMaxResults
	if a.MaxResults != nil && *a.MaxResults > 0 {
		maxResults = *a.MaxResults
	}

	contextLines := grepDefaultContextLines
	if a.ContextLines != nil && *a.ContextLines >= 0 {
		contextLines = *a.ContextLines
	}

	// Compila regex
	regexPattern := a.Pattern
	if !caseSensitive {
		regexPattern = "(?i)" + regexPattern
	}

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		// Se regex inválida, tenta como literal
		escaped := regexp.QuoteMeta(a.Pattern)
		if !caseSensitive {
			escaped = "(?i)" + escaped
		}
		re, err = regexp.Compile(escaped)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Padrão de busca inválido: %v", err), IsError: true}, nil
		}
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

	if err := validatePathWithPolicy(ctx, fullBase, t.workDir, ToolPolicy(), "grep"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se é arquivo ou diretório
	info, err := os.Stat(fullBase)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Content: fmt.Sprintf("Caminho não encontrado: %s", basePath), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar caminho: %v", err), IsError: true}, nil
	}

	// Se for um arquivo, busca direto nele
	if !info.IsDir() {
		matches, _ := t.searchFile(ctx, fullBase, re, maxResults, contextLines)
		if len(matches) == 0 {
			return tools.ToolResult{
				Content:  fmt.Sprintf("Nenhuma correspondência para '%s' em '%s'", a.Pattern, basePath),
				Metadata: map[string]any{"results": 0},
			}, nil
		}
		return t.formatResults(a.Pattern, basePath, matches, 1, false, maxResults), nil
	}

	// Busca recursiva em diretório
	var allMatches []grepMatch
	filesScanned := 0
	truncated := false

	// Compila glob de inclusão se fornecido
	var includePattern string
	if a.Include != "" {
		includePattern = a.Include
	}

	_ = filepath.WalkDir(fullBase, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Ignora erros de permissão
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Pula diretórios ignorados
		if d.IsDir() && shouldSkipDir(d.Name()) {
			return filepath.SkipDir
		}

		// Enforcement por skill: não vazar nomes/conteúdo fora do escopo
		if err := validateSkillFilesystemAllowlist(ctx, path, t.workDir, "grep"); err != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		// Toolcalling: não vazar conteúdo de arquivos sensíveis
		if ToolPolicy().BlockSensitive && isSensitiveFile(path) {
			return nil
		}

		// Aplica filtro de inclusão
		if includePattern != "" {
			matched := matchIncludePattern(d.Name(), includePattern)
			if !matched {
				return nil
			}
		}

		// Pula arquivos binários/grandes
		if isBinaryExtension(d.Name()) {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			return nil
		}
		if fileInfo.Size() > grepMaxFileSize {
			return nil
		}

		// Limite de arquivos escaneados
		filesScanned++
		if filesScanned > grepMaxFilesScanned {
			truncated = true
			return filepath.SkipAll
		}

		// Busca neste arquivo
		remaining := maxResults - len(allMatches)
		if remaining <= 0 {
			truncated = true
			return filepath.SkipAll
		}

		fileMatches, _ := t.searchFile(ctx, path, re, remaining, contextLines)

		// Converte para caminho relativo
		for i := range fileMatches {
			relPath, relErr := filepath.Rel(fullBase, path)
			if relErr == nil {
				fileMatches[i].File = filepath.ToSlash(relPath)
			}
		}

		allMatches = append(allMatches, fileMatches...)
		return nil
	})

	if ctx.Err() != nil {
		return tools.ToolResult{Content: "Busca cancelada pelo usuário", IsError: true}, nil
	}

	if len(allMatches) == 0 {
		msg := fmt.Sprintf("Nenhuma correspondência para '%s' em '%s'", a.Pattern, basePath)
		if includePattern != "" {
			msg += fmt.Sprintf(" (filtro: %s)", includePattern)
		}
		return tools.ToolResult{
			Content:  msg,
			Metadata: map[string]any{"results": 0, "files_scanned": filesScanned},
		}, nil
	}

	return t.formatResults(a.Pattern, basePath, allMatches, filesScanned, truncated, maxResults), nil
}

// searchFile busca por matches em um único arquivo, incluindo linhas de contexto.
func (t *GrepSearch) searchFile(ctx context.Context, filePath string, re *regexp.Regexp, maxMatches, contextLines int) ([]grepMatch, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Lê todas as linhas para suporte a contexto
	var lines []string
	scanner := bufio.NewScanner(f)
	// Buffer de 1MB para linhas longas
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	var matches []grepMatch
	// Track quais linhas já foram incluídas para evitar duplicatas no contexto
	includedLines := make(map[int]bool)

	for lineIdx, line := range lines {
		select {
		case <-ctx.Done():
			return matches, ctx.Err()
		default:
		}

		if re.MatchString(line) {
			// Adiciona linhas de contexto antes
			startCtx := lineIdx - contextLines
			if startCtx < 0 {
				startCtx = 0
			}
			for i := startCtx; i < lineIdx; i++ {
				if !includedLines[i] {
					matches = append(matches, grepMatch{
						File:       filePath,
						LineNumber: i + 1,
						LineText:   fmt.Sprintf("  %6d- %s", i+1, lines[i]),
					})
					includedLines[i] = true
				}
			}

			// Adiciona a linha do match
			if !includedLines[lineIdx] {
				matches = append(matches, grepMatch{
					File:       filePath,
					LineNumber: lineIdx + 1,
					LineText:   fmt.Sprintf("  %6d: %s", lineIdx+1, line),
				})
				includedLines[lineIdx] = true
			}

			// Adiciona linhas de contexto depois
			endCtx := lineIdx + contextLines + 1
			if endCtx > len(lines) {
				endCtx = len(lines)
			}
			for i := lineIdx + 1; i < endCtx; i++ {
				if !includedLines[i] {
					matches = append(matches, grepMatch{
						File:       filePath,
						LineNumber: i + 1,
						LineText:   fmt.Sprintf("  %6d- %s", i+1, lines[i]),
					})
					includedLines[i] = true
				}
			}

			if len(matches) >= maxMatches {
				break
			}
		}
	}

	return matches, nil
}

// formatResults formata os resultados de busca para exibição.
func (t *GrepSearch) formatResults(pattern, basePath string, matches []grepMatch, filesScanned int, truncated bool, maxResults int) tools.ToolResult {
	// Agrupa matches por arquivo
	type fileGroup struct {
		file  string
		lines []string
	}
	var groups []fileGroup
	groupMap := make(map[string]int)

	for _, m := range matches {
		idx, exists := groupMap[m.File]
		if !exists {
			idx = len(groups)
			groupMap[m.File] = idx
			groups = append(groups, fileGroup{file: m.File})
		}
		groups[idx].lines = append(groups[idx].lines, m.LineText)
	}

	// Conta apenas linhas de match (não contexto)
	matchCount := 0
	for _, m := range matches {
		if strings.Contains(m.LineText, ":") && !strings.HasSuffix(strings.TrimSpace(strings.SplitN(m.LineText, ":", 1)[0]), "-") {
			matchCount++
		}
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Busca: '%s' em '%s'\n", pattern, basePath)
	_, _ = fmt.Fprintf(&sb, "%d arquivo(s) com correspondências (%d arquivos escaneados)\n", len(groups), filesScanned)
	if truncated {
		_, _ = fmt.Fprintf(&sb, "(TRUNCADO: limite de %d resultados atingido)\n", maxResults)
	}
	sb.WriteString("\n")

	for _, g := range groups {
		sb.WriteString(g.file + "\n")
		for _, line := range g.lines {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	return tools.ToolResult{
		Content: sb.String(),
		Metadata: map[string]any{
			"files_matched": len(groups),
			"files_scanned": filesScanned,
			"truncated":     truncated,
		},
	}
}

func (t *GrepSearch) resolvePath(path string) (string, error) {
	return resolveFilePath(path, t.workDir)
}

// matchIncludePattern verifica se um nome de arquivo casa com um padrão de inclusão.
// Suporta padrões simples (*.go) e com chaves (*.{ts,tsx}).
func matchIncludePattern(filename, pattern string) bool {
	// Trata padrão com chaves: *.{ts,tsx} → testa cada extensão
	if strings.Contains(pattern, "{") && strings.Contains(pattern, "}") {
		start := strings.Index(pattern, "{")
		end := strings.Index(pattern, "}")
		prefix := pattern[:start]
		suffix := pattern[end+1:]
		alternatives := strings.Split(pattern[start+1:end], ",")

		for _, alt := range alternatives {
			expandedPattern := prefix + strings.TrimSpace(alt) + suffix
			if matched, _ := filepath.Match(expandedPattern, filename); matched {
				return true
			}
		}
		return false
	}

	matched, _ := filepath.Match(pattern, filename)
	return matched
}

// isBinaryExtension retorna true para extensões de arquivos binários comuns
func isBinaryExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	binary := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".bin": true, ".obj": true, ".o": true, ".a": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".bmp": true, ".ico": true, ".webp": true, ".svg": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mkv": true,
		".zip": true, ".tar": true, ".gz": true, ".rar": true, ".7z": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
		".db": true, ".sqlite": true, ".sqlite3": true,
		".wasm": true, ".pyc": true, ".class": true,
	}
	return binary[ext]
}
