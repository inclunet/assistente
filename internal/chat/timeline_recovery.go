package chat

import (
	"context"
	"errors"
	"sort"

	"assistente/internal/database"
	"github.com/google/uuid"
)

// TurnRecovery separa autoridades. Persisted é uma leitura transacional atual;
// Active só existe quando executionId e incarnation conferem. Retained nunca é
// mesclado ao texto persistido: contém tentativas voláteis ou desvinculadas.
// Não é DTO wire; o cutover de janelas/eventos permanece separado.
type TurnRecovery struct {
	Item                 *TimelineItem
	Persisted            *TimelineTurnSnapshot
	PersistedExecutionID string
	Epoch                string
	Active               *LiveTurnSnapshot
	Retained             []*LiveTurnSnapshot
	RecoveryRequired     bool
}

var ErrProjectionActive = errors.New("execução ativa não pode ser descartada")
var ErrProjectionRecoveryRequired = errors.New("conteúdo não confirmado exige recuperação")

// ReadRecovery mantém a barreira durante a autorização/leitura transacional e
// a captura dos buffers. Não retorna conteúdo em memória quando storage recusa
// a leitura (inclusive exclusão, revogação ou falha de consulta).
func (p *LiveProjection) ReadRecovery(ctx context.Context, conversationID, turnID string, reader TurnSnapshotReader) (*TurnRecovery, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	c, release := p.lockConversation(projectionKey{userID, conversationID}, true)
	defer release()
	persisted, err := reader.ReadTurn(ctx, conversationID, turnID)
	if err != nil {
		return nil, err
	}
	result := &TurnRecovery{Persisted: persisted}
	reconcileProjectionVersion(c, persisted.Version)
	result.Epoch = c.epoch
	if t := c.turns[turnID]; t != nil {
		compatible := reconcileProjectedTurn(c, t, persisted.Version, persisted.Turn)
		switch {
		case t.detached:
			result.RecoveryRequired = true
			result.Retained = append(result.Retained, snapshotLiveTurn(c, t))
		case !t.finished && compatible:
			result.Active = snapshotLiveTurn(c, t)
			result.PersistedExecutionID = t.executionID
		case t.volatile:
			result.Retained = append(result.Retained, snapshotLiveTurn(c, t))
		case t.finished:
			if t.committedVersion == persisted.Version && t.committedVersion.Incarnation != "" {
				result.PersistedExecutionID = t.executionID
			} else {
				result.RecoveryRequired = true
			}
		}
	}
	for _, t := range c.retained {
		if t.turnID == turnID {
			result.Retained = append(result.Retained, snapshotLiveTurn(c, t))
		}
	}
	if result.RecoveryRequired && c.lastRecoveryVersion != persisted.Version {
		c.epoch = uuid.NewString()
		c.lastRecoveryVersion = persisted.Version
		c.revision++
		result.Epoch = c.epoch
		for _, retained := range result.Retained {
			retained.Epoch = c.epoch
			retained.Revision = c.revision
		}
	}
	if persisted.Response != nil {
		result.Item = persisted.Response
		if result.Active != nil {
			result.Item, err = composeResponse(persisted.Response, result.Active)
			if err != nil {
				return nil, err
			}
		}
	}
	if result.Item != nil {
		c.publishCanonical(conversationID, []*TimelineItem{result.Item})
	}
	sort.Slice(result.Retained, func(i, j int) bool { return result.Retained[i].ExecutionID < result.Retained[j].ExecutionID })
	return result, nil
}

// CanUsePersistedPatch nunca usa texto confirmado de outra tentativa para
// substituir conteúdo desta execução ainda não confirmado. Tentativas antigas
// retidas não impedem o patch de uma nova execução concluída com sucesso.
func (r *TurnRecovery) CanUsePersistedPatch(executionID string) bool {
	if r.RecoveryRequired || (executionID != "" && r.PersistedExecutionID != executionID) || (r.Active != nil && r.Active.Volatile) {
		return false
	}
	for _, t := range r.Retained {
		if t.ExecutionID == executionID {
			return false
		}
	}
	return true
}

