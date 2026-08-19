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

// rejectExistingDocument classifica o arquivo existente (se houver) e rejeita
// escrita em documento. Lê só o prefixo: o tipo é decidido pelos primeiros bytes
// mais a extensão, então não há motivo para carregar um binário inteiro apenas
// para recusá-lo. Se o arquivo existe mas não pode ser lido, falha fechado
// (AEP-0093).
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
// acima do teto de extração antes de carregar tudo em memória. Texto/código não
// tem teto: segue o comportamento anterior ao AEP-0093.
func rejectOversizedDocument(fullPath, displayPath string, size int64) (content string, rejected bool) {
	if size <= docextract.MaxExtractBytes {
		return "", false
	}
	prefix, err := readFilePrefix(fullPath, docextract.DetectPrefixBytes)
	if err != nil {
		return fmt.Sprintf("não foi possível classificar o arquivo antes de ler: %v", err), true
	}
	if docextract.Detect(prefix, displayPath) == docextract.KindText {
		return "", false
	}
	return docextract.ErrTooLargeToExtract(size).Error(), true
}

func documentReadError(err error) string {
	return fmt.Sprintf("%v", err)
}
