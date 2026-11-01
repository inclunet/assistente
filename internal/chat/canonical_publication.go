package chat

import (
	"assistente/internal/database"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// CanonicalItem é a projeção leve usada por snapshot e operações. Não contém
// mensagens físicas completas nem argumentos/resultados integrais de ferramentas.
type CanonicalItem struct {
	ItemID                  string             `json:"itemId"`
	ConversationID          string             `json:"conversationId"`
	ScopeParentID           *string            `json:"scopeParentId"`
	Kind                    string             `json:"kind"`
	TurnID                  *string            `json:"turnId"`
	ReplyToItemID           string             `json:"replyToItemId"`
	MessageIDs              []string           `json:"messageIds"`
	RepresentativeMessageID *string            `json:"representativeMessageId"`
	Position                string             `json:"position"`
	Revision                string             `json:"revision"`
	State                   string             `json:"state"`
	Durability              string             `json:"durability"`
	Segments                []CanonicalSegment `json:"segments"`
	Metadata                CanonicalMetadata  `json:"metadata"`
}
type CanonicalSegment struct {
	SegmentID   string                `json:"segmentId"`
	Kind        string                `json:"kind"`
	Text        string                `json:"text,omitempty"`
	Invocations []TurnSegmentToolCall `json:"invocations,omitempty"`
}
type CanonicalMetadata struct {
	Origin           string `json:"origin"`
	Role             string `json:"role"`
	Pinned           bool   `json:"pinned"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
}
type CanonicalOperation struct {
	Kind      string            `json:"kind"`
	ItemID    string            `json:"itemId"`
	Revision  string            `json:"revision"`
	Item      *CanonicalItem    `json:"item,omitempty"`
	SegmentID string            `json:"segmentId,omitempty"`
	Text      string            `json:"text,omitempty"`
	Segment   *CanonicalSegment `json:"segment,omitempty"`
}
type CanonicalBatch struct {
	ConversationID string               `json:"conversationId"`
	Epoch          string               `json:"epoch"`
	Sequence       string               `json:"sequence"`
	Operations     []CanonicalOperation `json:"operations"`
}

type canonicalPublication struct {
	itemSizes map[string]int
	itemOrder []string
	itemBytes int
	epoch     string
	sequence  uint64
	items     map[string]CanonicalItem
	batches   []CanonicalBatch
	sizes     []int
	bytes     int
}

const canonicalItemCacheBytes = 8 * 1024 * 1024
const canonicalItemCacheCount = 512
const canonicalReplayBatches = 256
const canonicalReplayBytes = 2 * 1024 * 1024

func canonicalItem(item *TimelineItem) CanonicalItem {
	view := CanonicalItem{ItemID: item.ItemID, ConversationID: item.ConversationID, ScopeParentID: item.ScopeParentID, Kind: item.Kind, ReplyToItemID: item.ReplyToItemID, MessageIDs: append([]string{}, item.MessageIDs...), State: item.State, Durability: item.Durability, Segments: make([]CanonicalSegment, 0, len(item.Segments)), Metadata: CanonicalMetadata{Origin: item.Origin}}
	if item.TurnID != "" {
		id := item.TurnID
		view.TurnID = &id
	}
	if item.Representative != nil {
		m := item.Representative
		id := m.ID
		view.RepresentativeMessageID = &id
		view.Metadata.Role = m.Role
		view.Metadata.Pinned = m.Pinned
		view.Metadata.Model = m.Model
		view.Metadata.PromptTokens = m.PromptTokens
		view.Metadata.CompletionTokens = m.CompletionTokens
		view.Metadata.TotalTokens = m.TotalTokens
	} else if item.Kind == "response" {
		view.Metadata.Role = "assistant"
	}
	for _, s := range item.Segments {
		view.Segments = append(view.Segments, CanonicalSegment{SegmentID: s.SegmentID, Kind: s.Kind, Text: s.Text, Invocations: append([]TurnSegmentToolCall(nil), s.Invocations...)})
	}
	if item.ScopeParentID != nil {
		parent := *item.ScopeParentID
		view.ScopeParentID = &parent
	}
	for i := range view.Segments {
		for j := range view.Segments[i].Invocations {
			call := &view.Segments[i].Invocations[j]
			call.ExecutionID = ""
			call.ExecutionDepth = 0
			call.AssistantMessageID = ""
			call.ACPTextOffset = nil
		}
	}
	view.Position = encodeItemPosition(item)
	return view
}

func canonicalDiff(previous *CanonicalItem, next CanonicalItem) []CanonicalOperation {
	if previous == nil {
		return []CanonicalOperation{{Kind: "item_upsert", ItemID: next.ItemID, Item: &next}}
	}
	old := *previous
	old.Revision = ""
	next.Revision = ""
	if reflect.DeepEqual(old, next) {
		return nil
	}
	oldSegments, newSegments := old.Segments, next.Segments
	old.Segments = nil
	next.Segments = nil
	structural := !reflect.DeepEqual(old, next) || len(oldSegments) != len(newSegments)
	if !structural {
		for i := range oldSegments {
			if oldSegments[i].SegmentID != newSegments[i].SegmentID || oldSegments[i].Kind != newSegments[i].Kind {
				structural = true
				break
			}
		}
	}
	next.Segments = newSegments
	if structural {
		return []CanonicalOperation{{Kind: "item_upsert", ItemID: next.ItemID, Item: &next}}
	}
	operations := make([]CanonicalOperation, 0)
	for i, segment := range newSegments {
		before := oldSegments[i]
		if reflect.DeepEqual(before, segment) {
			continue
		}
		op := CanonicalOperation{Kind: "segment_replace", ItemID: next.ItemID, SegmentID: segment.SegmentID, Segment: &segment}
		if reflect.DeepEqual(before.Invocations, segment.Invocations) && strings.HasPrefix(segment.Text, before.Text) {
			op.Kind = "segment_append"
			op.Text = strings.TrimPrefix(segment.Text, before.Text)
			op.Segment = nil
		}
		operations = append(operations, op)
	}
	return operations
}

// Publica os mesmos valores canônicos que serão devolvidos na leitura, sem
// interpretar comandos do handler. Deve rodar sob a barreira da conversa.
func (c *projectionConversation) publishCanonical(conversationID string, items []*TimelineItem) {
	c.publishCanonicalChanges(conversationID, items, nil)
}

func (c *projectionConversation) publishCanonicalChanges(conversationID string, items []*TimelineItem, removed []string) {
	p := &c.publication
	if p.epoch != c.epoch {
		*p = canonicalPublication{epoch: c.epoch, items: make(map[string]CanonicalItem), itemSizes: make(map[string]int)}
	}
	changed := make([]CanonicalItem, 0)
	operations := make([]CanonicalOperation, 0)
	for _, id := range removed {
		operations = append(operations, CanonicalOperation{Kind: "item_remove", ItemID: id})
		p.removeItem(id)
	}
	for _, item := range items {
		next := canonicalItem(item)
		var previous *CanonicalItem
		if old, ok := p.items[next.ItemID]; ok {
			previous = &old
		}
		delta := canonicalDiff(previous, next)
		if len(delta) == 0 {
			continue
		}
		changed = append(changed, next)
		operations = append(operations, delta...)
	}
	if len(operations) == 0 {
		return
	}
	p.sequence++
	revision := strconv.FormatUint(p.sequence, 10)
	for _, item := range changed {
		item.Revision = revision
		p.cacheItem(item)
	}
	for i := range operations {
		operations[i].Revision = revision
		if operations[i].Item != nil {
			operations[i].Item.Revision = revision
		}
	}
	batch := CanonicalBatch{ConversationID: conversationID, Epoch: c.epoch, Sequence: revision, Operations: operations}
	encoded, _ := json.Marshal(batch)
	if len(encoded) > canonicalReplayBytes {
		p.batches = nil
		p.sizes = nil
		p.bytes = 0
		return
	}
	p.batches = append(p.batches, batch)
	p.sizes = append(p.sizes, len(encoded))
	p.bytes += len(encoded)
	for len(p.batches) > canonicalReplayBatches || p.bytes > canonicalReplayBytes {
		p.bytes -= p.sizes[0]
		p.batches[0] = CanonicalBatch{}
		p.batches = p.batches[1:]
		p.sizes = p.sizes[1:]
	}
}

func (p *canonicalPublication) removeItem(id string) {
	p.itemBytes -= p.itemSizes[id]
	delete(p.itemSizes, id)
	delete(p.items, id)
	p.itemOrder = slices.DeleteFunc(p.itemOrder, func(value string) bool { return value == id })
}

func encodeItemPosition(item *TimelineItem) string {
	kind := database.MessageWindowItemKindMessage
	id := strings.TrimPrefix(item.ItemID, "message:")
	if item.Kind == "response" {
		kind = database.MessageWindowItemKindTurn
		id = item.TurnID
	}
	return database.EncodeTimelineCursor(item.ConversationID, item.ScopeParentID, database.MessageWindowItem{Kind: kind, ID: id, AnchorID: item.Position.AnchorID, AnchorCreatedAt: item.Position.AnchorCreatedAt})
}

// Este cache é apenas a base de diff, não o conteúdo vivo nem a retenção da UI.
// Evicção perde a base: a próxima publicação obrigatoriamente usa upsert.
func (p *canonicalPublication) cacheItem(item CanonicalItem) {
	encoded, _ := json.Marshal(item)
	if old, ok := p.itemSizes[item.ItemID]; ok {
		p.itemBytes -= old
	}
	if len(encoded) > canonicalItemCacheBytes {
		p.itemOrder = slices.DeleteFunc(p.itemOrder, func(id string) bool { return id == item.ItemID })
		delete(p.items, item.ItemID)
		delete(p.itemSizes, item.ItemID)
		return
	}
	if _, ok := p.items[item.ItemID]; !ok {
		p.itemOrder = append(p.itemOrder, item.ItemID)
	}
	p.items[item.ItemID] = item
	p.itemSizes[item.ItemID] = len(encoded)
	p.itemBytes += len(encoded)
	for len(p.items) > canonicalItemCacheCount || p.itemBytes > canonicalItemCacheBytes {
		id := p.itemOrder[0]
		p.itemOrder = p.itemOrder[1:]
		p.itemBytes -= p.itemSizes[id]
		delete(p.itemSizes, id)
		delete(p.items, id)
	}
}

// Apenas lotes já publicados. Não afirma que fontes externas foram atualizadas.
// A borda futura precisa conectar mutações/invalidações antes de oferecer sync.
func (p *canonicalPublication) replay(epoch string, after uint64) ([]CanonicalBatch, bool) {
	if epoch != p.epoch || after > p.sequence {
		return nil, false
	}
	if after == p.sequence {
		return []CanonicalBatch{}, true
	}
	first := p.sequence - uint64(len(p.batches)) + 1
	if after+1 < first {
		return nil, false
	}
	start := int(after + 1 - first)
	data, _ := json.Marshal(p.batches[start:])
	var batches []CanonicalBatch
	if json.Unmarshal(data, &batches) != nil {
		return nil, false
	}
	return batches, true
}
