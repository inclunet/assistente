package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/llm"
)

func TestProviderUsesExplicitKeyWithoutCredentialManager(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer teste-efemero" {
			t.Errorf("Authorization: %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeTestStream(t, w, "data: {\"id\":\"chat-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"modelo\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n")
		writeTestStream(t, w, "data: {\"id\":\"chat-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"modelo\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	p := llm.NewOpenAIProviderWithAPIKey(&llm.ProviderConfig{BaseURL: server.URL}, "teste-efemero")
	h := &iteration{}
	p.StreamChat(context.Background(), []llm.Message{{Role: "user", Content: "teste"}}, llm.ChatParams{Model: "modelo"}, h)
	if h.err != "" || !h.finished || h.text != "ok" {
		t.Fatalf("resultado: %#v", h)
	}
}

func TestProviderRejectsRedirectToDifferentOrigin(t *testing.T) {
	received := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = true; w.WriteHeader(200) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	p := llm.NewOpenAIProviderWithAPIKey(&llm.ProviderConfig{BaseURL: source.URL}, "secret")
	h := &iteration{}
	p.StreamChat(context.Background(), []llm.Message{{Role: "user", Content: "teste"}}, llm.ChatParams{Model: "modelo"}, h)
	if received || h.err == "" {
		t.Fatalf("recebido=%v erro=%q", received, h.err)
	}
}

func TestCompleteRunWithHTTPProvider(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("conteudo-do-checkout"), 0600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Error("credencial ausente")
		}
		if requests == 1 && !strings.Contains(string(data), "instrucao-da-skill") {
			t.Errorf("skill ausente: %s", data)
		}
		if requests == 2 && !strings.Contains(string(data), "conteudo-do-checkout") {
			t.Errorf("resultado ausente: %s", data)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			writeTestStream(t, w, "data: {\"id\":\"id1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"modelo\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call1\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"f.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
			writeTestStream(t, w, "data: {\"id\":\"id1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"modelo\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		} else {
			writeTestStream(t, w, "data: {\"id\":\"id2\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"modelo\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"concluido\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	p := llm.NewOpenAIProviderWithAPIKey(&llm.ProviderConfig{BaseURL: server.URL}, "key")
	got, err := run(context.Background(), p, "modelo", "instrucao-da-skill", "revise", dir)
	if err != nil || got != "concluido" || requests != 2 {
		t.Fatalf("got=%q err=%v requests=%d", got, err, requests)
	}
}

func writeTestStream(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	if _, err := fmt.Fprint(w, content); err != nil {
		t.Errorf("escrever resposta HTTP: %v", err)
	}
}