// DiscardRetained é explícito, idempotente e endereçado por identidade completa.
// Autoriza novamente pelo storage mesmo se o alvo já tiver sido descartado.
// Execução ativa só pode ser descartada depois de comprovadamente desvinculada.
func (p *LiveProjection) DiscardRetained(ctx context.Context, conversationID, turnID, executionID string, reader TurnSnapshotReader) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if executionID == "" {
		return database.ErrChatTurnInvalid
	}
	key := projectionKey{userID, conversationID}
	c, release := p.lockConversation(key, true)
	defer release()
	persisted, err := reader.ReadTurn(ctx, conversationID, turnID)
	if err != nil {
		return err
	}
	t := c.retained[executionID]
	current := false
	if candidate := c.turns[turnID]; candidate != nil && candidate.executionID == executionID {
		t = candidate
		current = true
	}
	if t == nil || t.turnID != turnID {
		return nil
	}
	if !t.finished {
		stillActive := t.incarnation == persisted.Version.Incarnation && persisted.Turn != nil && persisted.Turn.ActiveExecutionID != nil && *persisted.Turn.ActiveExecutionID == executionID
		if !t.detached || stillActive {
			return ErrProjectionActive
		}
	}
	if current {
		delete(c.turns, turnID)
	} else {
		delete(c.retained, executionID)
	}
	c.revision++
	return nil
}

// CommitResponse mantém gravação final e confirmação do projetor sob a mesma
// barreira. Erro deixa a execução disponível para o tratamento terminal único.
func (e *ExecutionProjection) CommitResponse(state, reason, text, reasoning string, write func() (database.ChatTimelineVersion, error)) error {
	if e == nil {
		_, err := write()
		return err
	}
	c, release := e.lockStorage()
	defer release()
	if c == nil {
		return database.ErrChatTurnStale
	}
	t := e.currentLocked()
	if t == nil || t.finished {
		return database.ErrChatTurnStale
	}
	version, err := write()
	if err != nil {
		return err
	}
	t.committedVersion = version
	confirmStoredContentLocked(t, -1, text, reasoning, t.responseID)
	finishLiveTurnLocked(c, t, state, reason, true)
	return nil
}

// CommitTerminal confirma somente metadados; preserva conteúdo não confirmado.
// Os callbacks de storage não podem reentrar no projetor nem emitir eventos.
func (e *ExecutionProjection) CommitTerminal(state, reason string, write func() (database.ChatTimelineVersion, error)) error {
	if e == nil {
		_, err := write()
		return err
	}
	c, release := e.lockStorage()
	defer release()
	if c == nil {
		return database.ErrChatTurnStale
	}
	t := e.currentLocked()
	if t == nil || t.finished {
		return database.ErrChatTurnStale
	}
	version, err := write()
	if errors.Is(err, database.ErrChatTurnStale) {
		return err
	}
	if err == nil {
		t.committedVersion = version
	}
	finishLiveTurnLocked(c, t, state, reason, err == nil && reason != "persistence_error")
	return err
}

func (e *ExecutionProjection) CommitContent(iteration int, text, reasoning string, write func() (string, error)) error {
	if e == nil {
		_, err := write()
		return err
	}
	c, release := e.lockStorage()
	defer release()
	if c == nil {
		return database.ErrChatTurnStale
	}
	t := e.currentLocked()
	if t == nil || t.finished {
		return database.ErrChatTurnStale
	}
	messageID, err := write()
	if err != nil {
		return err
	}
	if messageID == "" {
		return errors.New("confirmação de conteúdo sem identidade persistida")
	}
	if confirmStoredContentLocked(t, iteration, text, reasoning, messageID) {
		c.revision++
	}
	return nil
}

