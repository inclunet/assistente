package sip

import (
	"fmt"
	"sync"
	"time"
)

// CallState representa o estado de uma chamada SIP.
type CallState string

const (
	CallStateIdle    CallState = "idle"
	CallStateRinging CallState = "ringing"
	CallStateActive  CallState = "active"
	CallStateEnded   CallState = "ended"
)

// CallSession gerencia o estado de uma chamada SIP individual.
type CallSession struct {
	// ID ├® o identificador ├║nico da sess├úo (Dialog ID do SIP).
	ID string

	// CallerID ├® o identificador do chamador (From header).
	CallerID string

	// CallerName ├® o display name do chamador.
	CallerName string

	// State ├® o estado atual da chamada.
	State CallState

	// StartedAt ├® o momento em que a chamada foi atendida.
	StartedAt time.Time

	// EndedAt ├® o momento em que a chamada foi encerrada.
	EndedAt time.Time

	// ConversationID ├® o ID da conversa associada no banco de dados.
	ConversationID uint

	// Dialog ├® a sess├úo SIP associada (para playback de ├íudio).
	// Aceita tanto DialogServerSession (inbound) quanto DialogClientSession (outbound).
	Dialog DialogSession

	// Pipeline ├® o pipeline de ├íudio da chamada.
	Pipeline *AudioPipeline

	mu sync.RWMutex
}

// NewCallSession cria uma nova sess├úo de chamada.
func NewCallSession(id, callerID, callerName string) *CallSession {
	return &CallSession{
		ID:         id,
		CallerID:   callerID,
		CallerName: callerName,
		State:      CallStateRinging,
	}
}

// SetState atualiza o estado da chamada com timestamps autom├íticos.
func (cs *CallSession) SetState(state CallState) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.State = state
	switch state {
	case CallStateActive:
		cs.StartedAt = time.Now()
	case CallStateEnded:
		cs.EndedAt = time.Now()
	}
}

// GetState retorna o estado atual da chamada.
func (cs *CallSession) GetState() CallState {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.State
}

// Duration retorna a dura├º├úo da chamada (desde atendimento at├® agora ou encerramento).
func (cs *CallSession) Duration() time.Duration {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.StartedAt.IsZero() {
		return 0
	}
	if !cs.EndedAt.IsZero() {
		return cs.EndedAt.Sub(cs.StartedAt)
	}
	return time.Since(cs.StartedAt)
}

// String retorna uma representa├º├úo leg├¡vel da sess├úo.
func (cs *CallSession) String() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return fmt.Sprintf("Call[%s] from=%s state=%s", cs.ID, cs.CallerID, cs.State)
}