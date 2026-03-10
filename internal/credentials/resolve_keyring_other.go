//go:build !windows

package credentials

import "fmt"

// resolveKeyringDirect não é suportado fora do Windows.
// Em outras plataformas, usar o formato keyring://service/user com go-keyring.
func resolveKeyringDirect(target string) (string, error) {
	return "", fmt.Errorf("lookup direto por target (%q) não suportado nesta plataforma; use o formato keyring://service/user", target)
}
