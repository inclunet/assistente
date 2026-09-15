package commandexecution

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandjson"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

// EnvelopeCandidate contém somente a intenção reapresentável. Identidade,
// origem física, epochs, versões e contexto são sempre derivados pelo host.
type EnvelopeCandidate struct {
	InvocationID  string          `json:"invocation_id"`
	CorrelationID string          `json:"correlation_id"`
	CommandID     string          `json:"command_id,omitempty"`
	Arguments     json.RawMessage `json:"arguments"`
	TriggerType   string          `json:"trigger_type,omitempty"`
	TriggerSpec   json.RawMessage `json:"trigger_spec,omitempty"`
	WorkspaceID   *string         `json:"workspace_id,omitempty"`
}

type EnvelopeResolution struct {
	Mode       commandcontract.ResolutionMode
	CommandID  string
	Arguments  json.RawMessage
	BindingIDs []string
	LayerRefs  []string
}

// EnvelopeConfig é uma porta interna do bootstrap, nunca preenchida por Wails.
// Snapshot/Resolve/Authorize/Actor são consultas locais curtas sob o gate do
// EpochService. Snapshot reconsulta fontes, não aceita o snapshot de cliente.
// Snapshot deve recusar cofre/SO bloqueado, sessão em transição, origem física
// sem ownership ou capabilities indisponíveis. Não existe fallback permissivo.
// O host publica mudanças de mapa/configuração nesse MESMO gate.
// Context providers também precisam ser locais/não bloqueantes na admissão;
// um roundtrip à UI/rede não pode ser instalado como provider do gate.
type EnvelopeConfig struct {
	// Identity habilita origens não locais por portas internas completas. Quando
	// presente, nenhum callback local ou fallback é consultado.
	Identity        *EnvelopeIdentityPorts
	Snapshot        func(context.Context, auth.LocalSessionPrincipal, EnvelopeCandidate) (commandcontract.Envelope, error)
	Resolve         func(context.Context, auth.LocalSessionPrincipal, EnvelopeCandidate, commandcontract.Envelope) (EnvelopeResolution, error)
	Authorize       func(context.Context, auth.LocalSessionPrincipal, commandcontract.Envelope, commandcatalog.Definition) error
	AuthorizeLookup func(context.Context, auth.LocalSessionPrincipal, commandledger.FullRecord) error
	Actor           func(context.Context, auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error)
	Context         *commandcontext.FactBus
	Decisions       *commanddecision.Store
	DecisionBody    func(commandcatalog.Definition, commandcontract.Envelope) (string, error)
	DecisionTTL     time.Duration
	// AwaitQueue é a espera do scheduler, fora do gate; não inicia efeitos.
	// nil representa retirada imediata da fila lógica persistida.
	AwaitQueue func(context.Context, commandcontract.Envelope) error
}

type preparedEnvelope struct {
	candidate            EnvelopeCandidate
	epoch                commandsecurity.EpochSnapshot
	principal            auth.LocalSessionPrincipal
	identity             EnvelopeAuthenticatedIdentity
	envelope             commandcontract.Envelope
	definition           commandcatalog.Definition
	mode                 commandcontract.ResolutionMode
	proof                commandcontext.FactProof
	scope                commandcontext.Scope
	owner                commandledger.FullOwnership
	layerRefs            []string
	hostProvenance       *json.RawMessage
	inputFingerprint     string
	argumentsFingerprint string
	expires              time.Time
	denied               bool
}

