package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/llm"
)

// agenteDeModelos é o agente do outro lado: responde a descoberta com a lista do
// momento e conta quantas vezes foi perguntado, que é o que distingue lista viva
// de lista guardada.
type agenteDeModelos struct {
	mu      sync.Mutex
	modelos []string
	// nomes é o rótulo que o agente dá a cada modelo, quando dá.
	nomes map[string]string
	// semEscolha é o agente que não expõe opção de modelo nenhuma: quem
	// escolhe o modelo do turno é ele.
	semEscolha bool
	perguntas  int
	invalidou  int
	// guardada é a lista que o agente já entregou. Existe para este falso imitar
	// o cache por processo do transporte de verdade: sem ela o teste não saberia
	// dizer se o refresh atravessou as camadas ou se o agente é que respondeu de
	// novo por conta.
	guardada []acp.ConfigOption
}

func (a *agenteDeModelos) NewSession(context.Context, string) (acp.Session, error) {
	return nil, errors.New("este teste não abre sessão de conversa")
}

func (a *agenteDeModelos) LoadSession(context.Context, string, string) (acp.Session, error) {
	return nil, errors.New("sem retomada")
}

func (a *agenteDeModelos) Capabilities(context.Context) (acp.Capabilities, error) {
	return acp.Capabilities{AgentName: "falso"}, nil
}

func (a *agenteDeModelos) CloseSession(context.Context, string) error { return nil }

func (a *agenteDeModelos) Options(context.Context, string) ([]acp.ConfigOption, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.guardada != nil {
		return a.guardada, nil
	}
	a.perguntas++
	if a.semEscolha {
		a.guardada = []acp.ConfigOption{}
		return a.guardada, nil
	}
	a.guardada = []acp.ConfigOption{{
		ID:           "model",
		Category:     acp.CategoryModel,
		CurrentValue: a.modelos[0],
		Values:       valoresDe(a.modelos, a.nomes),
	}}
	return a.guardada, nil
}

func (a *agenteDeModelos) InvalidateOptions() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.invalidou++
	a.guardada = nil
}

func (a *agenteDeModelos) Call(context.Context, string, any) (json.RawMessage, error) {
	return nil, errors.New("método não suportado")
}

func (a *agenteDeModelos) Close() error { return nil }

func (a *agenteDeModelos) passaAOferecer(modelos []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.modelos = modelos
}

func (a *agenteDeModelos) contadores() (perguntas, invalidacoes int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.perguntas, a.invalidou
}

func valoresDe(modelos []string, nomes map[string]string) []acp.ConfigValue {
	out := make([]acp.ConfigValue, 0, len(modelos))
	for _, modelo := range modelos {
		out = append(out, acp.ConfigValue{Value: modelo, Name: nomes[modelo]})
	}
	return out
}

var _ acp.Client = (*agenteDeModelos)(nil)

// serviceComAgente monta o serviço como a produção o monta: provedor de agente
// criado pelo caminho normal e um manager de verdade, com o processo trocado
// pelo falso. É o caminho inteiro que a tela de configurações percorre.
func serviceComAgente(t *testing.T, agente *agenteDeModelos) *Service {
	t.Helper()
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
		Dial: func(acp.Config, acp.RequestHandler) (acp.Client, error) {
			return agente, nil
		},
	})
	t.Cleanup(mgr.Shutdown)

	svc := NewService(ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  &credSpy{},
		Store:    NewMemoryStore(),
		// O limite de uso entra aqui porque entra na produção: sem ele o provider
		// sairia cru da fábrica, e o teste não veria o embrulho pelo qual a
		// chamada da tela realmente passa.
		RateLimiter:      llm.NewRateLimiter(llm.RateLimitConfig{Enabled: true, RequestsPerMinute: 600, Burst: 600}),
		RateLimitKeyFunc: func(context.Context) string { return "dono-1" },
		ACPManager:       mgr,
	})
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", ACPArgs: []string{"acp"},
	}); err != nil {
		t.Fatalf("criar o provedor de agente: %v", err)
	}
	return svc
}

func TestListarModelosDoAgentePelaTelaDeConfiguracoes(t *testing.T) {
	agente := &agenteDeModelos{modelos: []string{"modelo-a", "modelo-b"}}
	svc := serviceComAgente(t, agente)
	ctx := context.Background()

	modelos, err := svc.GetModelsByProvider(ctx, "cursor")
	if err != nil {
		t.Fatalf("GetModelsByProvider: %v", err)
	}
	if len(modelos) != 2 || modelos[0] != "modelo-a" || modelos[1] != "modelo-b" {
		t.Fatalf("modelos = %v, esperado os dois do agente", modelos)
	}
}

