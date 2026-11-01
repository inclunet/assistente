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
// callbacks externos sob o mutex (somente storage no coordenador). Revision é interna; não é sequence do protocolo wire.
type LiveProjection struct {
	reader        PublicationSnapshotReader
	mu            sync.Mutex
	conversations map[projectionKey]*projectionConversation
}

type projectionKey struct{ userID, conversationID string }
type projectionConversation struct {
	storagePublicationErr error
	pendingStorageIDs     map[string]bool
	publicationTimer      *time.Timer
	publicationGeneration uint64
	windows               map[string]*subscribedWindow
	publication           canonicalPublication
	mu                    sync.Mutex
	readers               int // protegido por LiveProjection.mu; inclui operações de storage
	epoch                 string
	revision              uint64
	incarnation           string
	lastRecoveryVersion   database.ChatTimelineVersion
	turns                 map[string]*liveTurn
	retained              map[string]*liveTurn // tentativas volatile substituídas por retry explícito
	timer                 *time.Timer
}
type liveTurn struct {
	publicationBase                                *TimelineItem
	publicationBytes                               int
	publicationErr                                 error
	publicationDirty                               bool
	committedVersion                               database.ChatTimelineVersion
	turnID, executionID, responseID, state, reason string
	incarnation                                    string
	finished, volatile                             bool
	detached                                       bool
	finishedAt                                     time.Time
	generation                                     uint64
	streams                                        map[int]*liveStream
}
type liveStream struct {
	generation uint64
	sealed     bool
	confirmed  bool
	messageID  string
	parts      []*livePart
}
type livePart struct{ text, reasoning strings.Builder }

// Fragmentos guardam apenas texto e raciocínio; o ledger permanece autoridade
// das invocações e de seus detalhes.
type LiveStreamSnapshot struct {
	Generation         uint64
	Iteration          int
	PersistedMessageID string
	Confirmed          bool
	Segments           []ResponseSegment
}
type LiveTurnSnapshot struct {
	Detached                                               bool
	Epoch                                                  string
	Incarnation                                            string
	Revision                                               uint64
	ItemID, TurnID, ExecutionID, ResponseID, State, Reason string
	Finished, Volatile                                     bool
	Streams                                                []LiveStreamSnapshot
}

