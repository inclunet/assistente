package providers

import (
	"context"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/profiles"
)

func perfilApontandoPara(providerID string) *profiles.Profile {
	p := profiles.DefaultProfile()
	p.Chat.LLMProvider = providerID
	return p
}

func criarAgente(t *testing.T, svc *Service, id string) {
	t.Helper()
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID: id, Name: id, Type: string(llm.ProviderCustom),
		APIFormat:  string(llm.APIFormatACP),
		ACPCommand: "cursor-agent",
	}); err != nil {
		t.Fatalf("criar agente %s: %v", id, err)
	}
}

// O turno de um agente externo precisa ser reconhecido antes do planejamento de
// tools (AEP-0084 D7): é essa resposta que desliga as ferramentas do app.
func TestPerfilComAgenteExternoUsaTurnoDeAgente(t *testing.T) {
	svc, _ := acpService(t)
	criarAgente(t, svc, "cursor")

	if !svc.UsesAgentTurn(context.Background(), perfilApontandoPara("cursor")) {
		t.Error("perfil apontando para agente ACP deveria usar turno de agente")
	}
}

func TestPerfilComProvedorHTTPNaoUsaTurnoDeAgente(t *testing.T) {
	svc, _ := acpService(t)
	if _, err := svc.Create(context.Background(), CreateRequest{
		ID: "openai", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.openai.com/v1",
	}); err != nil {
		t.Fatalf("criar provedor http: %v", err)
	}

	if svc.UsesAgentTurn(context.Background(), perfilApontandoPara("openai")) {
		t.Error("provedor http não conduz o turno")
	}
}

// Perfil sem provedor resolvível não pode virar turno de agente por descuido:
// desligar as tools nesse caso esconderia um erro de configuração.
func TestPerfilSemProvedorConhecidoNaoUsaTurnoDeAgente(t *testing.T) {
	svc, _ := acpService(t)

	if svc.UsesAgentTurn(context.Background(), nil) {
		t.Error("perfil nil não usa turno de agente")
	}
	if svc.UsesAgentTurn(context.Background(), perfilApontandoPara("inexistente")) {
		t.Error("provedor inexistente não usa turno de agente")
	}
}

// O perfil pode apontar para o sentinela `$default`. Quem responde pelo padrão é
// o provedor default — se ele for um agente, o turno também é.
func TestSentinelaDePadraoResolveParaOAgente(t *testing.T) {
	svc, _ := acpService(t)
	ctx := context.Background()
	criarAgente(t, svc, "cursor")
	if err := svc.store.SetDefault(ctx, "cursor"); err != nil {
		t.Fatalf("definir padrão: %v", err)
	}

	if !svc.UsesAgentTurn(ctx, perfilApontandoPara(profiles.DefaultProviderSentinel)) {
		t.Error("sentinela apontando para agente deveria usar turno de agente")
	}
}
