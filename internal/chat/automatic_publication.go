package chat

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
)

const canonicalCoalescing = 24 * time.Millisecond
const canonicalReadTimeout = 3 * time.Second
const canonicalSourceBytes = 8 * 1024 * 1024

var ErrTimelinePublication = errors.New("publicação de timeline exige recuperação")
var ErrTimelineSourceLimit = errors.New("base da projeção excede o limite de memória")

// A base é renovada apenas em transições persistidas. Tokens não fazem I/O.
// O writer já concluiu: erro aqui suspende publicação, nunca desfaz seu resultado.
func (e *ExecutionProjection) refreshPublicationLocked() {
	if e.owner.reader == nil {
		return // fixtures explícitas somente de buffers
	}
	c := e.conversation
	t := c.turns[e.turnID]
	if t == nil || t.executionID != e.executionID {
		return
	}
	ctx, cancel := context.WithTimeout(e.ctx, canonicalReadTimeout)
	defer cancel()
	snapshot, err := e.owner.reader.ReadPublicationTurn(ctx, e.key.conversationID, e.turnID)
	if err == nil && (snapshot == nil || snapshot.Response == nil || snapshot.Turn == nil || snapshot.Turn.CurrentExecutionID == nil || *snapshot.Turn.CurrentExecutionID != e.executionID || snapshot.Version.Incarnation != t.incarnation) {
		err = ErrProjectionRecoveryRequired
	}
	if err != nil {
		c.failPublicationLocked(t, err)
		return
	}
	c.setPublicationBaseLocked(t, snapshot.Response)
}

func (c *projectionConversation) failPublicationLocked(t *liveTurn, err error) {
	if c.publicationErrorLocked() == nil {
		c.epoch = uuid.NewString()
	}
	t.publicationBase, t.publicationBytes = nil, 0
	t.publicationErr = err
	t.publicationDirty = false
}

func (c *projectionConversation) publicationErrorLocked() error {
	if c.storagePublicationErr != nil {
		return errors.Join(ErrTimelinePublication, c.storagePublicationErr)
	}
	for _, t := range c.turns {
		if t.publicationErr != nil {
			return errors.Join(ErrTimelinePublication, t.publicationErr)
		}
	}
	return nil
}

// Remove mídia e áudio da base auxiliar, sem alterar o snapshot do chamador.
// O orçamento cobre a conversa inteira; excesso exige recuperação explícita.
func (c *projectionConversation) setPublicationBaseLocked(t *liveTurn, source *TimelineItem) {
	if !publicationSourceWithinBudget(source) {
		c.failPublicationLocked(t, ErrTimelineSourceLimit)
		return
	}
	base := *source
	base.legacyRepresentative = nil
	base.ScopeParentID = copyPublicationValue(source.ScopeParentID)
	base.MessageIDs = append([]string(nil), source.MessageIDs...)
	base.invocations = copyPublicationCalls(source.invocations)
	base.Segments = append([]ResponseSegment(nil), source.Segments...)
	for i := range base.Segments {
		base.Segments[i].Invocations = copyPublicationCalls(base.Segments[i].Invocations)
	}
	base.messages = append([]Message(nil), source.messages...)
	for i := range base.messages {
		base.messages[i] = publicationSourceMessage(base.messages[i])
	}
	if source.Representative != nil {
		message := publicationSourceMessage(*source.Representative)
		base.Representative = &message
	}
	encoded, err := json.Marshal(struct {
		Item     *TimelineItem
		Messages []Message
		Calls    []TurnSegmentToolCall
	}{&base, base.messages, base.invocations})
	if err != nil {
		c.failPublicationLocked(t, err)
		return
	}
	total := len(encoded)
	for _, other := range c.turns {
		if other != t {
			total += other.publicationBytes
		}
	}
	if total > canonicalSourceBytes {
		c.failPublicationLocked(t, ErrTimelineSourceLimit)
		return
	}
	t.publicationBase, t.publicationBytes = &base, len(encoded)
	t.publicationErr = nil
	t.publicationDirty = true
}

func copyPublicationValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func publicationSourceMessage(message Message) Message {
	message.Media, message.Audio, message.AudioMimeType = "", "", ""
	message.TurnID = copyPublicationValue(message.TurnID)
	message.ParentID = copyPublicationValue(message.ParentID)
	message.ChatExecutionID = copyPublicationValue(message.ChatExecutionID)
	return message
}

func copyPublicationCalls(calls []TurnSegmentToolCall) []TurnSegmentToolCall {
	result := append([]TurnSegmentToolCall(nil), calls...)
	for i := range result {
		result[i].ACPTextOffset = copyPublicationValue(result[i].ACPTextOffset)
	}
	return result
}

func (e *ExecutionProjection) schedulePublicationLocked() {
	c := e.conversation
	t := e.currentLocked()
	if t == nil || t.publicationBase == nil {
		return
	}
	t.publicationDirty = true
	if c.publicationTimer != nil {
		return
	}
	c.publicationGeneration++
	generation := c.publicationGeneration
	c.publicationTimer = time.AfterFunc(canonicalCoalescing, func() {
		e.owner.flushScheduledPublication(e.key, c, generation)
	})
}

func (p *LiveProjection) flushScheduledPublication(key projectionKey, expected *projectionConversation, generation uint64) {
	c, release := p.lockConversation(key, false)
	defer release()
	if c == nil || c != expected || c.publicationGeneration != generation {
		return
	}
	c.flushPublicationLocked(key.conversationID)
}

func (c *projectionConversation) flushPublicationLocked(conversationID string) {
	if c.publicationTimer != nil {
		c.publicationTimer.Stop()
		c.publicationTimer = nil
	}
	c.publicationGeneration++
	if c.publicationErrorLocked() != nil {
		return
	}
	ids := make([]string, 0)
	for id, t := range c.turns {
		if t.publicationDirty && t.publicationBase != nil && !t.detached {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	items := make([]*TimelineItem, 0, len(ids))
	for _, id := range ids {
		t := c.turns[id]
		item, err := composeResponse(t.publicationBase, snapshotLiveTurn(c, t))
		if err != nil {
			c.failPublicationLocked(t, err)
			return
		}
		items = append(items, item)
	}
	c.publishCanonical(conversationID, items)
	for _, id := range ids {
		t := c.turns[id]
		t.publicationDirty = false
		if t.finished {
			t.publicationBase, t.publicationBytes = nil, 0
		}
	}
}

// Recovery usa uma nova base autorizada, preservando os buffers mais recentes.
func (p *LiveProjection) recoverPublicationLocked(c *projectionConversation, turnID string, item *TimelineItem) {
	if p.reader == nil || item == nil {
		return
	}
	t := c.turns[turnID]
	if t != nil && !t.detached && (!t.finished || t.publicationErr != nil) {
		c.setPublicationBaseLocked(t, item)
	}
}

// Snapshot e publicação conservam também o terminal volátil. Retained continua
// expondo a tentativa separadamente para as ações explícitas de recuperação.
func projectLiveItemLocked(c *projectionConversation, item *TimelineItem) (*TimelineItem, error) {
	t := c.turns[item.TurnID]
	if item.Kind != "response" || t == nil || t.detached || (t.finished && !t.volatile) {
		return item, nil
	}
	return composeResponse(item, snapshotLiveTurn(c, t))
}
