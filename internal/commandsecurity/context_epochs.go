package commandsecurity

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// ContextPrincipal só é retornado por autenticação interna do host. Nunca é
// lido do envelope de cliente. A chave interna não é exposta como session_id.
type ContextPrincipal struct{ UserID, Type, ID, GroupID string }

func (p ContextPrincipal) groupKey() (string, error) {
	if p.GroupID == "" {
		return "", nil
	}
	if p.Type != "external_token" || len(p.GroupID) > 1024 || !utf8.ValidString(p.GroupID) || strings.TrimSpace(p.GroupID) != p.GroupID || strings.ContainsRune(p.GroupID, '\x00') {
		return "", ErrInvalidEpochInput
	}
	raw, _ := json.Marshal([]string{p.Type, p.GroupID})
	return string(raw), nil
}

func (p ContextPrincipal) key() (string, error) {
	if _, err := p.groupKey(); err != nil {
		return "", err
	}
	if p.ID == "" || len(p.ID) > 1024 || !utf8.ValidString(p.ID) || strings.TrimSpace(p.ID) != p.ID || strings.ContainsRune(p.ID, '\x00') {
		return "", ErrInvalidEpochInput
	}
	switch p.Type {
	case "local_session":
		if !epochID(p.UserID) || !epochID(p.ID) {
			return "", ErrInvalidEpochInput
		}
		return p.ID, nil
	case "external_token", "job_service":
		if !epochID(p.UserID) {
			return "", ErrInvalidEpochInput
		}
	case "system":
		if p.UserID != "" {
			return "", ErrInvalidEpochInput
		}
	default:
		return "", ErrInvalidEpochInput
	}
	raw, _ := json.Marshal([]string{p.Type, p.UserID, p.ID})
	return "context:" + string(raw), nil
}

// CaptureContextAuthenticated usa o MESMO mapa, gate, security epoch e watches
// da sessão local. A callback deve revalidar fonte/owner antes de retornar.
func (s *EpochService) CaptureContextAuthenticated(ctx context.Context, authenticate func(context.Context) (ContextPrincipal, error)) (EpochSnapshot, error) {
	if !s.valid() || ctx == nil || authenticate == nil {
		return EpochSnapshot{}, ErrInvalidEpochInput
	}
	var result EpochSnapshot
	err := s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.transitions != 0 {
			return ErrStaleEpoch
		}
		p, e := authenticate(ctx)
		if e != nil {
			return e
		}
		key, e := p.key()
		if e != nil {
			return e
		}
		group, e := p.groupKey()
		if e != nil {
			return e
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		current, ok := s.sessions[key]
		if ok && (current.user != p.UserID || current.group != group) {
			return ErrInvalidEpochInput
		}
		if !ok {
			g, e := s.next()
			if e != nil {
				return e
			}
			current = sessionEpoch{user: p.UserID, group: group, generation: g}
			s.sessions[key] = current
		}
		result = EpochSnapshot{UserID: p.UserID, SessionID: key, AuthGeneration: current.generation, SecurityGeneration: s.security}
		return nil
	})
	if err != nil {
		return EpochSnapshot{}, err
	}
	return result, nil
}

// MutateContextGroup revoga todos os contextos external_token do usuário e
// grupo antes do efeito administrativo, sob o mesmo gate usado por captura,
// admissão e watches. A invalidação permanece mesmo quando action falha.
func (s *EpochService) MutateContextGroup(ctx context.Context, p ContextPrincipal, action func() error) error {
	if !s.valid() || ctx == nil || action == nil || p.Type != "external_token" || !epochID(p.UserID) {
		return ErrInvalidEpochInput
	}
	group, err := p.groupKey()
	if err != nil || group == "" {
		return ErrInvalidEpochInput
	}
	if p.ID != "" {
		if _, err := p.key(); err != nil {
			return err
		}
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.closing {
			return ErrStaleEpoch
		}
		sessions := make(map[string]struct{})
		for key, current := range s.sessions {
			if current.user == p.UserID && current.group == group {
				delete(s.sessions, key)
				sessions[key] = struct{}{}
			}
		}
		s.watchesMu.Lock()
		for watch := range s.watches {
			if watch.user == p.UserID {
				if _, ok := sessions[watch.session]; ok {
					watch.cancel()
					delete(s.watches, watch)
				}
			}
		}
		s.watchesMu.Unlock()
		return action()
	})
}

// MutateContext revoga o contexto antes do efeito administrativo sob o gate.
// Falha do efeito mantém a invalidação conservadora; jamais publica sucesso.
func (s *EpochService) MutateContext(ctx context.Context, p ContextPrincipal, action func() error) error {
	if !s.valid() || ctx == nil || action == nil {
		return ErrInvalidEpochInput
	}
	key, e := p.key()
	if e != nil {
		return e
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.closing {
			return ErrStaleEpoch
		}
		if current, ok := s.sessions[key]; ok && current.user != p.UserID {
			return ErrInvalidEpochInput
		}
		delete(s.sessions, key)
		s.cancelExecutions(key, false)
		return action()
	})
}
