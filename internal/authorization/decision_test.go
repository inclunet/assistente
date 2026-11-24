package authorization_test

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/authorization"
	"assistente/internal/testutil/authorizationfixture"
)

func TestDecisaoReavaliaNegativaAntesDeLembrar(t *testing.T) {
	denied := false
	request := authorization.Request{Domain: "shell", Operation: "execute", Resource: "command", AvailableScopes: []authorization.Scope{authorization.ScopeGlobal}}
	result, err := authorization.Resolve(authorizationfixture.Context(), request, authorization.Flow[string]{
		Evaluate: func(context.Context) (authorization.Decision, error) {
			if denied {
				return authorization.Decision{Effect: "deny", Reason: "política atualizada"}, nil
			}
			return authorization.Decision{Effect: "ask"}, nil
		},
		Ask: func(context.Context) (authorization.Consent[string], error) {
			denied = true
			return authorization.Consent[string]{Effect: "allow", Scope: "global"}, nil
		},
		Remember: func(context.Context, authorization.Consent[string]) error {
			t.Fatal("gravou após nova negativa")
			return nil
		},
	})
	if err != nil || result.Effect != "deny" {
		t.Fatalf("decisão: %+v, %v", result, err)
	}
}

func TestFalhaDeGravacaoInformaAlcancePontual(t *testing.T) {
	failure := errors.New("falha de escrita")
	notified := false
	result, err := authorization.Resolve(authorizationfixture.Context(), authorization.Request{Domain: "network", Operation: "connect", Resource: "host", AvailableScopes: []authorization.Scope{authorization.ScopeProfile}}, authorization.Flow[string]{
		Evaluate: func(context.Context) (authorization.Decision, error) {
			return authorization.Decision{Effect: "ask"}, nil
		},
		Ask: func(context.Context) (authorization.Consent[string], error) {
			return authorization.Consent[string]{Effect: "allow", Scope: "profile"}, nil
		},
		Remember:          func(context.Context, authorization.Consent[string]) error { return failure },
		PersistenceFailed: func(_ context.Context, _ authorization.Request, err error) { notified = errors.Is(err, failure) },
	})
	if err != nil || result.Effect != "allow" || result.Scope != "once" || !errors.Is(result.PersistenceError, failure) || !notified {
		t.Fatalf("resultado: %+v, %v, aviso=%v", result, err, notified)
	}
}

func TestFalhaDeConsultaECancelamentoNaoAutorizam(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "leitura", true: "cancelamento"}[cancelled], func(t *testing.T) {
			ctx, cancel := context.WithCancel(authorizationfixture.Context())
			defer cancel()
			lookupFailure := false
			result, err := authorization.Resolve(ctx, authorization.Request{Domain: "filesystem", Operation: "read", Resource: "path"}, authorization.Flow[string]{
				Evaluate: func(context.Context) (authorization.Decision, error) {
					if lookupFailure {
						return authorization.Decision{}, errors.New("consulta falhou")
					}
					return authorization.Decision{Effect: "ask"}, nil
				},
				Ask: func(context.Context) (authorization.Consent[string], error) {
					if cancelled {
						cancel()
					} else {
						lookupFailure = true
					}
					return authorization.Consent[string]{Effect: "allow", Scope: "once"}, nil
				},
			})
			if err == nil || result.Effect == "allow" {
				t.Fatalf("liberou: %+v, %v", result, err)
			}
		})
	}
}

func TestRecusaUnicaSobreviveIndisponibilidadeDaPolitica(t *testing.T) {
	for _, domain := range []string{"shell", "network", "filesystem"} {
		t.Run(domain, func(t *testing.T) { checkOnceDenial(t, domain) })
	}
}

func TestACPRecusaUnicaSobreviveIndisponibilidadeDaPolitica(t *testing.T) {
	checkOnceDenial(t, "acp")
}

func checkOnceDenial(t *testing.T, domain string) {
	t.Helper()
	for _, scenario := range []string{"politica-indisponivel", "contexto-encerrado"} {
		for _, scope := range []authorization.Scope{"", authorization.ScopeOnce} {
			t.Run(scenario+"/scope-"+string(scope), func(t *testing.T) {
				request := authorization.Request{Domain: domain, Operation: "operation", Resource: "resource"}
				reads := 0
				ctx, cancel := context.WithCancel(authorizationfixture.Context())
				defer cancel()
				got, err := authorization.Resolve(ctx, request, authorization.Flow[string]{
					Evaluate: func(context.Context) (authorization.Decision, error) {
						reads++
						if reads > 1 {
							return authorization.Decision{}, errors.New("indisponível")
						}
						return authorization.Decision{Effect: "ask"}, nil
					},
					Ask: func(context.Context) (authorization.Consent[string], error) {
						if scenario == "contexto-encerrado" {
							cancel()
						}
						return authorization.Consent[string]{Effect: "deny", Scope: scope}, nil
					},
				})
				if err != nil || got.Effect != "deny" || got.Scope != authorization.ScopeOnce || reads != 1 {
					t.Fatalf("decisão=%+v leituras=%d erro=%v", got, reads, err)
				}
			})
		}
	}
}
