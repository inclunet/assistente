package filesystem

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"assistente/internal/docextract"
	"assistente/internal/tools"
)

// GrepSearch busca por padrão (regex ou literal) dentro do conteúdo de arquivos.
// Similar ao ripgrep/grep, retorna linhas correspondentes com contexto.
type GrepSearch struct {
	workDir string
	cache   *docextract.ProjectionCache
}

// NewGrepSearch cria uma nova instância de GrepSearch.
func NewGrepSearch(workDir string, caches ...*docextract.ProjectionCache) *GrepSearch {
	var cache *docextract.ProjectionCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	if cache == nil {
		cache = docextract.NewProjectionCache(docextract.DefaultCacheConfig())
	}
	return &GrepSearch{workDir: workDir, cache: cache}
}

func (t *GrepSearch) Name() string { return "grep_search" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *GrepSearch) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"}
}

func (t *GrepSearch) Description() string {
	return "Searches file contents by pattern (Go regex or literal). Text is searched as stored; opaque documents (PDF, DOCX, XLSX, PPTX, ODT/ODS/ODP, EPUB) are searched through an in-memory Markdown projection. Set document_mode=\"markdown\" to also project textual formats such as CSV/RTF. Extraction failures skip only that document and are reported as warnings. OCR is not available in this phase. Returns matching lines with line numbers and context."
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
			},
			"document_mode": {
				"type": "string",
				"enum": ["auto", "markdown"],
				"description": "auto (padrão): texto é pesquisado como está e só documentos opacos viram Markdown. markdown: também projeta formatos textuais com extrator, como CSV/RTF."
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
	DocumentMode  string `json:"document_mode,omitempty"`
}

// grepMatch representa um match individual
type grepMatch struct {
	File       string
	LineNumber int
	LineText   string
	Projection docextract.Kind
	// IsMatch separa a linha que casa com o padrão das linhas de contexto.
	IsMatch bool
}

type grepWarning struct {
	File   string
	Reason string
}

type grepStats struct {
	filesConsidered    int
	documentsProjected int
	documentsExtracted int
	// documentsCoalesced conta projeções que pegaram carona em uma extração já
	// em andamento: não foram cache pronto nem custaram uma nova extração.
	documentsCoalesced int
	cacheHits          int
	warnings           []grepWarning
	warningsOmitted    int
}

var errGrepBinaryContent = errors.New("conteúdo não é texto UTF-8 válido")

// Limites de segurança
const (
	grepDefaultMaxResults   = 100
	grepDefaultContextLines = 2
	grepMaxFileSize         = 5 * 1024 * 1024 // 5MB — pula arquivos maiores
	grepMaxFilesScanned     = 10000           // Limite de arquivos escaneados
	grepMaxWarnings         = 20
)

func (t *GrepSearch) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a grepSearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Pattern == "" {
		return tools.ToolResult{Content: "Parâmetro 'pattern' é obrigatório", IsError: true}, nil
	}
	mode, err := parseDocumentMode(a.DocumentMode)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
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
		stats := &grepStats{filesConsidered: 1}
		matches, searched := t.searchPath(ctx, fullBase, info, re, maxResults, contextLines, mode, stats)
		if ctx.Err() != nil {
			return tools.ToolResult{Content: "Busca cancelada pelo usuário", IsError: true}, nil
		}
		if len(matches) == 0 {
			result := tools.ToolResult{
				Content:  fmt.Sprintf("Nenhuma correspondência para '%s' em '%s'", a.Pattern, basePath),
				Metadata: map[string]any{"results": 0, "files_scanned": boolInt(searched)},
			}
			t.appendSearchStats(&result, stats)
			return result, nil
		}
		result := t.formatResults(a.Pattern, basePath, matches, boolInt(searched), false, maxResults)
		t.appendSearchStats(&result, stats)
		return result, nil
	}

	// Busca recursiva em diretório
	var allMatches []grepMatch
	filesScanned := 0
	truncated := false
	stats := &grepStats{}

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

		// Link apontando para fora do sandbox: não vazar conteúdo externo.
		// Vem antes do skill para a omissão ser silenciosa (AEP-0092 D-Q7) e
		// para não avaliar allowlist de caminho que já será descartado.
		if walkEntryEscapesSandbox(path, d.Type(), t.workDir) {
			return nil
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
		if ToolPolicy().BlockSensitive && isSensitiveEntry(path, d.Type()) {
			return nil
		}

		// Aplica filtro de inclusão
		if includePattern != "" {
			matched := matchIncludePattern(d.Name(), includePattern)
			if !matched {
				return nil
			}
		}

		fileInfo, err := d.Info()
		if err != nil {
			return nil
		}

		// Busca neste arquivo
		remaining := maxResults - len(allMatches)
		if remaining <= 0 {
			truncated = true
			return filepath.SkipAll
		}
		// O teto limita o custo do walk, não apenas leituras bem-sucedidas:
		// binários e arquivos grandes também contam depois dos filtros.
		if stats.filesConsidered >= grepMaxFilesScanned {
			truncated = true
			return filepath.SkipAll
		}
		stats.filesConsidered++

		fileMatches, searched := t.searchPath(ctx, path, fileInfo, re, remaining, contextLines, mode, stats)
		if searched {
			filesScanned++
		}

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
		result := tools.ToolResult{
			Content:  msg,
			Metadata: map[string]any{"results": 0, "files_scanned": filesScanned},
		}
		t.appendSearchStats(&result, stats)
		return result, nil
	}

	result := t.formatResults(a.Pattern, basePath, allMatches, filesScanned, truncated, maxResults)
	t.appendSearchStats(&result, stats)
	return result, nil
}

