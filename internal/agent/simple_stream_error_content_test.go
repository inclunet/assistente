package agent

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
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

// persistErrorWhenEmpty cobre o caminho de erro puro (sem OnDone): o loop
// simples termina na tentativa final só com OnError, e o placeholder ficaria
// vazio sem esta gravação.
func TestPersistErrorWhenEmptyGravaErroNoPlaceholderVazio(t *testing.T) {
	repo := &repoCapturaConteudo{}
	svc := NewService(ServiceConfig{Emitter: &mockEmitter{}, MsgRepo: repo})

	svc.persistErrorWhenEmpty(context.Background(), "msg-1", "processo do agente caiu")

	conteudo, ok := repo.conteudos["msg-1"]
	if !ok {
		t.Fatal("placeholder vazio não recebeu o texto do erro")
	}
	if !strings.HasPrefix(conteudo, "Falha na resposta do agente:") || !strings.Contains(conteudo, "processo do agente caiu") {
		t.Errorf("conteúdo=%q, esperava o motivo do erro", conteudo)
	}
}

func TestPersistErrorWhenEmptyPreservaConteudoExistente(t *testing.T) {
	repo := &repoCapturaConteudo{mockMsgRepo: mockMsgRepo{
		messagesByID: map[string]*chat.Message{
			"msg-1": {UUIDModel: database.UUIDModel{ID: "msg-1"}, Content: "parcial do agente"},
		},
	}}
	svc := NewService(ServiceConfig{Emitter: &mockEmitter{}, MsgRepo: repo})

	svc.persistErrorWhenEmpty(context.Background(), "msg-1", "processo do agente caiu")

	if conteudo, ok := repo.conteudos["msg-1"]; ok {
		t.Errorf("conteúdo parcial %q foi sobrescrito pelo erro", conteudo)
	}
}
