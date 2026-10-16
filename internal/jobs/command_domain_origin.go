package jobs

import (
	"context"
	"fmt"
	"strings"

	"assistente/internal/commandjobactivation"
	"assistente/internal/database"
	"assistente/internal/eventctx"
	"github.com/google/uuid"
)

// TasklistDomainEventSink é a ponte estreita usada pelo tasklist. Não expõe
// nenhum argumento de raiz: a origem interna só pode ser criada neste Manager,
// após autenticação e prova viva do runtime de comandos.
type TasklistDomainEventSink interface {
	PublishDomainEvent(context.Context, string, map[string]any) error
	HasDomainListener(string) bool
}

type tasklistDomainEventSink struct{ manager *Manager }

// TasklistDomainEventSink devolve a ponte de eventos de tasklist. O sink não é
// uma variante genérica de PublishDomainEvent: aceita somente nomes do catálogo
// tasklist e carimba uma raiz interna privada para mutações humanas autenticadas.
func (m *Manager) TasklistDomainEventSink() TasklistDomainEventSink {
	return tasklistDomainEventSink{manager: m}
}

func (s tasklistDomainEventSink) HasDomainListener(name string) bool {
	return isTasklistDomainEvent(name) && s.manager != nil && s.manager.HasDomainListener(name)
}

func (s tasklistDomainEventSink) PublishDomainEvent(ctx context.Context, name string, payload map[string]any) error {
	if ctx == nil {
		return fmt.Errorf("contexto de publicação ausente")
	}
	if eventctx.IsSuppressed(ctx) {
		return nil
	}
	if s.manager == nil || !isTasklistDomainEvent(name) {
		return fmt.Errorf("evento de tasklist não permitido")
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	// Uma origem privada já existente, inclusive unknown/foreign, tem prioridade;
	// porém o payload reservado nunca é copiado. Um eventctx público reconstrói
	// apenas seus próprios campos explícitos e não promove a raiz.
	if origin, ok := ctx.Value(commandEventOriginKey{}).(commandEventOrigin); ok {
		return s.manager.publishTasklistEvent(ctx, name, payload, tasklistPublicProvenanceFromOrigin(origin, userID))
	}
	if provenance, ok := eventctx.From(ctx); ok {
		return s.manager.publishTasklistEvent(ctx, name, payload, provenance)
	}
	if !s.HasDomainListener(name) {
		return nil
	}

	identity, watchCtx, release, err := s.manager.captureTasklistRuntimeIdentity(ctx, userID)
	if release != nil {
		defer release()
	}
	if err != nil {
		return err
	}
	rootID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	expected := identity
	guard := s.manager.tasklistRuntimeGuard(userID, expected)
	origin := commandEventOrigin{
		userID: userID, rootType: "internal_event", rootID: rootID.String(),
		chainID: rootID.String(), history: []string{},
		runtimeIdentity: &expected, runtimeGuard: guard,
	}
	marked := context.WithValue(ctx, commandEventOriginKey{}, origin)
	if watchCtx == nil || watchCtx.Err() != nil {
		return fmt.Errorf("runtime de comandos indisponível")
	}
	return s.manager.publishTasklistEvent(marked, name, payload, eventctx.Provenance{Source: "user", ChainID: rootID.String(), ChainHistory: []string{}})
}

func (m *Manager) publishTasklistEvent(ctx context.Context, name string, payload map[string]any, provenance eventctx.Provenance) error {
	if !m.HasDomainListener(name) {
		return nil
	}
	m.eventBus.Publish(context.WithoutCancel(ctx), name, tasklistPayloadWithoutAuthority(payload, provenance))
	return nil
}

func tasklistPayloadWithoutAuthority(payload map[string]any, provenance eventctx.Provenance) map[string]any {
	out := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		if key == "root_origin_type" || key == "root_origin_id" || key == "_source" || key == "_source_job_id" || key == "command_chain_history" || strings.HasPrefix(key, "_chain") {
			continue
		}
		out[key] = value
	}
	out["_source"] = provenance.Source
	out["_source_job_id"] = provenance.SourceJobID
	if provenance.ChainID != "" {
		out["_chain_id"] = provenance.ChainID
	}
	if provenance.ChainHistory != nil {
		out["_chain_history"] = append([]string{}, provenance.ChainHistory...)
	}
	return out
}

