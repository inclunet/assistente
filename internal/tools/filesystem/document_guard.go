package filesystem

import (
	"fmt"
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

func documentReadError(err error) string {
	return fmt.Sprintf("%v", err)
}