// O recarregar da tela precisa atravessar as camadas até a invalidação: entre a
// tela e o agente há o embrulho de limite de uso e a fábrica de providers, e
// qualquer um dos dois esconderia a capacidade sem ninguém notar — a lista velha
// continuaria aparecendo, correta o suficiente para não levantar suspeita
// (AEP-0084 D6).
func TestRecarregarNaTelaFazOAgentePerguntarDeNovo(t *testing.T) {
	agente := &agenteDeModelos{modelos: []string{"modelo-a"}}
	svc := serviceComAgente(t, agente)
	ctx := context.Background()

	if _, err := svc.GetModelsByProvider(ctx, "cursor"); err != nil {
		t.Fatalf("primeira listagem: %v", err)
	}
	// O agente passou a oferecer um modelo novo. Sem o refresh, quem olha a tela
	// nunca o veria.
	agente.passaAOferecer([]string{"modelo-a", "modelo-novo"})

	guardados, err := svc.GetModelsByProvider(ctx, "cursor")
	if err != nil {
		t.Fatalf("segunda listagem: %v", err)
	}
	if len(guardados) != 1 {
		t.Fatalf("a listagem comum deveria servir o que já sabia, veio %v", guardados)
	}

	recarregados, err := svc.RefreshModelsByProvider(ctx, "cursor")
	if err != nil {
		t.Fatalf("RefreshModelsByProvider: %v", err)
	}
	if len(recarregados) != 2 || recarregados[1] != "modelo-novo" {
		t.Fatalf("modelos após recarregar = %v, esperado incluir o novo", recarregados)
	}
	perguntas, invalidacoes := agente.contadores()
	if invalidacoes != 1 {
		t.Fatalf("invalidações = %d, esperado 1", invalidacoes)
	}
	if perguntas != 2 {
		t.Fatalf("o agente foi perguntado %d vezes, esperado 2 (a primeira e a do recarregar)", perguntas)
	}
}

// O identificador de um modelo de agente é do protocolo, não da pessoa. A
// escolha de modelo do perfil precisa do nome que o agente deu (AEP-0084,
// Fase 8).
func TestOCatalogoDoAgenteLevaONomeLegivelDoModelo(t *testing.T) {
	agente := &agenteDeModelos{modelos: []string{"grok-4.5[max]"}, nomes: map[string]string{
		"grok-4.5[max]": "Grok 4.5 (max)",
	}}
	svc := serviceComAgente(t, agente)

	catalogo, err := svc.GetModelCatalogByProvider(context.Background(), "cursor")
	if err != nil {
		t.Fatalf("GetModelCatalogByProvider: %v", err)
	}
	if !catalogo.Agent {
		t.Error("o catálogo não disse que quem respondeu foi um agente")
	}
	if len(catalogo.Models) != 1 {
		t.Fatalf("modelos = %+v, esperado um", catalogo.Models)
	}
	if catalogo.Models[0].Value != "grok-4.5[max]" {
		t.Errorf("valor = %q, o que volta ao agente precisa ser o identificador", catalogo.Models[0].Value)
	}
	if catalogo.Models[0].Label != "Grok 4.5 (max)" {
		t.Errorf("rótulo = %q, esperado o nome que o agente deu", catalogo.Models[0].Label)
	}
}

// Nome vindo do agente é fonte não confiável (D11): ele chega à tela e é lido
// em voz alta.
func TestNomeDeModeloVemSaneadoDoAgente(t *testing.T) {
	agente := &agenteDeModelos{modelos: []string{"m1"}, nomes: map[string]string{
		"m1": "\x1b[31mGrok\x1b[0m\t4.5",
	}}
	svc := serviceComAgente(t, agente)

	catalogo, err := svc.GetModelCatalogByProvider(context.Background(), "cursor")
	if err != nil {
		t.Fatalf("GetModelCatalogByProvider: %v", err)
	}
	if rotulo := catalogo.Models[0].Label; strings.ContainsAny(rotulo, "\x1b\t") {
		t.Fatalf("rótulo = %q, esperado sem escape de terminal nem tabulação", rotulo)
	}
}

// Modelo sem nome continua escolhível: quem exibe cai no identificador em vez
// de mostrar uma linha em branco na lista.
func TestModeloSemNomeCaiNoIdentificador(t *testing.T) {
	agente := &agenteDeModelos{modelos: []string{"m1"}}
	svc := serviceComAgente(t, agente)

	catalogo, err := svc.GetModelCatalogByProvider(context.Background(), "cursor")
	if err != nil {
		t.Fatalf("GetModelCatalogByProvider: %v", err)
	}
	if catalogo.Models[0].Label != "m1" {
		t.Fatalf("rótulo = %q, esperado o identificador", catalogo.Models[0].Label)
	}
}