func canonicalCandidate(candidate EnvelopeCandidate) (EnvelopeCandidate, error) {
	if !validID(candidate.InvocationID) || !validID(candidate.CorrelationID) ||
		(candidate.CommandID == "") == (candidate.TriggerType == "") {
		return EnvelopeCandidate{}, ErrInvalidRequest
	}
	if candidate.CommandID != "" && (!commandIDPattern.MatchString(candidate.CommandID) || len(candidate.TriggerSpec) != 0) {
		return EnvelopeCandidate{}, ErrInvalidRequest
	}
	if candidate.TriggerType != "" && len(candidate.TriggerSpec) == 0 {
		return EnvelopeCandidate{}, ErrInvalidRequest
	}
	if len(candidate.Arguments) == 0 {
		candidate.Arguments = json.RawMessage(`{}`)
	}
	args, err := commandjson.Canonicalize(candidate.Arguments)
	if err != nil || len(args) == 0 || args[0] != '{' {
		return EnvelopeCandidate{}, ErrInvalidRequest
	}
	canonical, err := commandjson.Marshal(candidate)
	if err != nil {
		return EnvelopeCandidate{}, ErrInvalidRequest
	}
	var detached EnvelopeCandidate
	if json.Unmarshal(canonical, &detached) != nil {
		return EnvelopeCandidate{}, ErrInvalidRequest
	}
	// Não afrouxar version:1 inteiro de trigger através da canonicalização.
	if candidate.TriggerType != "" {
		var doc map[string]json.RawMessage
		if json.Unmarshal(candidate.TriggerSpec, &doc) != nil || string(doc["version"]) != "1" {
			return EnvelopeCandidate{}, ErrInvalidRequest
		}
	}
	return detached, nil
}

func (s *Service) inputFingerprint(ctx context.Context, c EnvelopeCandidate, o commandledger.FullOwnership, version string) (string, error) {
	if !keyVersionPattern.MatchString(version) {
		return "", ErrInvalidRequest
	}
	key, err := s.config.Keys(ctx, "command-request-hmac:"+version)
	if err != nil || len(key) < 32 {
		return "", ErrExecution
	}
	copyKey := append([]byte(nil), key...)
	defer clear(copyKey)
	raw, err := commandjson.Marshal(map[string]any{"version": 1, "candidate": c, "owner": o, "source": s.config.Source})
	if err != nil {
		return "", ErrInvalidRequest
	}
	return commandjson.HMAC(copyKey, "assistente.command.input.v1", raw)
}

func sameEnvelopeOwner(a, b commandledger.FullOwnership) bool {
	return (a.UserID == nil) == (b.UserID == nil) && (a.UserID == nil || *a.UserID == *b.UserID) && a.AuthContextType == b.AuthContextType && a.AuthContextID == b.AuthContextID && a.ActorType == b.ActorType && a.ActorID == b.ActorID
}

func sameEnvelopeContext(a, b EnvelopeAuthenticatedIdentity) bool {
	return a.ContextPrincipal == b.ContextPrincipal
}

func (s *Service) bindHostEnvelope(ctx context.Context, identity EnvelopeAuthenticatedIdentity, c EnvelopeCandidate, epoch commandsecurity.EpochSnapshot) (commandcontract.Envelope, error) {
	o := identity.Ownership
	e, err := s.snapshotEnvelope(ctx, identity, c)
	if err != nil {
		return e, ErrStale
	}
	// A porta instala somente estado autoritativo. Cliente não escolhe auth.
	e.Version = commandcontract.EnvelopeVersion
	e.InvocationID = c.InvocationID
	e.CorrelationID = c.CorrelationID
	e.UserID = o.UserID
	e.AuthContextType = o.AuthContextType
	e.AuthContextID = o.AuthContextID
	e.SessionID = nil
	if identity.WireSessionID != nil {
		wireSessionID := *identity.WireSessionID
		e.SessionID = &wireSessionID
	}
	e.ActorType = o.ActorType
	e.ActorID = o.ActorID
	e.AuthGeneration = epoch.AuthGeneration
	e.SecurityGeneration = epoch.SecurityGeneration
	e.RequestFingerprint = nil
	e.RequestFingerprintVersion = nil
	e.AuthorizationDecisionID = nil
	e.ReceivedAt = s.config.Now().UTC()
	e.ClientRequestedAt = nil
	e.CommandID = nil
	e.Arguments = nil
	e.BindingIDs = []string{}
	e.ContextVersion = nil
	e.ContextCapturedAtByProvider = nil
	source := commandcontract.SourceType(s.config.Source)
	e.SourceType = &source
	if c.WorkspaceID != nil && (e.WorkspaceID == nil || *c.WorkspaceID != *e.WorkspaceID) {
		return e, ErrDenied
	}
	if c.CommandID != "" {
		e.CommandID = &c.CommandID
		e.TriggerType = nil
		e.ObservedTriggerType = nil
		e.TriggerSpec = nil
	} else {
		e.TriggerType = &c.TriggerType
		spec := append(json.RawMessage(nil), c.TriggerSpec...)
		e.TriggerSpec = &spec
	}
	args := append(json.RawMessage(nil), c.Arguments...)
	e.Arguments = &args
	// O snapshot pode conter ponteiros reutilizados pelo host. A invocação fixa
	// sua própria cópia; atualizações posteriores só entram pela revalidação.
	return detachedEnvelope(e)
}

