package fstrust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/logging"
	"assistente/internal/tools"
)

// PromptRequest é o pedido de consentimento quando um path fora do sandbox não
// está em nenhuma allowlist.
type PromptRequest struct {
	Path         string // path solicitado (antes ou além da resolução de symlink)
	Operation    string
	ResolvedPath string // após EvalSymlinks quando possível; igual a Path se falhar
	SkillSlug    string
}

// PromptDecision é a resposta do usuário ao pedido de autorização.
type PromptDecision struct {
	Approve bool
	Scope   Scope
	// Kind=dir concede o diretório pai (ou o próprio path se já for dir) para a
	// operação; Kind=file concede o path resolvido exato.
	Kind Kind
}

// Prompter apresenta o pedido de consentimento e devolve a decisão.
type Prompter interface {
	PromptPathAuthorization(ctx context.Context, req PromptRequest) (PromptDecision, error)
}

// Authorizer conecta o Manager (allowlist) ao Prompter (consentimento).
type Authorizer struct {
	mgr    *Manager
	prompt Prompter
}

// NewAuthorizer cria o authorizer. prompt pode ser nil (só allowlist, sem UI).
func NewAuthorizer(mgr *Manager, prompt Prompter) *Authorizer {
	return &Authorizer{mgr: mgr, prompt: prompt}
}

// Authorize decide se absPath+operation pode ser liberado.
// nil = permitido; erro descritivo se negado ou se o prompt falhar.
func (a *Authorizer) Authorize(ctx context.Context, absPath, operation string) error {
	requested := NormalizePath(absPath)
	if requested == "" {
		return fmt.Errorf("path vazio não pode ser autorizado")
	}

	resolved, err := resolveSymlinks(requested)
	if err != nil {
		return fmt.Errorf("não foi possível resolver o destino real de %s: %w", requested, err)
	}

	// 1) Allowlist existente (sempre no path resolvido).
	if a.mgr != nil {
		if decision := a.mgr.Match(ctx, resolved, operation); decision.Allowed {
			logging.Infof(ctx, "fstrust.authorizer",
				"[FsTrust] match em allowlist: path=%s op=%s escopo=%s",
				resolved, operation, decision.Scope)
			return nil
		}
	}

	// 2) Consentimento explícito
	if a.prompt == nil {
		return fmt.Errorf("caminho fora do sandbox e sem authorizer: %s (%s)", requested, operation)
	}

	skillSlug := skillSlugFrom(ctx)
	req := PromptRequest{
		Path:         requested,
		Operation:    operation,
		ResolvedPath: resolved,
		SkillSlug:    skillSlug,
	}

	logging.Infof(ctx, "fstrust.authorizer",
		"[FsTrust] solicitando autorização: path=%s resolved=%s op=%s skill=%s",
		requested, resolved, operation, skillSlug)

	decision, err := a.prompt.PromptPathAuthorization(ctx, req)
	if err != nil {
		return fmt.Errorf("falha ao solicitar autorização de path %s (%s): %w", requested, operation, err)
	}
	if !decision.Approve {
		logging.Infof(ctx, "fstrust.authorizer",
			"[FsTrust] autorização negada: path=%s op=%s", requested, operation)
		return fmt.Errorf("autorização negada para path %s (operação %s)", requested, operation)
	}

	scope := decision.Scope
	if !ValidScope(scope) {
		scope = ScopeOnce
	}
	kind := decision.Kind
	if !ValidKind(kind) {
		kind = KindFile
	}

	entryPath := resolved
	if kind == KindDir {
		entryPath = dirGrantRoot(resolved)
	}

	if a.mgr != nil && scope != ScopeOnce {
		entry := AllowlistEntry{
			Path:      entryPath,
			Kind:      kind,
			Operation: operation,
			Scope:     scope,
			CreatedBy: creatorFor(skillSlug),
		}
		if err := a.mgr.Add(ctx, entry); err != nil {
			logging.Errorf(ctx, "fstrust.authorizer",
				"[FsTrust] falha ao persistir allowlist (escopo %s) para path=%s: %v — liberando apenas esta tentativa (once)",
				scope, entryPath, err)
		}
	} else if a.mgr != nil && scope == ScopeOnce {
		// once: Add é no-op; a tentativa corrente já está liberada pelo return nil.
		_ = a.mgr.Add(ctx, AllowlistEntry{
			Path:      entryPath,
			Kind:      kind,
			Operation: operation,
			Scope:     ScopeOnce,
			CreatedBy: creatorFor(skillSlug),
		})
	}

	logging.Infof(ctx, "fstrust.authorizer",
		"[FsTrust] autorização concedida: path=%s kind=%s op=%s escopo=%s",
		entryPath, kind, operation, scope)
	return nil
}

// resolveSymlinks resolve cada componente com Lstat/Readlink, inclusive quando
// o alvo do link ainda não existe. Isso mantém persistência e match no mesmo
// destino real antes e depois da criação do arquivo.
func resolveSymlinks(absPath string) (string, error) {
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
			return NormalizePath(root), nil
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
					return NormalizePath(filepath.Join(current, filepath.Join(parts[i:]...))), nil
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
			return NormalizePath(current), nil
		}
	}
	return "", fmt.Errorf("muitos níveis de symlink em %s", absPath)
}

// dirGrantRoot devolve o diretório a autorizar: o próprio path se já for
// diretório existente; caso contrário, o diretório pai.
func dirGrantRoot(resolved string) string {
	info, err := os.Stat(resolved)
	if err == nil && info.IsDir() {
		return NormalizePath(resolved)
	}
	return NormalizePath(filepath.Dir(resolved))
}

func skillSlugFrom(ctx context.Context) string {
	if ec, ok := tools.GetExecutionContext(ctx); ok {
		return ec.InvokedSkillSlug
	}
	return ""
}

func creatorFor(skillSlug string) string {
	if skillSlug != "" {
		return "skill:" + skillSlug
	}
	return "user"
}
