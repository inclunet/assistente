// Package commandinput contém o estado mínimo de entradas físicas.
//
// O pacote não conhece o sistema operacional nem executa comandos. O host é
// responsável por escolher gerações monotônicas ao abrir, reconectar ou
// invalidar uma instância física.
package commandinput

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrInvalidEvent      = errors.New("evento de entrada inválido")
	ErrInvalidGeneration = errors.New("geração inválida")
	ErrStaleGeneration   = errors.New("geração obsoleta")
	ErrNonMonotonicReset = errors.New("reset não é monotônico")
)

// EventKind descreve a transição física observada.
type EventKind uint8

const (
	KeyDown EventKind = iota + 1
	KeyUp
)

// Event é uma observação de uma tecla de uma instância física.
// Generation é fornecida pelo host e não é criada por este pacote.
type Event struct {
	SourceInstance string
	Key            string
	Generation     uint64
	Kind           EventKind
	Repeat         bool
}

// Tracker mantém o estado pressionado separado por instância física e chave.
// A geração é global ao Tracker: o host deve usar um Tracker por domínio de
// invalidação, não esperar gerações independentes por SourceInstance. O valor
// retornado por Transition indica uma nova borda de pressionamento.
type Tracker struct {
	mu         sync.Mutex
	generation uint64
	pressed    map[physicalKey]struct{}
}

type physicalKey struct {
	sourceInstance string
	key            string
}

// New cria um tracker sem geração ativa. O host deve chamar Reset antes do
// primeiro evento; isso torna explícita a abertura da primeira conexão.
func New() *Tracker {
	return &Tracker{pressed: make(map[physicalKey]struct{})}
}

// Reset troca a geração ativa e limpa todas as teclas pressionadas. Gerações
// antigas, iguais ou zero são rejeitadas sem modificar o estado corrente.
func (t *Tracker) Reset(generation uint64) error {
	if t == nil {
		return ErrInvalidGeneration
	}
	if generation == 0 {
		return ErrInvalidGeneration
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if generation <= t.generation {
		return fmt.Errorf("%w: atual=%d recebida=%d", ErrNonMonotonicReset, t.generation, generation)
	}
	t.generation = generation
	if t.pressed == nil {
		t.pressed = make(map[physicalKey]struct{})
	}
	clear(t.pressed)
	return nil
}

// Transition aplica um evento da geração ativa. Uma tecla pressionada sem
// repeat produz true apenas uma vez, até seu release. Eventos obsoletos não
// alteram o estado; eventos de geração futura também são rejeitados até que o
// host faça Reset explicitamente.
func (t *Tracker) Transition(event Event) (bool, error) {
	if t == nil || strings.TrimSpace(event.SourceInstance) == "" || strings.TrimSpace(event.Key) == "" || event.Generation == 0 {
		return false, ErrInvalidEvent
	}
	if event.Kind != KeyDown && event.Kind != KeyUp {
		return false, ErrInvalidEvent
	}
	if event.Kind == KeyUp && event.Repeat {
		return false, ErrInvalidEvent
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if event.Generation != t.generation {
		return false, fmt.Errorf("%w: atual=%d recebida=%d", ErrStaleGeneration, t.generation, event.Generation)
	}

	k := physicalKey{sourceInstance: event.SourceInstance, key: event.Key}
	if event.Kind == KeyUp {
		delete(t.pressed, k)
		return false, nil
	}
	if event.Repeat {
		return false, nil
	}
	if t.pressed == nil {
		t.pressed = make(map[physicalKey]struct{})
	}
	if _, ok := t.pressed[k]; ok {
		return false, nil
	}
	t.pressed[k] = struct{}{}
	return true, nil
}

// Generation devolve a geração ativa para diagnóstico e testes.
func (t *Tracker) Generation() uint64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.generation
}
