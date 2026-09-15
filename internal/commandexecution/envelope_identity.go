package commandexecution

import (
	"context"
	"strings"
	"unicode/utf8"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

// EnvelopeAuthenticatedIdentity é uma projeção curta produzida somente por
// uma porta de confiança do host. Ownership é a identidade wire/persistida;
// ContextPrincipal é a chave autoritativa usada pelo mapa interno de epochs.
// Essas duas identidades deliberadamente não são intercambiáveis.
type EnvelopeAuthenticatedIdentity struct {
	Ownership        commandledger.FullOwnership
	ContextPrincipal commandsecurity.ContextPrincipal
	// WireSessionID só é permitido para AuthLocalSession. Para os demais
	// contextos, o envelope não recebe uma sessão local sintética.
	WireSessionID *string
}

// EnvelopeIdentityPorts são portas internas autenticadas do host. Não são
// expostas por Wails nem aceitam owner/origem vindos do candidato.
// Todas as consultas são executadas no gate de admissão pelo pipeline.
type EnvelopeIdentityPorts struct {
	Authenticate    func(context.Context, string) (EnvelopeAuthenticatedIdentity, error)
	Snapshot        func(context.Context, commandledger.FullOwnership, EnvelopeCandidate) (commandcontract.Envelope, error)
	Resolve         func(context.Context, commandledger.FullOwnership, EnvelopeCandidate, commandcontract.Envelope) (EnvelopeResolution, error)
	Authorize       func(context.Context, commandledger.FullOwnership, commandcontract.Envelope, commandcatalog.Definition) error
	AuthorizeLookup func(context.Context, commandledger.FullOwnership, commandledger.FullRecord) error
}

func (p *EnvelopeIdentityPorts) complete() bool {
	return p != nil && p.Authenticate != nil && p.Snapshot != nil && p.Resolve != nil && p.Authorize != nil && p.AuthorizeLookup != nil
}

func (s *Service) authenticateEnvelope(ctx context.Context, token string) (EnvelopeAuthenticatedIdentity, auth.LocalSessionPrincipal, error) {
	if s == nil || s.config.Envelope == nil {
		return EnvelopeAuthenticatedIdentity{}, auth.LocalSessionPrincipal{}, ErrDenied
	}
	if ports := s.config.Envelope.Identity; ports != nil {
		if !ports.complete() {
			return EnvelopeAuthenticatedIdentity{}, auth.LocalSessionPrincipal{}, ErrInvalidConfiguration
		}
		identity, err := ports.Authenticate(ctx, token)
		if err != nil {
			return EnvelopeAuthenticatedIdentity{}, auth.LocalSessionPrincipal{}, ErrDenied
		}
		if err := validateEnvelopeIdentity(identity, s.config.Source); err != nil {
			return EnvelopeAuthenticatedIdentity{}, auth.LocalSessionPrincipal{}, ErrDenied
		}
		return identity, envelopePrincipal(identity), nil
	}

	p, err := s.config.Sessions.AuthenticateLocalAccess(ctx, token)
	if err != nil {
		return EnvelopeAuthenticatedIdentity{}, p, ErrDenied
	}
	actor, id, err := s.config.Envelope.Actor(ctx, p)
	if err != nil {
		return EnvelopeAuthenticatedIdentity{}, p, ErrDenied
	}
	user := p.UserID
	wireSessionID := p.SessionID
	return EnvelopeAuthenticatedIdentity{
		Ownership:        commandledger.FullOwnership{UserID: &user, AuthContextType: commandcontract.AuthLocalSession, AuthContextID: p.SessionID, ActorType: actor, ActorID: id},
		ContextPrincipal: commandsecurity.ContextPrincipal{UserID: p.UserID, Type: string(commandcontract.AuthLocalSession), ID: p.SessionID},
		WireSessionID:    &wireSessionID,
	}, p, nil
}

func envelopePrincipal(identity EnvelopeAuthenticatedIdentity) auth.LocalSessionPrincipal {
	p := auth.LocalSessionPrincipal{}
	if identity.Ownership.UserID != nil {
		p.UserID = *identity.Ownership.UserID
	}
	if identity.WireSessionID != nil {
		p.SessionID = *identity.WireSessionID
	}
	return p
}

func validateEnvelopeIdentity(identity EnvelopeAuthenticatedIdentity, source commandcatalog.Source) error {
	o := identity.Ownership
	if o.AuthContextID == "" || strings.TrimSpace(o.AuthContextID) != o.AuthContextID ||
		o.ActorID == "" || strings.TrimSpace(o.ActorID) != o.ActorID || !utf8.ValidString(o.AuthContextID) || !utf8.ValidString(o.ActorID) {
		return ErrDenied
	}
	if o.UserID != nil {
		id, err := uuid.Parse(*o.UserID)
		if err != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != *o.UserID {
			return ErrDenied
		}
	}
	switch o.AuthContextType {
	case commandcontract.AuthLocalSession, commandcontract.AuthExternalToken, commandcontract.AuthJobService, commandcontract.AuthSystem:
	default:
		return ErrDenied
	}
	switch o.ActorType {
	case commandcontract.ActorUser, commandcontract.ActorAgent, commandcontract.ActorAutomation:
	default:
		return ErrDenied
	}
	if o.AuthContextType == commandcontract.AuthSystem {
		if o.UserID != nil || o.ActorType == commandcontract.ActorUser || source != commandcatalog.System {
			return ErrDenied
		}
	} else if o.UserID == nil || source == commandcatalog.System {
		return ErrDenied
	}
	physical := source == commandcatalog.KeyboardLocal || source == commandcatalog.KeyboardGlobal || source == commandcatalog.StreamDeck
	if physical || source == commandcatalog.Palette || source == commandcatalog.UI || source == commandcatalog.Chat || source == commandcatalog.CLI || source == commandcatalog.Event {
		if physical && o.AuthContextType != commandcontract.AuthLocalSession {
			return ErrDenied
		}
	} else if source != commandcatalog.System {
		return ErrDenied
	}
	if o.AuthContextType != commandcontract.AuthLocalSession && identity.WireSessionID != nil {
		return ErrDenied
	}
	if o.AuthContextType == commandcontract.AuthLocalSession {
		if identity.WireSessionID == nil || *identity.WireSessionID != o.AuthContextID {
			return ErrDenied
		}
	}
	cp := identity.ContextPrincipal
	if cp.Type != string(o.AuthContextType) || cp.ID == "" || strings.TrimSpace(cp.ID) != cp.ID {
		return ErrDenied
	}
	if o.UserID == nil {
		if cp.UserID != "" || cp.Type != string(commandcontract.AuthSystem) {
			return ErrDenied
		}
	} else if cp.UserID != *o.UserID {
		return ErrDenied
	}
	return nil
}

func (s *Service) captureEnvelopeIdentity(ctx context.Context, token string) (EnvelopeAuthenticatedIdentity, auth.LocalSessionPrincipal, commandsecurity.EpochSnapshot, error) {
	if s.config.Envelope.Identity != nil {
		var identity EnvelopeAuthenticatedIdentity
		epoch, err := s.config.Epochs.CaptureContextAuthenticated(ctx, func(ctx context.Context) (commandsecurity.ContextPrincipal, error) {
			var err error
			identity, _, err = s.authenticateEnvelope(ctx, token)
			if err != nil {
				return commandsecurity.ContextPrincipal{}, err
			}
			return identity.ContextPrincipal, nil
		})
		return identity, envelopePrincipal(identity), epoch, err
	}
	var identity EnvelopeAuthenticatedIdentity
	var principal auth.LocalSessionPrincipal
	epoch, err := s.config.Epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		var err error
		identity, principal, err = s.authenticateEnvelope(ctx, token)
		if err != nil {
			return "", "", err
		}
		return principal.UserID, principal.SessionID, nil
	})
	return identity, principal, epoch, err
}

