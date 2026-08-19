package filesystem

import (
	"errors"
	"fmt"
	"io"
	"os"

	"assistente/internal/docextract"
)

// rejectDocumentWrite retorna ToolResult de erro se data não for texto gravável (AEP-0093).
func rejectDocumentWrite(data []byte, pathForDetect string) (content string, rejected bool) {
	if err := docextract.CheckWritable(data, pathForDetect); err != nil {
		return err.Error(), true
	}
	return "", false
}

// rejectDocumentWriteString é a mesma verificação para conteúdo que já está em
// string, sem copiar tudo para []byte só para classificar.
func rejectDocumentWriteString(content, pathForDetect string) (msg string, rejected bool) {
	if err := docextract.CheckWritableString(content, pathForDetect); err != nil {
		return err.Error(), true
	}
	return "", false
}

// rejectExistingDocument classifica o arquivo existente (se houver) e rejeita
// escrita em documento. Lê só o prefixo, que cobre os magic bytes e a heurística
// de texto; a extensão completa a classificação quando o magic não basta (é o
// caso dos containers ZIP, cuja estrutura interna só apareceria lendo o arquivo
// todo). Um ZIP com extensão que não denuncia o formato cai em
// KindUnsupportedBinary, que também não é gravável — a recusa se mantém. Quando
// o arquivo existe mas não pode ser lido, a escrita é recusada em vez de seguir
// adiante (fail closed, AEP-0093).
func rejectExistingDocument(fullPath, displayPath string) (content string, rejected bool) {
	prefix, err := readFilePrefix(fullPath, docextract.DetectPrefixBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		return fmt.Sprintf("não foi possível classificar o arquivo existente antes de escrever: %v", err), true
	}
	if msg, ok := rejectDocumentWrite(prefix, displayPath); ok {
		return msg, true
	}
	return "", false
}

// readFilePrefix devolve até n bytes do início do arquivo.
func readFilePrefix(fullPath string, n int) ([]byte, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:read], nil
}

// rejectOversizedDocument classifica pelo prefixo do arquivo e recusa documentos
// acima do teto de extração antes de carregar tudo em memória. O teto é da
// extração, não da leitura: arquivo que o modo devolve como texto (código, CSV,
// RTF em ModeAuto) passa direto, como antes do AEP-0093.
func rejectOversizedDocument(fullPath, displayPath string, size int64, mode docextract.Mode) (content string, rejected bool) {
	if size <= docextract.MaxExtractBytes {
		return "", false
	}
	prefix, err := readFilePrefix(fullPath, docextract.DetectPrefixBytes)
	if err != nil {
		return fmt.Sprintf("não foi possível classificar o arquivo antes de ler: %v", err), true
	}
	kind := docextract.Detect(prefix, displayPath)
	if kind == docextract.KindUnsupportedBinary {
		// O tamanho não é o motivo: esse formato não tem leitura convertida.
		return docextract.ErrUnsupportedBinary().Error(), true
	}
	if !willProject(kind, mode) {
		return "", false
	}
	return docextract.ErrTooLargeToExtract(size).Error(), true
}

// willProject diz se o conteúdo desse kind será convertido para Markdown no modo
// pedido — ou seja, se precisará ser extraído inteiro (D12).
func willProject(kind docextract.Kind, mode docextract.Mode) bool {
	if docextract.IsOpaqueDocument(kind) {
		return true
	}
	return mode == docextract.ModeMarkdown && docextract.IsDocument(kind)
}

func documentReadError(err error) string {
	return fmt.Sprintf("%v", err)
}