func (s *Service) prepareEnvelope(ctx context.Context, token string, c EnvelopeCandidate) (preparedEnvelope, *commandledger.FullRecord, error) {
	var p preparedEnvelope
	p.candidate = c
	var prior *commandledger.FullRecord
	identity, principal, epoch, err := s.captureEnvelopeIdentity(ctx, token)
	if err != nil {
		return p, nil, err
	}
	p.identity, p.principal, p.owner = identity, principal, identity.Ownership
	p.epoch = epoch
	// Consultar antes de resolver: o mesmo ID nunca vira uma nova intenção após
	// mudar configuração. O HMAC de ingresso permite comparar sem guardar args.
	existing, err := s.config.Store.GetEnvelopeByID(ctx, p.owner, c.InvocationID)
	version := s.config.KeyVersion
	if err == nil {
		version = existing.RequestFingerprintVersion
		prior = &existing
	} else if !errors.Is(err, commandledger.ErrNotFound) {
		return p, nil, err
	}
	p.inputFingerprint, err = s.inputFingerprint(ctx, c, p.owner, version)
	if err != nil {
		return p, nil, err
	}
	if prior != nil {
		if prior.InputFingerprint == "" || subtle.ConstantTimeCompare([]byte(prior.InputFingerprint), []byte(p.inputFingerprint)) != 1 {
			return p, nil, commandledger.ErrConflict
		}
		if err := s.config.Epochs.Admit(ctx, epoch, func(ctx context.Context) error {
			current, _, err := s.authenticateEnvelope(ctx, token)
			if err != nil || !sameEnvelopeOwner(current.Ownership, p.owner) || !sameEnvelopeContext(current, p.identity) {
				return ErrDenied
			}
			if s.authorizeEnvelopeLookup(ctx, current, *prior) != nil {
				return ErrDenied
			}
			return nil
		}, func() error { return nil }); err != nil {
			return p, nil, err
		}
		return p, prior, nil
	}
	err = s.config.Epochs.Admit(ctx, epoch, func(ctx context.Context) error {
		current, _, err := s.authenticateEnvelope(ctx, token)
		if err != nil || !sameEnvelopeOwner(current.Ownership, p.owner) || !sameEnvelopeContext(current, p.identity) {
			return ErrDenied
		}
		p.envelope, err = s.bindHostEnvelope(ctx, current, c, epoch)
		if err != nil {
			return err
		}
		p.mode = commandcontract.ResolutionExecute
		if c.CommandID == "" {
			resolution, err := s.resolveEnvelope(ctx, current, c, p.envelope)
			if err != nil {
				p.mode = commandcontract.ResolutionDenied
				p.denied = true
				return nil
			}
			p.mode = resolution.Mode
			p.envelope.BindingIDs = append([]string{}, resolution.BindingIDs...)
			p.layerRefs = append([]string{}, resolution.LayerRefs...)
			if resolution.CommandID != "" {
				id := resolution.CommandID
				p.envelope.CommandID = &id
			}
			if len(resolution.Arguments) != 0 {
				args := append(json.RawMessage(nil), resolution.Arguments...)
				p.envelope.Arguments = &args
			}
		}
		return nil
	}, func() error { return nil })
	if err != nil {
		return p, nil, err
	}
	if p.owner.UserID == nil {
		if p.envelope.AuthContextType != commandcontract.AuthSystem || p.envelope.SourceType == nil || *p.envelope.SourceType != commandcontract.SourceSystem {
			p.denied = true
			p.mode = commandcontract.ResolutionDenied
		}
	}
	if p.owner.UserID != nil {
		p.scope = commandcontext.Scope{UserID: *p.owner.UserID, AuthContextID: p.owner.AuthContextID, WorkspaceID: p.envelope.WorkspaceID}
	}
	// System é deliberadamente sem escopo de workspace/surface/trigger. Ele
	// só pode alcançar comandos internos read-only com contexto.none.
	if p.owner.AuthContextType == commandcontract.AuthSystem && (p.envelope.WorkspaceID != nil || p.envelope.SurfaceType != nil || p.envelope.SurfaceID != nil || p.envelope.TriggerType != nil || p.envelope.ObservedTriggerType != nil) {
		p.denied = true
		p.mode = commandcontract.ResolutionDenied
	}
	p.expires = p.envelope.ReceivedAt.Add(s.config.Retention)
	if p.envelope.SourceReplayDeadline != nil {
		p.expires = *p.envelope.SourceReplayDeadline
	}
	if p.envelope.CommandID != nil {
		definition, ok := s.config.Registry.Lookup(*p.envelope.CommandID)
		if !ok {
			p.denied = true
			p.mode = commandcontract.ResolutionDenied
		} else {
			p.definition = definition
			if definition.Context.None {
				p.envelope.ForegroundSnapshot = nil
			}
			args, err := definition.ValidateArguments(*p.envelope.Arguments)
			if err != nil {
				p.denied = true
				p.mode = commandcontract.ResolutionDenied
			} else {
				raw := json.RawMessage(args)
				p.envelope.Arguments = &raw
			}
			if !p.denied {
				if p.owner.UserID == nil && (!definition.Context.None || definition.Effect != commandcatalog.Read || definition.Decision != commandcatalog.NoDecision || definition.MutatesEffectiveCapability || definition.HasMutableTarget || definition.HandlerClassification != commandcatalog.HandlerInternal) {
					p.denied = true
					p.mode = commandcontract.ResolutionDenied
				} else if p.owner.UserID == nil {
					// Não há facts para o contexto system; o bypass abaixo só vale
					// para esta combinação fechada, nunca para workspace/surface.
					p.proof = commandcontext.FactProof{}
				} else {
					p.proof, err = s.config.Envelope.Context.Capture(ctx, p.scope, definition.Context)
					if err != nil {
						p.denied = true
						p.mode = commandcontract.ResolutionDenied
					} else if !definition.Context.None {
						version := p.proof.ContextVersion()
						p.envelope.ContextVersion = &version
						if captured := p.proof.ProviderCapturedAt(); len(captured) != 0 {
							p.envelope.ContextCapturedAtByProvider = &captured
						}
					}
				}
			}
		}
	}
	if p.mode == commandcontract.ResolutionExecute && !p.denied && p.definition.ID != "" {
		p.hostProvenance = cloneRawMessage(p.envelope.Provenance)
		p.envelope, err = prepareCommandChain(p.envelope, p.definition, p.layerRefs)
		if err != nil {
			p.denied = true
			p.mode = commandcontract.ResolutionDenied
		}
	}
	if p.mode == commandcontract.ResolutionSuppress {
		p.envelope.CommandID = nil
		p.envelope.ForegroundSnapshot = nil
		args := json.RawMessage(`{}`)
		p.envelope.Arguments = &args
		p.definition = commandcatalog.Definition{ID: "command.suppressed", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, AllowedSources: []commandcatalog.Source{s.config.Source}, Context: commandcatalog.ContextPolicy{None: true}}
	}
	if p.definition.ID == "" {
		p.mode = commandcontract.ResolutionDenied
		p.denied = true
		p.envelope, err = p.envelope.SignRefusal(ctx, version, commandcontract.FingerprintKeyProvider(s.config.Keys))
	} else {
		p.envelope, err = p.envelope.SignResolved(ctx, p.mode, p.definition, version, commandcontract.FingerprintKeyProvider(s.config.Keys))
	}
	if err != nil {
		return p, nil, err
	}
	p.argumentsFingerprint, err = p.envelope.ArgumentsHMAC(ctx, version, commandcontract.FingerprintKeyProvider(s.config.Keys))
	return p, nil, err
}

