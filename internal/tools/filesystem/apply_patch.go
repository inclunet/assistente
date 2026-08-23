package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"assistente/internal/tools"
)

const (
	applyPatchMaxHunks       = 100
	applyPatchMaxPayloadSize = 5 * 1024 * 1024
)

// ApplyPatch aplica múltiplas substituições exatas sobre um único snapshot.
// O arquivo só é gravado depois que todos os hunks forem validados.
type ApplyPatch struct {
	workDir  string
	questMgr QuestionnaireRequester
	onWrite  FileWriteObserver
}

// ApplyPatchOption configura integrações opcionais da tool.
type ApplyPatchOption func(*ApplyPatch)

// WithApplyPatchWriteObserver registra um observador para a gravação da tool.
func WithApplyPatchWriteObserver(observer FileWriteObserver) ApplyPatchOption {
	return func(t *ApplyPatch) {
		t.onWrite = observer
	}
}

// NewApplyPatch cria a tool canônica de patch multi-hunk.
func NewApplyPatch(workDir string, questMgr QuestionnaireRequester, opts ...ApplyPatchOption) *ApplyPatch {
	t := &ApplyPatch{workDir: workDir, questMgr: questMgr}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

func (t *ApplyPatch) Name() string { return "apply_patch" }

func (t *ApplyPatch) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{
		Category: "filesystem",
		Class:    "edit_files",
		Package:  "coding_edit",
		Risk:     "write",
	}
}

func (t *ApplyPatch) Description() string {
	return "Atomically applies multiple exact replacements (hunks) to one existing text file. " +
		"Every old_string must be non-empty and unique in the same original snapshot; hunks must not overlap. " +
		"If any hunk fails, nothing is written and the structured result identifies that hunk. " +
		"Use write_file to create or fully replace a file, and edit_file for replace_all."
}

func (t *ApplyPatch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo de texto existente (absoluto ou relativo ao diretório de trabalho)"
			},
			"hunks": {
				"type": "array",
				"description": "Substituições exatas, todas resolvidas sobre o mesmo conteúdo original",
				"minItems": 1,
				"maxItems": 100,
				"items": {
					"type": "object",
					"properties": {
						"old_string": {
							"type": "string",
							"description": "Texto exato, não vazio e único no snapshot original"
						},
						"new_string": {
							"type": "string",
							"description": "Texto final que substituirá old_string; pode ser vazio para remover"
						}
					},
					"required": ["old_string", "new_string"],
					"additionalProperties": false
				}
			}
		},
		"required": ["path", "hunks"],
		"additionalProperties": false
	}`)
}

type applyPatchArgs struct {
	Path  string           `json:"path"`
	Hunks []applyPatchHunk `json:"hunks"`
}

type applyPatchHunk struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

type applyPatchError struct {
	Hunk           int    `json:"hunk"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Occurrences    int    `json:"occurrences,omitempty"`
	CandidateLines []int  `json:"candidate_lines,omitempty"`
	ConflictsWith  int    `json:"conflicts_with,omitempty"`
}

type applyPatchResponse struct {
	Status     string            `json:"status"`
	Applied    bool              `json:"applied"`
	Path       string            `json:"path,omitempty"`
	Hunks      int               `json:"hunks,omitempty"`
	LineDiff   int               `json:"line_diff,omitempty"`
	TotalLines int               `json:"total_lines,omitempty"`
	Errors     []applyPatchError `json:"errors,omitempty"`
}

type applyPatchSpan struct {
	hunk        int
	start       int
	end         int
	replacement string
}

