package filesystem

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"assistente/internal/docextract"
	"assistente/internal/tools"
)

// A partir deste tamanho, um recorte por linhas de arquivo de texto é lido em
// streaming, sem materializar o conteúdo inteiro (AEP-0093, D8).
const streamTextMinBytes = 4 << 20

// Linha maior que isto significa que o arquivo não é realmente "linhas"; servir
// o recorte exigiria carregar tudo em memória, então a leitura falha.
const maxStreamLineBytes = 16 << 20

// Tamanho do bloco lido por vez; também limita quanto uma única leitura aloca.
const streamBufferBytes = 64 << 10

var errStreamLineTooLong = errors.New("linha longa demais para leitura em streaming")

// scanTextLines percorre as linhas do arquivo com a mesma semântica de
// strings.Split(conteúdo, "\n"): arquivo terminado em nova linha tem uma última
// linha vazia, e o "\r" de CRLF é preservado. visit devolve false para parar.
func scanTextLines(ctx context.Context, fullPath string, visit func(idx int, line string) bool) error {
	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReaderSize(f, streamBufferBytes)
	for idx := 0; ; idx++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, atEOF, err := readStreamLine(r)
		if err != nil {
			return err
		}
		if atEOF {
			visit(idx, line)
			return nil
		}
		if !visit(idx, line) {
			return nil
		}
	}
}

// readStreamLine lê uma linha em blocos do tamanho do buffer, abortando assim que
// o acumulado passa do teto — assim uma "linha" gigante nunca é materializada.
func readStreamLine(r *bufio.Reader) (line string, atEOF bool, err error) {
	var b strings.Builder
	for {
		chunk, err := r.ReadSlice('\n')
		if b.Len()+len(chunk) > maxStreamLineBytes {
			return "", false, errStreamLineTooLong
		}
		b.Write(chunk)
		switch {
		case err == nil:
			return strings.TrimSuffix(b.String(), "\n"), false, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return b.String(), true, nil
		default:
			return "", false, err
		}
	}
}

// streamFailure decide o que fazer quando o streaming não conclui. Linha longa
// demais vira erro: cair no caminho que lê tudo reintroduziria justamente o pico
// de memória que o streaming evita. Outras falhas devolvem o controle ao
// chamador, que ainda pode reportar o erro de leitura como antes.
func streamFailure(err error, size int64, raw bool, budget int) (tools.ToolResult, bool) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return tools.ToolResult{Content: "Leitura cancelada pelo usuário", IsError: true}, true
	}
	if errors.Is(err, errStreamLineTooLong) {
		if raw {
			return rawReadLimitExceeded(budget), true
		}
		return tools.ToolResult{
			Content: fmt.Sprintf(
				"arquivo de %d bytes tem linha maior que %d bytes; recorte por linhas não é possível sem carregar tudo em memória",
				size, maxStreamLineBytes,
			),
			IsError: true,
		}, true
	}
	return tools.ToolResult{}, false
}

