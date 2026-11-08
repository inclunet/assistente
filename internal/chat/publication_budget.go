package chat

import (
	"encoding/json"
	"reflect"

	"sync"
	"unicode/utf8"
)

// Limite conservador antes de copiar/serializar: contempla o pior escape JSON
// (seis bytes por byte de string) e encerra a travessia assim que esgota a cota.
// Só recebe os structs acíclicos da projeção; não percorre graphs arbitrários.
func publicationSourceWithinBudget(source *TimelineItem) bool {
	budget := projectionBudget{remaining: canonicalSourceBytes}
	return budget.item(source)
}

type projectionBudget struct {
	remaining    int
	exactStrings bool
}

// Só descreve os tipos internos visitados pelo orçamento; não guarda dados
// de conversa. Evita reconstruir StructField/Index a cada campo de cada item.
var projectionFieldSizes sync.Map // reflect.Type -> []int imutável

func projectionFieldNameSizes(kind reflect.Type) []int {
	if cached, ok := projectionFieldSizes.Load(kind); ok {
		return cached.([]int)
	}
	names := make([]int, kind.NumField())
	for i := range names {
		names[i] = len(kind.Field(i).Name)
	}
	actual, _ := projectionFieldSizes.LoadOrStore(kind, names)
	return actual.([]int)
}

func (b *projectionBudget) visit(v reflect.Value) bool {
	b.remaining -= 32
	if b.remaining < 0 {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		if !b.stringValue(v.String()) {
			return false
		}
	case reflect.Pointer:
		if !v.IsNil() {
			return b.visit(v.Elem())
		}
	case reflect.Struct:
		names := projectionFieldNameSizes(v.Type())
		for i := 0; i < v.NumField(); i++ {
			b.remaining -= names[i]
			if !b.visit(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !b.visit(v.Index(i)) {
				return false
			}
		}
	}
	return b.remaining >= 0
}

func (b *projectionBudget) item(source *TimelineItem) bool {
	base := *source
	base.messages = nil
	if source.Representative != nil {
		message := publicationSourceMessage(*source.Representative)
		base.Representative = &message
	}
	if !b.visit(reflect.ValueOf(base)) {
		return false
	}
	for _, message := range source.messages {
		if !b.visit(reflect.ValueOf(publicationSourceMessage(message))) {
			return false
		}
	}
	return true
}

// Janela inteira, sob a barreira, antes de snapshotLiveTurn clonar os buffers.
// A estimativa é conservadora: inclui o DTO persistido mesmo quando o live
// vai substituí-las. Não percorre retained nem turnos fora da página.
func windowCompositionWithinBudget(c *projectionConversation, items []*TimelineItem) bool {
	budget := projectionBudget{remaining: canonicalSourceBytes, exactStrings: true}
	for _, item := range items {
		if !budget.visit(reflect.ValueOf(canonicalItem(item))) {
			return false
		}
		t := c.turns[item.TurnID]
		if item.Kind != "response" || t == nil || t.detached || (t.finished && !t.volatile) {
			continue
		}
		// Cada chamada pode virar um grupo próprio e cortar mais um segmento.
		// Conta identidades derivadas mesmo quando o texto persistido ainda não
		// permite posicionar a chamada, antes de clonar ou compor o live.
		maxSourceID := len(t.executionID)*2 + 128
		for _, message := range item.messages {
			maxSourceID = max(maxSourceID, len(message.ID))
		}
		for _, stream := range t.streams {
			maxSourceID = max(maxSourceID, len(stream.messageID))
		}
		for _, segment := range item.Segments {
			maxSourceID = max(maxSourceID, len(segment.SourceMessageID), len(segment.SegmentID))
		}
		for _, segment := range item.Segments {
			for _, call := range segment.Invocations {
				budget.remaining -= 1024
				if !budget.stringBytes(len(call.InvocationID)) || !budget.stringBytes(maxSourceID) || !budget.stringBytes(maxSourceID) {
					return false
				}
			}
		}
		for _, value := range []string{t.turnID, t.executionID, t.responseID, t.state, t.reason} {
			if !budget.stringValue(value) {
				return false
			}
		}
		for _, stream := range t.streams {
			if !budget.stringValue(stream.messageID) {
				return false
			}
			budget.remaining -= 1024
			if budget.remaining < 0 {
				return false
			}
			for _, part := range stream.parts {
				// IDs draft codificam executionID em base64 por segmento. A margem
				// de quatro cópias cobre os dois segmentos e suas identidades.
				for range 4 {
					if !budget.stringBytes(len(t.executionID)) {
						return false
					}
				}
				budget.remaining -= 1024
				if !budget.stringValue(part.text.String()) || !budget.stringValue(part.reasoning.String()) {
					return false
				}
			}
		}
	}
	return true
}

func (b *projectionBudget) stringBytes(size int) bool {
	if b.remaining < 0 || size > b.remaining/6 {
		return false
	}
	b.remaining -= size * 6
	return true
}

// Conta o escape de encoding/json sem construir uma cópia do texto. O modo
// conservador anterior continua usado para bases internas de publicação.
func (b *projectionBudget) stringValue(value string) bool {
	if !b.exactStrings {
		return b.stringBytes(len(value))
	}
	if len(value) > b.remaining {
		return false
	}
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		cost := size
		switch {
		case r == '"' || r == '\\' || r == '\b' || r == '\f' || r == '\n' || r == '\r' || r == '\t':
			cost = 2
		case r < 0x20 || r == '<' || r == '>' || r == '&' || r == 0x2028 || r == 0x2029:
			cost = 6
		case r == utf8.RuneError && size == 1:
			cost = invalidUTF8JSONBytes
		}
		b.remaining -= cost
		if b.remaining < 0 {
			return false
		}
		value = value[size:]
	}
	return true
}

// Go 1.25 escapa a substituição; versões mais novas podem emitir U+FFFD em
// UTF-8. Medir uma constante uma vez mantém a conta alinhada ao encoder ativo.
var invalidUTF8JSONBytes = func() int {
	encoded, _ := json.Marshal(string([]byte{0xff}))
	return len(encoded) - 2
}()
