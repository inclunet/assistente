package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/logging"
)

// GetModels lista os modelos que o agente oferece (AEP-0084 D6). A descoberta no
// ACP é acoplada a uma sessão, e esta consulta vem de fora de qualquer conversa —
// a tela de configurações —, então quem responde é a sessão de descoberta do
// serviço: sem prompt, na mesma conexão de todo mundo, com o resultado guardado
// por processo.
//
// Lista vazia é resposta legítima: um agente pode não expor escolha de modelo, e
// quem escolhe nesse caso é ele.
func (p *ACPChatProvider) GetModels(ctx context.Context) ([]string, error) {
	options, err := p.providerOptions(ctx)
	if err != nil {
		return nil, err
	}
	return acp.ModelValues(options), nil
}

// RefreshModels descarta o que foi descoberto e lista de novo. É o caminho do
// recarregar da tela: sem ele a lista guardada seria servida para sempre, e a
// pessoa que instalou um modelo novo no agente não teria como vê-lo aparecer
// (AEP-0084 D6).
func (p *ACPChatProvider) RefreshModels(ctx context.Context) ([]string, error) {
	if p.provider == nil {
		return nil, errors.New("provedor de agente sem configuração")
	}
	if p.agents == nil {
		return nil, errors.New("serviço de agentes de código indisponível: reinicie o app")
	}
	p.agents.InvalidateProviderOptions(p.provider.ID)
	return p.GetModels(ctx)
}

func (p *ACPChatProvider) providerOptions(ctx context.Context) ([]acp.ConfigOption, error) {
	if p.provider == nil {
		return nil, errors.New("provedor de agente sem configuração")
	}
	if p.agents == nil {
		return nil, errors.New("serviço de agentes de código indisponível: reinicie o app")
	}
	options, err := p.agents.ProviderOptions(ctx, p.spec())
	if err != nil {
		return nil, fmt.Errorf("listar modelos do agente %s: %w", p.provider.Name, err)
	}
	return options, nil
}

// spec descreve o agente para o serviço. É o mesmo descritor que o turno usa, e
// de propósito: consulta de modelos e conversa precisam cair no mesmo processo,
// senão a lista sairia de um agente e o turno de outro (AEP-0084 D3).
func (p *ACPChatProvider) spec() acp.ProviderSpec {
	return acp.ProviderSpec{
		ID:      p.provider.ID,
		Name:    p.provider.Name,
		Command: p.provider.ACPCommand,
		Args:    p.provider.ACPArgs,
		Env:     p.provider.ACPEnv,
	}
}

// applyModel põe a sessão no modelo que o perfil pede, antes de o turno sair
// (AEP-0084 D6). Vale tanto para a sessão que acabou de nascer — é aqui que o
// Chat.Model do perfil chega ao agente — quanto para a troca feita depois: um
// caminho só, porque "aplicar na criação" e "aplicar quando muda" são a mesma
// pergunta feita em momentos diferentes.
//
// Falha na troca não derruba o turno. O agente segue no modelo dele, que é uma
// resposta pior do que a pedida mas melhor do que nenhuma; o que não pode é
// acontecer em silêncio, senão a pessoa lê a resposta achando que ela veio do
// modelo que escolheu.
func (p *ACPChatProvider) applyModel(ctx context.Context, conv *acp.Conversation, model string) (notice TurnNoticeKind, ok bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false
	}
	option, exists := acp.OptionByCategory(conv.Options(), acp.CategoryModel)
	if !exists {
		// Agente que não expõe escolha de modelo: quem escolhe é ele, e avisar
		// a cada turno seria repetir uma característica do agente como se fosse
		// problema do turno.
		logging.Debugf(ctx, acpProviderComponent,
			"[ACP] o agente não oferece escolha de modelo; o perfil pedia %q", model)
		return "", false
	}
	if option.CurrentValue == model {
		return "", false
	}
	if !option.Offers(model) {
		// O perfil aponta para um modelo que este agente não tem — o que
		// acontece ao trocar de provider sem trocar o modelo, ou quando o agente
		// deixa de oferecer um que oferecia.
		logging.Warnf(ctx, acpProviderComponent,
			"[ACP] o agente não oferece o modelo %q pedido pelo perfil; o turno segue em %q", model, option.CurrentValue)
		return TurnNoticeModelNotOffered, true
	}
	if _, err := conv.SetOption(ctx, option.ID, model); err != nil {
		logging.Warnf(ctx, acpProviderComponent,
			"[ACP] não foi possível pôr a sessão no modelo %q: %v", model, err)
		return TurnNoticeModelNotApplied, true
	}
	return "", false
}