func (t *ApplyPatch) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a applyPatchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return patchErrorResult("", applyPatchError{
			Code:    "invalid_arguments",
			Message: "Não foi possível interpretar os argumentos: " + err.Error(),
		}), nil
	}
	if strings.TrimSpace(a.Path) == "" {
		return patchErrorResult(a.Path, applyPatchError{
			Code:    "invalid_arguments",
			Message: "Parâmetro 'path' é obrigatório.",
		}), nil
	}
	if len(a.Hunks) == 0 || len(a.Hunks) > applyPatchMaxHunks {
		return patchErrorResult(a.Path, applyPatchError{
			Code: "invalid_arguments",
			Message: fmt.Sprintf(
				"Parâmetro 'hunks' deve conter entre 1 e %d itens.", applyPatchMaxHunks,
			),
		}), nil
	}

	payloadSize := 0
	for _, hunk := range a.Hunks {
		payloadSize += len(hunk.OldString) + len(hunk.NewString)
	}
	if payloadSize > applyPatchMaxPayloadSize {
		return patchErrorResult(a.Path, applyPatchError{
			Code: "payload_too_large",
			Message: fmt.Sprintf(
				"Conteúdo dos hunks tem %d bytes; o limite é %d bytes.",
				payloadSize, applyPatchMaxPayloadSize,
			),
		}), nil
	}

	fullPath, err := resolveFilePath(a.Path, t.workDir)
	if err != nil {
		return patchFileErrorResult(a.Path, "invalid_path", err), nil
	}
	resolvedPath, err := validatePathWithPolicyResolved(ctx, fullPath, t.workDir, ToolPolicy(), "edit")
	if err != nil {
		return patchFileErrorResult(a.Path, "path_denied", err), nil
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return patchErrorResult(a.Path, applyPatchError{
				Code:    "file_not_found",
				Message: fmt.Sprintf("Arquivo não encontrado: %s. Use write_file para criar arquivos.", a.Path),
			}), nil
		}
		return patchFileErrorResult(a.Path, "stat_failed", err), nil
	}
	if info.IsDir() {
		return patchErrorResult(a.Path, applyPatchError{
			Code:    "not_a_file",
			Message: fmt.Sprintf("%q é um diretório, não um arquivo.", a.Path),
		}), nil
	}
	if !info.Mode().IsRegular() {
		return patchErrorResult(a.Path, applyPatchError{
			Code:    "not_a_regular_file",
			Message: fmt.Sprintf("%q não é um arquivo regular de texto.", a.Path),
		}), nil
	}
	if msg, ok := rejectExistingDocument(resolvedPath, a.Path); ok {
		return patchErrorResult(a.Path, applyPatchError{
			Code:    "unsupported_document",
			Message: msg,
		}), nil
	}

	original, err := ReadFileBytes(resolvedPath)
	if err != nil {
		return patchFileErrorResult(a.Path, "read_failed", err), nil
	}
	normalized, format, err := normalizePatchText(original)
	if err != nil {
		return patchFileErrorResult(a.Path, "invalid_text", err), nil
	}

	finalNormalized, validationErrors := applyPatchHunks(normalized, a.Hunks)
	if len(validationErrors) > 0 {
		return patchErrorsResult(a.Path, validationErrors), nil
	}
	finalBytes := restorePatchText(finalNormalized, format)
	if msg, ok := rejectDocumentWriteString(string(finalBytes), a.Path); ok {
		return patchErrorResult(a.Path, applyPatchError{
			Code:    "unsupported_document",
			Message: msg,
		}), nil
	}

	if resolveEditPolicy(ctx, fullPath) == policyConfirmWithDiff {
		confirmed, toolResult := confirmBeforeAfter(
			ctx,
			t.questMgr,
			editConfirmTitle(),
			a.Path,
			truncateForPreview(string(original)),
			truncateForPreview(string(finalBytes)),
		)
		if !confirmed {
			return patchErrorResult(a.Path, applyPatchError{
				Code:    "confirmation_not_applied",
				Message: toolResult.Content,
			}), nil
		}
	}

	// A confirmação não segura o lock: autosave e outras escritas podem seguir
	// enquanto o usuário decide. Depois da resposta, o lock é adquirido e o
	// snapshot é conferido novamente no handle fixado antes de qualquer efeito.
	unlock := lockFileMutation(resolvedPath)
	defer unlock()

	// Abre o destino real autorizado, fixa o handle e compara identidade +
	// conteúdo antes de truncar. Uma troca posterior do path (inclusive por
	// symlink) não redireciona a gravação para outro arquivo.
	file, openErr := os.OpenFile(resolvedPath, os.O_RDWR, 0)
	if openErr != nil {
		return patchFileErrorResult(a.Path, "open_failed", openErr), nil
	}
	defer func() { _ = file.Close() }()
	currentInfo, statErr := file.Stat()
	if statErr != nil {
		return patchFileErrorResult(a.Path, "stat_failed", statErr), nil
	}
	if !os.SameFile(info, currentInfo) {
		return patchErrorResult(a.Path, applyPatchError{
			Code:    "stale_file",
			Message: "O arquivo foi substituído desde a leitura do snapshot. Leia a versão atual e refaça o patch.",
		}), nil
	}
	current, readErr := io.ReadAll(file)
	if readErr != nil {
		return patchFileErrorResult(a.Path, "read_failed", readErr), nil
	}
	if !bytes.Equal(current, original) {
		return patchErrorResult(a.Path, applyPatchError{
			Code:    "stale_file",
			Message: "O arquivo mudou desde a leitura do snapshot. Leia a versão atual e refaça o patch.",
		}), nil
	}

	var finishWrite func(bool)
	if t.onWrite != nil {
		finishWrite = t.onWrite(fullPath)
	}
	if err := rewriteOpenFile(file, finalBytes); err != nil {
		if finishWrite != nil {
			finishWrite(false)
		}
		return patchFileErrorResult(a.Path, "write_failed", err), nil
	}
	if finishWrite != nil {
		finishWrite(true)
	}

	lineDiff := lineCount(finalNormalized) - lineCount(normalized)
	response := applyPatchResponse{
		Status:     "ok",
		Applied:    true,
		Path:       a.Path,
		Hunks:      len(a.Hunks),
		LineDiff:   lineDiff,
		TotalLines: lineCount(finalNormalized),
	}
	return patchResult(response, false, map[string]any{
		"path":        a.Path,
		"hunks":       len(a.Hunks),
		"line_diff":   lineDiff,
		"total_lines": response.TotalLines,
	}), nil
}