// ExecutionProjection é um capability imutável admitido pelo lifecycle.
// Um binding antigo nunca aponta para a execução que o substituiu.
type ExecutionProjection struct {
	ctx                 context.Context
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

func NewLiveProjection(reader PublicationSnapshotReader) *LiveProjection {
	return &LiveProjection{reader: reader, conversations: make(map[projectionKey]*projectionConversation)}
}

// Admit serializa a admissão no storage e o binding na mesma barreira da leitura.
// O callback deve somente consultar/escrever storage; não pode chamar o projetor
// nem emitir eventos. A versão deve ser lida antes da admissão autorizada.
func (p *LiveProjection) Admit(ctx context.Context, conversationID, turnID, executionID string, admit func() (database.ChatTimelineVersion, error)) (*ExecutionProjection, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if conversationID == "" || turnID == "" || executionID == "" || admit == nil {
		return nil, errors.New("identidade incompleta de projeção")
	}
	key := projectionKey{userID, conversationID}
	c, release := p.lockConversation(key, true)
	defer release()
	if old := c.turns[turnID]; old != nil && old.executionID == executionID {
		return nil, errors.New("execução já projetada")
	}
	version, err := admit()
	if err != nil {
		return nil, err
	}
	reconcileProjectionVersion(c, version)
	if old := c.turns[turnID]; old != nil && (old.volatile || old.detached || hasUncommittedContent(old)) {
		old.publicationBase, old.publicationBytes = nil, 0
		if !old.finished {
			old.detached = true
		}
		c.retained[old.executionID] = old
	}
	c.turns[turnID] = &liveTurn{turnID: turnID, executionID: executionID, incarnation: version.Incarnation, state: database.TurnPreparing, streams: make(map[int]*liveStream)}
	c.revision++
	e := &ExecutionProjection{ctx: context.WithoutCancel(ctx), owner: p, key: key, conversation: c, turnID: turnID, executionID: executionID}
	e.refreshPublicationLocked()
	c.flushPublicationLocked(conversationID)
	return e, nil
}

func (e *ExecutionProjection) currentLocked() *liveTurn {
	t := e.conversation.turns[e.turnID]
	if t == nil || t.executionID != e.executionID || t.detached || t.incarnation != e.conversation.incarnation {
		return nil
	}
	return t
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
	e.schedulePublicationLocked()
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
	s.execution.schedulePublicationLocked()
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

// OwnsStream inclui o resultado já selado, mas rejeita binding substituído.
// O runner usa essa prova antes de interpretar o resultado de um callback.
func (s *StreamProjection) OwnsStream() bool { return s.current(true) }
func (s *StreamProjection) Current() bool    { return s.current(false) }
func (s *StreamProjection) current(allowSealed bool) bool {
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
	return stream != nil && stream.generation == s.generation && (allowSealed || !stream.sealed)
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

// Confirma somente bytes enviados ao writer desta iteração. Não seleciona
// mensagens por texto nem confirma deltas recebidos depois da escrita.
func confirmStoredContentLocked(t *liveTurn, iteration int, text, reasoning, messageID string) bool {
	if iteration == -1 {
		iteration = latestIteration(t)
	}
	stream := t.streams[iteration]
	if stream == nil {
		return false
	}
	var content, thoughts strings.Builder
	for _, part := range stream.parts {
		content.WriteString(part.text.String())
		thoughts.WriteString(part.reasoning.String())
	}
	if content.String() == text && thoughts.String() == reasoning {
		stream.confirmed = true
		stream.messageID = messageID
		return true
	}
	return false
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

func finishLiveTurnLocked(c *projectionConversation, t *liveTurn, state, reason string, persisted bool) {
	uncommitted := hasUncommittedContent(t)
	t.state, t.reason, t.finished, t.volatile, t.finishedAt = state, reason, true, !persisted || uncommitted, time.Now()
	if !persisted {
		t.state, t.reason = database.TurnFailed, "persistence_error"
	}
	c.revision++
}

func hasUncommittedContent(t *liveTurn) bool {
	uncommitted := false
	for _, stream := range t.streams {
		if stream.confirmed {
			continue
		}
		for _, part := range stream.parts {
			uncommitted = uncommitted || strings.TrimSpace(part.text.String()) != "" || strings.TrimSpace(part.reasoning.String()) != ""
		}
	}
	return uncommitted
}

const projectionRetention = 60 * time.Second

// Há no máximo um timer de retenção por conversa. Cada vencimento calcula o
// próximo; tráfego em outro turno não prolonga a retenção de conteúdo dispensável.
// Referências de operação impedem evicção sem segurar o mutex do registro
// enquanto outra conversa aguarda I/O. Ordem: registro (breve), conversa, DB.
func (p *LiveProjection) lockConversation(key projectionKey, create bool) (*projectionConversation, func()) {
	p.mu.Lock()
	c := p.conversations[key]
	if c == nil && create {
		c = &projectionConversation{epoch: uuid.NewString(), turns: make(map[string]*liveTurn), retained: make(map[string]*liveTurn)}
		p.conversations[key] = c
	}
	if c == nil {
		p.mu.Unlock()
		return nil, func() {}
	}
	c.readers++
	p.mu.Unlock()
	c.mu.Lock()
	return c, func() {
		c.mu.Unlock()
		p.mu.Lock()
		c.readers--
		p.mu.Unlock()
		p.scheduleCleanup(key, c)
	}
}

func (e *ExecutionProjection) lockStorage() (*projectionConversation, func()) {
	c, release := e.owner.lockConversation(e.key, false)
	if c != e.conversation {
		release()
		return nil, func() {}
	}
	return c, release
}

func (p *LiveProjection) scheduleCleanup(key projectionKey, c *projectionConversation) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conversations[key] != c || c.timer != nil || c.readers != 0 {
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
	if len(c.turns) == 0 && len(c.retained) == 0 && len(c.windows) == 0 {
		if c.publicationTimer != nil {
			c.publicationTimer.Stop()
			c.publicationTimer = nil
		}
		c.publicationGeneration++
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
		c.timer = nil
		if c.readers != 0 {
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
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
	c, release := p.lockConversation(projectionKey{userID, conversationID}, false)
	defer release()
	if c == nil {
		return nil, nil
	}
	t := c.turns[turnID]
	if t == nil {
		return nil, nil
	}
	return snapshotLiveTurn(c, t), nil
}

func snapshotLiveTurn(c *projectionConversation, t *liveTurn) *LiveTurnSnapshot {
	result := &LiveTurnSnapshot{Detached: t.detached, Epoch: c.epoch, Incarnation: t.incarnation, Revision: c.revision, ItemID: ports.ResponseItemID(t.turnID), TurnID: t.turnID, ExecutionID: t.executionID, ResponseID: t.responseID, State: t.state, Reason: t.reason, Finished: t.finished, Volatile: t.volatile || hasUncommittedContent(t)}
	iterations := make([]int, 0, len(t.streams))
	for iteration := range t.streams {
		iterations = append(iterations, iteration)
	}
	sort.Ints(iterations)
	for _, iteration := range iterations {
		stream := t.streams[iteration]
		target := LiveStreamSnapshot{Generation: stream.generation, Iteration: iteration, PersistedMessageID: stream.messageID, Confirmed: stream.confirmed}
		for index := range stream.parts {
			part := stream.parts[index]
			identity := fmt.Sprintf("draft:%s:%d:%d:%d", base64.RawURLEncoding.EncodeToString([]byte(t.executionID)), iteration, stream.generation, index)
			if part.reasoning.Len() != 0 {
				target.Segments = append(target.Segments, ResponseSegment{SegmentID: identity + ":reasoning", Kind: "reasoning", Text: strings.Clone(part.reasoning.String())})
			}
			if part.text.Len() != 0 {
				target.Segments = append(target.Segments, ResponseSegment{SegmentID: identity + ":text", Kind: "text", Text: strings.Clone(part.text.String())})
			}
		}
		result.Streams = append(result.Streams, target)
	}
	return result
}
