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
	phase, lastEventType                     string
}

type streamDiagnosticsKey struct{}

func newStreamDiagnostics() *streamDiagnostics {
	now := time.Now()
	return &streamDiagnostics{started: now, phaseSince: now, phase: "request", lastEventType: "none"}
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

func (d *streamDiagnostics) setPhase(phase string, eventType string, wd *streamWatchdog) {
	// A decisão do timeout e a mudança de fase compartilham esta barreira.
	wd.mu.Lock()
	defer wd.mu.Unlock()
	if wd.timedOut {
		return
	}
	d.mu.Lock()
	now := time.Now()
	d.phase, d.phaseSince = phase, now
	if phase == "processing_event" {
		d.lastEventType = diagnosticEventType(eventType)
		d.events++
		d.lastEvent = now
	}
	d.mu.Unlock()
}

// Tipos fora do protocolo podem conter texto arbitrário enviado pelo endpoint.
func diagnosticEventType(eventType string) string {
	switch eventType {
	case "error",
		"response.audio.delta",
		"response.audio.done",
		"response.audio.transcript.delta",
		"response.audio.transcript.done",
		"response.code_interpreter_call.completed",
		"response.code_interpreter_call.in_progress",
		"response.code_interpreter_call.interpreting",
		"response.code_interpreter_call_code.delta",
		"response.code_interpreter_call_code.done",
		"response.completed",
		"response.content_part.added",
		"response.content_part.done",
		"response.created",
		"response.failed",
		"response.file_search_call.completed",
		"response.file_search_call.in_progress",
		"response.file_search_call.searching",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
		"response.image_generation_call.completed",
		"response.image_generation_call.generating",
		"response.image_generation_call.in_progress",
		"response.image_generation_call.partial_image",
		"response.in_progress",
		"response.incomplete",
		"response.mcp_call.completed",
		"response.mcp_call.failed",
		"response.mcp_call.in_progress",
		"response.mcp_call_arguments.delta",
		"response.mcp_call_arguments.done",
		"response.mcp_list_tools.completed",
		"response.mcp_list_tools.failed",
		"response.mcp_list_tools.in_progress",
		"response.output_item.added",
		"response.output_item.done",
		"response.output_text.annotation.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.queued",
		"response.reasoning_summary.delta",
		"response.reasoning_summary.done",
		"response.reasoning_summary_part.added",
		"response.reasoning_summary_part.done",
		"response.reasoning_summary_text.delta",
		"response.reasoning_summary_text.done",
		"response.reasoning_text.delta",
		"response.reasoning_text.done",
		"response.refusal.delta",
		"response.refusal.done",
		"response.web_search_call.completed",
		"response.web_search_call.in_progress",
		"response.web_search_call.searching":
		return eventType
	default:
		return "unknown"
	}
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
	logging.Warnf(ctx, "llm.stream-diagnostics", "stream_idle_diagnostic phase=%s elapsed_ms=%d phase_ms=%d bytes=%d reads=%d events=%d last_read_ms=%d last_event_ms=%d last_event_type=%s", d.phase, age(d.started), age(d.phaseSince), d.bytes, d.reads, d.events, age(d.lastRead), age(d.lastEvent), d.lastEventType)
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
