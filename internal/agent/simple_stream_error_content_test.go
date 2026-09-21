package agent

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/llm"
)

// repoCapturaConteudo registra o conteúdo gravado no placeholder assistant.
type repoCapturaConteudo struct {
	mockMsgRepo
	conteudos map[string]string
}

func (r *repoCapturaConteudo) UpdateMessageContentAndReasoning(_ context.Context, messageID string, content string, _ string, _, _, _ int, _ string) error {
	if r.conteudos == nil {
		r.conteudos = map[string]string{}
	}
	r.conteudos[messageID] = content
	return nil
}

func novoHandlerComCaptura(t *testing.T, emitter *mockEmitter) (*SimpleStreamHandler, *repoCapturaConteudo) {
	t.Helper()
	repo := &repoCapturaConteudo{}
	svc := NewService(ServiceConfig{Emitter: emitter, MsgRepo: repo})
	handler, err := svc.NewSimpleStreamHandler(context.Background(), "conversa-1", "turno-1", "perfil", nil)
	if err != nil {
		t.Fatalf("NewSimpleStreamHandler: %v", err)
	}
	return handler, repo
}

// AEP-0108 D4: turno que termina sem conteúdo e com erro persiste o texto do
// erro (sanitizado) em vez de deixar o placeholder assistant vazio.
func TestOnDoneSemConteudoComErroPersisteTextoDoErro(t *testing.T) {
	emitter := &mockEmitter{}
	handler, repo := novoHandlerComCaptura(t, emitter)

	handler.OnError("certificate is not yet valid")
	handler.OnDone("", llm.Usage{}, "modelo")

	conteudo, ok := repo.conteudos[handler.assistantMessageID]
	if !ok {
		t.Fatalf("placeholder %q não foi atualizado", handler.assistantMessageID)
	}
	if !strings.HasPrefix(conteudo, "Falha na resposta do agente:") {
		t.Errorf("conteúdo=%q, esperava prefixo de erro", conteudo)
	}
	if !strings.Contains(conteudo, "certificate is not yet valid") {
		t.Errorf("conteúdo=%q, esperava o motivo do erro", conteudo)
	}
}

// Sem erro, resposta vazia continua vazia: não inventamos conteúdo.
func TestOnDoneSemConteudoSemErroNaoInventaConteudo(t *testing.T) {
	emitter := &mockEmitter{}
	handler, repo := novoHandlerComCaptura(t, emitter)

	handler.OnDone("", llm.Usage{}, "modelo")

	if conteudo, ok := repo.conteudos[handler.assistantMessageID]; ok && strings.TrimSpace(conteudo) != "" {
		t.Errorf("conteúdo=%q, esperava placeholder intocado sem erro", conteudo)
	}
}

// Resposta normal não é afetada pela regra do erro.
func TestOnDoneComConteudoMantemResposta(t *testing.T) {
	emitter := &mockEmitter{}
	handler, repo := novoHandlerComCaptura(t, emitter)

	handler.OnDone("resposta do agente", llm.Usage{}, "modelo")

	if conteudo := repo.conteudos[handler.assistantMessageID]; conteudo != "resposta do agente" {
		t.Errorf("conteúdo=%q, esperava a resposta intacta", conteudo)
	}
}