type patchTextFormat struct {
	bom     bool
	newline string
}

func normalizePatchText(data []byte) (string, patchTextFormat, error) {
	format := patchTextFormat{newline: dominantNewline(data)}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		format.bom = true
		data = data[3:]
	}
	if !utf8.Valid(data) {
		return "", format, fmt.Errorf("o arquivo não contém texto UTF-8 válido")
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content, format, nil
}

func dominantNewline(data []byte) string {
	var crlf, lf, cr int
	for index := 0; index < len(data); index++ {
		switch data[index] {
		case '\r':
			if index+1 < len(data) && data[index+1] == '\n' {
				crlf++
				index++
			} else {
				cr++
			}
		case '\n':
			lf++
		}
	}
	switch {
	case crlf >= lf && crlf >= cr && crlf > 0:
		return "\r\n"
	case cr > lf && cr > 0:
		return "\r"
	default:
		return "\n"
	}
}

func restorePatchText(content string, format patchTextFormat) []byte {
	if format.newline != "\n" {
		content = strings.ReplaceAll(content, "\n", format.newline)
	}
	data := []byte(content)
	if format.bom {
		data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
	}
	return data
}

func rewriteOpenFile(file *os.File, content []byte) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	written, err := file.Write(content)
	if err != nil {
		return err
	}
	if written != len(content) {
		return io.ErrShortWrite
	}
	return file.Sync()
}

