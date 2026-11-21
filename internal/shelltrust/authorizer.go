package shelltrust

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/commandpolicy"
	"assistente/internal/logging"
	"assistente/internal/tools/invocationctx"
)

// ErrConfirmationUnavailable indica que a decisão exige confirmação e não há interlocutor.
var ErrConfirmationUnavailable = errors.New("confirmação shell sem interlocutor")

// ErrPrerequisiteUnavailable identifica falha ao validar o diretório, a política ou as concessões.
var ErrPrerequisiteUnavailable = errors.New("pré-requisito de autorização shell indisponível")

type prerequisiteReason string

const (
	directoryChanged  prerequisiteReason = "directory_changed"
	storeUnavailable  prerequisiteReason = "store_unavailable"
	policyUnavailable prerequisiteReason = "policy_unavailable"
)

func (r prerequisiteReason) Error() string        { return string(r) }
func (r prerequisiteReason) Is(target error) bool { return target == ErrPrerequisiteUnavailable }

// PrerequisiteReason retorna apenas categorias internas que podem aparecer no log.
func PrerequisiteReason(err error) string {
	var reason prerequisiteReason
	if errors.As(err, &reason) {
		return string(reason)
	}
	return "unknown"
}

type PromptRequest struct {
	PersistentTerminal bool
	ProtectedCommand   bool
	LiteralPrefix      string
	PublicTemplate     string
	Command            string
	Directory          string
	AvailableScopes    []authorization.Scope
}
type PromptDecision struct {
	Approve bool
	Scope   authorization.Scope
}
type Authorizer struct {
	Manager           *Manager
	Policy            func(context.Context) (*allowlist.Allowlist, error)
	Prompt            func(context.Context, PromptRequest) (PromptDecision, error)
	PersistentAllowed func(context.Context) bool
	PersistenceFailed func(context.Context, authorization.Request, error)
}

type decisionObserverKey struct{}
type confirmationReceiptKey struct{}

type DecisionOutcome uint8

const (
	DeniedByUser DecisionOutcome = iota
	DeniedByPolicy
	Approved
)

// WithDecisionObserver permite ao solicitante receber a conclusão da autorização
// antes do log, pelo mesmo contrato de retorno de Authorize.
func WithDecisionObserver(ctx context.Context, observe func(DecisionOutcome, error), receipt *invocationctx.ConfirmationReceipt) context.Context {
	ctx = context.WithValue(ctx, confirmationReceiptKey{}, receipt)
	return context.WithValue(ctx, decisionObserverKey{}, observe)
}

// ConfirmationReceipt arbitra o aceite da resposta e o encerramento da espera,
// preservando a recusa aceita mesmo antes da entrega ao solicitante.
func ConfirmationReceipt(ctx context.Context) *invocationctx.ConfirmationReceipt {
	flag, _ := ctx.Value(confirmationReceiptKey{}).(*invocationctx.ConfirmationReceipt)
	return flag
}

func (a *Authorizer) Authorize(ctx context.Context, command, directory string) (bool, error) {
	if a.Manager != nil {
		ctx = a.Manager.Capture(ctx)
	}
	persistent := invocationctx.PersistentShell(ctx)
	_, protected := invocationctx.LookupShellCommandMetadata(ctx)
	var selector Selector
	var reusable bool
	if !persistent && !protected {
		selector, reusable = Represent(command, directory)
	}
	scopes := []authorization.Scope{authorization.ScopeOnce}
	if reusable && a.Manager != nil && a.PersistentAllowed != nil && a.PersistentAllowed(ctx) {
		var err error
		scopes, err = a.Manager.AvailableScopes(ctx)
		if err != nil {
			return false, errors.Join(storeUnavailable, err)
		}
	}
	outcome := DeniedByUser
	flow := authorization.Flow[Entry]{
		Evaluate: func(ctx context.Context) (authorization.Decision, error) {
			if reusable {
				current, valid := Represent(command, directory)
				if !valid || current.Directory != selector.Directory {
					return authorization.Decision{}, directoryChanged
				}
			}
			if a.Manager != nil {
				if _, err := a.Manager.AvailableScopes(ctx); err != nil {
					return authorization.Decision{}, errors.Join(storeUnavailable, err)
				}
			}
			var policy *allowlist.Allowlist
			if a.Policy != nil {
				var err error
				policy, err = a.Policy(ctx)
				if err != nil {
					return authorization.Decision{}, errors.Join(policyUnavailable, err)
				}
			}
			result := commandpolicy.Evaluate(command, policy)
			if result.Decision == allowlist.DecisionDeny {
				outcome = DeniedByPolicy
				if protected {
					return authorization.Decision{Effect: "deny", Reason: "política de comandos"}, nil
				}
				return authorization.Decision{Effect: "deny", Reason: strings.Join(result.Reasons, "; ")}, nil
			}
			if result.Decision == allowlist.DecisionApprove {
				return authorization.Decision{Effect: "allow", Reason: "política de comandos"}, nil
			}
			if reusable && a.Manager != nil {
				match := a.Manager.Find(ctx, selector)
				if match.Err != nil {
					return authorization.Decision{}, errors.Join(storeUnavailable, match.Err)
				}
				if match.Found {
					return authorization.Decision{Effect: "allow", Scope: match.Scope}, nil
				}
			}
			return authorization.Decision{Effect: "ask"}, nil
		},
		PersistenceFailed: func(ctx context.Context, req authorization.Request, err error) {
			logging.Errorf(ctx, "shelltrust", "Falha ao lembrar autorização shell: %v", err)
			if a.PersistenceFailed != nil {
				a.PersistenceFailed(ctx, req, err)
			}
		},
	}
	if observe, ok := ctx.Value(decisionObserverKey{}).(func(DecisionOutcome, error)); ok && observe != nil {
		flow.Resolved = func(decision authorization.Decision, err error) {
			if decision.Effect == "allow" {
				outcome = Approved
			}
			observe(outcome, err)
		}
	}
	if a.Manager != nil {
		flow.Remember = func(ctx context.Context, choice authorization.Consent[Entry]) error {
			return a.Manager.Add(ctx, choice.Value)
		}
	}
	flow.Ask = func(ctx context.Context) (authorization.Consent[Entry], error) {
		if a.Prompt == nil {
			return authorization.Consent[Entry]{}, ErrConfirmationUnavailable
		}
		req := PromptRequest{AvailableScopes: scopes, PersistentTerminal: persistent, ProtectedCommand: protected}
		if protected {
			// Nem o diretório resolvido tem proveniência pública garantida.
			// Um trim do template pode colar o segredo ao token que o precede.
			req.PublicTemplate = invocationctx.ShellConsentTemplate(ctx, command)
			prefix := invocationctx.ShellConsentPrefix(ctx)
			rest, matches := strings.CutPrefix(strings.TrimLeft(command, " \t"), prefix)
			if prefix != "" && matches && (strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t")) {
				req.LiteralPrefix = prefix
			}
		} else {
			req.Command, req.Directory = command, directory
		}
		choice, err := a.Prompt(ctx, req)
		if err != nil {
			return authorization.Consent[Entry]{}, err
		}
		if !choice.Approve {
			return authorization.Consent[Entry]{Effect: "deny", Scope: authorization.ScopeOnce}, nil
		}
		return authorization.Consent[Entry]{Effect: "allow", Scope: choice.Scope, Value: Entry{Selector: selector, Scope: choice.Scope}}, nil
	}
	result, err := authorization.Resolve(ctx, authorization.Request{Domain: "shell", Operation: "execute", Resource: "operação shell", AvailableScopes: scopes}, flow)
	return result.Effect == "allow", err
}
