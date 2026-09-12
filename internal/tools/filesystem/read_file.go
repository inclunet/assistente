package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/docextract"
	"assistente/internal/tools"
)

// ReadFile lê o conteúdo de um arquivo no disco.
// Suporta offset e limit para ler arquivos grandes parcialmente.
// Documentos V1 (AEP-0093) são projetados para Markdown.
type ReadFile struct {
	// workDir é o diretório base para caminhos relativos
	workDir string
	cache   *docextract.ProjectionCache
}

// NewReadFile cria uma nova instância de ReadFile.
// workDir define o diretório base para resolução de caminhos relativos.
func NewReadFile(workDir string, caches ...*docextract.ProjectionCache) *ReadFile {
	var cache *docextract.ProjectionCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	if cache == nil {
		cache = docextract.NewProjectionCache(docextract.DefaultCacheConfig())
	}
	return &ReadFile{workDir: workDir, cache: cache}
}

func (t *ReadFile) Name() string { return "read_file" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *ReadFile) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"}
}

func (t *ReadFile) Description() string {
	return "Read one known file in resumable windows of at most 2,000 lines and 50 KiB, whichever comes first. Use it when the path is already known. Do not use it to discover paths (use search_files), search across contents (use grep_search), or inspect a directory (use list_directory). The output_window annotation reports total lines, returned interval, has_more and next_offset. Set raw=true only for exact text without header, line numbers or envelope: oversized raw slices fail and must be retried with smaller offset/limit. Opaque documents are projected to Markdown (32 MiB input limit, no OCR). Risk: read-only."
}

func (t *ReadFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path or path relative to the working directory of the single file to read; use list_directory or search_files first if the path is unknown."
			},
			"offset": {
				"type": "integer",
				"description": "First line to return: positive values are 1-indexed; negative values count backward from the end."
			},
			"limit": {
				"type": "integer",
				"description": "Maximum number of lines requested from offset. Model-facing output is still capped at 2,000 lines and 50 KiB."
			},
			"raw": {
				"type": "boolean",
				"description": "Return only the exact textual slice, without envelope, header or line numbers. If the complete requested slice exceeds 50 KiB or the executor limit, the call fails; use a smaller offset/limit."
			},
			"document_mode": {
				"type": "string",
				"enum": ["auto", "markdown"],
				"description": "Projection mode. auto (default) returns text verbatim and projects only opaque supported documents to Markdown; markdown also projects supported textual formats, such as CSV to a table. OCR is unavailable."
			}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

// readFileArgs são os argumentos parseados de read_file
type readFileArgs struct {
	Path         string `json:"path"`
	Offset       *int   `json:"offset,omitempty"`
	Limit        *int   `json:"limit,omitempty"`
	DocumentMode string `json:"document_mode,omitempty"`
	Raw          bool   `json:"raw,omitempty"`
}

// parseDocumentMode valida o modo pedido. Modo desconhecido é erro em vez de
// virar auto: silenciar o engano faria o chamador achar que pediu conversão e
// receber o texto cru sem aviso.
//
// "ocr" fica fora do enum do schema: o modo não existe neste recorte
// (AEP-0093, issue #565). Anunciar um valor que sempre falha só convidaria o
// modelo a escolhê-lo. O caso continua tratado aqui para quem não valida pelo
// schema receber a razão certa em vez de um "valor inválido" genérico.
func parseDocumentMode(raw string) (docextract.Mode, error) {
	switch raw {
	case "", string(docextract.ModeAuto):
		return docextract.ModeAuto, nil
	case string(docextract.ModeMarkdown):
		return docextract.ModeMarkdown, nil
	case "ocr":
		return "", fmt.Errorf("document_mode %q não está disponível (OCR adiado, AEP-0093, issue #565)", raw)
	default:
		return "", fmt.Errorf("document_mode inválido: %q (use \"auto\" ou \"markdown\")", raw)
	}
}