func (s *Service) checkEnvelope(ctx context.Context, token string, p preparedEnvelope) error {
	identity, _, err := s.authenticateEnvelope(ctx, token)
	if err != nil || !sameEnvelopeOwner(identity.Ownership, p.owner) || !sameEnvelopeContext(identity, p.identity) {
		return ErrStale
	}
	current, err := s.bindHostEnvelope(ctx, identity, p.candidate, p.epoch)
	if err != nil {
		return err
	}
	if current.RegistryVersion != p.envelope.RegistryVersion || current.RegistryVersion != s.config.RegistryVersion ||
		!sameString(current.GlobalConfigGeneration, p.envelope.GlobalConfigGeneration) || !sameString(current.WorkspaceConfigGeneration, p.envelope.WorkspaceConfigGeneration) ||
		!sameString(current.ActiveLayersGeneration, p.envelope.ActiveLayersGeneration) || !sameString(current.WorkspaceID, p.envelope.WorkspaceID) ||
		!sameString(current.SurfaceType, p.envelope.SurfaceType) || !sameString(current.SurfaceID, p.envelope.SurfaceID) || !sameString(current.SurfaceSnapshotVersion, p.envelope.SurfaceSnapshotVersion) {
		return ErrStale
	}
	comparisonEnvelope := p.envelope
	comparisonEnvelope.Provenance = p.hostProvenance
	want, err := hostEnvelopeIdentity(comparisonEnvelope)
	if err != nil {
		return ErrStale
	}
	got, err := hostEnvelopeIdentity(current)
	if err != nil || !bytes.Equal(want, got) {
		return ErrStale
	}
	if p.candidate.CommandID == "" {
		resolution, err := s.resolveEnvelope(ctx, identity, p.candidate, current)
		if err != nil || resolution.Mode != p.mode {
			return ErrStale
		}
		id := ""
		if p.envelope.CommandID != nil {
			id = *p.envelope.CommandID
		}
		if resolution.CommandID != id {
			return ErrStale
		}
		if len(resolution.Arguments) == 0 {
			resolution.Arguments = json.RawMessage(`{}`)
		}
		arguments, err := commandjson.Canonicalize(resolution.Arguments)
		if err != nil || p.envelope.Arguments == nil || !bytes.Equal(arguments, *p.envelope.Arguments) {
			return ErrStale
		}
		bindings, err := commandjson.Marshal(append([]string{}, resolution.BindingIDs...))
		wantBindings, werr := commandjson.Marshal(p.envelope.BindingIDs)
		if err != nil || werr != nil || !bytes.Equal(bindings, wantBindings) {
			return ErrStale
		}
		layers, err := commandjson.Marshal(append([]string{}, resolution.LayerRefs...))
		wantLayers, werr := commandjson.Marshal(append([]string{}, p.layerRefs...))
		if err != nil || werr != nil || !bytes.Equal(layers, wantLayers) {
			return ErrStale
		}
	}
	if p.mode != commandcontract.ResolutionExecute {
		return nil
	}
	if p.definition.Availability.Status != commandcatalog.Available || !p.definition.AllowsSource(s.config.Source) {
		return ErrDenied
	}
	if p.owner.UserID != nil {
		if err := s.config.Envelope.Context.Revalidate(ctx, p.scope, p.definition.Context, p.proof, s.config.Now()); err != nil {
			return ErrStale
		}
	} else if p.definition.Context.None && p.definition.Effect == commandcatalog.Read && p.definition.Decision == commandcatalog.NoDecision && !p.definition.HasMutableTarget && !p.definition.MutatesEffectiveCapability && p.definition.HandlerClassification == commandcatalog.HandlerInternal {
		// System não possui Scope/FactProof. Este bypass é restrito ao
		// comando interno read-only e context.none validado na preparação.
	} else {
		return ErrDenied
	}
	copyEnvelope, err := detachedEnvelope(p.envelope)
	if err != nil {
		return ErrStale
	}
	return s.authorizeEnvelope(ctx, identity, copyEnvelope, p.definition)
}