func (s *Service) resolveEnvelopePorts() *EnvelopeIdentityPorts {
	if s != nil && s.config.Envelope != nil {
		return s.config.Envelope.Identity
	}
	return nil
}

func (s *Service) snapshotEnvelope(ctx context.Context, identity EnvelopeAuthenticatedIdentity, candidate EnvelopeCandidate) (commandcontract.Envelope, error) {
	if ports := s.resolveEnvelopePorts(); ports != nil {
		return ports.Snapshot(ctx, identity.Ownership, candidate)
	}
	return s.config.Envelope.Snapshot(ctx, envelopePrincipal(identity), candidate)
}

func (s *Service) resolveEnvelope(ctx context.Context, identity EnvelopeAuthenticatedIdentity, candidate EnvelopeCandidate, envelope commandcontract.Envelope) (EnvelopeResolution, error) {
	if ports := s.resolveEnvelopePorts(); ports != nil {
		return ports.Resolve(ctx, identity.Ownership, candidate, envelope)
	}
	return s.config.Envelope.Resolve(ctx, envelopePrincipal(identity), candidate, envelope)
}

func (s *Service) authorizeEnvelope(ctx context.Context, identity EnvelopeAuthenticatedIdentity, envelope commandcontract.Envelope, definition commandcatalog.Definition) error {
	if ports := s.resolveEnvelopePorts(); ports != nil {
		return ports.Authorize(ctx, identity.Ownership, envelope, definition)
	}
	return s.config.Envelope.Authorize(ctx, envelopePrincipal(identity), envelope, definition)
}

func (s *Service) authorizeEnvelopeLookup(ctx context.Context, identity EnvelopeAuthenticatedIdentity, record commandledger.FullRecord) error {
	if ports := s.resolveEnvelopePorts(); ports != nil {
		return ports.AuthorizeLookup(ctx, identity.Ownership, record)
	}
	return s.config.Envelope.AuthorizeLookup(ctx, envelopePrincipal(identity), record)
}
