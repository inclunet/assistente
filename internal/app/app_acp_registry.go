package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/acpregistry"
	"assistente/internal/apidto"
	"assistente/internal/logging"
)

const acpCatalogComponent = "app.app-acp-registry"

// GetACPCatalog / RefreshACPCatalog migraram para wailsapi.ACPRegistry (AEP-0088).
// Os helpers abaixo (acpCatalogOf, acpAppInstalls, …) permanecem no App porque
// acp_install e a montagem do catálogo ainda dependem deles.

// acpCatalogOf pergunta à máquina o que ela tem e monta o catálogo da tela.
//
// A detecção é consultada uma vez por tipo de agente, e o runtime uma vez por
// runtime — não uma vez por linha: a procura vai ao sistema de arquivos, e
// repeti-la por agente custaria dezenas de varreduras para responder sobre dois.
func (a *App) acpCatalogOf(ctx context.Context, catalog acpregistry.Catalog) apidto.ACPCatalog {
	platform := acpregistry.Platform()
	if len(catalog.Agents) == 0 {
		// Sem agente não há o que procurar na máquina, e a tela só precisa do
		// motivo. Varrer o disco para montar uma lista vazia seria trabalho para
		// responder nada.
		return acpCatalogFrom(catalog, platform, acpMachine{})
	}
	return acpCatalogFrom(catalog, platform, acpMachine{
		detected:  detectACPInstalls(ctx),
		runtimes:  detectACPRuntimes(ctx, catalog.Agents, platform),
		installed: a.acpAppInstalls(),
	})
}

// acpMachine é o que esta máquina respondeu sobre os agentes do catálogo: o que
// a detecção achou instalado por fora (D1), quais runtimes estão aqui (D7) e o
// que o próprio app instalou (D5).
//
// Os três andam juntos porque são a mesma pergunta — "o que existe nesta
// máquina" — e porque é este conjunto que o teste descreve para exercitar a
// tradução contra uma máquina inventada em vez da que roda o teste.
type acpMachine struct {
	detected  map[acp.AgentKind]acpDetection
	runtimes  map[acp.Runtime]acp.RuntimeInstall
	installed map[string]acpinstall.Installation
}

// acpAppInstalls é o que este app instalou, indexado pelo identificador do
// registro (D5).
//
// Sem isto o catálogo só enxergaria a detecção escrita à mão, que conhece dois
// agentes dos 38 (D1) — e um agente instalado pelo próprio app apareceria na
// lista como "o app não sabe procurar este", que é falso justamente para o
// agente sobre o qual ele mais sabe.
func (a *App) acpAppInstalls() map[string]acpinstall.Installation {
	services := a.acpCatalogServices()
	if services == nil || services.installer == nil {
		return nil
	}
	installations := services.installer.List()
	if len(installations) == 0 {
		return nil
	}
	out := make(map[string]acpinstall.Installation, len(installations))
	for _, installation := range installations {
		out[installation.AgentID] = installation
	}
	return out
}

// acpCatalogFrom traduz o catálogo do pacote para o que a tela consome, já com o
// estado de cada agente nesta máquina.
//
// A máquina entra por parâmetro, e não é consultada aqui, para esta tradução
// poder ser exercitada contra uma máquina descrita — com Node e sem Cursor, com
// a procura falhando — em vez de contra a máquina que roda o teste.
func acpCatalogFrom(catalog acpregistry.Catalog, platform string, machine acpMachine) apidto.ACPCatalog {
	out := apidto.ACPCatalog{
		Version:      catalog.Version,
		Agents:       make([]apidto.ACPCatalogAgent, 0, len(catalog.Agents)),
		AgeSeconds:   int64(catalog.Age.Seconds()),
		FromCache:    catalog.FromCache,
		Stale:        catalog.Stale,
		ReasonCode:   string(catalog.ReasonCode),
		ReasonDetail: catalog.ReasonDetail,
		Platform:     platform,
	}
	if !catalog.FetchedAt.IsZero() {
		out.FetchedAt = catalog.FetchedAt.UTC().Format(time.RFC3339)
	}

	for _, agent := range catalog.Agents {
		out.Agents = append(out.Agents, acpCatalogAgentFrom(agent, platform, machine))
	}
	// Ordenada por nome, como a Fase 2 pede, com o identificador desempatando:
	// dois agentes de mesmo nome existiriam em ordem sorteada, e a lista mudaria
	// de posição entre duas aberturas da tela.
	slices.SortFunc(out.Agents, func(x, y apidto.ACPCatalogAgent) int {
		if order := strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)); order != 0 {
			return order
		}
		return strings.Compare(x.ID, y.ID)
	})
	return out
}

// acpDetection é o que a procura por um agente respondeu, com a falha guardada
// de lado: procura que não deu para concluir não é "não encontrado".
type acpDetection struct {
	install acp.Install
	err     error
}

// detectACPInstalls procura, uma vez cada, os agentes que a detecção conhece
// (D1). Erro não interrompe o catálogo: ele fica preso ao agente que não pôde
// ser conferido.
func detectACPInstalls(ctx context.Context) map[acp.AgentKind]acpDetection {
	kinds := acpregistry.DetectableKinds()
	found := make(map[acp.AgentKind]acpDetection, len(kinds))
	for _, kind := range kinds {
		install, err := acp.DetectAgent(kind)
		if err != nil {
			logging.Warnf(ctx, acpCatalogComponent,
				"catálogo ACP: a procura pelo agente %q não pôde ser concluída: %v", kind, err)
		}
		found[kind] = acpDetection{install: install, err: err}
	}
	return found
}

