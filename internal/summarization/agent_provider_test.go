package summarization

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// repoDeMentira registra o que a sumarização chegou a pedir do banco.
type repoDeMentira struct {
	mensagensPedidas int
	emAndamento      []bool
	resumoSalvo      string
}

func (r *repoDeMentira) GetMessages(context.Context, string) ([]chat.Message, error) {
	r.mensagensPedidas++
	return nil, nil
}

func (r *repoDeMentira) GetConversationSummary(context.Context, string) (string, string, error) {
	return "", "", nil
}

func (r *repoDeMentira) IsSummarizingInProgress(context.Context, string) (bool, error) {
	return false, nil
}

func (r *repoDeMentira) SetSummarizingInProgress(_ context.Context, _ string, inProgress bool) error {
	r.emAndamento = append(r.emAndamento, inProgress)
	return nil
}

func (r *repoDeMentira) UpdateConversationSummary(_ context.Context, _ string, summary string, _ string) error {
	r.resumoSalvo = summary
	return nil
}

// emissorEspiao guarda os eventos na ordem em que saíram.
type emissorEspiao struct {
	eventos []string
	erros   []ports.SummaryErrorEvent
}

func (e *emissorEspiao) Emit(nome string, payload any) {
	e.eventos = append(e.eventos, nome)
	if evt, ok := payload.(ports.SummaryErrorEvent); ok {
		e.erros = append(e.erros, evt)
	}
}

func (e *emissorEspiao) emitiu(nome string) bool {
	for _, evento := range e.eventos {
		if evento == nome {
			return true
		}
	}
	return false
}

func registroComAgente(t *testing.T) *llm.ProviderRegistry {
	t.Helper()
	registro := llm.NewProviderRegistry()
	if err := registro.Register(&llm.ProviderConfig{
		ID: "cursor", Name: "Cursor", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatACP, ACPCommand: "cursor-agent",
	}); err != nil {
		t.Fatalf("registrar agente: %v", err)
	}
	return registro
}

// resolvePara imita o ProfileResolver do app: troca o sentinela `$default` pelo
// provedor que responde pelo padrão global.
func resolvePara(providerID string) func(context.Context, *profiles.Profile) *profiles.Profile {
	return func(_ context.Context, p *profiles.Profile) *profiles.Profile {
		resolvido := *p
		resolvido.Chat.LLMProvider = providerID
		return &resolvido
	}
}

func perfilDeAgente(providerID string) *profiles.Profile {
	perfil := profiles.DefaultProfile()
	perfil.Chat.LLMProvider = providerID
	perfil.Chat.Model = "auto"
	perfil.Chat.ContextWindow = 8000
	return perfil
}

// Quem administra o contexto de uma conversa ACP é o próprio agente (AEP-0084
// D14). O check nem chega a ler as mensagens para decidir.
func TestConversaDeAgenteNaoDisparaSumarizacaoAutomatica(t *testing.T) {
	profileMgr := setupProfileTestEnv(t)
	slug, err := profileMgr.Create(perfilDeAgente("cursor"))
	if err != nil {
		t.Fatalf("criar perfil: %v", err)
	}
	repo := &repoDeMentira{}
	svc := NewService(ServiceConfig{
		Repo:           repo,
		Emitter:        &emissorEspiao{},
		LLMRegistry:    registroComAgente(t),
		ProfileManager: profileMgr,
	})

	svc.CheckAndTriggerSummarization(context.Background(), "conversa-1", slug)

	if repo.mensagensPedidas != 0 {
		t.Errorf("a sumarização carregou mensagens de uma conversa de agente (%d vezes)", repo.mensagensPedidas)
	}
}

