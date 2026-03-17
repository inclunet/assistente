package questionnaire

import (
	"context"
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
			mgr.Respond(dataMap["id"].(string), map[string]any{}, true)
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

func TestRequestQuestionnaire_ZeroTimeoutUsesDefault(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(_ string, data any) {
		go func() {
			dataMap := data.(map[string]any)
			mgr.Respond(dataMap["id"].(string), map[string]any{"q1": "ok"}, false)
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
