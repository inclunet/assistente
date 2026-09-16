package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// mcpLogCapture captura records do slog para asserção de nível em testes.
type mcpLogCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (*mcpLogCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h *mcpLogCapture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Clone())
	h.mu.Unlock()
	return nil
}
func (h *mcpLogCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *mcpLogCapture) WithGroup(string) slog.Handler      { return h }
func (h *mcpLogCapture) count(level slog.Level, substr string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for i := range h.records {
		if h.records[i].Level == level && strings.Contains(h.records[i].Message, substr) {
			n++
		}
	}
	return n
}

func TestBenignCtxCancel(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	// ctx-pai com deadline já estourado (ex.: timeout de login) — benigno.
	deadlined, cancelD := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelD()

	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"ctx cancelado", canceled, nil, true},
		{"ctx-pai deadline estourado", deadlined, nil, true},
		{"err context.Canceled", context.Background(), context.Canceled, true},
		{"err envolve Canceled", context.Background(), fmt.Errorf("falha: %w", context.Canceled), true},
		// DeadlineExceeded solto = timeout real de handshake por conexão: NÃO benigno.
		{"err DeadlineExceeded solto", context.Background(), context.DeadlineExceeded, false},
		{"err comum", context.Background(), fmt.Errorf("Unauthorized"), false},
		{"nil tudo", nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := benignCtxCancel(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("benignCtxCancel = %v; want %v", got, tc.want)
			}
		})
	}
}

// TestAutoConnectAll_CancelamentoNaoEhErro garante que o cancelamento de
// contexto (shutdown/troca de usuário) durante o AutoConnectAll é logado em
// Debug, não em ERRO — regressão do ruído "[MCP] AutoConnectAll cancelado"
// visto no assistente.log a cada encerramento.
func TestAutoConnectAll_CancelamentoNaoEhErro(t *testing.T) {
	m := newTestManagerWithTempDir(t)
	for _, slug := range []string{"a", "b", "c"} {
		m.servers[slug] = &ServerStatus{
			Slug: slug,
			Config: ServerConfig{
				Enabled:     true,
				AutoConnect: true,
				Transport:   TransportStreamable,
				URL:         "http://127.0.0.1:1/mcp",
			},
		}
	}

	cap := &mcpLogCapture{}
	old := slog.Default()
	slog.SetDefault(slog.New(cap))
	defer slog.SetDefault(old)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		m.AutoConnectAll(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("AutoConnectAll não retornou com ctx cancelado")
	}

	if got := cap.count(slog.LevelError, "AutoConnectAll"); got != 0 {
		t.Fatalf("cancelamento não pode gerar ERRO de AutoConnectAll; obtido %d", got)
	}
	if got := cap.count(slog.LevelDebug, "AutoConnectAll interrompido"); got != 1 {
		t.Fatalf("esperado 1 Debug 'AutoConnectAll interrompido', obtido %d", got)
	}
}
