package filesystem

import (
	"fmt"
	"os"
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
// (ex.: innocent.txt) apontando para um arquivo sensível (.env, id_rsa).
//
// Falha fechado: se EvalSymlinks falha mas o path É um symlink existente, não
// conseguimos garantir que o alvo não é sensível, então tratamos como sensível
// (bloqueia). Path inexistente (ex.: write criando arquivo novo) ou não-symlink
// que não resolve → considera só o basename literal, sem relaxar a política.
func isSensitiveFileResolved(path string) bool {
	if isSensitiveFile(path) {
		return true
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if info, lerr := os.Lstat(path); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			return true
		}
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
	case "delete":
		return fmt.Errorf("não é permitido excluir arquivos sensíveis")
	default:
		return fmt.Errorf("operação não permitida em arquivos sensíveis")
	}
}
