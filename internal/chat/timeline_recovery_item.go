package chat

import (
	"context"
	"encoding/json"
	"strconv"

	"assistente/internal/apidto"
)

// Limite do DTO reservado, não de blobs ou detalhes externos de ferramentas.
const recoveryItemBytes = 512 * 1024

type recoveryItemReservation struct {
	identity apidto.TimelineRecoveryItemRequest
	encoded  []byte
}

func (w *subscribedWindow) recoveryUsage() (count, size int) {
	for _, held := range w.recoveryReservations {
		count++
		size += held.bytes
	}
	for _, held := range w.recoveryItems {
		count++
		size += len(held.encoded)
	}
	return
}

func validRecoveryItemRequest(request apidto.TimelineRecoveryItemRequest) bool {
	version, err := strconv.ParseUint(request.Revision, 10, 64)
	return err == nil && strconv.FormatUint(version, 10) == request.Revision &&
		request.ItemID != "" && request.Epoch != "" &&
		len(request.ItemID)+len(request.Epoch)+len(request.Revision) <= recoveryIdentityBytes
}

// ReserveRecoveryItem captura somente a revisão publicada pedida. Não recompõe
// fontes, não elege uma revisão mais nova e não promove a janela do consumidor.
func (s *TimelineSubscription) ReserveRecoveryItem(ctx context.Context, request apidto.TimelineRecoveryItemRequest, reader TimelineLifecycleReader) (*apidto.TimelineRecoveryItemReservation, error) {
	if !validRecoveryItemRequest(request) {
		return nil, ErrRecoveryCursorInvalid
	}
	target, release, err := s.lockRecoveryTarget(ctx, request.Reference, reader)
	if err != nil {
		return nil, err
	}
	defer release()
	w := s.conversation.windows[s.windowID]
	result := &apidto.TimelineRecoveryItemReservation{ConversationID: s.key.conversationID, Identity: request}
	if held, ok := w.recoveryItems[request.Reference]; ok {
		if held.identity != request {
			return nil, ErrRecoveryCursorStale
		}
		return result, nil
	}
	if target.turn == nil {
		return nil, ErrRecoveryUnavailable
	}
	if !target.turn.finished {
		return nil, ErrRecoveryMutable
	}
	c := s.conversation
	if c.publicationStage != nil || c.publicationErrorLocked() != nil {
		return nil, ErrProjectionRecoveryRequired
	}
	item, ok := c.publication.items[request.ItemID]
	if !ok || item.RecoveryReference == nil || *item.RecoveryReference != request.Reference ||
		item.TurnID == nil || *item.TurnID != request.Reference.TurnID {
		return nil, ErrRecoveryUnavailable
	}
	if request.Epoch != c.epoch || request.Epoch != c.publication.epoch || item.Revision != request.Revision {
		return nil, ErrRecoveryCursorStale
	}
	count, used := w.recoveryUsage()
	size := c.publication.itemSizes[request.ItemID]
	if count >= recoveryReservationLimit || size <= 0 || size > recoveryItemBytes || size > recoveryReservationBytes-used {
		return nil, ErrRecoveryReservationLimit
	}
	// JSON é a representação exata do wire e uma cópia independente de todos os
	// contêineres. Não retém backing buffers, turno ou fontes da publicação.
	encoded, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	if len(encoded) > recoveryItemBytes || len(encoded) > recoveryReservationBytes-used {
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
	if w.recoveryItems == nil {
		w.recoveryItems = make(map[TimelineRecoveryReference]recoveryItemReservation)
	}
	w.recoveryItems[request.Reference] = recoveryItemReservation{request, encoded}
	return result, nil
}

// A leitura reautoriza conversa/sessão e retorna apenas a reserva desta janela.
// Ausência é erro, nunca autorização para descartar a apresentação anterior.
func (s *TimelineSubscription) ReadRecoveryItem(ctx context.Context, request apidto.TimelineRecoveryItemRequest, reader TimelineLifecycleReader) (*CanonicalItem, error) {
	if !validRecoveryItemRequest(request) {
		return nil, ErrRecoveryCursorInvalid
	}
	_, release, err := s.lockRecoveryTarget(ctx, request.Reference, reader)
	if err != nil {
		return nil, err
	}
	defer release()
	held, ok := s.conversation.windows[s.windowID].recoveryItems[request.Reference]
	if !ok {
		return nil, ErrRecoveryUnavailable
	}
	if held.identity != request {
		return nil, ErrRecoveryCursorStale
	}
	var result CanonicalItem
	if err := json.Unmarshal(held.encoded, &result); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *TimelineSubscription) ReleaseRecoveryItem(ctx context.Context, request apidto.TimelineRecoveryItemRequest) error {
	_, c, release, err := s.lockAuthorized(ctx)
	if err != nil {
		return err
	}
	defer release()
	w := c.windows[s.windowID]
	if held, ok := w.recoveryItems[request.Reference]; ok {
		if held.identity != request {
			return ErrRecoveryCursorStale
		}
		delete(w.recoveryItems, request.Reference)
	}
	return nil
}

// ValidatePublishedCopy authorizes only the exact terminal item still owned by
// this window. It neither reserves bytes nor falls back to a newer execution.
func (s *TimelineSubscription) ValidatePublishedCopy(ctx context.Context, request apidto.TimelineRecoveryItemRequest, reader TimelineLifecycleReader) error {
	if !validRecoveryItemRequest(request) {
		return ErrRecoveryCursorInvalid
	}
	target, release, err := s.lockRecoveryTarget(ctx, request.Reference, reader)
	if err != nil {
		return err
	}
	defer release()
	c := s.conversation
	w := c.windows[s.windowID]
	if target.turn == nil || !target.turn.finished {
		return ErrRecoveryMutable
	}
	if c.publicationStage != nil || c.publicationErrorLocked() != nil || w.syncRequired {
		return ErrProjectionRecoveryRequired
	}
	item, ok := c.publication.items[request.ItemID]
	if !ok || !w.itemIDs[request.ItemID] || item.RecoveryReference == nil || *item.RecoveryReference != request.Reference ||
		item.TurnID == nil || *item.TurnID != request.Reference.TurnID || item.RepresentativeMessageID != nil {
		return ErrRecoveryUnavailable
	}
	if request.Epoch != c.epoch || request.Epoch != c.publication.epoch || item.Revision != request.Revision {
		return ErrRecoveryCursorStale
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = s.authorize(ctx)
	return err
}