// Compara todo estado derivado pelo host que seleciona um alvo ou autoridade.
// Timestamps locais e foreground event_snapshot não substituem a prova inicial.
func hostEnvelopeIdentity(e commandcontract.Envelope) ([]byte, error) {
	return commandjson.Marshal(map[string]any{
		"registry": e.RegistryVersion, "global": e.GlobalConfigGeneration, "workspace_config": e.WorkspaceConfigGeneration, "layers": e.ActiveLayersGeneration,
		"workspace": e.WorkspaceID, "conversation": e.ConversationID, "turn": e.TurnID,
		"surface_type": e.SurfaceType, "surface_id": e.SurfaceID, "surface_version": e.SurfaceSnapshotVersion,
		"source": e.SourceType, "observer": e.ObserverType, "observed_trigger": e.ObservedTriggerType, "instance": e.SourceInstanceID, "event": e.SourceEventID,
		"occurred": e.SourceOccurredAt, "replay_epoch": e.SourceReplayPolicyGeneration, "replay_deadline": e.SourceReplayDeadline,
		"source_profile": e.SourceProfileSlug, "target_profile": e.TargetProfileSlug, "delegation": e.DelegationFingerprint, "grant": e.GrantGeneration,
		"job": e.JobID, "job_slug": e.JobSlug, "job_definition": e.JobDefinitionFingerprint, "run": e.RunID, "provenance": e.Provenance,
	})
}

