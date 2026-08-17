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

// errOutsideAllowedDirs é retornado quando um caminho está fora dos diretórios permitidos
// (workspace ativo / workDir da tool e ~/.assistente) e ainda não passou pelo
// PathAuthorizer (AEP-0092).
var errOutsideAllowedDirs = errors.New("caminho fora dos diretórios permitidos")

// PathAuthorizer autoriza paths fora do sandbox (allowlist escopável + DecisionDialog).
// Implementado por internal/fstrust.Authorizer; nil = deny seco (testes / bootstrap).
type PathAuthorizer interface {
	Authorize(ctx context.Context, absPath, operation string) error
}

var (
	pathAuthorizer PathAuthorizer
	// sandboxRootFunc, quando definido, devolve a raiz do workspace ATIVO
	// (AEP-0092 D5). Avaliado a cada validatePath; "" cai no workDir da tool.
	sandboxRootFunc func() string
)

// SetPathAuthorizer instala o authorizer de paths fora do sandbox (AEP-0092).
func SetPathAuthorizer(a PathAuthorizer) {
	pathAuthorizer = a
}

// SetSandboxRootFunc instala o resolvedor dinâmico da raiz permitida (workspace
// ativo). Sem isto, as tools ficam amarradas ao cwd do boot.
func SetSandboxRootFunc(f func() string) {
	sandboxRootFunc = f
}

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
//  3. Se relativo, resolve em relação ao workspace ativo; sem workspace ativo,
//     cai no workDir estático da tool.
func resolveFilePath(path, workDir string) (string, error) {
	// Primeiro expande tilde
	expanded := expandTilde(path)

	// Se é absoluto, retorna limpo
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}

	// Relativo: acompanha a mesma raiz dinâmica usada pelo sandbox. Isso evita
	// resolver no cwd do boot depois que o usuário troca o workspace ativo.
	root, err := effectiveSandboxRoot(workDir)
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(root, expanded))
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

func effectiveSandboxRoot(workDir string) (string, error) {
	if sandboxRootFunc != nil {
		if root := strings.TrimSpace(sandboxRootFunc()); root != "" {
			abs, err := filepath.Abs(filepath.Clean(root))
			if err != nil {
				return "", fmt.Errorf("sandbox root inválido: %v", err)
			}
			return abs, nil
		}
	}
	wd := strings.TrimSpace(workDir)
	if wd == "" {
		return "", fmt.Errorf("workDir inválido")
	}
	abs, err := filepath.Abs(filepath.Clean(wd))
	if err != nil {
		return "", fmt.Errorf("workDir inválido: %v", err)
	}
	return abs, nil
}

// resolveForComparison resolve symlinks do path para comparação de sandbox,
// inclusive links cujo alvo ainda não existe. Percorre cada componente com
// Lstat/Readlink; ao encontrar um link, substitui pelo alvo e reinicia a
// resolução com os componentes restantes.
//
// Necessário para fechar o escape do sandbox via link dentro da raiz apontando
// para fora (ex.: workDir/link -> /etc): a comparação passa a olhar o destino
// REAL, não o nome literal do link (AEP-0092).
func resolveForComparison(absPath string) (string, error) {
	path, err := filepath.Abs(filepath.Clean(absPath))
	if err != nil {
		return "", err
	}

	const maxSymlinks = 64
	for followed := 0; followed < maxSymlinks; followed++ {
		volume := filepath.VolumeName(path)
		root := volume + string(filepath.Separator)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		if rel == "." {
			return root, nil
		}

		parts := strings.Split(rel, string(filepath.Separator))
		current := root
		restarted := false
		for i, part := range parts {
			if part == "" || part == "." {
				continue
			}
			candidate := filepath.Join(current, part)
			info, statErr := os.Lstat(candidate)
			if statErr != nil {
				if errors.Is(statErr, os.ErrNotExist) {
					return filepath.Join(current, filepath.Join(parts[i:]...)), nil
				}
				return "", fmt.Errorf("não foi possível inspecionar %s: %w", candidate, statErr)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				current = candidate
				continue
			}

			target, readErr := os.Readlink(candidate)
			if readErr != nil {
				return "", fmt.Errorf("não foi possível resolver symlink %s: %w", candidate, readErr)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(current, target)
			}
			path = filepath.Clean(filepath.Join(append([]string{target}, parts[i+1:]...)...))
			restarted = true
			break
		}
		if !restarted {
			return current, nil
		}
	}
	return "", fmt.Errorf("muitos níveis de symlink em %s", absPath)
}

