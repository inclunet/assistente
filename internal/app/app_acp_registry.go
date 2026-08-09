package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/acpregistry"
	"assistente/internal/logging"
)

const acpCatalogComponent = "app.app-acp-registry"

// Estados de um agente do catálogo nesta máquina (AEP-0086 Fase 2).
//
// São seis, e não os três que a Fase 2 enumera, porque três não cobrem o que o
// app de fato sabe. A Fase 2 lista "encontrado por detecção", "não encontrado" e
// "requisito ausente"; os outros três existem para não transformar ignorância em
// afirmação:
//
//   - a detecção sabe procurar dois agentes dos 38 (D1), e dizer "não
//     encontrado" para os outros 36 alegaria uma procura que não aconteceu;
//   - um agente distribuído só como binário pode não ter alvo para esta
//     plataforma — em Windows ARM isso é a regra, não a exceção;
//   - a procura pode não ter dado para ser concluída (permissão negada), e aí
//     "não encontrado" mandaria reinstalar o que talvez já esteja lá.
const (
	// ACPCatalogStateInstalled é o agente que está nesta máquina, seja porque o
	// app o instalou (D5), seja porque a detecção o achou instalado por fora
	// (D1). Qual dos dois foi está em InstalledByApp, porque o que dá para fazer
	// com um e com o outro é diferente.
	ACPCatalogStateInstalled = "installed"

	// ACPCatalogStateRequirementMissing é o runtime que o agente exige e não
	// está aqui (D7). Vence "não encontrado" porque é o que bloqueia primeiro:
	// sem Node, instalar o pacote npm não é uma opção.
	ACPCatalogStateRequirementMissing = "requirement_missing"

	// ACPCatalogStateNoPlatformTarget é o agente sem forma de chegar a esta
	// máquina: só binário, e sem alvo para este sistema e arquitetura.
	ACPCatalogStateNoPlatformTarget = "no_platform_target"

	// ACPCatalogStateNotInstalled é o agente que a detecção procurou e não
	// achou. Só vale para os agentes que a detecção conhece.
	ACPCatalogStateNotInstalled = "not_installed"

	// ACPCatalogStateNoDetection é o agente para o qual este app não tem
	// detecção. Não é "não instalado": o app não olhou, e não tem como olhar.
	ACPCatalogStateNoDetection = "no_detection"

	// ACPCatalogStateDetectionFailed é a procura que não pôde ser concluída.
	ACPCatalogStateDetectionFailed = "detection_failed"
)

// ACPCatalog é o catálogo do registro ACP pronto para a tela (AEP-0086 Fase 2).
//
// Ele já vem resolvido: o estado de cada agente nesta máquina, o runtime que ele
// exige e o que se sabe sobre a integridade do artefato saem de regras do AEP, e
// deixar a tela deduzi-las do documento cru seria reimplementar o AEP em
// TypeScript.
type ACPCatalog struct {
	// Version é o `version` do documento do registro, já validado.
	Version string `json:"version,omitempty"`

	// Agents é o catálogo inteiro, ordenado por nome. Vazio quando não houve
	// como carregá-lo — e aí ReasonCode diz por quê (D2).
	Agents []ACPCatalogAgent `json:"agents"`

	// FetchedAt é quando o catálogo foi coletado, em RFC 3339. Vazio quando não
	// há catálogo. A tela mostra a idade em texto, e é ela quem formata: só ela
	// sabe o idioma de quem lê.
	FetchedAt string `json:"fetched_at,omitempty"`

	// AgeSeconds é a idade do que está sendo servido.
	AgeSeconds int64 `json:"age_seconds"`

	// FromCache diz que o catálogo veio do que estava guardado em disco.
	FromCache bool `json:"from_cache"`

	// Stale diz que a idade passou do prazo de revalidação. O conteúdo continua
	// útil, e a revalidação já foi disparada em segundo plano (D2).
	Stale bool `json:"stale"`

	// ReasonCode explica, como vocabulário fechado, por que o catálogo está
	// vazio ou por que a última atualização não aconteceu. A frase é da tela,
	// nos três locales.
	ReasonCode string `json:"reason_code,omitempty"`

	// ReasonDetail é a parte variável do motivo, quando existe: o erro de
	// transporte, já saneado.
	ReasonDetail string `json:"reason_detail,omitempty"`

	// Platform é o alvo desta máquina no vocabulário do registro
	// (`windows-x86_64`). Vazio quando o registro não nomeia esta combinação de
	// sistema e arquitetura.
	Platform string `json:"platform,omitempty"`
}