// detectACPRuntimes procura os runtimes que o catálogo de fato exige nesta
// máquina, uma vez cada. Um catálogo só de agentes binários não procura nada.
func detectACPRuntimes(ctx context.Context, agents []acpregistry.Agent, platform string) map[acp.Runtime]acp.RuntimeInstall {
	needed := make(map[acp.Runtime]struct{}, 2)
	for _, agent := range agents {
		if rt := acpregistry.FitFor(agent, platform).Runtime; rt != "" {
			needed[rt] = struct{}{}
		}
	}
	found := make(map[acp.Runtime]acp.RuntimeInstall, len(needed))
	for rt := range needed {
		install, err := acp.DetectRuntime(rt)
		if err != nil {
			// A procura pelo runtime que não pôde ser concluída conta como
			// ausente: é a direção segura, e o motivo fica no log de quem vai
			// diagnosticar. Dizer que o runtime está lá faria a tela oferecer
			// uma instalação que morre no primeiro comando.
			logging.Warnf(ctx, acpCatalogComponent,
				"catálogo ACP: a procura pelo runtime %q não pôde ser concluída: %v", rt, err)
		}
		found[rt] = install
	}
	return found
}

// acpCatalogAgentFrom monta a linha da tela a partir da entrada do registro, do
// que a detecção achou e do que esta máquina tem.
func acpCatalogAgentFrom(agent acpregistry.Agent, platform string, machine acpMachine) apidto.ACPCatalogAgent {
	fit := acpregistry.FitFor(agent, platform)
	row := apidto.ACPCatalogAgent{
		ID:            agent.ID,
		Name:          agent.Name,
		Version:       agent.Version,
		Description:   agent.Description,
		Authors:       agent.Authors,
		License:       agent.License,
		Website:       agent.Website,
		Repository:    agent.Repository,
		Distributions: fit.Distributions,
		Runtime:       string(fit.Runtime),
		Integrity:     string(fit.Integrity),
	}
	if row.Distributions == nil {
		// Lista sempre presente: `null` faria a tela distinguir "sem
		// distribuição" de "campo ausente" antes de montar o item.
		row.Distributions = []string{}
	}

	runtimeOK := true
	if fit.Runtime != "" {
		install := machine.runtimes[fit.Runtime]
		row.RuntimeFound = install.Found
		// O caminho é saneado como qualquer texto que vai à tela: ele é montado
		// a partir de variáveis de ambiente e do PATH, e uma marca invisível de
		// direção no meio dele faria o nome acessível do item ser lido diferente
		// do que ele é.
		row.RuntimePath = acp.SanitizeLabel(install.Path)
		runtimeOK = install.Found
	}

	// A instalação do app responde antes da detecção porque ela é a mais certa
	// das duas: o app sabe o que escreveu e onde. A de fora continua valendo
	// para quem já tinha o agente, e é o que a detecção responde a seguir.
	if installation, ok := machine.installed[agent.ID]; ok {
		row.State = apidto.ACPCatalogStateInstalled
		row.StateDetail = acp.SanitizeLabel(installation.Dir)
		row.InstalledByApp = true
		row.InstalledVersion = acp.SanitizeLabel(installation.Version)
		row.InstalledUnverified = unverifiedInstall(installation)
		return row
	}

	row.State, row.StateDetail, row.DetectedVersion = acpCatalogState(agent.ID, fit, runtimeOK, machine.detected)
	return row
}

// unverifiedInstall diz se o artefato que está no disco não foi conferido
// contra um digest publicado (D4).
//
// A regra mora no pacote que escreve o `installed.json`, e aqui é só a leitura
// dela na direção que a tela usa: a mesma resposta decide se a atualização pode
// trocar uma instalação conferida por outra (D10), e duas cópias dela viriam a
// discordar exatamente no caso que importa.
func unverifiedInstall(installation acpinstall.Installation) bool {
	return !acpinstall.Verified(installation)
}

// acpCatalogState decide o estado da linha nesta máquina.
//
// A ordem é a de quem vai agir: uma instalação que já existe torna o resto
// irrelevante; depois vem o que bloqueia — o runtime ausente e a plataforma sem
// alvo —; e só então a diferença entre não ter achado e não saber procurar.
func acpCatalogState(
	id string,
	fit acpregistry.Fit,
	runtimeOK bool,
	installs map[acp.AgentKind]acpDetection,
) (state, detail, detectedVersion string) {
	kind, detectable := acpregistry.DetectableKind(id)
	if detectable {
		found := installs[kind]
		switch {
		case found.install.Found:
			return apidto.ACPCatalogStateInstalled, acp.SanitizeLabel(found.install.Source), found.install.Version
		case found.err != nil:
			return apidto.ACPCatalogStateDetectionFailed, acp.SanitizeLabel(found.err.Error()), ""
		}
	}

	switch {
	case !runtimeOK:
		return apidto.ACPCatalogStateRequirementMissing, "", ""
	case fit.Integrity == acpregistry.IntegrityNoPlatformTarget && fit.Runtime == "":
		// Só binário, e sem alvo para este sistema: não há caminho nenhum aqui,
		// e não há o que instalar nem o que procurar.
		return apidto.ACPCatalogStateNoPlatformTarget, "", ""
	case detectable:
		return apidto.ACPCatalogStateNotInstalled, "", ""
	default:
		return apidto.ACPCatalogStateNoDetection, "", ""
	}
}
