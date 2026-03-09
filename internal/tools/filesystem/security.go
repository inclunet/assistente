package filesystem

import (
	"path/filepath"
	"strings"
)

// Policy define diferenças explícitas entre operações do editor (ação explícita do usuário)
// e operações por toolcalling (mais restritas).
type Policy struct {
	BlockSensitive bool
}

func ToolPolicy() Policy { return Policy{BlockSensitive: true} }

func EditorPolicy() Policy { return Policy{BlockSensitive: false} }

// isSensitiveFile verifica se o caminho é um arquivo sensível que não deve ser modificado.
// Usado para impor restrições adicionais em operações por toolcalling.
func isSensitiveFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))

	// Arquivos de ambiente com secrets
	sensitiveNames := map[string]bool{
		".env":            true,
		".env.local":      true,
		".env.prod":       true,
		".env.production": true,
		"id_rsa":          true,
		"id_ed25519":      true,
		"known_hosts":     true,
		"authorized_keys": true,
	}

	if sensitiveNames[name] {
		return true
	}

	// Extensões de certificados e chaves
	ext := strings.ToLower(filepath.Ext(path))
	sensitiveExts := map[string]bool{
		".pem": true,
		".key": true,
		".crt": true,
		".cer": true,
		".p12": true,
		".pfx": true,
	}

	return sensitiveExts[ext]
}
