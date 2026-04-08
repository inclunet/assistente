package credentials

import (
	"fmt"
	"sort"
	"strings"
)

// MaskIdentifier mascara um identificador deixando visíveis apenas os últimos 4 caracteres.
// Usado para exibição segura de tokens, chaves de API, etc.
func MaskIdentifier(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	visible := value[len(value)-4:]
	return strings.Repeat("*", len(value)-4) + visible
}

// MaskCredentialValue mascara um valor de credencial com bullets (••••) nos primeiros chars.
func MaskCredentialValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}

// SummarizeAuth retorna uma representação mascarada de uma AuthConfig para exibição.
// Nunca expõe valores sensíveis em texto claro.
func SummarizeAuth(auth *AuthConfig) string {
	if auth == nil {
		return ""
	}
	switch auth.Type {
	case "bearer", "oauth2", "secret":
		if IsExternalRef(auth.Token) {
			return auth.Token
		}
		return MaskCredentialValue(auth.Token)
	case "basic":
		if auth.Username == "" && auth.Password == "" {
			return ""
		}
		pwd := MaskCredentialValue(auth.Password)
		if IsExternalRef(auth.Password) {
			pwd = auth.Password
		}
		return fmt.Sprintf("%s:%s", auth.Username, pwd)
	case "custom":
		if len(auth.Headers) == 0 {
			return ""
		}
		keys := make([]string, 0, len(auth.Headers))
		for k := range auth.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		first := keys[0]
		val := MaskCredentialValue(auth.Headers[first])
		if IsExternalRef(auth.Headers[first]) {
			val = auth.Headers[first]
		}
		return fmt.Sprintf("%s: %s", first, val)
	default:
		return ""
	}
}

// ResolveSecretFromAuth extrai o valor secreto principal de uma AuthConfig.
// Retorna o primeiro campo não-vazio dentre Token, Password, primeiro Header.
func ResolveSecretFromAuth(auth *AuthConfig) string {
	if auth == nil {
		return ""
	}
	if auth.Token != "" {
		return auth.Token
	}
	if auth.Password != "" {
		return auth.Password
	}
	if len(auth.Headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(auth.Headers))
	for k := range auth.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return auth.Headers[keys[0]]
}
