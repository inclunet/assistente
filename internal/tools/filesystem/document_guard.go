package filesystem

import (
	"fmt"

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
func rejectExistingDocument(fullPath, displayPath string) (content string, rejected bool) {
	data, err := ReadFileBytes(fullPath)
	if err != nil {
		// Arquivo novo — quem chama decide sobre o conteúdo a gravar
		return "", false
	}
	if msg, ok := rejectDocumentWrite(data, displayPath); ok {
		return msg, true
	}
	return "", false
}

func documentReadError(err error) string {
	return fmt.Sprintf("%v", err)
}
