package fstrust

import (
	"assistente/internal/authorization"
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
	AvailableScopes []Scope
	Path            string // path solicitado (antes ou além da resolução de symlink)
	Operation       string
	ResolvedPath    string // após EvalSymlinks quando possível; igual a Path se falhar
	SkillSlug       string
}

// PromptDecision é a resposta do usuário ao pedido de autorização.
type PromptDecision struct {
	Approve bool
	Scope   Scope
	// Effect indica allow ou deny. Deny em ScopeOnce bloqueia só a tentativa;
	// nos demais escopos, registra uma recusa lembrada. Allow segue o fluxo de
	// autorização e persistência abaixo.
	Effect Effect
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
	ScopeFilter       func(context.Context, []Scope) []Scope
	PersistenceFailed func(context.Context, authorization.Request, error)
	mgr               *Manager
	prompt            Prompter
}

// NewAuthorizer cria o authorizer. prompt pode ser nil (só allowlist, sem UI).
func NewAuthorizer(mgr *Manager, prompt Prompter) *Authorizer {
	return &Authorizer{mgr: mgr, prompt: prompt}
}

// Authorize decide se absPath+operation pode ser liberado.
// nil = permitido; erro descritivo se negado ou se o prompt falhar.
func (a *Authorizer) Authorize(ctx context.Context, absPath, operation string) error {
	if a.mgr != nil {
		ctx = a.mgr.Capture(ctx)
	}
	requested := NormalizePath(absPath)
	if requested == "" {
		return fmt.Errorf("path vazio não pode ser autorizado")
	}
	resolved, err := resolveSymlinks(requested)
	if err != nil {
		return fmt.Errorf("não foi possível resolver o destino real de %s: %w", requested, err)
	}
	skillSlug := skillSlugFrom(ctx)
	scopes := []Scope{ScopeOnce}
	if a.mgr != nil {
		scopes, err = a.mgr.AvailableScopes(ctx)
		if err != nil {
			return err
		}
	}
	prompted := false
	if a.ScopeFilter != nil {
		scopes = a.ScopeFilter(ctx, scopes)
	}
	flow := authorization.Flow[AllowlistEntry]{
		Evaluate: func(ctx context.Context) (authorization.Decision, error) {
			if a.mgr != nil {
				entries, err := a.mgr.List(ctx)
				if err != nil {
					return authorization.Decision{}, err
				}
				result := authorization.Decision{Effect: "ask"}
				for _, entry := range entries {
					if !entry.Matches(resolved, operation) {
						continue
					}
					if NormalizedEffect(entry.Effect) == EffectDeny {
						return authorization.Decision{}, deniedEntry(ctx, requested, resolved, operation, entry.Scope)
					}
					if result.Effect == "ask" {
						result = authorization.Decision{Effect: "allow", Scope: entry.Scope}
					}
				}
				return result, nil
			}
			return authorization.Decision{Effect: "ask"}, nil
		},
		PersistenceFailed: func(ctx context.Context, req authorization.Request, err error) {
			if a.PersistenceFailed != nil {
				a.PersistenceFailed(ctx, req, err)
			}
			logging.Errorf(ctx, "fstrust.authorizer", "[FsTrust] falha ao lembrar decisão path=%s op=%s: %v — válida somente nesta tentativa", resolved, operation, err)
		},
	}
	if a.mgr != nil {
		flow.Remember = func(ctx context.Context, choice authorization.Consent[AllowlistEntry]) error {
			return a.mgr.Add(ctx, choice.Value)
		}
	}
	if a.prompt != nil {
		flow.Ask = func(ctx context.Context) (authorization.Consent[AllowlistEntry], error) {
			prompted = true
			choice, err := a.prompt.PromptPathAuthorization(ctx, PromptRequest{AvailableScopes: scopes, Path: requested, Operation: operation, ResolvedPath: resolved, SkillSlug: skillSlug})
			if err != nil {
				return authorization.Consent[AllowlistEntry]{}, fmt.Errorf("falha ao solicitar autorização de path %s (%s): %w", requested, operation, err)
			}
			scope := choice.Scope
			if !ValidScope(scope) {
				scope = ScopeOnce
			}
			kind := choice.Kind
			if !ValidKind(kind) {
				kind = KindFile
			}
			effect := EffectAllow
			if !choice.Approve {
				effect = EffectDeny
				if NormalizedEffect(choice.Effect) != EffectDeny {
					scope = ScopeOnce
				}
			}
			path := resolved
			if kind == KindDir {
				path = dirGrantRoot(resolved)
			}
			entry := AllowlistEntry{Path: path, Kind: kind, Operation: operation, Effect: effect, Scope: scope, CreatedBy: creatorFor(skillSlug)}
			return authorization.Consent[AllowlistEntry]{Effect: string(effect), Scope: scope, Value: entry}, nil
		}
	}
	result, err := authorization.Resolve(ctx, authorization.Request{Domain: "filesystem", Operation: operation, Resource: resolved, AvailableScopes: scopes}, flow)
	if err != nil {
		return err
	}
	if result.Effect != "allow" {
		reason, source := "autorização negada pelo usuário", "user"
		if !prompted {
			reason, source = "sem prompter de consentimento", "no_prompter"
		}
		tools.RecordSecuritySignal(ctx, tools.SecuritySignal{Version: 1, Domain: "filesystem", Outcome: "blocked", Source: source, Scope: string(result.Scope), Operation: operation})
		return newDeniedPathError(requested, operation, reason, prompted)
	}
	if prompted {
		tools.RecordSecuritySignal(ctx, tools.SecuritySignal{Version: 1, Domain: "filesystem", Outcome: "approved", Source: "user", Scope: string(result.Scope), Operation: operation})
	}
	logging.Infof(ctx, "fstrust.authorizer", "[FsTrust] autorização concedida: path=%s op=%s escopo=%s", resolved, operation, result.Scope)
	return nil
}

