package acp

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func captureCompatibilityLogs(t *testing.T) *synchronizedBuffer {
	t.Helper()
	var output synchronizedBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &output
}

func TestPayloadAnteriorDeModelosRegistraUsoRealDaCompatibilidade(t *testing.T) {
	output := captureCompatibilityLogs(t)
	ctx := testContext(t)
	client := newTestClient(t, scriptSoLegado, nil)

	_ = startSession(t, client, ctx)

	logs := output.String()
	if !strings.Contains(logs, `"compatibility_feature":"legacy_models_payload"`) {
		t.Fatalf("uso do payload anterior não foi registrado: %s", logs)
	}
}

func TestFormatoEstavelNaoEhContadoComoCompatibilidade(t *testing.T) {
	output := captureCompatibilityLogs(t)
	ctx := testContext(t)
	client := newTestClient(t, scriptModelo, nil)

	_ = startSession(t, client, ctx)

	if logs := output.String(); strings.Contains(logs, `"compatibility_feature":"legacy_models_payload"`) {
		t.Fatalf("formato estável foi contado como compatibilidade: %s", logs)
	}
}

func TestAgenteHibridoPriorizaFormatoEstavelSemContarCompatibilidade(t *testing.T) {
	output := captureCompatibilityLogs(t)
	ctx := testContext(t)
	client := newTestClient(t, scriptHibrido, nil)

	_ = startSession(t, client, ctx)

	if logs := output.String(); strings.Contains(logs, `"compatibility_feature":"legacy_models_payload"`) {
		t.Fatalf("payload redundante do agente híbrido foi contado como compatibilidade: %s", logs)
	}
}

func TestSeletorAnteriorRegistraMetodoEfetivamenteUsado(t *testing.T) {
	output := captureCompatibilityLogs(t)
	ctx := testContext(t)
	client := newTestClient(t, scriptLegado, nil)
	sess := startSession(t, client, ctx)

	if _, err := sess.SetConfigOption(ctx, "model", "modelo-b"); err != nil {
		t.Fatalf("trocar modelo pelo seletor anterior: %v", err)
	}

	logs := output.String()
	if !strings.Contains(logs, `"compatibility_feature":"legacy_session_selector"`) {
		t.Fatalf("uso do seletor anterior não foi registrado: %s", logs)
	}
	if !strings.Contains(logs, `"selector_method":"session/set_model"`) {
		t.Fatalf("método do seletor não foi registrado: %s", logs)
	}
}
