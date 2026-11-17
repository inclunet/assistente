package app

import (
	"assistente/internal/acp"
	"assistente/internal/acptrust"
	"assistente/internal/core/ports"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/trustscope"
	"context"
	"errors"
	"gorm.io/gorm"
	"testing"
)

// Estes testes pertencem à suíte ACP executada exclusivamente no CI remoto.
func TestACPGravacaoFalhaMantemSomenteOperacaoEInformaConversa(t *testing.T) {
	repo, db := authorizationfixture.Open(t, t.TempDir())
	if err := db.Callback().Create().Before("gorm:create").Register("test:permission-write-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "authorization_rules" {
			_ = tx.AddError(errors.New("write unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	store := acptrust.NewStore(trustscope.Options{Repository: repo, HomeDir: func() string { return "" }})
	tela := novaTelaFalsa(escolhendo("Neste perfil"))
	h := handlerCom(tela, acp.TurnOwner{UserID: "authorization-test-user", ConversationID: "chat", ProfileSlug: "p", Interactive: true}, true)
	h.trust = func() *acptrust.Store { return store }
	emitter := escutandoAvisos(h)
	if out := h.RequestPermission(context.Background(), pedidoDeExecucao()); out.OptionID != "allow-once" {
		t.Fatalf("decisão: %+v", out)
	}
	if notice := avisoNaConversa(t, emitter); notice.Kind != ports.ChatNoticeKindPermissionAlwaysNotSaved {
		t.Fatalf("aviso: %+v", notice)
	}
	if allowed, err := allowsTestAgent(store, authorizationfixture.Context(), "p", "execute"); err != nil || allowed {
		t.Fatalf("persistiu apesar da falha: %v %v", allowed, err)
	}
}

func TestACPConcessaoLocalUsaOpcaoPontualEIsolaAgente(t *testing.T) {
	tela := novaTelaFalsa(escolhendo("Neste perfil"))
	h := handlerCom(tela, acp.TurnOwner{UserID: "authorization-test-user", ConversationID: "chat", ProfileSlug: "p", AgentID: "agent-a", Interactive: true}, true)
	lembrandoAutorizacoes(t, h, "p")
	request := pedidoDeExecucao()
	request.Options = []acp.PermissionOption{{ID: "literal-once", Kind: "allow_once"}, {ID: "no", Kind: "reject_once"}}
	for range 2 {
		if out := h.RequestPermission(context.Background(), request); out.OptionID != "literal-once" {
			t.Fatalf("inventou opção remota: %+v", out)
		}
	}
	if tela.quantasPerguntas() != 1 {
		t.Fatal("não reutilizou concessão local")
	}
	h.owner = func(string) (acp.TurnOwner, bool) {
		return acp.TurnOwner{UserID: "authorization-test-user", ConversationID: "chat", ProfileSlug: "p", AgentID: "agent-b", Interactive: true}, true
	}
	h.RequestPermission(context.Background(), request)
	if tela.quantasPerguntas() != 2 {
		t.Fatal("outro agente herdou a concessão")
	}
}