// Denied reporta se absPath+operation está em alguma denylist (qualquer escopo).
// Usado também dentro do sandbox: deny não depende de estar fora das raízes.
func (a *Authorizer) Denied(ctx context.Context, absPath, operation string) error {
	if a == nil || a.mgr == nil {
		return nil
	}
	requested := NormalizePath(absPath)
	if requested == "" {
		return nil
	}
	resolved, err := resolveSymlinks(requested)
	if err != nil {
		// Falha fechado: sem destino real confiável, não dá para garantir que
		// não há deny apontando para o alvo — bloqueia.
		return newDeniedPathError(requested, operation, "não foi possível resolver o destino real para avaliar denylist", false)
	}
	return a.deniedResolved(ctx, requested, resolved, operation)
}

// DeniedResolved é Denied para quando o chamador JÁ resolveu os symlinks do path
// (ex.: o sandbox em validatePath), evitando um resolveSymlinks redundante.
func (a *Authorizer) DeniedResolved(ctx context.Context, resolvedPath, operation string) error {
	if a == nil || a.mgr == nil {
		return nil
	}
	resolved := NormalizePath(resolvedPath)
	if resolved == "" {
		return nil
	}
	return a.deniedResolved(ctx, resolved, resolved, operation)
}

// deniedResolved avalia a denylist com o destino real já resolvido, evitando
// um resolveSymlinks redundante quando o chamador (ex.: Authorize) já resolveu.
func (a *Authorizer) deniedResolved(ctx context.Context, requested, resolved, operation string) error {
	if a == nil || a.mgr == nil {
		return nil
	}
	decision := a.mgr.MatchDeny(ctx, resolved, operation)
	if decision.Err != nil {
		return decision.Err
	}
	if !decision.Matched {
		return nil
	}
	return deniedEntry(ctx, requested, resolved, operation, decision.Scope)
}

func deniedEntry(ctx context.Context, requested, resolved, operation string, scope Scope) error {
	logging.Infof(ctx, "fstrust.authorizer",
		"[FsTrust] bloqueado por denylist: path=%s op=%s escopo=%s",
		resolved, operation, scope)
	tools.RecordSecuritySignal(ctx, tools.SecuritySignal{Version: 1, Domain: "filesystem", Outcome: "blocked", Source: "denylist", Scope: string(scope), Operation: operation})
	return newDeniedPathError(requested, operation, fmt.Sprintf("bloqueado pela denylist (escopo %s)", scope), false)
}

// ResolvePath devolve o destino real (symlinks resolvidos) e normalizado de um
// path. É o mesmo pré-processamento que o Authorize faz antes de casar allow/deny,
// e serve para persistir entradas (ex.: deny criado pela UI) no mesmo formato que
// o match usa em tempo de acesso — sem isso, um alias/symlink não casaria a regra.
func ResolvePath(absPath string) (string, error) {
	requested := NormalizePath(absPath)
	if requested == "" {
		return "", fmt.Errorf("path vazio não pode ser resolvido")
	}
	return resolveSymlinks(requested)
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
