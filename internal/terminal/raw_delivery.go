package terminal

import "sync"

// Cada leitura do PTY tem até ptyReadSize bytes. Durante a troca de entrada,
// o leitor aguarda espaço depois de oito entregas, sem bloquear os locks da sessão.
const rawDeliverySlots = 8

type rawChunk struct{ commandID, text string }

type rawDeliveryOwner struct {
	publish  func(rawChunk)
	mu       sync.Mutex
	changed  *sync.Cond
	opening  bool
	draining bool
	pending  int
	queue    []rawChunk
	// No encerramento forçado restam no máximo a leitura em curso e a cauda do decodificador.
	finalSlots int
	// Notifica o fluxo de saída seguinte e o encerramento após o último bloco.
	after []func()
}

func newRawDeliveryOwner(opening bool, emit func(rawChunk)) *rawDeliveryOwner {
	o := &rawDeliveryOwner{opening: opening, publish: emit}
	o.changed = sync.NewCond(&o.mu)
	if opening {
		o.pending = 1
	}
	return o
}

// reserve ocorre antes de soltar outputMu, de modo que o próximo dono possa
// aguardar exatamente as entregas que já pertencem ao anterior.
func (o *rawDeliveryOwner) reserve() {
	o.mu.Lock()
	o.pending++
	o.mu.Unlock()
}

func (o *rawDeliveryOwner) complete() {
	o.mu.Lock()
	o.pending--
	var after []func()
	if o.pending == 0 {
		after, o.after = o.after, nil
		o.changed.Broadcast()
	}
	o.mu.Unlock()
	for _, notify := range after {
		notify()
	}
}

func (o *rawDeliveryOwner) wait() {
	o.mu.Lock()
	for o.pending != 0 {
		o.changed.Wait()
	}
	o.mu.Unlock()
}

func (o *rawDeliveryOwner) afterDelivery(notify func()) {
	o.mu.Lock()
	if o.pending != 0 {
		o.after = append(o.after, notify)
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()
	notify()
}

func (o *rawDeliveryOwner) finishReader() {
	o.mu.Lock()
	o.finalSlots = 2
	o.changed.Broadcast()
	o.mu.Unlock()
}

func (o *rawDeliveryOwner) submit(raw rawChunk) {
	o.mu.Lock()
	for len(o.queue) >= rawDeliverySlots+o.finalSlots {
		o.changed.Wait()
	}
	if o.opening || o.draining {
		o.queue = append(o.queue, raw)
		o.mu.Unlock()
		return
	}
	o.draining = true
	o.mu.Unlock()
	o.drain(raw)
}

// take entrega o próximo bloco na ordem do leitor. Chamado sob mu.
func (o *rawDeliveryOwner) take() rawChunk {
	o.draining = len(o.queue) > 0
	if !o.draining {
		return rawChunk{}
	}
	raw := o.queue[0]
	o.queue[0] = rawChunk{}
	o.queue = o.queue[1:]
	o.changed.Broadcast()
	return raw
}

func (o *rawDeliveryOwner) drain(raw rawChunk) {
	for raw.text != "" {
		o.publish(raw)
		o.complete()
		o.mu.Lock()
		raw = o.take()
		o.mu.Unlock()
	}
}

func (o *rawDeliveryOwner) opened() {
	o.mu.Lock()
	o.opening = false
	first := o.take()
	o.mu.Unlock()
	o.complete()
	// A escrita não espera que o consumidor alcance um terminal que continua
	// produzindo. A goroutine só existe se a abertura realmente reteve saída.
	if first.text != "" {
		go o.drain(first)
	}
}
