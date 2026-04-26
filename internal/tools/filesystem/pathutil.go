package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"assistente/internal/configdir"
	"assistente/internal/tools"
)

// errOutsideAllowedDirs é retornado quando um caminho está fora dos diretórios permitidos.
// Distingue-se de erros de input inválido (workDir vazio, caminho malformado) para que
// a exceção de open editors só se aplique a esse caso específico.
var errOutsideAllowedDirs = errors.New("caminho fora dos diretórios permitidos")

// expandTilde expande ~ e ~/ no início de um caminho para o diretório home do usuário.
// No Windows, ~ não é expandido pelo sistema — esta função resolve isso de forma portável.
// Exemplos:
//
//	"~/docs/file.txt" → "C:\Users\user\docs\file.txt" (Windows)
//	"~/.assistente"   → "/home/user/.assistente" (Linux/macOS)
//	"/absolute/path"  → "/absolute/path" (inalterado)
//	"relative/path"   → "relative/path" (inalterado)
func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path // fallback: retorna sem expandir
		}
		if path == "~" {
			return home
		}
		// Remove o ~ e junta com o home dir
		return filepath.Join(home, path[2:])
	}
	return path
}

// resolveFilePath resolve um caminho de arquivo, expandindo ~ e caminhos relativos.
// Ordem de resolução:
//  1. Expande ~ para home directory
//  2. Se absoluto, retorna limpo
//  3. Se relativo, resolve em relação ao workDir
func resolveFilePath(path, workDir string) (string, error) {
	// Primeiro expande tilde
	expanded := expandTilde(path)

	// Se é absoluto, retorna limpo
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}

	// Relativo: resolve em relação ao workDir
	return filepath.Abs(filepath.Join(workDir, expanded))
}

// validatePath verifica se um caminho é seguro para acesso.
// Bloqueia path traversal e acesso a diretórios sensíveis.

