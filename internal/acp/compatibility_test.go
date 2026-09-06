package acp

import (
	"testing"
)

func newCompatibilityTestClient(t *testing.T, script string) (Client, *[]compatibilityEvent) {
	t.Helper()
	var events []compatibilityEvent
	cfg := fakeConfig(t, script)
	cfg.observeCompatibility = func(event compatibilityEvent) {
		events = append(events, event)
	}
	client, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, &events
}

func TestPayloadAnteriorDeModelosRegistraUsoRealDaCompatibilidade(t *testing.T) {
	ctx := testContext(t)
	client, events := newCompatibilityTestClient(t, scriptSoLegado)

	_ = startSession(t, client, ctx)

	if len(*events) != 1 || (*events)[0].Feature != compatibilityLegacyModelsPayload {
		t.Fatalf("observações = %#v, esperada uma do payload anterior", *events)
	}
}

func TestFormatoEstavelNaoEhContadoComoCompatibilidade(t *testing.T) {
	ctx := testContext(t)
	client, events := newCompatibilityTestClient(t, scriptModelo)

	_ = startSession(t, client, ctx)

	if len(*events) != 0 {
		t.Fatalf("formato estável foi contado como compatibilidade: %#v", *events)
	}
}

func TestAgenteHibridoPriorizaFormatoEstavelSemContarCompatibilidade(t *testing.T) {
	ctx := testContext(t)
	client, events := newCompatibilityTestClient(t, scriptHibrido)

	_ = startSession(t, client, ctx)

	if len(*events) != 0 {
		t.Fatalf("payload redundante do agente híbrido foi contado como compatibilidade: %#v", *events)
	}
}

func TestSeletorAnteriorRegistraMetodoEfetivamenteUsado(t *testing.T) {
	ctx := testContext(t)
	client, events := newCompatibilityTestClient(t, scriptLegado)
	sess := startSession(t, client, ctx)

	if _, err := sess.SetConfigOption(ctx, "model", "modelo-b"); err != nil {
		t.Fatalf("trocar modelo pelo seletor anterior: %v", err)
	}

	if len(*events) != 1 {
		t.Fatalf("observações = %#v, esperada uma do seletor anterior", *events)
	}
	event := (*events)[0]
	if event.Feature != compatibilityLegacySelector ||
		event.SelectorMethod != legacySetModelMethod ||
		event.OptionCategory != CategoryModel {
		t.Fatalf("observação inesperada: %#v", event)
	}
}