func sameString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func cloneRawMessage(value *json.RawMessage) *json.RawMessage {
	if value == nil {
		return nil
	}
	copyValue := append(json.RawMessage(nil), (*value)...)
	return &copyValue
}

// ExecuteEnvelope usa o mesmo Service e as mesmas portas ledger/epoch dos
// comandos legados. A espera da fila, decisão e resultado fica fora do gate.
func (s *Service) ExecuteEnvelope(ctx context.Context, token string, candidate EnvelopeCandidate) (record commandledger.FullRecord, err error) {
	if s == nil || !s.complete || s.config.Envelope == nil || ctx == nil {
		return record, ErrInvalidRequest
	}
	ctx, releaseOperation, err := s.lifecycle.enter(ctx)
	if err != nil {
		return record, err
	}
	defer releaseOperation()
	candidate, err = canonicalCandidate(candidate)
	if err != nil {
		return record, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.config.ExecutionTimeout)
	defer cancel()
	var p preparedEnvelope
	var prior *commandledger.FullRecord
	if err = protect(func() error { var e error; p, prior, e = s.prepareEnvelope(ctx, token, candidate); return e }); err != nil {
		return record, err
	}
	if prior != nil {
		return *prior, nil
	}
	risk := string(p.definition.Risk)
	if risk == "" {
		risk = "low"
	}
	var reservation commandledger.EnvelopeReservation
	reserve := func(stale bool) error {
		var e error
		reservation, e = s.config.Store.ReserveEnvelope(ctx, commandledger.EnvelopeRequest{Envelope: p.envelope, Mode: p.mode, ArgumentsFingerprint: p.argumentsFingerprint, InputFingerprint: p.inputFingerprint, ExpiresAt: p.expires, Risk: risk, RejectedStale: stale})
		return e
	}
	// Consumo do tombstone e sua revalidação compartilham o gate; nenhuma
	// mutação pode entrar entre o check e a reserva terminal.
	if p.mode == commandcontract.ResolutionSuppress {
		attempted := false
		err = protect(func() error {
			return s.config.Epochs.Admit(ctx, p.epoch, func(ctx context.Context) error { return s.checkEnvelope(ctx, token, p) }, func() error { attempted = true; return reserve(false) })
		})
		if err != nil && !attempted {
			err = reserve(true)
		}
	} else {
		err = reserve(false)
	}
	if err != nil {
		return record, err
	}
	record = reservation.Record
	if !reservation.Created || p.mode != commandcontract.ResolutionExecute || record.Status != commandledger.Evaluating {
		return record, nil
	}
	status := commandledger.Evaluating
	finish := func(to commandledger.Status) error {
		cleanup, release := context.WithTimeout(context.WithoutCancel(ctx), s.config.FinalizationTimeout)
		defer release()
		changed, e := s.config.Store.CompareAndSwapEnvelope(cleanup, p.owner, p.envelope.InvocationID, status, to)
		if e != nil {
			return e
		}
		if !changed {
			return commandledger.ErrInconsistent
		}
		status = to
		record, e = s.config.Store.GetEnvelopeByID(cleanup, p.owner, p.envelope.InvocationID)
		return e
	}
	defer func() {
		if recover() != nil {
			err = ErrExecution
			switch status {
			case commandledger.Running:
				_ = finish(commandledger.OutcomeUnknown)
			case commandledger.Evaluating, commandledger.Queued:
				_ = finish(commandledger.Failed)
			}
		}
	}()
	// Nunca abrir decisão para solicitação cuja política/contexto já recusam.
	if checkErr := s.config.Epochs.Admit(ctx, p.epoch, func(ctx context.Context) error { return s.checkEnvelope(ctx, token, p) }, func() error { return nil }); checkErr != nil {
		to := commandledger.CancelledStale
		if errors.Is(checkErr, ErrDenied) {
			to = commandledger.Denied
		}
		err = finish(to)
		return record, err
	}
	var decision *commanddecision.Request
	interactive := p.definition.Decision == commandcatalog.Interactive ||
		(p.owner.ActorType != commandcontract.ActorUser && p.definition.MutatesEffectiveCapability)
	if interactive {
		if s.config.Source == commandcatalog.CLI || s.config.Source == commandcatalog.Event || s.config.Source == commandcatalog.System || p.owner.AuthContextType != commandcontract.AuthLocalSession || s.config.Envelope.Decisions == nil || s.config.Envelope.DecisionBody == nil {
			err = finish(commandledger.Denied)
			return record, err
		}
		copyEnvelope, e := detachedEnvelope(p.envelope)
		if e != nil {
			err = finish(commandledger.Denied)
			return record, err
		}
		body, e := s.config.Envelope.DecisionBody(p.definition, copyEnvelope)
		if e != nil {
			err = finish(commandledger.Denied)
			return record, err
		}
		deadline := s.config.Now().Add(s.config.Envelope.DecisionTTL)
		if deadline.After(p.expires) {
			deadline = p.expires
		}
		userID, sessionID := "", ""
		if p.owner.UserID != nil {
			userID = *p.owner.UserID
		}
		if p.identity.WireSessionID != nil {
			sessionID = *p.identity.WireSessionID
		}
		request := commanddecision.Request{SubjectType: "invocation", DecisionID: uuid.Must(uuid.NewV7()).String(), MutationID: p.envelope.InvocationID, UserID: userID, SessionID: sessionID, Fingerprint: *p.envelope.RequestFingerprint, AuthGeneration: p.epoch.AuthGeneration, SecurityGeneration: p.epoch.SecurityGeneration, ExpiresAt: deadline, Body: body}
		request.Destructive = p.definition.Effect == commandcatalog.Destructive
		watched, release, e := s.config.Epochs.WatchEpoch(ctx, p.epoch)
		if e != nil {
			err = finish(commandledger.CancelledStale)
			return record, err
		}
		state, e := s.config.Envelope.Decisions.Decide(watched, request)
		release()
		if e != nil || state != commanddecision.Accepted {
			err = finish(commandledger.Denied)
			return record, err
		}
		decision = &request
	}
	err = s.config.Epochs.Admit(ctx, p.epoch, func(ctx context.Context) error { return s.checkEnvelope(ctx, token, p) }, func() error {
		var changed bool
		var e error
		if decision != nil {
			changed, e = s.config.Store.CompareAndSwapEnvelopeWithDecision(ctx, p.owner, p.envelope.InvocationID, *decision, s.config.Envelope.Decisions)
		} else {
			changed, e = s.config.Store.CompareAndSwapEnvelope(ctx, p.owner, p.envelope.InvocationID, commandledger.Evaluating, commandledger.Queued)
		}
		if e != nil {
			return e
		}
		if !changed {
			return commandledger.ErrInconsistent
		}
		status = commandledger.Queued
		return nil
	})
	if err != nil {
		to := commandledger.CancelledStale
		if errors.Is(err, ErrDenied) {
			to = commandledger.Denied
		}
		err = finish(to)
		return record, err
	}
	if s.config.Envelope.AwaitQueue != nil {
		copyEnvelope, e := detachedEnvelope(p.envelope)
		if e != nil {
			err = finish(commandledger.CancelledStale)
			return record, err
		}
		if e := s.config.Envelope.AwaitQueue(ctx, copyEnvelope); e != nil {
			err = finish(commandledger.CancelledStale)
			return record, err
		}
	}
	var handle ExecutionHandle
	var runCtx context.Context
	release, admitErr := s.config.Epochs.AdmitExecution(ctx, p.epoch, func(ctx context.Context) error { return s.checkEnvelope(ctx, token, p) }, func(executionCtx context.Context) error {
		changed, e := s.config.Store.CompareAndSwapEnvelope(ctx, p.owner, p.envelope.InvocationID, commandledger.Queued, commandledger.Running)
		if e != nil {
			return e
		}
		if !changed {
			return commandledger.ErrInconsistent
		}
		status = commandledger.Running
		runCtx = executionCtx
		copyEnvelope, cloneErr := detachedEnvelope(p.envelope)
		if cloneErr != nil {
			return cloneErr
		}
		return s.lifecycle.handoff(executionCtx, func() error {
			handle, e = s.config.Handlers[*p.envelope.CommandID].Start(executionCtx, Invocation{ID: p.envelope.InvocationID, CorrelationID: p.envelope.CorrelationID, CommandID: *p.envelope.CommandID, Principal: p.principal, Source: s.config.Source, Envelope: &copyEnvelope})
			return e
		})
	})
	if release != nil {
		defer release()
	}
	if admitErr != nil {
		to := commandledger.CancelledStale
		if status == commandledger.Running {
			safeCancel(handle.Cancel)
			to = commandledger.OutcomeUnknown
		}
		err = finish(to)
		return record, err
	}
	result := awaitEnvelopeOutcome(runCtx, handle, p.definition)
	err = finish(result)
	return record, err
}

