package filesystem

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"assistente/internal/docextract"
	"assistente/internal/tools"
)

// A partir deste tamanho, um recorte por linhas de arquivo de texto é lido em
// streaming, sem materializar o conteúdo inteiro (AEP-0093, D8).
const streamTextMinBytes = 4 << 20

// Linha maior que isto significa que o arquivo não é realmente "linhas"; servir
// o recorte exigiria carregar tudo em memória, então a leitura falha.
const maxStreamLineBytes = 16 << 20

var errStreamLineTooLong = errors.New("linha longa demais para leitura em streaming")

// scanTextLines percorre as linhas do arquivo com a mesma semântica de
// strings.Split(conteúdo, "\n"): arquivo terminado em nova linha tem uma última
// linha vazia, e o "\r" de CRLF é preservado. visit devolve false para parar.
func scanTextLines(fullPath string, visit func(idx int, line string) bool) error {
	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReaderSize(f, 64<<10)
	for idx := 0; ; idx++ {
		s, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(s) > maxStreamLineBytes {
			return errStreamLineTooLong
		}
		if errors.Is(err, io.EOF) {
			visit(idx, s)
			return nil
		}
		if !visit(idx, strings.TrimSuffix(s, "\n")) {
			return nil
		}
	}
}

// streamFailure decide o que fazer quando o streaming não conclui. Linha longa
// demais vira erro: cair no caminho que lê tudo reintroduziria justamente o pico
// de memória que o streaming evita. Outras falhas devolvem o controle ao
// chamador, que ainda pode reportar o erro de leitura como antes.
func streamFailure(err error, size int64) (tools.ToolResult, bool) {
	if errors.Is(err, errStreamLineTooLong) {
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

// readTextSliceStreaming devolve o recorte pedido de um arquivo de texto grande
// sem carregar tudo em memória. handled=false significa que o chamador deve
// seguir pelo caminho normal.
func readTextSliceStreaming(fullPath, displayPath string, size int64, offsetArg, limitArg *int) (result tools.ToolResult, handled bool) {
	if size < streamTextMinBytes || (offsetArg == nil && limitArg == nil) {
		return tools.ToolResult{}, false
	}
	prefix, err := readFilePrefix(fullPath, docextract.DetectPrefixBytes)
	if err != nil || docextract.Detect(prefix, displayPath) != docextract.KindText {
		return tools.ToolResult{}, false
	}

	totalLines := 0
	if err := scanTextLines(fullPath, func(int, string) bool {
		totalLines++
		return true
	}); err != nil {
		return streamFailure(err, size)
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

	end := totalLines
	if limitArg != nil && *limitArg > 0 {
		end = offset + *limitArg
		if end > totalLines {
			end = totalLines
		}
	}

	numbered := make([]string, 0, end-offset)
	if err := scanTextLines(fullPath, func(idx int, line string) bool {
		if idx >= offset && idx < end {
			numbered = append(numbered, fmt.Sprintf("%6d|%s", idx+1, line))
		}
		return idx+1 < end
	}); err != nil {
		return streamFailure(err, size)
	}

	header := fmt.Sprintf("Arquivo: %s (linhas %d-%d de %d)\n", displayPath, offset+1, end, totalLines)
	return tools.ToolResult{
		Content: header + strings.Join(numbered, "\n"),
		Metadata: map[string]any{
			"size_bytes":  size,
			"total_lines": totalLines,
			"offset":      offset + 1,
			"limit":       end - offset,
		},
	}, true
}
