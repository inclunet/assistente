package questionnaire

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/questionnaire"
)

type fakeManager struct {
	lastRequest questionnaire.RequestPayload
	response    questionnaire.Response
	err         error
}

func (f *fakeManager) RequestQuestionnaire(ctx context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	f.lastRequest = payload
	return f.response, f.err
}

func TestCollectResponsesTool_ValidateArgs(t *testing.T) {
	tool := NewCollectResponses(nil)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"questions":[]}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for empty questions")
	}
}

func TestCollectResponsesTool_ManagerMissing(t *testing.T) {
	tool := NewCollectResponses(nil)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"questions":[{"id":"q1","type":"text","prompt":"Pergunta"}]}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error when manager is nil")
	}
}

func TestCollectResponsesTool_InvalidQuestion(t *testing.T) {
	mgr := &fakeManager{}
	tool := NewCollectResponses(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"questions":[{"id":"q1","type":"invalid","prompt":"Pergunta"}]}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for invalid question type")
	}
}

func TestCollectResponsesTool_Success(t *testing.T) {
	mgr := &fakeManager{
		response: questionnaire.Response{
			ID:      "abc123",
			Answers: map[string]any{"q1": "ok"},
		},
	}
	tool := NewCollectResponses(mgr)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"title":"Teste","questions":[{"id":"q1","type":"text","prompt":"Pergunta"}]}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("expected JSON response, got error: %v", err)
	}
	if payload["id"] != "abc123" {
		t.Fatalf("unexpected id: %v", payload["id"])
	}
	answers := payload["answers"].(map[string]any)
	if answers["q1"] != "ok" {
		t.Fatalf("unexpected answers: %+v", answers)
	}
}
