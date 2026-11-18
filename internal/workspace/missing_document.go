package workspace

import "os"

// Uma pasta acessível com documento ausente comprova remoção. Pasta/volume
// indisponível, acesso negado e YAML inválido não removem referências.
func workspaceDocumentWasRemoved(document, directory string) bool {
	if _, err := os.Stat(document); !os.IsNotExist(err) {
		return false
	}
	info, err := os.Stat(directory)
	return err == nil && info.IsDir()
}
