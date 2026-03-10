//go:build windows

package credentials

import (
	"fmt"

	"github.com/danieljoos/wincred"
)

// resolveKeyringDirect busca uma credencial diretamente pelo TargetName no Windows Credential Manager.
// Usado quando a ref keyring:// contém um TargetName exato (sem separador service/user).
func resolveKeyringDirect(target string) (string, error) {
	cred, err := wincred.GetGenericCredential(target)
	if err != nil {
		return "", fmt.Errorf("erro ao buscar credencial do Windows (target=%q): %w", target, err)
	}
	if cred == nil {
		return "", fmt.Errorf("credencial não encontrada no Windows Credential Manager: %s", target)
	}
	return string(cred.CredentialBlob), nil
}
