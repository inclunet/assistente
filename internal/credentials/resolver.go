package credentials

import (
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

// ResolveExternalRef verifica se value é uma referência externa (keyring:// ou env://)
// e resolve o valor real. Se não for referência, retorna o valor original inalterado.
func ResolveExternalRef(value string) (string, error) {
	switch {
	case strings.HasPrefix(value, "keyring://"):
		return resolveKeyringRef(value)
	case strings.HasPrefix(value, "env://"):
		return resolveEnvRef(value)
	default:
		return value, nil
	}
}

// IsExternalRef retorna true se o valor é uma referência externa.
func IsExternalRef(value string) bool {
	return strings.HasPrefix(value, "keyring://") || strings.HasPrefix(value, "env://")
}

func resolveKeyringRef(ref string) (string, error) {
	path := strings.TrimPrefix(ref, "keyring://")
	if path == "" {
		return "", fmt.Errorf("referência keyring vazia: %s", ref)
	}

	// Formato keyring://service/user → go-keyring (cross-platform)
	if idx := strings.LastIndex(path, "/"); idx > 0 && idx < len(path)-1 {
		service := path[:idx]
		user := path[idx+1:]
		secret, err := keyring.Get(service, user)
		if err == nil {
			return secret, nil
		}
	}

	// Formato keyring://TargetName → lookup direto (Windows: wincred)
	// Usado pelo autocomplete que lista TargetNames exatos do Credential Manager.
	// Em !windows, resolveKeyringDirect sempre retorna erro (build tag); por isso
	// não inspecionamos err aqui — o retorno já carrega a falha tipada.
	return resolveKeyringDirect(path)
}

func resolveEnvRef(ref string) (string, error) {
	name := strings.TrimPrefix(ref, "env://")
	if name == "" {
		return "", fmt.Errorf("referência env vazia: %s", ref)
	}

	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("variável de ambiente %q não definida ou vazia", name)
	}
	return value, nil
}