func tasklistPublicProvenanceFromOrigin(origin commandEventOrigin, userID string) eventctx.Provenance {
	if origin.userID != userID {
		return eventctx.Provenance{Source: "user"}
	}
	source, sourceJobID := "user", ""
	if len(origin.history) > 0 {
		source, sourceJobID = "job", origin.history[len(origin.history)-1]
	}
	var history []string
	if origin.history != nil {
		history = append([]string{}, origin.history...)
	}
	return eventctx.Provenance{
		Source: source, SourceJobID: sourceJobID, ChainID: origin.chainID, ChainHistory: history,
	}
}

func isTasklistDomainEvent(name string) bool {
	return strings.HasPrefix(name, "tasklist.") && DomainEventSchema(name) != nil
}

func (m *Manager) captureTasklistRuntimeIdentity(ctx context.Context, userID string) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
	if m == nil || m.cfg.CommandRuntimeIdentity == nil {
		return commandjobactivation.RuntimeIdentity{}, nil, nil, fmt.Errorf("runtime de comandos não configurado")
	}
	identity, watchCtx, release, err := m.cfg.CommandRuntimeIdentity(ctx)
	if err != nil || watchCtx == nil || watchCtx.Err() != nil || identity.UserID != userID || identity.AuthContextType != "local_session" || identity.AuthContextID == "" || identity.AuthGeneration == "" || identity.SecurityGeneration == "" {
		if release != nil {
			release()
		}
		return commandjobactivation.RuntimeIdentity{}, nil, nil, fmt.Errorf("runtime de comandos não autenticado")
	}
	return identity, watchCtx, release, nil
}

func (m *Manager) tasklistRuntimeGuard(userID string, expected commandjobactivation.RuntimeIdentity) func(context.Context, commandjobactivation.RuntimeIdentity) bool {
	return func(ctx context.Context, _ commandjobactivation.RuntimeIdentity) bool {
		if ctx == nil {
			return false
		}
		currentUser, err := database.RequireUserID(ctx)
		if err != nil || currentUser != userID || m == nil || m.cfg.CommandRuntimeIdentity == nil {
			return false
		}
		current, watchCtx, release, err := m.cfg.CommandRuntimeIdentity(context.WithoutCancel(ctx))
		valid := err == nil && watchCtx != nil && watchCtx.Err() == nil && sameTasklistRuntimeIdentity(expected, current) && current.UserID == currentUser
		if release != nil {
			release()
		}
		return valid
	}
}

func sameTasklistRuntimeIdentity(a, b commandjobactivation.RuntimeIdentity) bool {
	return a.UserID == b.UserID && a.AuthContextType == b.AuthContextType && a.AuthContextID == b.AuthContextID && a.AuthGeneration == b.AuthGeneration && a.SecurityGeneration == b.SecurityGeneration
}

// Compara com a captura efetivamente anexada ao run, não com uma segunda
// consulta que poderia observar outra geração entre a verificação e queued.
func commandEventRuntimeMatches(ctx context.Context, current commandjobactivation.RuntimeIdentity) bool {
	if ctx == nil {
		return false
	}
	origin, ok := ctx.Value(commandEventOriginKey{}).(commandEventOrigin)
	if !ok || origin.runtimeIdentity == nil {
		return true
	}
	if origin.runtimeGuard == nil {
		return false
	}
	return sameTasklistRuntimeIdentity(*origin.runtimeIdentity, current)
}
