package llm

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"assistente/internal/logging"
)

// streamDiagnostics observa apenas metadados da tentativa; nunca armazena payload.
type streamDiagnostics struct {
	mu                                       sync.Mutex
	started, lastRead, lastEvent, phaseSince time.Time
	bytes, reads, events                     int64
	phase                                    string
}

type streamDiagnosticsKey struct{}

func newStreamDiagnostics() *streamDiagnostics {
	now := time.Now()
	return &streamDiagnostics{started: now, phaseSince: now, phase: "request"}
}

func (d *streamDiagnostics) read(n int) {
	if n <= 0 {
		return
	}
	d.mu.Lock()
	d.bytes += int64(n)
	d.reads++
	d.lastRead = time.Now()
	d.mu.Unlock()
}

func (d *streamDiagnostics) setPhase(phase string, event bool, wd *streamWatchdog) {
	// A decisão do timeout e a mudança de fase compartilham esta barreira.
	wd.mu.Lock()
	defer wd.mu.Unlock()
	if wd.timedOut {
		return
	}
	d.mu.Lock()
	now := time.Now()
	d.phase, d.phaseSince = phase, now
	if event {
		d.events++
		d.lastEvent = now
	}
	d.mu.Unlock()
}

func (d *streamDiagnostics) logTimeout(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	age := func(t time.Time) int64 {
		if t.IsZero() {
			return -1
		}
		return now.Sub(t).Milliseconds()
	}
	logging.Warnf(ctx, "llm.stream-diagnostics", "stream_idle_diagnostic phase=%s elapsed_ms=%d phase_ms=%d bytes=%d reads=%d events=%d last_read_ms=%d last_event_ms=%d", d.phase, age(d.started), age(d.phaseSince), d.bytes, d.reads, d.events, age(d.lastRead), age(d.lastEvent))
}

// O transport preserva autenticação e redirects e envolve somente respostas
// solicitadas com um diagnóstico associado ao contexto da tentativa.
type streamDiagnosticTransport struct{ base http.RoundTripper }

func (t streamDiagnosticTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (t streamDiagnosticTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	if d, ok := req.Context().Value(streamDiagnosticsKey{}).(*streamDiagnostics); ok {
		resp.Body = &streamDiagnosticBody{ReadCloser: resp.Body, diagnostics: d}
	}
	return resp, err
}

type streamDiagnosticBody struct {
	io.ReadCloser
	diagnostics *streamDiagnostics
}

func (b *streamDiagnosticBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.diagnostics.read(n)
	return n, err
}