func applyPatchHunks(content string, hunks []applyPatchHunk) (string, []applyPatchError) {
	spans := make([]applyPatchSpan, 0, len(hunks))
	errs := make([]applyPatchError, 0)

	for index, hunk := range hunks {
		hunkNumber := index + 1
		oldString := normalizePatchHunkText(hunk.OldString)
		newString := normalizePatchHunkText(hunk.NewString)
		if oldString == "" {
			errs = append(errs, applyPatchError{
				Hunk:    hunkNumber,
				Code:    "invalid_hunk",
				Message: "old_string não pode ser vazio.",
			})
			continue
		}
		if oldString == newString {
			errs = append(errs, applyPatchError{
				Hunk:    hunkNumber,
				Code:    "invalid_hunk",
				Message: "old_string e new_string são idênticos.",
			})
			continue
		}

		count, candidateLines := findOverlappingOccurrences(content, oldString, 20)
		if count == 0 {
			errs = append(errs, applyPatchError{
				Hunk:    hunkNumber,
				Code:    "context_not_found",
				Message: "old_string não foi encontrada no snapshot original; releia o arquivo e inclua contexto exato.",
			})
			continue
		}
		if count > 1 {
			errs = append(errs, applyPatchError{
				Hunk:           hunkNumber,
				Code:           "ambiguous_context",
				Message:        fmt.Sprintf("old_string ocorre %d vezes; inclua mais contexto para torná-la única.", count),
				Occurrences:    count,
				CandidateLines: candidateLines,
			})
			continue
		}

		start := strings.Index(content, oldString)
		spans = append(spans, applyPatchSpan{
			hunk:        hunkNumber,
			start:       start,
			end:         start + len(oldString),
			replacement: newString,
		})
	}
	if len(errs) > 0 {
		return content, errs
	}

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].hunk < spans[j].hunk
		}
		return spans[i].start < spans[j].start
	})
	covering := spans[0]
	for index := 1; index < len(spans); index++ {
		current := spans[index]
		if current.start < covering.end {
			errs = append(errs, applyPatchError{
				Hunk:          current.hunk,
				Code:          "overlapping_hunks",
				Message:       fmt.Sprintf("O intervalo do hunk %d sobrepõe o hunk %d.", current.hunk, covering.hunk),
				ConflictsWith: covering.hunk,
			})
		}
		if current.end > covering.end {
			covering = current
		}
	}
	if len(errs) > 0 {
		return content, errs
	}

	for index := len(spans) - 1; index >= 0; index-- {
		span := spans[index]
		content = content[:span.start] + span.replacement + content[span.end:]
	}
	return content, nil
}

func normalizePatchHunkText(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.ReplaceAll(content, "\r", "\n")
}

func findOverlappingOccurrences(content, oldString string, lineLimit int) (int, []int) {
	lines := make([]int, 0, lineLimit)
	searchFrom := 0
	count := 0
	for searchFrom <= len(content)-len(oldString) {
		index := strings.Index(content[searchFrom:], oldString)
		if index < 0 {
			break
		}
		absolute := searchFrom + index
		count++
		if len(lines) < lineLimit {
			lines = append(lines, strings.Count(content[:absolute], "\n")+1)
		}
		searchFrom = absolute + 1
	}
	return count, lines
}

func lineCount(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func patchFileErrorResult(path, code string, err error) tools.ToolResult {
	return patchErrorResult(path, applyPatchError{
		Code:    code,
		Message: err.Error(),
	})
}

func patchErrorResult(path string, patchErr applyPatchError) tools.ToolResult {
	return patchErrorsResult(path, []applyPatchError{patchErr})
}

func patchErrorsResult(path string, errs []applyPatchError) tools.ToolResult {
	return patchResult(applyPatchResponse{
		Status:  "error",
		Applied: false,
		Path:    path,
		Errors:  errs,
	}, true, map[string]any{
		"path":        path,
		"error_count": len(errs),
	})
}

func patchResult(response applyPatchResponse, isError bool, metadata map[string]any) tools.ToolResult {
	encoded, err := json.Marshal(response)
	if err != nil {
		return tools.ToolResult{
			Content: "Falha interna ao serializar o resultado de apply_patch: " + err.Error(),
			IsError: true,
		}
	}
	return tools.ToolResult{
		Content:    string(encoded),
		IsError:    isError,
		Metadata:   metadata,
		Structured: true,
	}
}