func (t *ReadFile) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Path == "" {
		return tools.ToolResult{Content: "Parâmetro 'path' é obrigatório", IsError: true}, nil
	}

	mode, err := parseDocumentMode(a.DocumentMode)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Resolve caminho
	fullPath, err := t.resolvePath(a.Path)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Valida segurança do caminho (toolcalling estrito)
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "read"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se existe e é um arquivo
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Content: fmt.Sprintf("Arquivo não encontrado: %s", a.Path), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar arquivo: %v", err), IsError: true}, nil
	}
	if info.IsDir() {
		return tools.ToolResult{Content: fmt.Sprintf("'%s' é um diretório, não um arquivo. Use list_directory.", a.Path), IsError: true}, nil
	}

	if msg, rejected := rejectOversizedDocument(fullPath, a.Path, info.Size(), mode); rejected {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}

	if res, handled := readTextSliceStreaming(ctx, fullPath, a.Path, info.Size(), a.Offset, a.Limit, mode, a.Raw); handled {
		return res, nil
	}

	if err := ctx.Err(); err != nil {
		return tools.ToolResult{Content: "Leitura cancelada pelo usuário", IsError: true}, nil
	}

	// Lê o arquivo
	data, err := ReadFileBytes(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler arquivo: %v", err), IsError: true}, nil
	}

	kind := docextract.Detect(data, a.Path)
	var extracted *docextract.Result
	origin := docextract.OriginLoaded
	if willProject(kind, mode) {
		identity := docextract.FileIdentityFromStat(info.Size(), info.ModTime().UnixNano())
		cacheKey := fullPath + "\x00" + string(mode)
		extracted, origin, err = t.cache.GetOrLoad(ctx, cacheKey, identity, func(ctx context.Context) (*docextract.Result, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			result, err := docextract.ExtractModeContext(ctx, data, a.Path, mode)
			if err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return result, nil
		})
	} else {
		extracted, err = docextract.ExtractMode(data, a.Path, mode)
	}
	if err != nil {
		if ctx.Err() != nil {
			return tools.ToolResult{Content: "Leitura cancelada pelo usuário", IsError: true}, nil
		}
		return tools.ToolResult{Content: documentReadError(err), IsError: true}, nil
	}
	// Source pertence à chamada, não à projeção cacheada: o mesmo arquivo pode
	// ser aberto por paths relativos diferentes.
	extracted.Source = a.Path

	var content string
	var meta map[string]any
	var annotations *tools.ResultAnnotations
	if extracted.Projected {
		content = extracted.Markdown
		meta = map[string]any{
			"projection": true,
			"format":     string(extracted.Kind),
			"size_bytes": int64(len(data)),
			// cache_hit é só a entrada já pronta; cache_origin distingue quem
			// extraiu de quem pegou carona em uma extração concorrente.
			"cache_hit":    origin == docextract.OriginCached,
			"cache_origin": origin.String(),
		}
		if extracted.Pages > 0 {
			meta["pages"] = extracted.Pages
		}
		annotations = &tools.ResultAnnotations{
			DocumentProjection: &tools.DocumentProjectionAnnotation{
				Source:   a.Path,
				Format:   string(extracted.Kind),
				ReadOnly: true,
				Pages:    extracted.Pages,
				Warnings: append([]string(nil), extracted.Warnings...),
			},
		}
	} else {
		content = extracted.Markdown
		meta = map[string]any{
			"size_bytes": int64(len(data)),
		}
	}

	return formatReadResult(ctx, a.Path, content, int64(len(data)), a.Offset, a.Limit, a.Raw, meta, annotations), nil
}

const (
	readModelMaxLines = 2000
	readModelMaxBytes = 50 * 1024
)