// searchPath escolhe entre o texto original e a projeção Markdown. O bool indica
// se o arquivo efetivamente contou como escaneado.
func (t *GrepSearch) searchPath(
	ctx context.Context,
	filePath string,
	info os.FileInfo,
	re *regexp.Regexp,
	maxMatches, contextLines int,
	mode docextract.Mode,
	stats *grepStats,
) ([]grepMatch, bool) {
	prefix, err := readFilePrefix(filePath, docextract.DetectPrefixBytes)
	if err != nil {
		stats.warn(filePath, fmt.Sprintf("não foi possível ler o prefixo: %v", err))
		return nil, true
	}
	kind := docextract.Detect(prefix, filePath)
	// Prefixo ZIP sozinho não basta: abrir todo .zip/.jar/.apk encontrado numa
	// árvore seria uma regressão de custo. Só inspecionamos o container completo
	// quando o nome finge ser texto, cenário de disfarce coberto por D10.
	zipCandidate := docextract.HasZipMagic(prefix) && isTextualDisguisePath(filePath)
	project := willProject(kind, mode) || zipCandidate

	if !project {
		// Conteúdo vence extensão: um arquivo chamado .pdf que na verdade é
		// texto UTF-8 segue o mesmo caminho do read_file (D4).
		if kind == docextract.KindUnsupportedBinary ||
			(isBinaryExtension(filePath) && kind != docextract.KindText) ||
			info.Size() > grepMaxFileSize {
			return nil, false
		}
		if docextract.IsWritableText(kind) && !docextract.IsLikelyText(prefix) {
			stats.warn(filePath, errGrepBinaryContent.Error())
			return nil, true
		}
		matches, err := t.searchFile(ctx, filePath, re, maxMatches, contextLines)
		if err != nil {
			stats.warn(filePath, err.Error())
			return nil, true
		}
		return matches, true
	}

	if info.Size() > docextract.MaxExtractBytes {
		stats.warn(filePath, docextract.ErrTooLargeToExtract(info.Size()).Error())
		return nil, true
	}
	identity := docextract.FileIdentityFromStat(info.Size(), info.ModTime().UnixNano())
	cacheKey := filePath + "\x00" + string(mode)
	result, origin, err := t.cache.GetOrLoad(ctx, cacheKey, identity, func(ctx context.Context) (*docextract.Result, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := ReadFileBytes(filePath)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := docextract.ExtractModeContext(ctx, data, filePath, mode)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		if ctx.Err() == nil {
			stats.warn(filePath, documentReadError(err))
		}
		return nil, true
	}
	if !result.Projected {
		// ZIP com magic mas sem formato documental reconhecido.
		stats.warn(filePath, docextract.ErrUnsupportedBinary().Error())
		return nil, true
	}
	stats.documentsProjected++
	switch origin {
	case docextract.OriginCached:
		stats.cacheHits++
	case docextract.OriginCoalesced:
		stats.documentsCoalesced++
	default:
		stats.documentsExtracted++
	}
	for _, warning := range result.Warnings {
		stats.warn(filePath, warning)
	}
	lines := strings.Split(result.Markdown, "\n")
	return searchLines(ctx, filePath, lines, re, maxMatches, contextLines, result.Kind), true
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := scanner.Text()
		if strings.IndexByte(line, 0) >= 0 || !utf8.ValidString(line) {
			return nil, errGrepBinaryContent
		}
		lines = append(lines, line)
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	return searchLines(ctx, filePath, lines, re, maxMatches, contextLines, ""), nil
}

func searchLines(
	ctx context.Context,
	filePath string,
	lines []string,
	re *regexp.Regexp,
	maxMatches, contextLines int,
	projection docextract.Kind,
) []grepMatch {
	var matches []grepMatch
	// Track quais linhas já foram incluídas para evitar duplicatas no contexto
	includedLines := make(map[int]bool)

	for lineIdx, line := range lines {
		select {
		case <-ctx.Done():
			return matches
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
						Projection: projection,
						IsMatch:    re.MatchString(lines[i]),
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
					Projection: projection,
					IsMatch:    true,
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
						Projection: projection,
						IsMatch:    re.MatchString(lines[i]),
					})
					includedLines[i] = true
				}
			}

			if len(matches) >= maxMatches {
				break
			}
		}
	}

	return matches
}

