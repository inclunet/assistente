package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/database"
	"github.com/google/uuid"
)

// LiveProjection conserva dados recebidos do provider, antes do armazenamento.
// A unidade de serialização é usuário/conversa. Não publica eventos nem chama
// callbacks sob o mutex. Revision é interna; não é sequence do protocolo wire.
type LiveProjection struct {
	mu            sync.Mutex
	conversations map[projectionKey]*projectionConversation
}

type projectionKey struct{ userID, conversationID string }
type projectionConversation struct {
	mu       sync.Mutex
	epoch    string
	revision uint64
	turns    map[string]*liveTurn
	retained map[string]*liveTurn // tentativas volatile substituídas por retry explícito
	timer    *time.Timer
}
type liveTurn struct {
	turnID, executionID, responseID, state, reason string
	finished, volatile                             bool
	finishedAt                                     time.Time
	generation                                     uint64
	streams                                        map[int]*liveStream
}
type liveStream struct {
	generation uint64
	sealed     bool
	confirmed  bool
	parts      []*livePart
}
type livePart struct{ text, reasoning strings.Builder }

// Fragmentos guardam apenas texto e raciocínio; o ledger permanece autoridade
// das invocações e de seus detalhes.
type LiveFragment struct{ ID, Text, Reasoning string }
type LiveStreamSnapshot struct {
	Iteration int
	Fragments []LiveFragment
}
type LiveTurnSnapshot struct {
	Epoch                                                  string
	Revision                                               uint64
	ItemID, TurnID, ExecutionID, ResponseID, State, Reason string
	Finished, Volatile                                     bool
	Streams                                                []LiveStreamSnapshot
}

// ExecutionProjection é um capability imutável admitido pelo lifecycle.
// Um binding antigo nunca aponta para a execução que o substituiu.
type ExecutionProjection struct {
	owner               *LiveProjection
	key                 projectionKey
	conversation        *projectionConversation
	turnID, executionID string
}
type StreamProjection struct {
	execution  *ExecutionProjection
	iteration  int
	generation uint64
}

func NewLiveProjection() *LiveProjection {
	return &LiveProjection{conversations: make(map[projectionKey]*projectionConversation)}
}

