package questionnaire

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRequestQuestionnaire_Answered(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(event string, data any) {
		if event != "tool:questionnaire" {
			t.Errorf("expected event 'tool:questionnaire', got %q", event)
			return
		}
		dataMap, ok := data.(map[string]any)
		if !ok {
			t.Errorf("expected map[string]any data")
			return
		}
		id, _ := dataMap["id"].(string)
		if id == "" {
			t.Errorf("expected id to be set")
			return
		}
		go func() {
			if err := mgr.Respond(id, map[string]any{"q1": "ok"}, false); err != nil {
				t.Errorf("Respond error: %v", err)
			}
		}()
	})

	resp, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Title: "Teste",
		Questions: []Question{
			{ID: "q1", Type: "text", Prompt: "Pergunta"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Answers["q1"] != "ok" {
		t.Fatalf("unexpected answer: %+v", resp.Answers)
	}
	if resp.Cancelled {
		t.Fatalf("expected cancelled=false")
	}
	if mgr.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", mgr.PendingCount())
	}
}

func TestRequestQuestionnaire_Cancelled(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(event string, data any) {
		go func() {
			time.Sleep(10 * time.Millisecond)
			dataMap := data.(map[string]any)
			_ = mgr.Respond(dataMap["id"].(string), map[string]any{}, true)
		}()
	})

	resp, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Title: "Teste",
		Questions: []Question{
			{ID: "q1", Type: "text", Prompt: "Pergunta"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Cancelled {
		t.Fatalf("expected cancelled=true")
	}
}

func TestRequestQuestionnaire_CancelledWithAnswers(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(_ string, data any) {
		dataMap := data.(map[string]any)
		if _, ok := dataMap["rejectReason"]; !ok {
			t.Error("payload do evento deve incluir rejectReason quando configurado")
		}
		go func() {
			_ = mgr.Respond(dataMap["id"].(string), map[string]any{"reject_reason": "motivo do usuário"}, true)
		}()
	})

	resp, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Title:        "Teste",
		Questions:    []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}},
		RejectReason: &RejectReasonConfig{ID: "reject_reason", Label: "Motivo"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Cancelled {
		t.Fatal("expected cancelled=true")
	}
	if resp.Answers["reject_reason"] != "motivo do usuário" {
		t.Fatalf("answers devem ser preservadas no cancelamento: %+v", resp.Answers)
	}
}

func TestRequestQuestionnaire_EventOmitsRejectReasonWhenAbsent(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(_ string, data any) {
		dataMap := data.(map[string]any)
		if _, ok := dataMap["rejectReason"]; ok {
			t.Error("payload do evento não deve incluir rejectReason quando não configurado")
		}
		go func() {
			_ = mgr.Respond(dataMap["id"].(string), map[string]any{}, true)
		}()
	})

	if _, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRespondQuestionnaire_NotFound(t *testing.T) {
	mgr := NewManager(func(string, any) {})
	if err := mgr.Respond("missing", map[string]any{}, false); err == nil {
		t.Fatalf("expected error for missing request")
	}
}

func TestRequestQuestionnaire_ContextCancelled(t *testing.T) {
	mgr := NewManager(func(string, any) {})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := mgr.RequestQuestionnaire(ctx, RequestPayload{Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}}})
	if err == nil {
		t.Fatalf("expected error on cancel")
	}
}

func TestRequestQuestionnaire_CustomTimeout(t *testing.T) {
	mgr := NewManager(func(string, any) {})

	start := time.Now()
	_, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}},
		Timeout:   100 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("custom timeout not respected: elapsed %v", elapsed)
	}
}

// eventoDeFechamento espera o aviso de que a pergunta perdeu o dono.
type eventoDeFechamento struct {
	mu     sync.Mutex
	id     string
	reason string
	visto  chan struct{}
}

func novoEventoDeFechamento() *eventoDeFechamento {
	return &eventoDeFechamento{visto: make(chan struct{}, 1)}
}

func (e *eventoDeFechamento) registrar(event string, data any) {
	if event != EventQuestionnaireClosed {
		return
	}
	payload, _ := data.(map[string]any)
	e.mu.Lock()
	e.id, _ = payload["id"].(string)
	e.reason, _ = payload["reason"].(string)
	e.mu.Unlock()
	select {
	case e.visto <- struct{}{}:
	default:
	}
}

func (e *eventoDeFechamento) esperar(t *testing.T) (id, reason string) {
	t.Helper()
	select {
	case <-e.visto:
	case <-time.After(2 * time.Second):
		t.Fatal("o diálogo não foi avisado de que a pergunta acabou")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.id, e.reason
}

func TestPerguntaComPrazoEstouradoFechaODialogo(t *testing.T) {
	fechamento := novoEventoDeFechamento()
	var aberto string
	mgr := NewManager(func(event string, data any) {
		if event == EventQuestionnaire {
			payload, _ := data.(map[string]any)
			aberto, _ = payload["id"].(string)
		}
		fechamento.registrar(event, data)
	})

	_, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}},
		Timeout:   50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("esperava erro de prazo")
	}

	id, reason := fechamento.esperar(t)
	if id != aberto {
		t.Errorf("fechou o diálogo %q, quer o %q que estava aberto", id, aberto)
	}
	if reason != ClosedTimeout {
		t.Errorf("motivo = %q, quer %q", reason, ClosedTimeout)
	}
}

func TestQuemDesistiuDaPerguntaTiraODialogoDaTela(t *testing.T) {
	// O turno foi cancelado enquanto a pessoa lia o diálogo: pedir decisão
	// sobre um turno abortado é ruído, e a resposta não iria a lugar nenhum.
	fechamento := novoEventoDeFechamento()
	mgr := NewManager(fechamento.registrar)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	if _, err := mgr.RequestQuestionnaire(ctx, RequestPayload{
		Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}},
	}); err == nil {
		t.Fatal("esperava erro de cancelamento")
	}

	if _, reason := fechamento.esperar(t); reason != ClosedCancelled {
		t.Errorf("motivo = %q, quer %q", reason, ClosedCancelled)
	}
}

func TestPerguntaRespondidaNaoAvisaFechamento(t *testing.T) {
	// O diálogo já se fechou sozinho ao ser respondido; um aviso aqui poderia
	// derrubar o diálogo seguinte, que reusa a mesma tela.
	fechamentos := 0
	var mu sync.Mutex
	var mgr *Manager
	mgr = NewManager(func(event string, data any) {
		if event == EventQuestionnaireClosed {
			mu.Lock()
			fechamentos++
			mu.Unlock()
			return
		}
		payload, _ := data.(map[string]any)
		id, _ := payload["id"].(string)
		go func() { _ = mgr.Respond(id, map[string]any{"q1": "ok"}, false) }()
	})

	if _, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}},
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if fechamentos != 0 {
		t.Errorf("avisos de fechamento = %d, quer 0", fechamentos)
	}
}

func TestRequestQuestionnaire_ZeroTimeoutUsesDefault(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(_ string, data any) {
		go func() {
			dataMap := data.(map[string]any)
			_ = mgr.Respond(dataMap["id"].(string), map[string]any{"q1": "ok"}, false)
		}()
	})

	resp, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}},
		Timeout:   0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Answers["q1"] != "ok" {
		t.Fatalf("unexpected answer: %+v", resp.Answers)
	}
}
