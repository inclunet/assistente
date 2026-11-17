package shelltrust

import (
	"context"
	"strings"

	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/commandpolicy"
	"assistente/internal/logging"
)

type PromptRequest struct {
	Command         string
	Directory       string
	AvailableScopes []authorization.Scope
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

func (a *Authorizer) Authorize(ctx context.Context, command, directory string) (bool, error) {
	if a.Manager != nil {
		ctx = a.Manager.Capture(ctx)
	}
	selector, reusable := Represent(command, directory)
	scopes := []authorization.Scope{authorization.ScopeOnce}
	if reusable && a.Manager != nil && a.PersistentAllowed != nil && a.PersistentAllowed(ctx) {
		var err error
		scopes, err = a.Manager.AvailableScopes(ctx)
		if err != nil {
			return false, err
		}
	}
	flow := authorization.Flow[Entry]{
		Evaluate: func(ctx context.Context) (authorization.Decision, error) {
			if a.Manager != nil {
				if _, err := a.Manager.AvailableScopes(ctx); err != nil {
					return authorization.Decision{}, err
				}
			}
			var policy *allowlist.Allowlist
			if a.Policy != nil {
				var err error
				policy, err = a.Policy(ctx)
				if err != nil {
					return authorization.Decision{}, err
				}
			}
			result := commandpolicy.Evaluate(command, policy)
			if result.Decision == allowlist.DecisionDeny {
				return authorization.Decision{Effect: "deny", Reason: strings.Join(result.Reasons, "; ")}, nil
			}
			if result.Decision == allowlist.DecisionApprove {
				return authorization.Decision{Effect: "allow", Reason: "política de comandos"}, nil
			}
			if reusable && a.Manager != nil {
				match := a.Manager.Find(ctx, selector)
				if match.Err != nil {
					return authorization.Decision{}, match.Err
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
	if a.Manager != nil {
		flow.Remember = func(ctx context.Context, choice authorization.Consent[Entry]) error {
			return a.Manager.Add(ctx, choice.Value)
		}
	}
	if a.Prompt != nil {
		flow.Ask = func(ctx context.Context) (authorization.Consent[Entry], error) {
			choice, err := a.Prompt(ctx, PromptRequest{Command: command, Directory: directory, AvailableScopes: scopes})
			if err != nil {
				return authorization.Consent[Entry]{}, err
			}
			if !choice.Approve {
				return authorization.Consent[Entry]{Effect: "deny", Scope: authorization.ScopeOnce}, nil
			}
			return authorization.Consent[Entry]{Effect: "allow", Scope: choice.Scope, Value: Entry{Selector: selector, Scope: choice.Scope}}, nil
		}
	}
	result, err := authorization.Resolve(ctx, authorization.Request{Domain: "shell", Operation: "execute", Resource: "operação shell", AvailableScopes: scopes}, flow)
	return result.Effect == "allow", err
}
