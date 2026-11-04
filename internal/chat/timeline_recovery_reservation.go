package chat

import (
	"context"
	"errors"
	"strconv"

	"assistente/internal/apidto"
)

const (
	recoveryReservationLimit  = 8
	recoveryReservationBytes  = 8 * 1024 * 1024
	recoveryReservationVisits = 4096
)

var ErrRecoveryUnavailable = errors.New("buffers de recuperação indisponíveis")
var ErrRecoveryMutable = errors.New("execução ainda não finalizada")
var ErrRecoveryReservationLimit = errors.New("limite de reservas de recuperação atingido")

// Visão mínima: os builders e o índice não mudam após finished. Não mantém
// publicationBase, erros ou outros metadados do turno vivos por causa da reserva.
type recoveryStream struct {
	generation uint64
	parts      []*livePart
}
type recoveryBuffers struct {
	liveStreams    map[int]*liveStream // usado somente durante leitura não reservada
	reserved       map[int]recoveryStream
	streamOrder    []int
	contentVersion uint64
}

func buffersOf(t *liveTurn) recoveryBuffers {
	return recoveryBuffers{liveStreams: t.streams, streamOrder: t.streamOrder, contentVersion: t.contentVersion}
}
func (b recoveryBuffers) stream(iteration int) (recoveryStream, bool) {
	if b.reserved != nil {
		stream, ok := b.reserved[iteration]
		return stream, ok
	}
	stream := b.liveStreams[iteration]
	if stream == nil {
		return recoveryStream{}, false
	}
	return recoveryStream{stream.generation, stream.parts}, true
}

type recoveryReservation struct {
	buffers recoveryBuffers
	bytes   int
}

// Copia apenas descritores limitados, sem IDs de mensagens ou outros metadados.
// Mede capacidade retida sem percorrer texto. Os builders finalizados são imutáveis.
func reserveBuffers(ctx context.Context, source recoveryBuffers) (recoveryBuffers, int, error) {
	result := recoveryBuffers{reserved: make(map[int]recoveryStream), contentVersion: source.contentVersion}
	used, visits := 0, 0
	for _, iteration := range source.streamOrder {
		visits++
		if visits > recoveryReservationVisits {
			return recoveryBuffers{}, 0, ErrRecoveryReservationLimit
		}
		stream, ok := source.stream(iteration)
		if !ok {
			return recoveryBuffers{}, 0, ErrRecoveryUnavailable
		}
		retained := recoveryStream{generation: stream.generation}
		for _, part := range stream.parts {
			if err := ctx.Err(); err != nil {
				return recoveryBuffers{}, 0, err
			}
			visits++
			if visits > recoveryReservationVisits {
				return recoveryBuffers{}, 0, ErrRecoveryReservationLimit
			}
			for _, size := range []int{part.text.Cap(), part.reasoning.Cap()} {
				if size > recoveryReservationBytes-used {
					return recoveryBuffers{}, 0, ErrRecoveryReservationLimit
				}
				used += size
			}
			retained.parts = append(retained.parts, part)
		}
		result.streamOrder = append(result.streamOrder, iteration)
		result.reserved[iteration] = retained
	}
	return result, used, nil
}
func (s *TimelineSubscription) ReserveRecovery(ctx context.Context, request apidto.TimelineRecoveryReservationRequest, reader TimelineLifecycleReader) (*apidto.TimelineRecoveryReservation, error) {
	if len(request.ContentVersion) == 0 || len(request.ContentVersion) > 20 {
		return nil, ErrRecoveryCursorInvalid
	}
	version, err := strconv.ParseUint(request.ContentVersion, 10, 64)
	if err != nil || strconv.FormatUint(version, 10) != request.ContentVersion {
		return nil, ErrRecoveryCursorInvalid
	}
	ref := request.Reference
	if len(ref.ProjectionID)+len(ref.Incarnation)+len(ref.TurnID)+len(ref.ExecutionID) > recoveryIdentityBytes {
		return nil, ErrTimelineSourceLimit
	}
	target, release, err := s.lockRecoveryTarget(ctx, ref, reader)
	if err != nil {
		return nil, err
	}
	defer release()
	if len(s.key.conversationID)+len(ref.ProjectionID)+len(ref.Incarnation)+len(ref.TurnID)+len(ref.ExecutionID) > recoveryIdentityBytes {
		return nil, ErrTimelineSourceLimit
	}
	w := s.conversation.windows[s.windowID]
	result := &apidto.TimelineRecoveryReservation{ConversationID: s.key.conversationID, Reference: ref, ContentVersion: request.ContentVersion}
	if held, ok := w.recoveryReservations[ref]; ok {
		if held.buffers.contentVersion != version {
			return nil, ErrRecoveryCursorStale
		}
		return result, nil
	}
	t := target.turn
	if t == nil {
		return nil, ErrRecoveryUnavailable
	}
	if t.contentVersion != version {
		return nil, ErrRecoveryCursorStale
	}
	if !t.finished {
		return nil, ErrRecoveryMutable
	}
	if len(w.recoveryReservations) >= recoveryReservationLimit {
		return nil, ErrRecoveryReservationLimit
	}
	source := buffersOf(t)
	buffers, size, err := reserveBuffers(ctx, source)
	if err != nil {
		return nil, err
	}
	used := 0
	for _, held := range w.recoveryReservations {
		used += held.bytes
	}
	if size > recoveryReservationBytes-used {
		return nil, ErrRecoveryReservationLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := s.authorize(ctx); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.recoveryReservations == nil {
		w.recoveryReservations = make(map[TimelineRecoveryReference]recoveryReservation)
	}
	w.recoveryReservations[ref] = recoveryReservation{buffers, size}
	return result, nil
}

// Release não descarta a fonte original. Close permanece o caminho de cleanup
// quando a sessão já foi revogada. Versão diferente não libera outra reserva.
func (s *TimelineSubscription) ReleaseRecovery(ctx context.Context, request apidto.TimelineRecoveryReservationRequest) error {
	_, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return err
	}
	defer release()
	w := c.windows[s.windowID]
	if held, ok := w.recoveryReservations[request.Reference]; ok {
		if strconv.FormatUint(held.buffers.contentVersion, 10) != request.ContentVersion {
			return ErrRecoveryCursorStale
		}
		delete(w.recoveryReservations, request.Reference)
	}
	return nil
}

func (c *projectionConversation) discardRecoveryReservations(turnID, executionID string) {
	for _, w := range c.windows {
		for ref := range w.recoveryReservations {
			if ref.TurnID == turnID && ref.ExecutionID == executionID {
				delete(w.recoveryReservations, ref)
			}
		}
	}
}
