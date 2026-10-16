package commandinput

import (
	"errors"
	"sync"
	"testing"
)

func TestPressRepeatDoubleDownRelease(t *testing.T) {
	tr := New()
	if err := tr.Reset(1); err != nil {
		t.Fatal(err)
	}
	down := Event{SourceInstance: "deck-a", Key: "K1", Generation: 1, Kind: KeyDown}
	for i, event := range []Event{down, down, {SourceInstance: "deck-a", Key: "K1", Generation: 1, Kind: KeyDown, Repeat: true}} {
		got, err := tr.Transition(event)
		if err != nil {
			t.Fatalf("evento %+v: borda=%v erro=%v", event, got, err)
		}
		if want := i == 0; got != want {
			t.Fatalf("evento %+v: borda=%v erro=%v", event, got, err)
		}
	}
	// A primeira ocorrência gerou a borda; o segundo down e o repeat não.
	if got, err := tr.Transition(Event{SourceInstance: "deck-a", Key: "K1", Generation: 1, Kind: KeyDown, Repeat: true}); err != nil || got {
		t.Fatalf("repeat: borda=%v erro=%v", got, err)
	}
	if got, err := tr.Transition(Event{SourceInstance: "deck-a", Key: "K1", Generation: 1, Kind: KeyUp}); err != nil || got {
		t.Fatalf("release: borda=%v erro=%v", got, err)
	}
	if got, err := tr.Transition(down); err != nil || !got {
		t.Fatalf("novo down após release: borda=%v erro=%v", got, err)
	}
}

func TestPressRepeatComoPrimeiroEventoNaoGeraBorda(t *testing.T) {
	tr := New()
	if err := tr.Reset(1); err != nil {
		t.Fatal(err)
	}
	repeat := Event{SourceInstance: "deck-a", Key: "K1", Generation: 1, Kind: KeyDown, Repeat: true}
	if got, err := tr.Transition(repeat); err != nil || got {
		t.Fatalf("repeat inicial: borda=%v erro=%v", got, err)
	}
	if got, err := tr.Transition(Event{SourceInstance: "deck-a", Key: "K1", Generation: 1, Kind: KeyDown}); err != nil || !got {
		t.Fatalf("down após repeat inicial: borda=%v erro=%v", got, err)
	}
}

func TestPressSeparatesChavesEInstancias(t *testing.T) {
	tr := New()
	if err := tr.Reset(3); err != nil {
		t.Fatal(err)
	}
	events := []Event{{"a", "K1", 3, KeyDown, false}, {"a", "K2", 3, KeyDown, false}, {"b", "K1", 3, KeyDown, false}}
	for _, event := range events {
		if got, err := tr.Transition(event); err != nil || !got {
			t.Fatalf("evento independente %+v: borda=%v erro=%v", event, got, err)
		}
	}
}

func TestPressGenerationObsoletaResetLimpaEResetAtrasadoNaoRegride(t *testing.T) {
	tr := New()
	if err := tr.Reset(10); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Transition(Event{"a", "K1", 10, KeyDown, false}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Reset(11); err != nil {
		t.Fatal(err)
	}
	if got, err := tr.Transition(Event{"a", "K1", 11, KeyDown, false}); err != nil || !got {
		t.Fatalf("estado não foi limpo: borda=%v erro=%v", got, err)
	}
	if err := tr.Reset(9); !errors.Is(err, ErrNonMonotonicReset) || tr.Generation() != 11 {
		t.Fatalf("reset atrasado: erro=%v geração=%d", err, tr.Generation())
	}
	if got, err := tr.Transition(Event{"a", "K1", 11, KeyDown, false}); err != nil || got {
		t.Fatalf("reset atrasado limpou estado: borda=%v erro=%v", got, err)
	}
	if _, err := tr.Transition(Event{"a", "K1", 10, KeyUp, false}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("release velho: %v", err)
	}
	if got, err := tr.Transition(Event{"a", "K1", 11, KeyDown, false}); err != nil || got {
		t.Fatalf("release velho liberou tecla: borda=%v erro=%v", got, err)
	}
	if _, err := tr.Transition(Event{"a", "K2", 10, KeyDown, false}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("evento velho: %v", err)
	}
	if _, err := tr.Transition(Event{"a", "K2", 12, KeyDown, false}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("evento futuro: %v", err)
	}
	if got, err := tr.Transition(Event{"a", "K2", 11, KeyDown, false}); err != nil || !got {
		t.Fatalf("evento futuro alterou estado: borda=%v erro=%v", got, err)
	}
	if err := tr.Reset(11); !errors.Is(err, ErrNonMonotonicReset) || tr.Generation() != 11 {
		t.Fatalf("reset igual: erro=%v geração=%d", err, tr.Generation())
	}
	if got, err := tr.Transition(Event{"a", "K2", 11, KeyDown, false}); err != nil || got {
		t.Fatalf("reset igual limpou estado: borda=%v erro=%v", got, err)
	}
}

func TestPressInputsInvalidosFalhamFechado(t *testing.T) {
	tr := New()
	if err := tr.Reset(1); err != nil {
		t.Fatal(err)
	}
	for _, event := range []Event{{Key: "K", Generation: 1, Kind: KeyDown}, {SourceInstance: "a", Generation: 1, Kind: KeyDown}, {SourceInstance: "a", Key: "K", Generation: 1}, {SourceInstance: "a", Key: "K", Generation: 0, Kind: KeyDown}, {SourceInstance: "a", Key: "K", Generation: 1, Kind: KeyUp, Repeat: true}} {
		if got, err := tr.Transition(event); got || !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("evento inválido %+v: borda=%v erro=%v", event, got, err)
		}
	}
	if err := tr.Reset(0); !errors.Is(err, ErrInvalidGeneration) {
		t.Fatalf("geração zero: %v", err)
	}
}

func TestPressConcorrenteUmaBorda(t *testing.T) {
	tr := New()
	if err := tr.Reset(1); err != nil {
		t.Fatal(err)
	}
	const n = 64
	var wg sync.WaitGroup
	var mu sync.Mutex
	borders := 0
	var transitionErr error
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := tr.Transition(Event{"deck", "K1", 1, KeyDown, false})
			if err != nil {
				mu.Lock()
				if transitionErr == nil {
					transitionErr = err
				}
				mu.Unlock()
			} else if got {
				mu.Lock()
				borders++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if transitionErr != nil {
		t.Fatalf("transição concorrente: %v", transitionErr)
	}
	if borders != 1 {
		t.Fatalf("bordas concorrentes=%d", borders)
	}
}
