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

// rejectExistingDocument lê o arquivo existente (se houver) e rejeita escrita em documento.
// Se o arquivo existe mas não pode ser lido, falha fechado (AEP-0093).
func rejectExistingDocument(fullPath, displayPath string) (content string, rejected bool) {
	data, err := ReadFileBytes(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		return fmt.Sprintf("não foi possível classificar o arquivo existente antes de escrever: %v", err), true
	}
	if msg, ok := rejectDocumentWrite(data, displayPath); ok {
		return msg, true
	}
	return "", false
}

// rejectOversizedDocument classifica pelo prefixo do arquivo e recusa documentos
// acima do teto de extração antes de carregar tudo em memória. Texto/código passa,
// porque a leitura é paginada por linhas.
func rejectOversizedDocument(fullPath, displayPath string, size int64) (content string, rejected bool) {
	if size <= docextract.MaxExtractBytes {
		return "", false
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return fmt.Sprintf("não foi possível classificar o arquivo antes de ler: %v", err), true
	}
	defer func() { _ = f.Close() }()

	prefix := make([]byte, docextract.DetectPrefixBytes)
	n, err := io.ReadFull(f, prefix)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return fmt.Sprintf("não foi possível classificar o arquivo antes de ler: %v", err), true
	}
	if docextract.Detect(prefix[:n], displayPath) == docextract.KindText {
		return "", false
	}
	return docextract.ErrTooLargeToExtract(size).Error(), true
}

func documentReadError(err error) string {
	return fmt.Sprintf("%v", err)
}