// ACPCatalogAgent é uma linha do catálogo com o que a tela precisa dizer sobre
// ela. Todo o texto já passou pelo saneamento da fronteira (D9).
type ACPCatalogAgent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	License     string   `json:"license,omitempty"`
	Website     string   `json:"website,omitempty"`
	Repository  string   `json:"repository,omitempty"`

	// Distributions são as formas de obter o agente: `binary`, `npx`, `uvx`.
	Distributions []string `json:"distributions"`

	// Runtime é o pré-requisito nomeado desta máquina — `node`, `uv` — ou vazio
	// quando o agente não exige nenhum (D7).
	Runtime string `json:"runtime,omitempty"`

	// RuntimeFound diz se esse pré-requisito está nesta máquina. Sem Runtime, é
	// sempre falso e não quer dizer nada: não há o que encontrar.
	RuntimeFound bool `json:"runtime_found"`

	// RuntimePath é o executável do runtime que atendeu. Ele fica visível porque
	// é a prova de qual instalação o app achou, que é o dado de quem tem duas
	// versões de Node na máquina.
	RuntimePath string `json:"runtime_path,omitempty"`

	// Integrity é o que se sabe sobre conferir o artefato binário nesta
	// plataforma (D4): `not_distributed`, `no_platform_target`, `digest` ou
	// `no_digest`.
	Integrity string `json:"integrity"`

	// State é o estado nesta máquina, entre as constantes ACPCatalogState*.
	State string `json:"state"`

	// StateDetail é o complemento técnico do estado, quando ele tem um: o
	// caminho da instalação encontrada, ou o motivo de a procura não ter dado.
	// Já saneado, e ele complementa a frase da tela em vez de substituí-la.
	StateDetail string `json:"state_detail,omitempty"`

	// DetectedVersion é a versão da instalação que a detecção achou, quando o
	// layout dela revela. Não é a versão do registro: são duas coisas, e é a
	// diferença entre as duas que a Fase 7 vai usar para avisar de atualização.
	DetectedVersion string `json:"detected_version,omitempty"`

	// InstalledByApp diz que quem pôs este agente na máquina foi este app (D5).
	// A distinção importa porque muda o que dá para fazer com ele: o que o app
	// instalou, ele sabe onde está, sabe atualizar e sabe remover; o que veio de
	// fora, ele apenas reconheceu.
	InstalledByApp bool `json:"installed_by_app,omitempty"`

	// InstalledVersion é a versão que o app instalou. Ela é separada da
	// detectada porque as duas têm destinos diferentes: é esta que a Fase 7
	// compara com a do registro, já que atualizar só faz sentido para o que o
	// app mesmo pôs ali.
	InstalledVersion string `json:"installed_version,omitempty"`

	// InstalledUnverified é a instalação cujo artefato o app não teve como
	// conferir contra digest publicado (D4). A marca acompanha o agente depois
	// de instalado, e não desaparece com o diálogo em que ela foi aceita: quem
	// abrir o catálogo semanas depois precisa saber o que aquele arquivo vale.
	InstalledUnverified bool `json:"installed_unverified,omitempty"`
}

// GetACPCatalog devolve o catálogo do registro ACP para a tela de provedores.
//
// Ela abre sem rede (D2): o que está em cache é servido na hora e a revalidação
// acontece em segundo plano. Sem cache e sem rede, o catálogo vem vazio com o
// motivo — e isso não é erro desta chamada, é o estado que a tela explica.
func (a *App) GetACPCatalog() (ACPCatalog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return ACPCatalog{}, err
	}
	if a.acpRegistry == nil {
		return ACPCatalog{}, errors.New("serviço do registro de agentes ACP não inicializado")
	}
	return a.acpCatalogOf(ctx, a.acpRegistry.Catalog(ctx)), nil
}

// RefreshACPCatalog busca o índice agora, a pedido de quem clicou.
//
// Ela existe porque recarregar é ato explícito (D2): a revalidação automática só
// acontece depois do prazo, e quem estava sem rede quando a tela abriu não tem
// por que esperar por ele para tentar de novo. Falha não custa o catálogo que já
// estava servindo — o que volta é o anterior, com o motivo.
func (a *App) RefreshACPCatalog() (ACPCatalog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return ACPCatalog{}, err
	}
	if a.acpRegistry == nil {
		return ACPCatalog{}, errors.New("serviço do registro de agentes ACP não inicializado")
	}
	// O erro não sobe: ele já está dentro do catálogo, como motivo que a tela
	// diz no idioma de quem lê. Subir também faria a tela mostrar duas versões
	// da mesma falha, uma delas em português.
	catalog, _ := a.acpRegistry.Refresh(ctx)
	return a.acpCatalogOf(ctx, catalog), nil
}

// acpCatalogOf pergunta à máquina o que ela tem e monta o catálogo da tela.
//
// A detecção é consultada uma vez por tipo de agente, e o runtime uma vez por
// runtime — não uma vez por linha: a procura vai ao sistema de arquivos, e
// repeti-la por agente custaria dezenas de varreduras para responder sobre dois.
func (a *App) acpCatalogOf(ctx context.Context, catalog acpregistry.Catalog) ACPCatalog {
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
func acpCatalogFrom(catalog acpregistry.Catalog, platform string, machine acpMachine) ACPCatalog {
	out := ACPCatalog{
		Version:      catalog.Version,
		Agents:       make([]ACPCatalogAgent, 0, len(catalog.Agents)),
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
	slices.SortFunc(out.Agents, func(x, y ACPCatalogAgent) int {
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
func acpCatalogAgentFrom(agent acpregistry.Agent, platform string, machine acpMachine) ACPCatalogAgent {
	fit := acpregistry.FitFor(agent, platform)
	row := ACPCatalogAgent{
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
		row.State = ACPCatalogStateInstalled
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
			return ACPCatalogStateInstalled, acp.SanitizeLabel(found.install.Source), found.install.Version
		case found.err != nil:
			return ACPCatalogStateDetectionFailed, acp.SanitizeLabel(found.err.Error()), ""
		}
	}

	switch {
	case !runtimeOK:
		return ACPCatalogStateRequirementMissing, "", ""
	case fit.Integrity == acpregistry.IntegrityNoPlatformTarget && fit.Runtime == "":
		// Só binário, e sem alvo para este sistema: não há caminho nenhum aqui,
		// e não há o que instalar nem o que procurar.
		return ACPCatalogStateNoPlatformTarget, "", ""
	case detectable:
		return ACPCatalogStateNotInstalled, "", ""
	default:
		return ACPCatalogStateNoDetection, "", ""
	}
}