func formatReadResult(ctx context.Context, path, content string, size int64, offsetArg, limitArg *int, raw bool, meta map[string]any, annotations *tools.ResultAnnotations) tools.ToolResult {
	lines := strings.Split(content, "\n")
	total := len(lines)
	offset := normalizedLineOffset(offsetArg, total)
	if offset >= total {
		shown := 0
		if offsetArg != nil {
			shown = *offsetArg
		}
		return tools.ToolResult{Content: fmt.Sprintf("Offset %d excede o número de linhas (%d)", shown, total), IsError: true}
	}
	requestedEnd := total
	if limitArg != nil && *limitArg > 0 && *limitArg < total-offset {
		requestedEnd = offset + *limitArg
	}
	budget := readModelMaxBytes
	if executorLimit := tools.MaxResultSizeFromContext(ctx); executorLimit > 0 && executorLimit < budget {
		budget = executorLimit
	}
	if raw {
		if requestedEnd-offset > readModelMaxLines {
			return rawReadTooManyLines(requestedEnd-offset, readModelMaxLines)
		}
		exact := strings.Join(lines[offset:requestedEnd], "\n")
		if requestedEnd < total {
			exact += "\n"
		}
		if len(exact) > budget {
			return rawReadTooLarge(len(exact), budget)
		}
		meta["total_lines"] = total
		meta["offset"] = offset + 1
		meta["limit"] = requestedEnd - offset
		return tools.ToolResult{Content: exact, RawExact: true, Metadata: meta, Annotations: annotations}
	}

	maxEnd := requestedEnd
	if maxEnd-offset > readModelMaxLines {
		maxEnd = offset + readModelMaxLines
	}
	body := make([]string, 0, maxEnd-offset)
	end := offset
	for i := offset; i < maxEnd; i++ {
		line := fmt.Sprintf("%6d|%s", i+1, lines[i])
		body = append(body, line)
		candidateEnd := i + 1
		if readModelFacingSize(path, body, offset, candidateEnd, total, annotations) > budget {
			body = body[:len(body)-1]
			break
		}
		end = candidateEnd
	}
	if end == offset {
		return tools.ToolResult{
			Content: fmt.Sprintf("A linha %d não cabe no limite model-facing de %d bytes; solicite um trecho textual menor.", offset+1, budget),
			IsError: true,
			Failure: &tools.ToolFailure{Code: "read_line_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
		}
	}
	window := readOutputWindow(offset, end, total)
	if annotations == nil {
		annotations = &tools.ResultAnnotations{}
	}
	annotations.OutputWindow = window
	meta["size_bytes"] = size
	meta["total_lines"] = total
	meta["offset"] = offset + 1
	meta["limit"] = end - offset
	return tools.ToolResult{
		Content:  formattedReadContent(path, body, offset, end, total),
		Metadata: meta, Annotations: annotations,
	}
}

func formattedReadContent(path string, body []string, offset, end, total int) string {
	return fmt.Sprintf("Arquivo: %s (linhas %d-%d de %d)\n%s", path, offset+1, end, total, strings.Join(body, "\n"))
}

func readOutputWindow(offset, end, total int) *tools.OutputWindowAnnotation {
	hasMore := end < total
	window := &tools.OutputWindowAnnotation{
		HasMore: hasMore, Unit: "lines", Offset: offset + 1,
		Returned: end - offset, Total: total,
	}
	if hasMore {
		window.NextOffset = end + 1
	}
	return window
}

func readModelFacingSize(path string, body []string, offset, end, total int, annotations *tools.ResultAnnotations) int {
	candidateAnnotations := &tools.ResultAnnotations{}
	if annotations != nil {
		candidateAnnotations.DocumentProjection = annotations.DocumentProjection
	}
	candidateAnnotations.OutputWindow = readOutputWindow(offset, end, total)
	return len(tools.ContentForModel(tools.ToolResult{
		Content:     formattedReadContent(path, body, offset, end, total),
		Annotations: candidateAnnotations,
	}))
}

func normalizedLineOffset(offsetArg *int, total int) int {
	if offsetArg == nil {
		return 0
	}
	offset := *offsetArg
	if offset < 0 {
		offset += total
		if offset < 0 {
			return 0
		}
		return offset
	}
	if offset > 0 {
		return offset - 1
	}
	return 0
}

func rawReadTooLarge(size, limit int) tools.ToolResult {
	return tools.ToolResult{
		Content: fmt.Sprintf("Trecho raw solicitado tem %d bytes, acima do limite de %d; use offset/limit menor.", size, limit),
		IsError: true,
		Failure: &tools.ToolFailure{Code: "raw_result_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
	}
}

func rawReadLimitExceeded(limit int) tools.ToolResult {
	return tools.ToolResult{
		Content: fmt.Sprintf("Trecho raw solicitado excede o limite de %d bytes; use offset/limit menor.", limit),
		IsError: true,
		Failure: &tools.ToolFailure{Code: "raw_result_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
	}
}

func rawReadTooManyLines(lines, limit int) tools.ToolResult {
	return tools.ToolResult{
		Content: fmt.Sprintf("Trecho raw solicitado tem %d linhas, acima do limite de %d; use offset/limit menor.", lines, limit),
		IsError: true,
		Failure: &tools.ToolFailure{Code: "raw_result_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
	}
}

// resolvePath converte caminho relativo para absoluto usando workDir
func (t *ReadFile) resolvePath(path string) (string, error) {
	return resolveFilePath(path, t.workDir)
}
