package authorization

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"assistente/internal/database"
	"assistente/internal/logging"
)

// Decision é o resultado comum; adaptadores mantêm a validação de seus
// recursos e traduzem o resultado ao transporte, arquivo ou protocolo ACP.
type Decision struct {
	Effect           string
	Scope            Scope
	Reason           string
	PersistenceError error
}

type Request struct {
	Domain          string
	Operation       string
	Resource        string
	AvailableScopes []Scope
}

// Consent associa a escolha à operação corrente. Value é o seletor tipado
// do adaptador, nunca um rótulo interpretado ou um ID fornecido pelo frontend.
type Consent[T any] struct {
	Effect string
	Scope  Scope
	Value  T
}

// Flow não mantém estado. O store comum controla identidade, sessão e banco;
// os adaptadores só fornecem avaliação, apresentação e seletor do domínio.
type Flow[T any] struct {
	Evaluate          func(context.Context) (Decision, error)
	Ask               func(context.Context) (Consent[T], error)
	Remember          func(context.Context, Consent[T]) error
	PersistenceFailed func(context.Context, Request, error)
	// Resolved entrega o resultado ao adaptador antes do log síncrono da decisão.
	Resolved func(Decision, error)
}

// Resolve concentra a sequência avaliação → consentimento → revalidação →
// persistência. Nenhum lock/transação fica aberto enquanto o usuário decide.
func Resolve[T any](ctx context.Context, request Request, flow Flow[T]) (out Decision, resolveErr error) {
	asked := false
	defer func() {
		if flow.Resolved != nil {
			flow.Resolved(out, resolveErr)
		}
		logger := logging.Logger(ctx, "authorization")
		if resolveErr != nil {
			logger.WarnContext(ctx, "Falha na decisão de autorização", "domain", request.Domain)
		} else if asked || out.Effect == "deny" {
			logger.InfoContext(ctx, "Decisão de autorização", "domain", request.Domain, "effect", out.Effect, "scope", out.Scope, "remember_failed", out.PersistenceError != nil)
		}
	}()
	if _, err := database.RequireUserID(ctx); err != nil {
		return Decision{}, err
	}
	if request.Domain == "" || request.Operation == "" || request.Resource == "" || flow.Evaluate == nil {
		return Decision{}, errors.New("pedido de autorização incompleto")
	}
	switch request.Domain {
	case "network", "filesystem", "shell", "acp", PluginHookDomain:
	default:
		return Decision{}, errors.New("domínio de autorização inválido")
	}
	evaluate := func() (Decision, error) {
		if err := ctx.Err(); err != nil {
			return Decision{}, err
		}
		result, err := flow.Evaluate(ctx)
		if err != nil {
			return Decision{}, err
		}
		if result.Effect != "allow" && result.Effect != "deny" && result.Effect != "ask" {
			return Decision{}, fmt.Errorf("efeito de autorização inválido: %q", result.Effect)
		}
		return result, nil
	}
	initial, err := evaluate()
	if err != nil || initial.Effect != "ask" {
		return initial, err
	}
	if flow.Ask == nil {
		return Decision{Effect: "deny", Reason: "sem interlocutor"}, nil
	}
	asked = true
	choice, err := flow.Ask(ctx)
	if err != nil {
		return Decision{}, err
	}
	// Uma recusa desta tentativa já encerra a decisão. A releitura protege
	// concessões e mudanças persistentes, e pode estar indisponível nesse ponto.
	if choice.Effect == "deny" && (choice.Scope == "" || choice.Scope == ScopeOnce) {
		return Decision{Effect: "deny", Scope: ScopeOnce, Reason: "decisão do usuário"}, nil
	}
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if choice.Effect != "allow" && choice.Effect != "deny" {
		return Decision{}, errors.New("resposta de autorização inválida")
	}
	if choice.Scope == "" {
		choice.Scope = "once"
	}
	switch choice.Scope {
	case "once", "session", "workspace", "profile", "global":
	default:
		return Decision{}, errors.New("escopo de autorização inválido")
	}
	current, err := evaluate()
	if err != nil {
		return Decision{}, err
	}
	if current.Effect == "deny" {
		return current, nil
	}
	if choice.Scope != ScopeOnce && !slices.Contains(request.AvailableScopes, choice.Scope) {
		return Decision{}, errors.New("escopo indisponível para a origem")
	}
	result := Decision{Effect: choice.Effect, Scope: choice.Scope, Reason: "decisão do usuário"}
	if choice.Scope == "once" {
		return result, nil
	}
	if flow.Remember == nil {
		err = errors.New("persistência de autorização indisponível")
	} else {
		err = flow.Remember(ctx, choice)
	}
	if err != nil {
		persistenceErr := err
		// Falha de identidade/leitura não pode ser degradada para aprovação
		// pontual. Uma negativa recém-criada continua prevalecendo.
		current, err = evaluate()
		if err != nil {
			return Decision{}, err
		}
		if current.Effect == "deny" {
			return current, nil
		}
		result.Scope = "once"
		result.PersistenceError = persistenceErr
		if flow.PersistenceFailed != nil {
			flow.PersistenceFailed(ctx, request, persistenceErr)
		}
	}
	return result, nil
}

// Notice é o aviso não bloqueante de persistência não concluída.
type Notice struct {
	Domain string `json:"domain"`
}
