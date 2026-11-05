package chat

import "crypto/sha256"

// Somente identidade e faixas: os bytes continuam pertencendo à fonte persistida.
type presentationSpan struct {
	id, kind   string
	start, end int
}
type presentationSource struct {
	textHash, reasoningHash [32]byte
	spans                   []presentationSpan
}
type presentationBoundaries struct {
	incarnation string
	sources     map[string]presentationSource
	bytes       int
}

const presentationBoundaryBudget = 8 * 1024 * 1024

// O vínculo foi fornecido pelo writer. A igualdade verifica os bytes confirmados;
// não descobre a mensagem por prefixo, semelhança ou posição na conversa.
func confirmedPresentationSource(message Message, stream LiveStreamSnapshot) (presentationSource, bool) {
	result := presentationSource{textHash: sha256.Sum256([]byte(message.Content)), reasoningHash: sha256.Sum256([]byte(message.Reasoning))}
	textOffset, reasoningOffset := 0, 0
	for _, segment := range stream.Segments {
		content, offset := message.Content, &textOffset
		if segment.Kind == "reasoning" {
			content, offset = message.Reasoning, &reasoningOffset
		} else if segment.Kind != "text" {
			return presentationSource{}, false
		}
		end := *offset + len(segment.Text)
		if end > len(content) || content[*offset:end] != segment.Text {
			return presentationSource{}, false
		}
		result.spans = append(result.spans, presentationSpan{segment.SegmentID, segment.Kind, *offset, end})
		*offset = end
	}
	return result, textOffset == len(message.Content) && reasoningOffset == len(message.Reasoning)
}

func (source presentationSource) matches(message Message) bool {
	return source.textHash == sha256.Sum256([]byte(message.Content)) && source.reasoningHash == sha256.Sum256([]byte(message.Reasoning))
}

func (source presentationSource) segments(message Message) ([]ResponseSegment, bool) {
	if !source.matches(message) {
		return nil, false
	}
	segments := make([]ResponseSegment, 0, len(source.spans))
	for _, span := range source.spans {
		content := message.Content
		if span.kind == "reasoning" {
			content = message.Reasoning
		}
		segments = append(segments, ResponseSegment{SourceMessageID: message.ID, SegmentID: span.id, Kind: span.kind, Text: content[span.start:span.end]})
	}
	return segments, true
}

func presentationSourceSize(id string, source presentationSource) int {
	size := len(id) + 128
	for _, span := range source.spans {
		size += len(span.id) + len(span.kind) + 32
	}
	return size
}

func (c *projectionConversation) rememberPresentationBoundaries(item *TimelineItem, live *LiveTurnSnapshot) error {
	// A admissão conta descritores antes de alocá-los; itens materializados nunca
	// perdem identidade para abrir espaço para uma nova fonte.
	active := make(map[string]bool)
	for _, window := range c.windows {
		for id := range window.itemIDs {
			active[id] = true
		}
	}
	used := len(item.ItemID) + len(c.incarnation) + 64
	for id, previous := range c.presentationBoundaries {
		if id != item.ItemID && active[id] {
			used += previous.bytes
		}
	}
	if used > presentationBoundaryBudget {
		return ErrTimelineSourceLimit
	}
	byID := make(map[string]Message, len(item.messages))
	for _, message := range item.messages {
		byID[message.ID] = message
	}
	record := presentationBoundaries{incarnation: c.incarnation, sources: make(map[string]presentationSource), bytes: len(item.ItemID) + len(c.incarnation) + 64}
	if previous, found := c.presentationBoundaries[item.ItemID]; found && previous.incarnation == c.incarnation {
		for id, source := range previous.sources {
			if message, present := byID[id]; present && source.matches(message) {
				size := presentationSourceSize(id, source)
				if size > presentationBoundaryBudget-used {
					return ErrTimelineSourceLimit
				}
				record.sources[id] = source
				record.bytes += size
				used += size
			}
		}
	}
	for _, stream := range live.Streams {
		if !stream.Confirmed {
			continue
		}
		message, found := byID[stream.PersistedMessageID]
		if !found {
			continue
		}
		size := len(message.ID) + 128
		for _, segment := range stream.Segments {
			size += len(segment.SegmentID) + len(segment.Kind) + 32
		}
		previousSize := 0
		if previous, exists := record.sources[message.ID]; exists {
			previousSize = presentationSourceSize(message.ID, previous)
		}
		if size > presentationBoundaryBudget-used+previousSize {
			return ErrTimelineSourceLimit
		}
		source, valid := confirmedPresentationSource(message, stream)
		if !valid {
			continue
		} // Uma edição persistida não recebe os cortes do buffer antigo.
		record.sources[message.ID] = source
		record.bytes += size - previousSize
		used += size - previousSize
	}
	if c.presentationBoundaries == nil {
		c.presentationBoundaries = make(map[string]presentationBoundaries)
	}
	for id := range c.presentationBoundaries {
		if !active[id] && id != item.ItemID {
			delete(c.presentationBoundaries, id)
		}
	}
	if len(record.sources) == 0 {
		delete(c.presentationBoundaries, item.ItemID)
	} else {
		c.presentationBoundaries[item.ItemID] = record
	}
	return nil
}

func (c *projectionConversation) presentationSources(item *TimelineItem) map[string][]ResponseSegment {
	record, found := c.presentationBoundaries[item.ItemID]
	if !found || record.incarnation != c.incarnation {
		return nil
	}
	sources := make(map[string][]ResponseSegment)
	for _, message := range item.messages {
		if source, ok := record.sources[message.ID]; ok {
			if segments, valid := source.segments(message); valid {
				sources[message.ID] = segments
			}
		}
	}
	return sources
}

func (c *projectionConversation) applyPresentationBoundaries(item *TimelineItem) *TimelineItem {
	sources := c.presentationSources(item)
	if len(sources) == 0 {
		return item
	}
	result := *item
	result.Segments = projectResponseSegmentsWithSources(item.messages, item.invocations, item.Representative, sources)
	return &result
}
