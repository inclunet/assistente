package fstrust

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

	resolved := resolveSymlinks(requested)

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

// resolveSymlinks tenta EvalSymlinks; se falhar, devolve o path original.
func resolveSymlinks(absPath string) string {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil || resolved == "" {
		return absPath
	}
	return NormalizePath(resolved)
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