func TestConversaComProvedorComumSegueSumarizando(t *testing.T) {
	profileMgr := setupProfileTestEnv(t)
	slug, err := profileMgr.Create(perfilDeAgente("openai"))
	if err != nil {
		t.Fatalf("criar perfil: %v", err)
	}
	registro := registroComAgente(t)
	if err := registro.Register(&llm.ProviderConfig{
		ID: "openai", Name: "OpenAI", Type: llm.ProviderOpenAI,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://api.openai.com/v1",
	}); err != nil {
		t.Fatalf("registrar provedor http: %v", err)
	}
	repo := &repoDeMentira{}
	svc := NewService(ServiceConfig{
		Repo:           repo,
		Emitter:        &emissorEspiao{},
		LLMRegistry:    registro,
		ProfileManager: profileMgr,
	})

	svc.CheckAndTriggerSummarization(context.Background(), "conversa-1", slug)

	if repo.mensagensPedidas == 0 {
		t.Error("conversa com provedor comum deveria continuar sendo avaliada")
	}
}

// Perfil que herda o provedor padrão global não escapa da guarda: quem resolve
// o sentinela `$default` é o ProfileResolver, e ele roda antes de decidir.
func TestConversaComPadraoGlobalDeAgenteNaoDisparaSumarizacao(t *testing.T) {
	profileMgr := setupProfileTestEnv(t)
	slug, err := profileMgr.Create(perfilDeAgente(profiles.DefaultProviderSentinel))
	if err != nil {
		t.Fatalf("criar perfil: %v", err)
	}
	repo := &repoDeMentira{}
	svc := NewService(ServiceConfig{
		Repo:            repo,
		Emitter:         &emissorEspiao{},
		LLMRegistry:     registroComAgente(t),
		ProfileManager:  profileMgr,
		ProfileResolver: resolvePara("cursor"),
	})

	svc.CheckAndTriggerSummarization(context.Background(), "conversa-1", slug)

	if repo.mensagensPedidas != 0 {
		t.Errorf("o sentinela escondeu o agente e o check seguiu adiante (%d leituras)", repo.mensagensPedidas)
	}
}

// A execução é por onde passa qualquer disparo de sumarização, inclusive os que
// não vieram do check — e uma recusa não anuncia um resumo que não vai sair.
func TestExecucaoRecusaResumoQuandoOProvedorEhAgente(t *testing.T) {
	repo := &repoDeMentira{}
	emissor := &emissorEspiao{}
	svc := NewService(ServiceConfig{
		Repo:            repo,
		Emitter:         emissor,
		LLMRegistry:     registroComAgente(t),
		ProfileResolver: resolvePara("cursor"),
	})

	svc.executeSummarization(
		context.Background(),
		"conversa-1",
		perfilDeAgente(profiles.DefaultProviderSentinel),
		"",
		[]chat.Message{{Role: "user", Content: "oi"}},
		"msg-1",
	)

	if emissor.emitiu("chat:summary_started") {
		t.Error("não se anuncia início de um resumo recusado")
	}
	if !emissor.emitiu("chat:summary_error") {
		t.Error("a recusa precisa chegar à interface com explicação")
	}
	if len(emissor.erros) != 1 {
		t.Fatalf("esperava um aviso de recusa, recebeu %d", len(emissor.erros))
	}
	// O código é o que a interface traduz; o texto é a sobra para quem não o
	// conhece, e por isso também precisa explicar o motivo.
	if emissor.erros[0].Code != ports.SummaryErrorCodeAgentProvider {
		t.Errorf("código do motivo = %q, esperado %q", emissor.erros[0].Code, ports.SummaryErrorCodeAgentProvider)
	}
	if !strings.Contains(emissor.erros[0].Error, "agente externo") {
		t.Errorf("a explicação não diz por que o resumo não saiu: %q", emissor.erros[0].Error)
	}
	if repo.resumoSalvo != "" {
		t.Errorf("nenhum resumo deveria ter sido salvo, salvou %q", repo.resumoSalvo)
	}
	if len(repo.emAndamento) != 1 || repo.emAndamento[0] {
		t.Errorf("a conversa ficou marcada como sumarizando: %v", repo.emAndamento)
	}
}