func (e *ExecutionProjection) AssignResponse(write func() (string, error)) (string, error) {
	if e == nil {
		return write()
	}
	c, release := e.lockStorage()
	defer release()
	if c == nil {
		return "", database.ErrChatTurnStale
	}
	t := e.currentLocked()
	if t == nil || t.finished {
		return "", database.ErrChatTurnStale
	}
	id, err := write()
	if err != nil {
		return "", err
	}
	t.responseID = id
	c.revision++
	return id, nil
}

func reconcileProjectionVersion(c *projectionConversation, version database.ChatTimelineVersion) {
	if c.incarnation != version.Incarnation {
		c.epoch = uuid.NewString()
		c.incarnation = version.Incarnation
		c.revision++
	}
}

// Reconcilia apenas fatos que pertencem ao snapshot autorizado. Ausência em outra
// página não significa exclusão; o chamador informa os turnos efetivamente lidos.
func reconcileProjectedTurn(c *projectionConversation, t *liveTurn, version database.ChatTimelineVersion, turn *database.ChatTurn) bool {
	compatible := t.incarnation == version.Incarnation && turn != nil && turn.ActiveExecutionID != nil && *turn.ActiveExecutionID == t.executionID
	if !t.finished && !compatible && !t.detached {
		t.detached = true
		c.revision++
	}
	return compatible
}

// ReadWindow usa a mesma barreira de admissão, deltas e commits do turno.
// O adaptador visual ainda retorna conteúdo persistido; não inventa nós live nem
// mistura tentativas voláteis com o banco. Recuperação de bytes usa ReadRecovery.
func (p *LiveProjection) ReadWindow(ctx context.Context, query database.MessageWindowQuery, reader WindowSnapshotReader) (*TimelineWindowSnapshot, error) {
	return p.readWindow(ctx, query, reader, false)
}

type CanonicalWindowSnapshotReader interface {
	ReadCanonicalWindow(context.Context, database.MessageWindowQuery) (*TimelineWindowSnapshot, error)
}
type canonicalWindowAdapter struct{ reader CanonicalWindowSnapshotReader }

func (a canonicalWindowAdapter) ReadWindow(ctx context.Context, q database.MessageWindowQuery) (*TimelineWindowSnapshot, error) {
	return a.reader.ReadCanonicalWindow(ctx, q)
}

func (p *LiveProjection) ReadCanonicalWindow(ctx context.Context, query database.MessageWindowQuery, reader CanonicalWindowSnapshotReader) (*TimelineWindowSnapshot, error) {
	return p.readWindow(ctx, query, canonicalWindowAdapter{reader}, true)
}

func (p *LiveProjection) readWindow(ctx context.Context, query database.MessageWindowQuery, reader WindowSnapshotReader, canonical bool) (*TimelineWindowSnapshot, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	c, release := p.lockConversation(projectionKey{userID, query.ConversationID}, true)
	defer release()
	return p.readWindowLocked(ctx, query, reader, canonical, c)
}

func (p *LiveProjection) readWindowLocked(ctx context.Context, query database.MessageWindowQuery, reader WindowSnapshotReader, canonical bool, c *projectionConversation) (*TimelineWindowSnapshot, error) {
	snapshot, err := reader.ReadWindow(ctx, query)
	if err != nil {
		return nil, err
	}
	reconcileProjectionVersion(c, snapshot.Version)
	for _, turnID := range snapshot.TurnIDs {
		if t := c.turns[turnID]; t != nil {
			reconcileProjectedTurn(c, t, snapshot.Version, snapshot.Turns[turnID])
		}
	}
	if canonical {
		for i, item := range snapshot.Items {
			t := c.turns[item.TurnID]
			if item.Kind != "response" || t == nil || t.finished || t.detached {
				continue
			}
			composed, err := composeResponse(item, snapshotLiveTurn(c, t))
			if err != nil {
				return nil, err
			}
			snapshot.Items[i] = composed
			snapshot.Responses[item.TurnID] = composed
		}
	}
	if canonical {
		c.publishCanonical(query.ConversationID, snapshot.Items)
	}
	return snapshot, nil
}
