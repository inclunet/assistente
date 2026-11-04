package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"assistente/internal/apidto"
)

const (
	recoveryPageTextBytes = 64 * 1024
	recoveryPageVisits    = 128
	recoveryIdentityBytes = 1024
	recoveryCursorBytes   = 16 * 1024
)

var ErrRecoveryCursorInvalid = errors.New("cursor de recuperação inválido")
var ErrRecoveryCursorStale = errors.New("conteúdo de recuperação mudou; reinicie a leitura")
var ErrRecoveryEncoding = errors.New("conteúdo de recuperação ainda não forma texto UTF-8 válido")

type recoveryPosition struct {
	Stream int `json:"stream"`
	Part   int `json:"part"`
	Field  int `json:"field"`
	Offset int `json:"offset"`
}
type recoveryPageCursor struct {
	Format         int                       `json:"format"`
	ConversationID string                    `json:"conversationId"`
	Reference      TimelineRecoveryReference `json:"reference"`
	ContentVersion string                    `json:"contentVersion"`
	Position       recoveryPosition          `json:"position"`
}

func decodeRecoveryCursor(value string) (*recoveryPageCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > recoveryCursorBytes {
		return nil, ErrRecoveryCursorInvalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return nil, ErrRecoveryCursorInvalid
	}
	var cursor recoveryPageCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.Format != 1 || cursor.ContentVersion == "" || cursor.Position.Stream < 0 || cursor.Position.Part < 0 || cursor.Position.Field < 0 || cursor.Position.Field > 1 || cursor.Position.Offset < 0 {
		return nil, ErrRecoveryCursorInvalid
	}
	return &cursor, nil
}

// ReadRecoveryPage lê somente buffers da referência exata. O cursor não concede
// acesso nem fixa uma cópia: mutações da execução invalidam a próxima página.
// Cada página visita até 128 buffers/iterações e copia até 64 KiB de texto bruto;
// envelope limitado mantém o JSON escapado abaixo de 512 KiB.
func (s *TimelineSubscription) ReadRecoveryPage(ctx context.Context, request apidto.TimelineRecoveryPageRequest, reader TimelineLifecycleReader) (*apidto.TimelineRecoveryPage, error) {
	reference := request.Reference
	if len(reference.ProjectionID)+len(reference.Incarnation)+len(reference.TurnID)+len(reference.ExecutionID) > recoveryIdentityBytes {
		return nil, ErrTimelineSourceLimit
	}
	cursor, err := decodeRecoveryCursor(request.Cursor)
	if err != nil {
		return nil, err
	}
	target, release, err := s.lockRecoveryTarget(ctx, reference, reader)
	if err != nil {
		return nil, err
	}
	defer release()
	if len(s.key.conversationID)+len(reference.ProjectionID)+len(reference.Incarnation)+len(reference.TurnID)+len(reference.ExecutionID) > recoveryIdentityBytes {
		return nil, ErrTimelineSourceLimit
	}
	if cursor != nil && (cursor.ConversationID != s.key.conversationID || cursor.Reference != reference) {
		return nil, ErrRecoveryCursorInvalid
	}
	page := &apidto.TimelineRecoveryPage{ConversationID: s.key.conversationID, Reference: reference, Chunks: []apidto.TimelineRecoveryChunk{}}
	if t := target.turn; t != nil {
		page.Found = true
		page.ContentVersion = strconv.FormatUint(t.contentVersion, 10)
		position := recoveryPosition{}
		if cursor != nil {
			if cursor.ContentVersion != page.ContentVersion {
				return nil, ErrRecoveryCursorStale
			}
			position = cursor.Position
			if position.Stream >= len(t.streamOrder) {
				return nil, ErrRecoveryCursorInvalid
			}
		}
		position, err = readRecoveryChunks(ctx, t, position, page)
		if err != nil {
			return nil, err
		}
		if position.Stream < len(t.streamOrder) {
			raw, encodeErr := json.Marshal(recoveryPageCursor{1, s.key.conversationID, reference, page.ContentVersion, position})
			if encodeErr != nil {
				return nil, encodeErr
			}
			page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = s.authorize(ctx); err != nil {
		return nil, err
	}
	return page, nil
}

func (p *recoveryPosition) advance(stream *liveStream) {
	p.Offset = 0
	p.Field++
	if p.Field == 2 {
		p.Field = 0
		p.Part++
	}
	if p.Part == len(stream.parts) {
		p.Stream++
		p.Part = 0
	}
}

func readRecoveryChunks(ctx context.Context, t *liveTurn, position recoveryPosition, page *apidto.TimelineRecoveryPage) (recoveryPosition, error) {
	remaining := recoveryPageTextBytes
	for visited := 0; visited < recoveryPageVisits && remaining > 0 && position.Stream < len(t.streamOrder); visited++ {
		if err := ctx.Err(); err != nil {
			return position, err
		}
		iteration := t.streamOrder[position.Stream]
		stream := t.streams[iteration]
		if stream == nil || position.Part >= len(stream.parts) {
			return position, ErrRecoveryCursorInvalid
		}
		part := stream.parts[position.Part]
		kind, text := "reasoning", part.reasoning.String()
		if position.Field == 1 {
			kind, text = "text", part.text.String()
		}
		if position.Offset > len(text) || (position.Offset > 0 && position.Offset < len(text) && !utf8.RuneStart(text[position.Offset])) {
			return position, ErrRecoveryCursorInvalid
		}
		if position.Offset == len(text) {
			position.advance(stream)
			continue
		}
		end := position.Offset + min(len(text)-position.Offset, remaining)
		for end > position.Offset && end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		if end == position.Offset {
			if len(page.Chunks) == 0 {
				return position, ErrRecoveryEncoding
			}
			break // a próxima rune não cabe no restante desta página
		}
		chunk := text[position.Offset:end]
		if !utf8.ValidString(chunk) {
			return position, ErrRecoveryEncoding
		}
		page.Chunks = append(page.Chunks, apidto.TimelineRecoveryChunk{Iteration: iteration, Generation: strconv.FormatUint(stream.generation, 10), Part: position.Part, Kind: kind, ByteOffset: position.Offset, Text: strings.Clone(chunk)})
		remaining -= len(chunk)
		position.Offset = end
		if end == len(text) {
			position.advance(stream)
		}
	}
	return position, nil
}