func validatePath(fullPath, workDir string) error {
	absPath, err := filepath.Abs(filepath.Clean(fullPath))
	if err != nil {
		return fmt.Errorf("caminho inválido: %v", err)
	}
	absRoot, err := effectiveSandboxRoot(workDir)
	if err != nil {
		return err
	}

	allowedRoots := []string{absRoot}
	if homeBase := strings.TrimSpace(configdir.GetHomeDir()); homeBase != "" {
		if absHomeBase, err := filepath.Abs(filepath.Clean(homeBase)); err == nil {
			if normalizeForComparison(absHomeBase) != normalizeForComparison(absRoot) {
				allowedRoots = append(allowedRoots, absHomeBase)
			}
		}
	}

	// Compara o destino REAL (symlinks resolvidos) contra raízes também
	// resolvidas — resolver só um lado quebraria em plataformas onde a raiz é
	// symlink (ex.: macOS /var -> /private/var).
	resolvedPath, err := resolveForComparison(absPath)
	if err != nil {
		return fmt.Errorf("não foi possível validar o destino real: %w", err)
	}
	for _, root := range allowedRoots {
		resolvedRoot, resolveErr := resolveForComparison(root)
		if resolveErr != nil {
			return fmt.Errorf("não foi possível validar a raiz permitida: %w", resolveErr)
		}
		if isWithinRoot(resolvedPath, resolvedRoot) {
			return nil
		}
	}
	return errOutsideAllowedDirs
}

// walkEntryEscapesSandbox reporta se uma entrada encontrada durante um walk
// aponta para fora das raízes permitidas. Só symlink precisa do teste: WalkDir
// não desce por links, então toda entrada comum abaixo de uma raiz já validada
// continua dentro dela — e restringir o teste ao link evita resolver o path de
// cada arquivo percorrido.
//
// Walkers pulam a entrada em silêncio em vez de acionar o PathAuthorizer: um
// prompt por arquivo percorrido inviabilizaria a busca. Ler o alvo continua
// possível pelo caminho real, aí sim com consentimento (AEP-0092).
func walkEntryEscapesSandbox(path string, mode os.FileMode, workDir string) bool {
	if mode&os.ModeSymlink == 0 {
		return false
	}
	return validatePath(path, workDir) != nil
}

// pathEscapesSandbox faz o teste completo, sem o atalho do symlink. É o que
// vale para resultados de filepath.Glob: diferente do WalkDir, o Glob atravessa
// diretório linkado quando o padrão nomeia o link (`linkdir/*.conf`), então o
// match pode ser um arquivo comum cujo destino real está fora da raiz.
func pathEscapesSandbox(path string, workDir string) bool {
	return validatePath(path, workDir) != nil
}

func validatePathWithPolicy(ctx context.Context, fullPath, workDir string, policy Policy, operation string) error {
	// Sensível é hard-deny e precede o PathAuthorizer: não faz sentido pedir
	// consentimento para algo que será negado (AEP-0092 D-Q5). A checagem
	// resolve symlinks para fechar bypass de link com nome inócuo apontando
	// para arquivo sensível — inclusive quando o link mora dentro do workDir.
	if policy.BlockSensitive {
		if err := blockSensitiveForOperation(fullPath, operation); err != nil {
			return err
		}
	}
	if err := validatePath(fullPath, workDir); err != nil {
		if !errors.Is(err, errOutsideAllowedDirs) {
			return err
		}
		// AEP-0092: fora do sandbox → allowlist / DecisionDialog (não open-editor bypass).
		if pathAuthorizer == nil {
			return err
		}
		if authErr := pathAuthorizer.Authorize(ctx, fullPath, operation); authErr != nil {
			return authErr
		}
	}
	if err := validateSkillFilesystemAllowlist(ctx, fullPath, workDir, operation); err != nil {
		return err
	}
	return nil
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
	// A política do skill precisa avaliar o mesmo destino REAL usado pelo
	// sandbox. Sem isso, um symlink permitido dentro do workspace poderia
	// apontar para fora e ainda casar a allowlist pelo nome literal do link.
	absPath, err = resolveForComparison(absPath)
	if err != nil {
		return fmt.Errorf("não foi possível validar o destino real para o skill: %w", err)
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
	// Compara sempre destino real contra destino real. Resolvido uma vez por
	// chamada, e não por pattern, para não multiplicar syscalls em walks.
	if resolved, err := resolveForComparison(absPath); err == nil {
		absPath = resolved
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

// resolveGlobFreePrefix resolve symlinks apenas na parte literal do padrão,
// parando no primeiro componente com metacaractere de glob. Sem isso, um padrão
// ancorado numa raiz que é symlink (ex.: macOS /var -> /private/var) nunca casa
// o path já resolvido, virando falso-negativo e bloqueio indevido.
func resolveGlobFreePrefix(pattern string) string {
	volume := filepath.VolumeName(pattern)
	root := volume + string(filepath.Separator)
	rel, err := filepath.Rel(root, pattern)
	if err != nil {
		return pattern
	}

	parts := strings.Split(rel, string(filepath.Separator))
	literal := root
	for i, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			resolved, resolveErr := resolveForComparison(literal)
			if resolveErr != nil {
				return pattern
			}
			return filepath.Join(resolved, filepath.Join(parts[i:]...))
		}
		literal = filepath.Join(literal, part)
	}

	resolved, resolveErr := resolveForComparison(literal)
	if resolveErr != nil {
		return pattern
	}
	return resolved
}

func filesystemPatternMatches(absPath string, workDir string, pattern string) bool {
	// Resolve o padrão como um path (expande ~ e caminhos relativos à mesma
	// raiz dinâmica do sandbox/workspace ativo).
	resolved, err := resolveFilePath(pattern, workDir)
	if err != nil {
		return false
	}
	resolved = resolveGlobFreePrefix(filepath.Clean(resolved))

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
