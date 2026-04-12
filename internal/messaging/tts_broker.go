package messaging

import (
	"log"
	"sync"
	"time"
)

// AudioPayload contém dados de áudio gerados pelo TTS proativo.
type AudioPayload struct {
	Data     []byte
	MIMEType string
}

// TTSBroker coordena a entrega de áudio TTS proativo entre onResponseSaved
// e o Gateway callback, evitando duplicação de chamadas TTS.
//
// Fluxo:
//  1. App salva mensagem do assistente → chama Prepare(messageID)
//  2. App chama Notify → Gateway callback inicia em goroutine
//  3. Gateway callback → cache miss → chama Wait(messageID, timeout) → bloqueia
//  4. onResponseSaved goroutine → gera TTS → chama Publish(messageID, audio)
//  5. Wait desbloqueia → Gateway envia áudio ao messenger (SIP, Telegram, etc.)
//
// Se timeout: Gateway envia texto → SIP faz SpeakText (fallback).
// Se TTS não aplicável: Cancel desbloqueia Wait imediatamente.
type TTSBroker struct {
	mu    sync.Mutex
	slots map[uint]chan AudioPayload // messageID → canal de entrega
}

// NewTTSBroker cria um novo broker de áudio TTS.
func NewTTSBroker() *TTSBroker {
	return &TTSBroker{slots: make(map[uint]chan AudioPayload)}
}

// Prepare registra que áudio TTS será gerado para esta mensagem.
// Deve ser chamado ANTES do Notify para garantir que Wait encontre o canal.
func (b *TTSBroker) Prepare(messageID uint) {
	if messageID == 0 {
		return
	}
	b.mu.Lock()
	b.slots[messageID] = make(chan AudioPayload, 1) // buffered: Publish não bloqueia
	b.mu.Unlock()
}

// Wait espera o áudio TTS ficar pronto (com timeout).
// Retorna (payload, true) se recebeu áudio, (zero, false) se timeout ou cancelado.
func (b *TTSBroker) Wait(messageID uint, timeout time.Duration) (AudioPayload, bool) {
	b.mu.Lock()
	ch, ok := b.slots[messageID]
	b.mu.Unlock()

	if !ok {
		return AudioPayload{}, false
	}

	select {
	case audio := <-ch:
		b.mu.Lock()
		delete(b.slots, messageID)
		b.mu.Unlock()
		return audio, len(audio.Data) > 0
	case <-time.After(timeout):
		b.mu.Lock()
		delete(b.slots, messageID)
		b.mu.Unlock()
		log.Printf("[TTSBroker] timeout (%v) aguardando áudio para msg %d", timeout, messageID)
		return AudioPayload{}, false
	}
}

// Publish envia áudio gerado para quem está esperando (Wait).
// Se ninguém estiver esperando, o áudio vai para o buffer do canal
// e será descartado quando o slot for sobrescrito pelo próximo Prepare.
func (b *TTSBroker) Publish(messageID uint, data []byte, mimeType string) {
	if messageID == 0 {
		return
	}
	b.mu.Lock()
	ch, ok := b.slots[messageID]
	b.mu.Unlock()

	if ok {
		select {
		case ch <- AudioPayload{Data: data, MIMEType: mimeType}:
			log.Printf("[TTSBroker] áudio publicado para msg %d (%d bytes)", messageID, len(data))
		default:
			// Canal cheio (já publicou ou ninguém esperando) — não bloqueia
		}
	}
}

// Cancel remove a expectativa pendente sem enviar áudio.
// Desbloqueia qualquer Wait em andamento.
func (b *TTSBroker) Cancel(messageID uint) {
	if messageID == 0 {
		return
	}
	b.mu.Lock()
	ch, ok := b.slots[messageID]
	if ok {
		delete(b.slots, messageID)
	}
	b.mu.Unlock()

	if ok {
		close(ch)
	}
}