// Begin deve ser chamado somente após a admissão autorizada no TurnStore.
func (p *LiveProjection) Begin(ctx context.Context, conversationID, turnID, executionID string) (*ExecutionProjection, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if conversationID == "" || turnID == "" || executionID == "" {
		return nil, errors.New("identidade incompleta de projeção")
	}
	key := projectionKey{userID, conversationID}
	p.mu.Lock()
	c := p.conversations[key]
	if c == nil {
		c = &projectionConversation{epoch: uuid.NewString(), turns: make(map[string]*liveTurn), retained: make(map[string]*liveTurn)}
		p.conversations[key] = c
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	defer p.mu.Unlock()
	if old := c.turns[turnID]; old != nil {
		if old.executionID == executionID {
			return nil, errors.New("execução já projetada")
		}
		if old.volatile {
			c.retained[old.executionID] = old
		}
	}
	c.turns[turnID] = &liveTurn{turnID: turnID, executionID: executionID, state: database.TurnPreparing, streams: make(map[int]*liveStream)}
	c.revision++
	return &ExecutionProjection{owner: p, key: key, conversation: c, turnID: turnID, executionID: executionID}, nil
}

func (e *ExecutionProjection) currentLocked() *liveTurn {
	t := e.conversation.turns[e.turnID]
	if t == nil || t.executionID != e.executionID {
		return nil
	}
	return t
}

func (e *ExecutionProjection) SetResponse(id string) {
	if e == nil {
		return
	}
	c := e.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := e.currentLocked(); t != nil && !t.finished {
		t.responseID = id
		c.revision++
	}
}

func (e *ExecutionProjection) StartStream(iteration int) *StreamProjection {
	if e == nil {
		return nil
	}
	c := e.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	t := e.currentLocked()
	if t == nil || t.finished {
		return &StreamProjection{execution: e, iteration: iteration}
	}
	t.generation++
	// Uma nova tentativa de transporte substitui somente sua iteração. O texto
	// anterior é substituído pelo comando explícito de início da tentativa.
	stream := &liveStream{parts: []*livePart{{}}}
	t.streams[iteration] = stream
	stream.generation, stream.sealed = t.generation, false
	t.state = database.TurnStreaming
	c.revision++
	return &StreamProjection{execution: e, iteration: iteration, generation: t.generation}
}

func (s *StreamProjection) apply(fn func(*liveStream)) bool {
	if s == nil {
		return true
	} // seams de handlers sem admissão; runtime sempre vincula
	c := s.execution.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	t := s.execution.currentLocked()
	if t == nil || t.finished {
		return false
	}
	stream := t.streams[s.iteration]
	if stream == nil || stream.generation != s.generation || stream.sealed {
		return false
	}
	fn(stream)
	stream.confirmed = false
	c.revision++
	return true
}

func (s *StreamProjection) Append(text string) bool {
	return s.apply(func(stream *liveStream) { stream.parts[len(stream.parts)-1].text.WriteString(text) })
}
func (s *StreamProjection) Initial(text string) bool {
	return s.apply(func(stream *liveStream) { stream.parts = []*livePart{{}}; stream.parts[0].text.WriteString(text) })
}
func (s *StreamProjection) Reasoning(text string, replace bool) bool {
	return s.apply(func(stream *liveStream) {
		if replace {
			for _, part := range stream.parts {
				part.reasoning.Reset()
			}
			stream.parts[0].reasoning.WriteString(text)
		} else {
			stream.parts[len(stream.parts)-1].reasoning.WriteString(text)
		}
	})
}
func (s *StreamProjection) Cut() bool {
	return s.apply(func(stream *liveStream) { stream.parts = append(stream.parts, &livePart{}) })
}
func (s *StreamProjection) Current() bool {
	if s == nil {
		return true
	}
	c := s.execution.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	t := s.execution.currentLocked()
	if t == nil || t.finished {
		return false
	}
	stream := t.streams[s.iteration]
	return stream != nil && stream.generation == s.generation && !stream.sealed
}

// Seal é uma substituição explícita quando o provider termina com um conteúdo
// diferente dos chunks. Não compara trechos para adivinhar proveniência.
func (s *StreamProjection) Seal(text, reasoning string) bool {
	return s.apply(func(stream *liveStream) {
		var content strings.Builder
		for i := range stream.parts {
			content.WriteString(stream.parts[i].text.String())
		}
		if content.String() != text {
			stream.parts = []*livePart{{}}
			stream.parts[0].text.WriteString(text)
		}
		for i := range stream.parts {
			stream.parts[i].reasoning.Reset()
		}
		stream.parts[0].reasoning.WriteString(reasoning)
		stream.sealed = true
	})
}

// ConfirmStoredContent acusa uma escrita concluída desta iteração. A comparação
// exata só evita confirmar deltas recebidos depois do conteúdo enviado ao storage;
// não escolhe mensagens nem reconstrói identidade a partir de texto.
// iteration=-1 identifica a última iteração da execução (resposta/parcial final).
func (e *ExecutionProjection) ConfirmStoredContent(iteration int, text, reasoning string) {
	if e == nil {
		return
	}
	c := e.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	t := e.currentLocked()
	if t == nil || t.finished {
		return
	}
	if iteration == -1 {
		iteration = latestIteration(t)
	}
	stream := t.streams[iteration]
	if stream == nil {
		return
	}
	var content, thoughts strings.Builder
	for _, part := range stream.parts {
		content.WriteString(part.text.String())
		thoughts.WriteString(part.reasoning.String())
	}
	if content.String() == text && thoughts.String() == reasoning {
		stream.confirmed = true
		c.revision++
	}
}

func latestIteration(t *liveTurn) int {
	latest := -1
	for iteration := range t.streams {
		if iteration > latest {
			latest = iteration
		}
	}
	return latest
}

func (e *ExecutionProjection) Finish(state, reason string, persisted, contentCommitted bool) {
	if e == nil {
		return
	}
	c := e.conversation
	c.mu.Lock()
	t := e.currentLocked()
	if t == nil || t.finished {
		c.mu.Unlock()
		return
	}
	if contentCommitted {
		if stream := t.streams[latestIteration(t)]; stream != nil {
			stream.confirmed = true
		}
	}
	uncommitted := false
	for _, stream := range t.streams {
		if stream.confirmed {
			continue
		}
		for _, part := range stream.parts {
			uncommitted = uncommitted || strings.TrimSpace(part.text.String()) != "" || strings.TrimSpace(part.reasoning.String()) != ""
		}
	}
	t.state, t.reason, t.finished, t.volatile, t.finishedAt = state, reason, true, !persisted || uncommitted, time.Now()
	if !persisted {
		t.state, t.reason = database.TurnFailed, "persistence_error"
	}
	c.revision++
	c.mu.Unlock()
	e.owner.scheduleCleanup(e.key, c)
}

const projectionRetention = 60 * time.Second

// Há no máximo um timer de retenção por conversa. Cada vencimento calcula o
// próximo; tráfego em outro turno não prolonga a retenção de conteúdo dispensável.
func (p *LiveProjection) scheduleCleanup(key projectionKey, c *projectionConversation) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conversations[key] != c || c.timer != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p.pruneLocked(key, c, time.Now())
}

