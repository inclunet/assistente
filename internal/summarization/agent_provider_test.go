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
	erros   []string
}

func (e *emissorEspiao) Emit(nome string, payload any) {
	e.eventos = append(e.eventos, nome)
	if evt, ok := payload.(ports.SummaryErrorEvent); ok {
		e.erros = append(e.erros, evt.Error)
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

// O sentinela `$default` só vira provedor concreto na execução, então a recusa
// precisa existir também lá — e sem anunciar um resumo que não vai acontecer.
func TestExecucaoRecusaResumoQuandoOProvedorEhAgente(t *testing.T) {
	repo := &repoDeMentira{}
	emissor := &emissorEspiao{}
	svc := NewService(ServiceConfig{
		Repo:        repo,
		Emitter:     emissor,
		LLMRegistry: registroComAgente(t),
		ProfileResolver: func(_ context.Context, p *profiles.Profile) *profiles.Profile {
			resolvido := *p
			resolvido.Chat.LLMProvider = "cursor"
			return &resolvido
		},
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
	if len(emissor.erros) != 1 || !strings.Contains(emissor.erros[0], "agente externo") {
		t.Errorf("a explicação não diz por que o resumo não saiu: %v", emissor.erros)
	}
	if repo.resumoSalvo != "" {
		t.Errorf("nenhum resumo deveria ter sido salvo, salvou %q", repo.resumoSalvo)
	}
	if len(repo.emAndamento) != 1 || repo.emAndamento[0] {
		t.Errorf("a conversa ficou marcada como sumarizando: %v", repo.emAndamento)
	}
}