// Provedor HTTP não é agente, e a tela precisa saber disso para não tratar
// lista vazia dele como "quem escolhe é o agente".
func TestCatalogoDeProvedorHTTPNaoSeDizAgente(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"object":"list","data":[{"id":"gpt-de-teste"}]}`)
	}))
	defer srv.Close()

	svc, _ := acpService(t)
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
		APIFormat: string(llm.APIFormatOpenAI),
		BaseURL:   srv.URL + "/v1",
	}); err != nil {
		t.Fatalf("criar provedor HTTP: %v", err)
	}

	catalogo, err := svc.GetModelCatalogByProvider(context.Background(), "openai")
	if err != nil {
		t.Fatalf("GetModelCatalogByProvider: %v", err)
	}
	if catalogo.Agent {
		t.Error("um provedor HTTP se disse agente")
	}
	if len(catalogo.Models) != 1 || catalogo.Models[0].Label != "gpt-de-teste" {
		t.Fatalf("modelos = %+v, esperado o que o servidor listou", catalogo.Models)
	}
}

// Agente que não expõe escolha de modelo respondeu: quem escolhe é ele. O
// catálogo precisa distinguir isso de falha para a tela não mandar a pessoa
// procurar conserto para o funcionamento normal.
func TestAgenteSemEscolhaDeModeloDevolveCatalogoVazioSemErro(t *testing.T) {
	agente := &agenteDeModelos{semEscolha: true}
	svc := serviceComAgente(t, agente)

	catalogo, err := svc.GetModelCatalogByProvider(context.Background(), "cursor")
	if err != nil {
		t.Fatalf("GetModelCatalogByProvider: %v", err)
	}
	if !catalogo.Agent {
		t.Error("o catálogo não disse que quem respondeu foi um agente")
	}
	if len(catalogo.Models) != 0 {
		t.Fatalf("modelos = %+v, esperado nenhum", catalogo.Models)
	}
}

// "executable file not found in %PATH%" não diz a quem lê o que fazer. A tela
// precisa reconhecer esta falha para mandar refazer a detecção do agente, e é
// pela marca na mensagem que ela reconhece — o erro atravessa a ponte para o
// frontend como texto (AEP-0084, Fase 8).
func TestAgenteQueNaoSobeSaiMarcadoParaATela(t *testing.T) {
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
		Dial: func(acp.Config, acp.RequestHandler) (acp.Client, error) {
			return nil, errors.New(`iniciar agente cursor-agent: exec: "cursor-agent": executable file not found in %PATH%`)
		},
	})
	t.Cleanup(mgr.Shutdown)
	svc := NewService(ServiceConfig{
		Registry:   llm.NewProviderRegistry(),
		CredMgr:    &credSpy{},
		Store:      NewMemoryStore(),
		ACPManager: mgr,
	})
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID: "cursor", Name: "Cursor", APIFormat: string(llm.APIFormatACP),
		ACPCommand: "cursor-agent", ACPArgs: []string{"acp"},
	}); err != nil {
		t.Fatalf("criar o provedor de agente: %v", err)
	}

	_, err := svc.GetModelCatalogByProvider(context.Background(), "cursor")
	if err == nil {
		t.Fatal("listar modelos de um agente que não sobe passou")
	}
	if !strings.Contains(err.Error(), llm.ErrCodeAgentUnavailable) {
		t.Fatalf("erro = %v, esperado marcado com %q", err, llm.ErrCodeAgentUnavailable)
	}
}

// Um provedor HTTP não guarda lista nenhuma: ele pergunta ao servidor a cada
// listagem. Para ele, recarregar tem que ser a listagem normal — e não um erro
// de "este provedor não sabe recarregar", que é o que um refresh implementado
// só para o agente produziria no botão comum a todos.
func TestRecarregarProvedorSemCacheListaNormalmente(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"object":"list","data":[{"id":"gpt-de-teste"}]}`)
	}))
	defer srv.Close()

	svc, _ := acpService(t)
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
		APIFormat: string(llm.APIFormatOpenAI),
		BaseURL:   srv.URL + "/v1",
	}); err != nil {
		t.Fatalf("criar provedor HTTP: %v", err)
	}

	modelos, err := svc.RefreshModelsByProvider(context.Background(), "openai")
	if err != nil {
		t.Fatalf("RefreshModelsByProvider: %v", err)
	}
	if len(modelos) != 1 || modelos[0] != "gpt-de-teste" {
		t.Fatalf("modelos = %v, esperado o que o servidor listou", modelos)
	}
}
