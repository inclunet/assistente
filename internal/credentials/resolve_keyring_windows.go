//go:build windows

package credentials

import (
	"fmt"

	"github.com/danieljoos/wincred"
)

// keyringDirectSupported diz se esta plataforma tem lookup por TargetName.
const keyringDirectSupported = true

// lookupKeyringTarget busca uma credencial diretamente pelo TargetName no Windows Credential Manager.
// Usado quando a ref keyring:// contém um TargetName exato (sem separador service/user).
func lookupKeyringTarget(target string) (secret string, found bool, err error) {
	cred, err := wincred.GetGenericCredential(target)
	if err != nil {
		return "", false, fmt.Errorf("erro ao buscar keyring://%s: credencial do Windows (target=%q): %w", target, target, err)
	}
	if cred == nil {
		return "", false, fmt.Errorf("erro ao buscar keyring://%s: credencial não encontrada no Windows Credential Manager", target)
	}
	return string(cred.CredentialBlob), true, nil
}
