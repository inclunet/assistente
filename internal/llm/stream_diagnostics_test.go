package llm

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type diagnosticCloseTransport struct{ closed bool }

func TestStreamDiagnosticDoesNotChangePhaseAfterTimeout(t *testing.T) {
	d := newStreamDiagnostics()
	wd := &streamWatchdog{}
	d.setPhase("processing_event", "private-from-provider", wd)
	wd.timedOut = true
	d.setPhase("waiting_event", "", wd)
	if d.phase != "processing_event" || d.events != 1 || d.lastEventType != "unknown" {
		t.Fatalf("fase alterada após timeout: phase=%s events=%d", d.phase, d.events)
	}
}

func (*diagnosticCloseTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }
func (t *diagnosticCloseTransport) CloseIdleConnections()                         { t.closed = true }

func TestStreamDiagnosticTransportForwardsCloseIdleConnections(t *testing.T) {
	base := &diagnosticCloseTransport{}
	streamDiagnosticTransport{base: base}.CloseIdleConnections()
	if !base.closed {
		t.Fatal("conexões ociosas não foram liberadas")
	}
}

type slowDiagnosticHandler struct{ spyHandler }

func (h *slowDiagnosticHandler) OnChunk(content string) {
	h.spyHandler.OnChunk(content)
	time.Sleep(1200 * time.Millisecond)
}

func TestResponsesIdleDiagnosticDetectsSlowHandler(t *testing.T) {
	logs := &logCaptureHandler{}
	old := slog.Default()
	slog.SetDefault(slog.New(logs))
	defer slog.SetDefault(old)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "diagnostic", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, StreamIdleTimeoutSeconds: 1}, nil)
	h := &slowDiagnosticHandler{}
	p.StreamChat(context.Background(), []Message{{Role: "user", Content: "hi"}}, ChatParams{Model: "model"}, h)
	if h.err != streamIdleErrorMessage || strings.Join(h.chunks, "") != "partial" {
		t.Fatalf("erro=%q conteúdo=%q", h.err, h.chunks)
	}
	if logs.find(slog.LevelWarn, "stream_idle_diagnostic phase=processing_event") == nil {
		t.Fatal("diagnóstico de processamento lento ausente")
	}
}

func TestResponsesIdleDiagnosticDetectsBytesWithoutCompleteEvent(t *testing.T) {
	logs := &logCaptureHandler{}
	old := slog.Default()
	slog.SetDefault(slog.New(logs))
	defer slog.SetDefault(old)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"private")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "diagnostic", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, StreamIdleTimeoutSeconds: 1}, nil)
	params := p.buildResponsesParams(context.Background(), "model", []Message{{Role: "user", Content: "hi"}}, ChatParams{}, nil)
	result := p.doStreamResponses(context.Background(), params, &spyHandler{}, nil, ChatParams{}, nil)
	if !result.retry {
		t.Fatal("esperava recuperação sem evento entregue")
	}
	var found bool
	for _, rec := range logs.snapshot() {
		if strings.Contains(rec.Message, "private") {
			t.Fatal("payload vazou")
		}
		if strings.Contains(rec.Message, "stream_idle_diagnostic phase=waiting_event") && strings.Contains(rec.Message, "events=0") && strings.Contains(rec.Message, "last_event_type=none") && !strings.Contains(rec.Message, "bytes=0 ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnóstico de evento incompleto ausente: %+v", logs.snapshot())
	}
}

func TestResponsesIdleDiagnosticPreservesPartialAndOmitsContent(t *testing.T) {
	const secret = "conteudo_privado_da_resposta"
	logs := &logCaptureHandler{}
	old := slog.Default()
	slog.SetDefault(slog.New(logs))
	defer slog.SetDefault(old)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"%s\"}\n\n", secret)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	p := NewOpenAIResponsesProvider(&ProviderConfig{ID: "diagnostic", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, StreamIdleTimeoutSeconds: 1}, nil)
	h := &spyHandler{}
	p.StreamChat(context.Background(), []Message{{Role: "user", Content: "hi"}}, ChatParams{Model: "model"}, h)
	if h.err != streamIdleErrorMessage || strings.Join(h.chunks, "") != secret || !h.nonRetryable {
		t.Fatalf("partial not preserved: chunks=%q err=%q nonretryable=%v", h.chunks, h.err, h.nonRetryable)
	}
	found := false
	for _, rec := range logs.snapshot() {
		if strings.Contains(rec.Message, secret) {
			t.Fatal("conteúdo vazou no log")
		}
		if strings.Contains(rec.Message, "stream_idle_diagnostic phase=waiting_event") && strings.Contains(rec.Message, "events=1") && strings.Contains(rec.Message, "last_event_type=response.output_text.delta") && !strings.Contains(rec.Message, "bytes=0 ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnóstico ausente: %+v", logs.snapshot())
	}
}
