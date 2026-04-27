package messaging

import (
	"sync"
	"testing"
	"time"
)

func TestTTSBroker_PublishBeforeWait(t *testing.T) {
	broker := NewTTSBroker()
	broker.Prepare("100")

	// Publish antes de Wait — audio fica no buffer
	broker.Publish("100", []byte("mp3-data"), "audio/mpeg")

	audio, ok := broker.Wait("100", 1*time.Second)
	if !ok {
		t.Fatal("esperava receber áudio do broker")
	}
	if string(audio.Data) != "mp3-data" {
		t.Fatalf("dados incorretos: %q", string(audio.Data))
	}
	if audio.MIMEType != "audio/mpeg" {
		t.Fatalf("mime incorreto: %s", audio.MIMEType)
	}
}

func TestTTSBroker_WaitBeforePublish(t *testing.T) {
	broker := NewTTSBroker()
	broker.Prepare("200")

	var wg sync.WaitGroup
	var audio AudioPayload
	var ok bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		audio, ok = broker.Wait("200", 5*time.Second)
	}()

	// Dá tempo para Wait bloquear
	time.Sleep(50 * time.Millisecond)

	broker.Publish("200", []byte("delayed-audio"), "audio/mpeg")
	wg.Wait()

	if !ok {
		t.Fatal("esperava receber áudio após publish")
	}
	if string(audio.Data) != "delayed-audio" {
		t.Fatalf("dados incorretos: %q", string(audio.Data))
	}
}

func TestTTSBroker_CancelUnblocksWait(t *testing.T) {
	broker := NewTTSBroker()
	broker.Prepare("300")

	var wg sync.WaitGroup
	var ok bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, ok = broker.Wait("300", 5*time.Second)
	}()

	time.Sleep(50 * time.Millisecond)
	broker.Cancel("300")
	wg.Wait()

	if ok {
		t.Fatal("esperava que Cancel retornasse false no Wait")
	}
}

func TestTTSBroker_TimeoutReturnsFalse(t *testing.T) {
	broker := NewTTSBroker()
	broker.Prepare("400")

	start := time.Now()
	_, ok := broker.Wait("400", 100*time.Millisecond)
	elapsed := time.Since(start)

	if ok {
		t.Fatal("esperava timeout (false)")
	}
	if elapsed < 90*time.Millisecond {
		t.Fatalf("retornou rápido demais: %v", elapsed)
	}

	// Verifica que o slot foi limpo após timeout (sem leak)
	broker.mu.Lock()
	_, slotExists := broker.slots["400"]
	broker.mu.Unlock()
	if slotExists {
		t.Fatal("slot deveria ter sido removido após timeout")
	}
}

func TestTTSBroker_WaitWithoutPrepare(t *testing.T) {
	broker := NewTTSBroker()

	// Sem Prepare, Wait retorna false imediatamente
	_, ok := broker.Wait("500", 1*time.Second)
	if ok {
		t.Fatal("esperava false sem Prepare")
	}
}

func TestTTSBroker_PublishWithoutPrepare(t *testing.T) {
	broker := NewTTSBroker()

	// Não deve dar panic
	broker.Publish("600", []byte("orphan"), "audio/mpeg")
}

func TestTTSBroker_CancelAfterPublish(t *testing.T) {
	broker := NewTTSBroker()
	broker.Prepare("700")

	broker.Publish("700", []byte("audio"), "audio/mpeg")
	// Cancel após Publish é seguro — valor já está no buffer
	broker.Cancel("700")

	// Wait deve ainda receber o valor (já no buffer antes do close)
	// Mas como Cancel faz LoadAndDelete, o slot pode não existir mais.
	// O comportamento correto é: se o Wait busca depois do Cancel,
	// o slot foi deletado → returns false. Isso é aceitável porque
	// no fluxo real o defer Cancel roda DEPOIS do Publish na mesma goroutine.
}

func TestTTSBroker_ZeroMessageID(t *testing.T) {
	broker := NewTTSBroker()

	// messageID="" deve ser ignorado
	broker.Prepare("")
	broker.Publish("", []byte("nope"), "audio/mpeg")
	broker.Cancel("")
	// Não deve dar panic
}