func readTextSliceStreamingForward(
	ctx context.Context,
	fullPath, displayPath string,
	size int64,
	offsetArg, limitArg *int,
	raw bool,
	budget int,
) (tools.ToolResult, bool) {
	offset := 0
	if offsetArg != nil && *offsetArg > 0 {
		offset = *offsetArg - 1
	}
	collectLimit := readModelMaxLines + 1
	if limitArg != nil && *limitArg > 0 && *limitArg < collectLimit {
		collectLimit = *limitArg
	}
	lines := make([]string, 0, collectLimit)
	collectedBytes := 0
	collectionTooLarge := false
	totalLines := 0
	binary := false
	invalidRawUTF8 := false
	err := scanTextLines(ctx, fullPath, func(idx int, line string) bool {
		totalLines++
		if strings.IndexByte(line, 0) >= 0 {
			binary = true
		}
		inRequestedRange := idx >= offset &&
			(limitArg == nil || *limitArg <= 0 || idx-offset < *limitArg)
		if raw && inRequestedRange && !utf8.ValidString(line) {
			invalidRawUTF8 = true
		}
		if idx >= offset && len(lines) < collectLimit && !collectionTooLarge {
			extra := len(line)
			if len(lines) > 0 {
				extra++
			}
			if !raw {
				extra += 7 // largura mínima do prefixo "%6d|"
			}
			if collectedBytes+extra > budget {
				collectionTooLarge = true
			} else {
				lines = append(lines, line)
				collectedBytes += extra
			}
		}
		return true
	})
	if err != nil {
		return streamFailure(err, size, raw, budget)
	}
	if binary {
		return tools.ToolResult{
			Content: fmt.Sprintf("%s tem conteúdo binário (byte NUL) apesar da extensão; não é lido como texto", displayPath),
			IsError: true,
		}, true
	}
	if invalidRawUTF8 {
		return rawReadInvalidUTF8(), true
	}
	if offset >= totalLines {
		shown := 0
		if offsetArg != nil {
			shown = *offsetArg
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Offset %d excede o número de linhas (%d)", shown, totalLines),
			IsError: true,
		}, true
	}
	requestedEnd := totalLines
	if limitArg != nil && *limitArg > 0 && *limitArg < totalLines-offset {
		requestedEnd = offset + *limitArg
	}
	if raw {
		if requestedEnd-offset > readModelMaxLines {
			return rawReadTooManyLines(requestedEnd-offset, readModelMaxLines), true
		}
		if collectionTooLarge || len(lines) < requestedEnd-offset {
			return rawReadLimitExceeded(budget), true
		}
		exact := strings.Join(lines[:requestedEnd-offset], "\n")
		if requestedEnd < totalLines {
			exact += "\n"
		}
		if len(exact) > budget {
			return rawReadTooLarge(len(exact), budget), true
		}
		return tools.ToolResult{
			Content: exact, RawExact: true,
			Metadata: map[string]any{
				"size_bytes": size, "total_lines": totalLines,
				"offset": offset + 1, "limit": requestedEnd - offset,
			},
		}, true
	}

	maxEnd := requestedEnd
	if maxEnd-offset > readModelMaxLines {
		maxEnd = offset + readModelMaxLines
	}
	selected := make([]string, 0, maxEnd-offset)
	selectedBytes := 0
	end := offset
	available := min(maxEnd-offset, len(lines))
	for i, line := range lines[:available] {
		lineNumber := offset + i + 1
		formatted := fmt.Sprintf("%6d|%s", lineNumber, line)
		candidateBytes := selectedBytes + len(formatted)
		if len(selected) > 0 {
			candidateBytes++
		}
		candidateEnd := lineNumber
		if readModelFacingSize(displayPath, candidateBytes, offset, candidateEnd, totalLines, nil) > budget {
			break
		}
		if !utf8.ValidString(line) {
			return textReadInvalidUTF8(), true
		}
		selected = append(selected, formatted)
		selectedBytes = candidateBytes
		end = candidateEnd
	}
	if end == offset {
		return tools.ToolResult{
			Content: fmt.Sprintf("A linha %d não cabe no limite model-facing de %d bytes; solicite um trecho textual menor.", offset+1, budget),
			IsError: true,
			Failure: &tools.ToolFailure{Code: "read_line_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
		}, true
	}
	return tools.ToolResult{
		Content: formattedReadContent(displayPath, selected, offset, end, totalLines),
		Metadata: map[string]any{
			"size_bytes": size, "total_lines": totalLines,
			"offset": offset + 1, "limit": end - offset,
		},
		Annotations: &tools.ResultAnnotations{OutputWindow: readOutputWindow(offset, end, totalLines)},
	}, true
}

// readTextSliceStreaming devolve o recorte pedido de um arquivo de texto grande
// sem carregar tudo em memória. handled=false significa que o chamador deve
// seguir pelo caminho normal.
func readTextSliceStreaming(ctx context.Context, fullPath, displayPath string, size int64, offsetArg, limitArg *int, mode docextract.Mode, raw bool) (result tools.ToolResult, handled bool) {
	if size < streamTextMinBytes {
		return tools.ToolResult{}, false
	}
	prefix, err := readFilePrefix(fullPath, docextract.DetectPrefixBytes)
	if err != nil {
		return tools.ToolResult{}, false
	}
	// Só serve o recorte em streaming quem sai como texto: quando há projeção, as
	// linhas são as do Markdown derivado, que só existe depois de extrair tudo.
	kind := docextract.Detect(prefix, displayPath)
	if willProject(kind, mode) || !docextract.IsWritableText(kind) {
		return tools.ToolResult{}, false
	}

	budget := readModelMaxBytes
	if executorLimit := tools.MaxResultSizeFromContext(ctx); executorLimit > 0 && executorLimit < budget {
		budget = executorLimit
	}
	if offsetArg == nil || *offsetArg >= 0 {
		return readTextSliceStreamingForward(ctx, fullPath, displayPath, size, offsetArg, limitArg, raw, budget)
	}

	// A classificação viu só o prefixo, mas quem lê o arquivo pequeno inteiro
	// recusa o conteúdo ao encontrar um byte NUL em qualquer posição. A primeira
	// passada já percorre tudo para contar linhas, então aplicar a mesma regra
	// aqui custa pouco e evita que o mesmo arquivo passe por ser grande.
	totalLines := 0
	if err := scanTextLines(ctx, fullPath, func(_ int, line string) bool {
		if strings.IndexByte(line, 0) >= 0 {
			totalLines = -1
			return false
		}
		totalLines++
		return true
	}); err != nil {
		return streamFailure(err, size, raw, budget)
	}
	if totalLines < 0 {
		return tools.ToolResult{
			Content: fmt.Sprintf("%s tem conteúdo binário (byte NUL) apesar da extensão; não é lido como texto", displayPath),
			IsError: true,
		}, true
	}

	offset := 0
	if offsetArg != nil {
		offset = *offsetArg
	}
	if offset < 0 {
		offset = totalLines + offset
		if offset < 0 {
			offset = 0
		}
	} else if offset > 0 {
		offset--
	}
	if offset >= totalLines {
		return tools.ToolResult{
			Content: fmt.Sprintf("Offset %d excede o número de linhas (%d)", *offsetArg, totalLines),
			IsError: true,
		}, true
	}

	requestedEnd := totalLines
	if limitArg != nil && *limitArg > 0 && *limitArg < totalLines-offset {
		requestedEnd = offset + *limitArg
	}
	if raw && requestedEnd-offset > readModelMaxLines {
		return rawReadTooManyLines(requestedEnd-offset, readModelMaxLines), true
	}

	selected := make([]string, 0, min(requestedEnd-offset, readModelMaxLines))
	selectedBytes := 0
	end := offset
	tooLargeRaw := false
	invalidRawUTF8 := false
	invalidTextUTF8 := false
	if err := scanTextLines(ctx, fullPath, func(idx int, line string) bool {
		if idx < offset {
			return true
		}
		if idx >= requestedEnd {
			return false
		}
		if raw {
			if !utf8.ValidString(line) {
				invalidRawUTF8 = true
				return false
			}
			extra := len(line)
			if len(selected) > 0 {
				extra++
			}
			if selectedBytes+extra > budget {
				tooLargeRaw = true
				return false
			}
			selected = append(selected, line)
			selectedBytes += extra
			end = idx + 1
			return true
		}
		if len(selected) >= readModelMaxLines {
			return false
		}
		formatted := fmt.Sprintf("%6d|%s", idx+1, line)
		candidateBytes := selectedBytes + len(formatted)
		if len(selected) > 0 {
			candidateBytes++
		}
		candidateEnd := idx + 1
		if readModelFacingSize(displayPath, candidateBytes, offset, candidateEnd, totalLines, nil) > budget {
			return false
		}
		if !utf8.ValidString(line) {
			invalidTextUTF8 = true
			return false
		}
		selected = append(selected, formatted)
		selectedBytes = candidateBytes
		end = candidateEnd
		return true
	}); err != nil {
		return streamFailure(err, size, raw, budget)
	}
	if raw {
		if invalidRawUTF8 {
			return rawReadInvalidUTF8(), true
		}
		if tooLargeRaw || end < requestedEnd {
			return rawReadLimitExceeded(budget), true
		}
		exact := strings.Join(selected, "\n")
		if requestedEnd < totalLines {
			exact += "\n"
		}
		if len(exact) > budget {
			return rawReadTooLarge(len(exact), budget), true
		}
		return tools.ToolResult{
			Content:  exact,
			RawExact: true,
			Metadata: map[string]any{"size_bytes": size, "total_lines": totalLines, "offset": offset + 1, "limit": end - offset},
		}, true
	}
	if invalidTextUTF8 {
		return textReadInvalidUTF8(), true
	}
	if end == offset {
		return tools.ToolResult{
			Content: fmt.Sprintf("A linha %d não cabe no limite model-facing de %d bytes; solicite um trecho textual menor.", offset+1, budget),
			IsError: true,
			Failure: &tools.ToolFailure{Code: "read_line_too_large", Kind: tools.ErrorKindUnknown, Retryable: false},
		}, true
	}

	window := readOutputWindow(offset, end, totalLines)
	return tools.ToolResult{
		Content: formattedReadContent(displayPath, selected, offset, end, totalLines),
		Metadata: map[string]any{
			"size_bytes":  size,
			"total_lines": totalLines,
			"offset":      offset + 1,
			"limit":       end - offset,
		},
		Annotations: &tools.ResultAnnotations{OutputWindow: window},
	}, true
}