func normalizeForComparison(p string) string {
	clean := filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func isWithinRoot(absPath string, absRoot string) bool {
	root := normalizeForComparison(absRoot)
	path := normalizeForComparison(absPath)

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func validatePath(fullPath, workDir string) error {
	wd := strings.TrimSpace(workDir)
	if wd == "" {
		return fmt.Errorf("workDir inválido")
	}

	absPath, err := filepath.Abs(filepath.Clean(fullPath))
	if err != nil {
		return fmt.Errorf("caminho inválido: %v", err)
	}
	absWorkDir, err := filepath.Abs(filepath.Clean(wd))
	if err != nil {
		return fmt.Errorf("workDir inválido: %v", err)
	}

	allowedRoots := []string{absWorkDir}
	if homeBase := strings.TrimSpace(configdir.GetHomeDir()); homeBase != "" {
		if absHomeBase, err := filepath.Abs(filepath.Clean(homeBase)); err == nil {
			if normalizeForComparison(absHomeBase) != normalizeForComparison(absWorkDir) {
				allowedRoots = append(allowedRoots, absHomeBase)
			}
		}
	}

	for _, root := range allowedRoots {
		if isWithinRoot(absPath, root) {
			return nil
		}
	}
	return errOutsideAllowedDirs
}

func validatePathWithPolicy(ctx context.Context, fullPath, workDir string, policy Policy, operation string) error {
	if err := validatePath(fullPath, workDir); err != nil {
		// Exceção de open editors: aplica-se APENAS quando o erro é "fora dos diretórios"
		// (não para workDir inválido, caminho malformado, etc.) e somente para read/write/edit/grep.
		if !errors.Is(err, errOutsideAllowedDirs) || !isOpenEditorAllowed(ctx, fullPath, operation) {
			return err
		}
	}
	if err := validateSkillFilesystemAllowlist(ctx, fullPath, workDir, operation); err != nil {
		return err
	}
	if policy.BlockSensitive && isSensitiveFile(fullPath) {
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
	return nil
}

// isOpenEditorAllowed verifica se o arquivo está aberto em uma aba de editor e se a operação é permitida.
// Leitura, escrita/edição e grep são permitidos; operações estruturais (list, search, move, delete) não.
// Rejeita diretórios para todas as operações — a exceção de open editors é apenas para arquivos exatos.
func isOpenEditorAllowed(ctx context.Context, fullPath, operation string) bool {
	switch operation {
	case "read", "write", "edit", "grep":
		if !tools.IsOpenEditorFile(ctx, fullPath) {
			return false
		}
		// Rejeita diretórios: exceção de open editors é apenas para arquivos exatos.
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			return false
		}
		return true
	default:
		return false
	}
}

func validateSkillFilesystemAllowlist(ctx context.Context, fullPath, workDir, operation string) error {
	ec, ok := tools.GetExecutionContext(ctx)
	if !ok || ec.Filesystem == nil {
		return nil
	}

	absPath, err := filepath.Abs(filepath.Clean(fullPath))
	if err != nil {
		return fmt.Errorf("caminho inválido: %v", err)
	}

	// Deny tem precedência.
	if matchesAnyFilesystemPattern(absPath, workDir, ec.Filesystem.Deny) {
		if ec.InvokedSkillSlug != "" {
			return fmt.Errorf("acesso ao filesystem bloqueado pela denylist do skill '%s'", ec.InvokedSkillSlug)
		}
		return fmt.Errorf("acesso ao filesystem bloqueado pela denylist do skill")
	}

	var allowed []string
	switch operation {
	case "read", "list", "search", "grep", "copy_from":
		allowed = ec.Filesystem.Read
	case "write", "edit", "copy_to", "move_from", "move_to", "delete", "mkdir":
		allowed = ec.Filesystem.Write
	default:
		// Se a operação não é conhecida, não aplica allowlist.
		return nil
	}

	if len(allowed) == 0 {
		if ec.InvokedSkillSlug != "" {
			return fmt.Errorf("skill '%s' não permite operação '%s' no filesystem", ec.InvokedSkillSlug, operation)
		}
		return fmt.Errorf("skill não permite operação '%s' no filesystem", operation)
	}

	if !matchesAnyFilesystemPattern(absPath, workDir, allowed) {
		if ec.InvokedSkillSlug != "" {
			return fmt.Errorf("caminho não permitido pelo skill '%s' para operação '%s'", ec.InvokedSkillSlug, operation)
		}
		return fmt.Errorf("caminho não permitido para operação '%s'", operation)
	}

	return nil
}

func matchesAnyFilesystemPattern(absPath string, workDir string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, p := range patterns {
		pat := strings.TrimSpace(p)
		if pat == "" {
			continue
		}
		if filesystemPatternMatches(absPath, workDir, pat) {
			return true
		}
	}
	return false
}

func filesystemPatternMatches(absPath string, workDir string, pattern string) bool {
	// Resolve o padrão como um path (expande ~ e caminhos relativos ao workDir)
	resolved, err := resolveFilePath(pattern, workDir)
	if err != nil {
		return false
	}
	resolved = filepath.Clean(resolved)

	// Suporte comum: /alguma/coisa/** → qualquer coisa abaixo do diretório
	sep := string(filepath.Separator)
	if strings.HasSuffix(resolved, sep+"**") {
		base := strings.TrimSuffix(resolved, sep+"**")
		if strings.TrimSpace(base) == "" {
			return false
		}
		return isWithinRoot(absPath, base)
	}

	// Match literal/glob de filepath.Match ("*" não cruza separadores)
	matched, _ := filepath.Match(normalizeForComparison(resolved), normalizeForComparison(absPath))
	if matched {
		return true
	}

	// Fallback: padrões com "**/" (ex: ".../**/foo.txt" ou "**/*.md")
	// Usa a lógica já existente de matchPath em modo slash-normalized.
	absSlash := filepath.ToSlash(normalizeForComparison(absPath))
	patSlash := filepath.ToSlash(normalizeForComparison(resolved))
	return matchPath(absSlash, patSlash)
}