// formatResults formata os resultados de busca para exibição.
func (t *GrepSearch) formatResults(pattern, basePath string, matches []grepMatch, filesScanned int, truncated bool, maxResults int) tools.ToolResult {
	// Agrupa matches por arquivo
	type fileGroup struct {
		file       string
		lines      []string
		projection docextract.Kind
	}
	var groups []fileGroup
	groupMap := make(map[string]int)

	for _, m := range matches {
		idx, exists := groupMap[m.File]
		if !exists {
			idx = len(groups)
			groupMap[m.File] = idx
			groups = append(groups, fileGroup{file: m.File, projection: m.Projection})
		}
		groups[idx].lines = append(groups[idx].lines, m.LineText)
	}

	// Conta apenas linhas que casam com o padrão, não as de contexto.
	matchCount := 0
	for _, m := range matches {
		if m.IsMatch {
			matchCount++
		}
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Busca: '%s' em '%s'\n", pattern, basePath)
	_, _ = fmt.Fprintf(
		&sb,
		"%d correspondência(s) em %d arquivo(s) com correspondências (%d arquivos escaneados)\n",
		matchCount, len(groups), filesScanned,
	)
	if truncated {
		_, _ = fmt.Fprintf(&sb, "(TRUNCADO: limite de %d resultados atingido)\n", maxResults)
	}
	sb.WriteString("\n")

	for _, g := range groups {
		sb.WriteString(g.file)
		if g.projection != "" {
			_, _ = fmt.Fprintf(&sb, " (projeção Markdown: %s)", g.projection)
		}
		sb.WriteString("\n")
		for _, line := range g.lines {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	return tools.ToolResult{
		Content: sb.String(),
		Metadata: map[string]any{
			"matches":       matchCount,
			"files_matched": len(groups),
			"files_scanned": filesScanned,
			"truncated":     truncated,
		},
	}
}

func (s *grepStats) warn(file, reason string) {
	if len(s.warnings) >= grepMaxWarnings {
		s.warningsOmitted++
		return
	}
	s.warnings = append(s.warnings, grepWarning{File: file, Reason: reason})
}

func (t *GrepSearch) appendSearchStats(result *tools.ToolResult, stats *grepStats) {
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["documents_projected"] = stats.documentsProjected
	result.Metadata["documents_extracted"] = stats.documentsExtracted
	result.Metadata["documents_coalesced"] = stats.documentsCoalesced
	result.Metadata["document_cache_hits"] = stats.cacheHits
	result.Metadata["document_warnings"] = len(stats.warnings) + stats.warningsOmitted
	result.Metadata["files_considered"] = stats.filesConsidered
	if len(stats.warnings) == 0 && stats.warningsOmitted == 0 {
		return
	}

	var warnings strings.Builder
	warnings.WriteString("\nAvisos de documentos:\n")
	for _, warning := range stats.warnings {
		display := warning.File
		if rel, err := filepath.Rel(t.workDir, warning.File); err == nil {
			display = filepath.ToSlash(rel)
		}
		_, _ = fmt.Fprintf(&warnings, "- %s: %s\n", display, warning.Reason)
	}
	if stats.warningsOmitted > 0 {
		_, _ = fmt.Fprintf(&warnings, "- ... e mais %d aviso(s) omitido(s)\n", stats.warningsOmitted)
	}
	result.Content += warnings.String()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
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

func isTextualDisguisePath(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt", ".md", ".csv", ".rtf", ".html", ".htm", ".xml", ".json",
		".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".log", ".sql",
		".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".css", ".scss",
		".sh", ".ps1", ".java", ".c", ".h", ".cpp", ".rs", ".rb", ".php":
		return true
	default:
		return false
	}
}
