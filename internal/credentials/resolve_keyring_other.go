//go:build !windows

package credentials

import "fmt"

// keyringDirectSupported diz se esta plataforma tem lookup por TargetName.
const keyringDirectSupported = false

// lookupKeyringTarget não é suportado fora do Windows.
// Em outras plataformas, usar o formato keyring://service/user com go-keyring.
func lookupKeyringTarget(target string) (secret string, found bool, err error) {
	return "", false, fmt.Errorf("erro ao buscar keyring://%s: lookup direto por target não suportado nesta plataforma; use o formato keyring://service/user", target)
}
