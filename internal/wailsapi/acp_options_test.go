package wailsapi

import (
	"errors"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/apidto"
)

func TestACPOptionsNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPOptions()
	if _, err := api.GetAgentSessionOptions("conversa-1"); !errors.Is(err, ErrACPOptionsNotWired) {
		t.Fatalf("GetAgentSessionOptions: got %v", err)
	}
	if _, err := api.SetAgentSessionOption("conversa-1", "model", "x"); !errors.Is(err, ErrACPOptionsNotWired) {
		t.Fatalf("SetAgentSessionOption: got %v", err)
	}
}

func TestACPOptionsPreservesConversationIDOnAuthError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("sem sessão")
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	api := NewACPOptions()
	AttachACPOptions(api, stubSession{err: wantErr}, mgr, nil)

	got, err := api.GetAgentSessionOptions(" conversa-1 ")

	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
	if got.ConversationID != "conversa-1" {
		t.Fatalf("conversationId = %q, want %q", got.ConversationID, "conversa-1")
	}
	if got.Options == nil {
		t.Fatal("options deve permanecer lista vazia, não nil")
	}

	gotSet, err := api.SetAgentSessionOption(" conversa-2 ", "model", "x")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Set: got error %v, want %v", err, wantErr)
	}
	if gotSet.ConversationID != "conversa-2" {
		t.Fatalf("Set conversationId = %q, want %q", gotSet.ConversationID, "conversa-2")
	}
}

// TestACPOptionsSemAuthNaoMexeNaSessaoDoAgente é o outro lado do fail-closed:
// além de o erro subir, nada do domínio chega a acontecer — a troca não é pedida
// ao agente e o aviso de barreira de permissão não é disparado.
func TestACPOptionsSemAuthNaoMexeNaSessaoDoAgente(t *testing.T) {
	t.Parallel()
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	api := NewACPOptions()
	AttachACPOptions(api, stubSession{err: errors.New("sem sessão")}, mgr,
		func(string, string, string, []apidto.AgentConfigOption) {
			t.Fatal("avisou mudança de barreira sem contexto autenticado")
		})

	if _, err := api.SetAgentSessionOption("conversa-1", "mode", "dontAsk"); err == nil {
		t.Fatal("trocou a opção sem contexto autenticado")
	}
}