// p.mu e c.mu devem estar adquiridos, nesta ordem.
func (p *LiveProjection) pruneLocked(key projectionKey, c *projectionConversation, now time.Time) {
	var next time.Time
	for id, t := range c.turns {
		if !t.finished || t.volatile {
			continue
		}
		deadline := t.finishedAt.Add(projectionRetention)
		if !deadline.After(now) {
			delete(c.turns, id)
			continue
		}
		if next.IsZero() || deadline.Before(next) {
			next = deadline
		}
	}
	if len(c.turns) == 0 && len(c.retained) == 0 {
		delete(p.conversations, key)
		return
	}
	if next.IsZero() {
		return
	}
	c.timer = time.AfterFunc(next.Sub(now), func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.conversations[key] != c {
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.timer = nil
		p.pruneLocked(key, c, time.Now())
	})
}

// Snapshot não consulta SQL. O chamador deve usar o coordenador de recuperação
// para combinar estes dados com persistência/autorização atualizada.
func (p *LiveProjection) Snapshot(ctx context.Context, conversationID, turnID string) (*LiveTurnSnapshot, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	c := p.conversations[projectionKey{userID, conversationID}]
	if c == nil {
		p.mu.Unlock()
		return nil, nil
	}
	c.mu.Lock()
	p.mu.Unlock()
	defer c.mu.Unlock()
	t := c.turns[turnID]
	if t == nil {
		return nil, nil
	}
	return snapshotLiveTurn(c, t), nil
}

func snapshotLiveTurn(c *projectionConversation, t *liveTurn) *LiveTurnSnapshot {
	result := &LiveTurnSnapshot{Epoch: c.epoch, Revision: c.revision, ItemID: ports.ResponseItemID(t.turnID), TurnID: t.turnID, ExecutionID: t.executionID, ResponseID: t.responseID, State: t.state, Reason: t.reason, Finished: t.finished, Volatile: t.volatile}
	iterations := make([]int, 0, len(t.streams))
	for iteration := range t.streams {
		iterations = append(iterations, iteration)
	}
	sort.Ints(iterations)
	for _, iteration := range iterations {
		stream := t.streams[iteration]
		target := LiveStreamSnapshot{Iteration: iteration}
		for index := range stream.parts {
			part := stream.parts[index]
			target.Fragments = append(target.Fragments, LiveFragment{ID: fmt.Sprintf("draft:%s:%d:%d", base64.RawURLEncoding.EncodeToString([]byte(t.executionID)), iteration, index), Text: strings.Clone(part.text.String()), Reasoning: strings.Clone(part.reasoning.String())})
		}
		result.Streams = append(result.Streams, target)
	}
	return result
}
