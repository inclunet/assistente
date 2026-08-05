package llm

import (
	"context"
	"errors"
	"sync"
	"testing"

	"assistente/internal/acp"
)

// registroEmMemoria guarda o vínculo conversa↔sessão como o banco guarda, para
// o teste montar a conversa que reabre em vez da que nasce agora.
type registroEmMemoria struct {
	mu    sync.Mutex
	linha *acp.StoredSession
}

func (r *registroEmMemoria) Load(context.Context, string, string) (*acp.StoredSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.linha == nil {
		return nil, nil
	}
	copia := *r.linha
	return &copia, nil
}

func (r *registroEmMemoria) Save(_ context.Context, rec acp.StoredSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.linha = &rec
	return nil
}

func (r *registroEmMemoria) SavePrefixHash(_ context.Context, _, _, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.linha == nil {
		return errors.New("sessão não registrada")
	}
	r.linha.PrefixHash = hash
	return nil
}

func (r *registroEmMemoria) Delete(context.Context, string) error { return nil }
func (r *registroEmMemoria) DeleteAll(context.Context) error      { return nil }

var _ acp.SessionStore = (*registroEmMemoria)(nil)

// providerDeConversaReaberta monta o provider de uma conversa que já tinha
// sessão registrada, com o agente respondendo ao session/load do jeito que
// `retomaFalha` mandar. É o caminho da reabertura inteiro: registro no banco,
// tentativa de retomada e a sessão que o turno acaba usando.
func providerDeConversaReaberta(t *testing.T, sessao *agenteFalso, retomaFalha bool) *ACPChatProvider {
	t.Helper()
	dir := t.TempDir()
	cliente := &clienteFalso{sessao: sessao, caps: acp.Capabilities{LoadSession: true}}
	if retomaFalha {
		cliente.erroAoRetomar = errors.New("sessão desconhecida")
	}
	registro := &registroEmMemoria{linha: &acp.StoredSession{
		ConversationID: "conversa-1",
		ProviderID:     "cursor",
		SessionID:      "sessao-de-ontem",
		WorkDir:        dir,
	}}
	mgr := acp.NewManager(acp.ManagerConfig{
		Store:   registro,
		WorkDir: func() (string, error) { return dir, nil },
		Dial: func(acp.Config, acp.RequestHandler) (acp.Client, error) {
			return cliente, nil
		},
	})
	t.Cleanup(mgr.Shutdown)

	return NewACPChatProvider(&ProviderConfig{
		ID:         "cursor",
		Name:       "Cursor",
		APIFormat:  APIFormatACP,
		ACPCommand: "cursor-agent",
		Model:      "auto",
	}, mgr)
}

func avisosDoTipo(avisos []TurnNotice, kind TurnNoticeKind) int {
	total := 0
	for _, aviso := range avisos {
		if aviso.Kind == kind {
			total++
		}
	}
	return total
}

// Reabrir a conversa com um agente que não reconhece a sessão registrada custa a
// memória dela: o que está na tela continua sendo o histórico da pessoa, mas o
// agente responde sem ter vivido nada disso. Descobrir pela resposta estranha é
// pior do que ouvir o aviso (AEP-0084 D4).
func TestConversaReabertaSemRetomadaAvisaQueOAgenteEsqueceu(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	provider := providerDeConversaReaberta(t, sessao, true)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "continua de onde paramos"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	if got := avisosDoTipo(handler.avisos, TurnNoticeAgentMemoryLost); got != 1 {
		t.Fatalf("avisos de memória perdida = %d, quer 1 (avisos: %+v)", got, handler.avisos)
	}

	// O turno seguinte fala com a mesma sessão: nada se perdeu de novo, e
	// repetir o aviso diria que o agente esqueceu outra vez.
	segundo := &espiao{}
	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "então siga"}},
		ChatParams{ConversationID: "conversa-1"}, segundo)
	if got := avisosDoTipo(segundo.avisos, TurnNoticeAgentMemoryLost); got != 0 {
		t.Fatalf("o aviso de memória perdida voltou no turno seguinte: %+v", segundo.avisos)
	}
}

// A retomada que dá certo não avisa nada: o agente continua sabendo o que foi
// conversado, e um aviso aqui mandaria a pessoa recontar o que ele já sabe.
func TestConversaReabertaComRetomadaNaoAvisaPerdaDeMemoria(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	provider := providerDeConversaReaberta(t, sessao, false)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "continua de onde paramos"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	if got := avisosDoTipo(handler.avisos, TurnNoticeAgentMemoryLost); got != 0 {
		t.Fatalf("a conversa retomada avisou perda de memória que não houve: %+v", handler.avisos)
	}
}
