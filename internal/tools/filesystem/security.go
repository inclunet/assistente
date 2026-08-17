package filesystem

import (
	"fmt"
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

// isSensitiveFileResolved reporta se o path é sensível OU se, ao resolver
// symlinks, o alvo real é sensível. Fecha o bypass de um link com nome inócuo
// (ex.: innocent.txt) apontando para um arquivo sensível (.env, id_rsa). Se o
// path não existe ou não pode ser resolvido, considera só o basename literal —
// não relaxa a política por falha de resolução.
func isSensitiveFileResolved(path string) bool {
	if isSensitiveFile(path) {
		return true
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	return isSensitiveFile(resolved)
}

// blockSensitiveForOperation devolve erro quando a operação sobre um arquivo
// sensível não é permitida (só relevante quando policy.BlockSensitive). É
// avaliada ANTES do PathAuthorizer: sensível é hard-deny e não deve gerar
// consentimento (AEP-0092 D-Q5).
func blockSensitiveForOperation(fullPath, operation string) error {
	if !isSensitiveFileResolved(fullPath) {
		return nil
	}
	switch operation {
	case "read", "copy_from":
		return fmt.Errorf("não é permitido ler arquivos sensíveis")
	case "write", "copy_to":
		return fmt.Errorf("não é permitido escrever em arquivos sensíveis")
	case "edit":
		return fmt.Errorf("não é permitido editar arquivos sensíveis")
	case "move_from", "move_to":
		return fmt.Errorf("não é permitido mover/renomear arquivos sensíveis")
	default:
		return fmt.Errorf("operação não permitida em arquivos sensíveis")
	}
}