func detachedEnvelope(e commandcontract.Envelope) (commandcontract.Envelope, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return commandcontract.Envelope{}, ErrExecution
	}
	var result commandcontract.Envelope
	if json.Unmarshal(raw, &result) != nil {
		return commandcontract.Envelope{}, ErrExecution
	}
	return result, nil
}

func awaitEnvelopeOutcome(ctx context.Context, handle ExecutionHandle, definition commandcatalog.Definition) commandledger.Status {
	if handle.ID == "" || handle.Done == nil || handle.Cancel == nil || ctx == nil || ctx.Err() != nil {
		safeCancel(handle.Cancel)
		return commandledger.OutcomeUnknown
	}
	select {
	case <-ctx.Done():
		safeCancel(handle.Cancel)
		return commandledger.OutcomeUnknown
	case outcome, ok := <-handle.Done:
		if !ok {
			safeCancel(handle.Cancel)
			return commandledger.OutcomeUnknown
		}
		if outcome.Status == commandledger.Succeeded {
			if _, err := definition.ValidateResult(outcome.Result); err != nil {
				return commandledger.OutcomeUnknown
			}
			return outcome.Status
		}
		if outcome.Status == commandledger.Failed || outcome.Status == commandledger.Cancelled {
			return outcome.Status
		}
		return commandledger.OutcomeUnknown
	}
}

func (s *Service) GetEnvelopeInvocation(ctx context.Context, token, id string) (record commandledger.FullRecord, err error) {
	if s == nil || !s.complete || ctx == nil || !validID(id) {
		return record, ErrInvalidRequest
	}
	err = protect(func() error {
		identity, _, epoch, e := s.captureEnvelopeIdentity(ctx, token)
		if e != nil {
			return e
		}
		return s.config.Epochs.Admit(ctx, epoch, func(ctx context.Context) error {
			current, _, err := s.authenticateEnvelope(ctx, token)
			if err != nil || !sameEnvelopeOwner(current.Ownership, identity.Ownership) || !sameEnvelopeContext(current, identity) {
				return ErrDenied
			}
			record, err = s.config.Store.GetEnvelopeByID(ctx, current.Ownership, id)
			if err != nil {
				return err
			}
			if err := s.authorizeEnvelopeLookup(ctx, current, record); err != nil {
				return ErrDenied
			}
			return nil
		}, func() error { return nil })
	})
	if err != nil {
		return commandledger.FullRecord{}, err
	}
	return record, nil
}
