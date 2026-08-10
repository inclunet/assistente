package acpinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
	"assistente/internal/configdir"
	"assistente/internal/logging"
	httpclient "assistente/internal/tools/http"
)

const component = "acpinstall"

// installedFileName é o registro que fica ao lado do que foi instalado (D5).
const installedFileName = "installed.json"

// rootSubdir é onde as instalações moram dentro de `~/.assistente/` (D5).
//
// É o diretório home que `internal/configdir` resolve, e não o do executável nem
// o do workspace: o do executável pode ser somente-leitura, e o do workspace muda
// quando a pessoa troca de projeto — um agente instalado não pode sumir porque
// alguém foi olhar outra pasta.
const rootSubdir = "agents"

// artifactTimeout é o prazo do download de um artefato.
//
// O padrão do cliente da casa são 30 segundos, que servem para uma chamada de
// API e cortariam pela metade o download de um agente: o teto de artefato deste
// pacote é de 512 MB, e há conexão doméstica onde isso leva a tarde inteira. O
// prazo daqui é folgado a ponto de só alcançar um download que travou —
// interromper o que está andando é decisão de quem clica em cancelar (D13).
const artifactTimeout = 2 * time.Hour

// Config monta o instalador.
type Config struct {
	// Root é onde as instalações moram. Vazio usa `~/.assistente/agents` (D5).
	Root string

	// Source é o catálogo do registro.
	Source CatalogSource

	// HTTP baixa os artefatos binários. Vazio monta o cliente da casa
	// (`internal/tools/http`) com o prazo deste pacote; quem quiser o do app,
	// com o wiring de credencial e de política de rede que ele carrega, injeta
	// o dele aqui.
	HTTP Doer

	// NPM executa o npm. Vazio usa o npm da instalação de Node encontrada na
	// máquina, procurada no momento do uso.
	NPM NPM

	// UV executa o uv. Vazio usa o uv encontrado na máquina, procurado no
	// momento do uso.
	UV UV

	// Runtime procura o Node. Vazio usa a procura de `internal/acp`.
	Runtime func() acp.NodeRuntime

	// UVRuntime procura o uv. Vazio usa a procura de `internal/acp`.
	UVRuntime func() acp.UVRuntime

	// Handshake confere que o comando resolvido fala ACP (D8). Sem ele a
	// instalação não tem como ser declarada concluída, e o padrão recusa em vez
	// de deixar passar sem prova: um provider salvo que nunca sobe é pior do que
	// uma instalação que falhou.
	Handshake Handshake

	// OnProgress recebe os marcos (D13). Opcional.
	OnProgress func(Progress)

	// Now é o relógio, para o teste poder carimbar o `installed.json`.
	Now func() time.Time
}

// Installer instala e remove agentes do catálogo.
//
// Uma instância atende a tela inteira: ela guarda as instalações em voo para
// poder cancelá-las e para recusar duas do mesmo agente ao mesmo tempo — duas
// escrevendo no mesmo diretório deixariam meio agente no disco.
type Installer struct {
	root       string
	source     CatalogSource
	http       Doer
	npm        NPM
	uv         UV
	runtime    func() acp.NodeRuntime
	uvRuntime  func() acp.UVRuntime
	handshake  Handshake
	progress   func(Progress)
	now        func() time.Time

	mu      sync.Mutex
	running map[string]context.CancelFunc

	// knownMu serializa a escrita da memória de artefatos, que é um arquivo só
	// para todos os agentes: a guarda acima é por agente, e dois agentes
	// instalando ao mesmo tempo perderiam uma entrada na corrida entre ler e
	// gravar.
	knownMu sync.Mutex
}

// New monta o instalador.
func New(cfg Config) *Installer {
	root := cfg.Root
	if root == "" {
		if home := configdir.GetHomeDir(); home != "" {
			root = filepath.Join(home, rootSubdir)
		}
	}
	lookup := cfg.Runtime
	if lookup == nil {
		lookup = acp.FindNodeRuntime
	}
	uvLookup := cfg.UVRuntime
	if uvLookup == nil {
		uvLookup = acp.FindUVRuntime
	}
	clock := cfg.Now
	if clock == nil {
		clock = time.Now
	}
	npm := cfg.NPM
	if npm == nil {
		npm = lazyNPM{lookup: lookup}
	}
	uv := cfg.UV
	if uv == nil {
		uv = lazyUV{lookup: uvLookup}
	}
	handshake := cfg.Handshake
	if handshake == nil {
		handshake = HandshakeUnsupported
	}
	client := cfg.HTTP
	if client == nil {
		client = httpclient.New(&httpclient.Config{Timeout: artifactTimeout}, map[string]string{})
	}
	return &Installer{
		root:      root,
		source:    cfg.Source,
		http:      client,
		npm:       npm,
		uv:        uv,
		runtime:   lookup,
		uvRuntime: uvLookup,
		handshake: handshake,
		progress:  cfg.OnProgress,
		now:       clock,
		running:   make(map[string]context.CancelFunc),
	}
}

// Root é onde as instalações moram. A tela mostra o caminho porque o app está
// escrevendo no disco de alguém.
func (i *Installer) Root() string { return i.root }

// Plan diz o que será instalado e se dá para instalar agora (D3, D7).
//
// Ele não baixa nada e não escreve nada: é o que alimenta o diálogo de
// confirmação e o estado do item na tela.
func (i *Installer) Plan(ctx context.Context, agentID string) (Plan, error) {
	agent, err := i.agent(ctx, agentID)
	if err != nil {
		return Plan{}, err
	}
	// Uma procura só para o plano inteiro: a linha de comando que ele mostra tem
	// de ser a do runtime que ele diz ter encontrado, e não a de uma segunda
	// procura feita alguns microssegundos depois.
	node := i.runtime()
	switch i.distributionFor(agent, node) {
	case DistributionBinary:
		return i.binaryPlan(agent, sanitizeVersion(agent.Version)), nil
	case DistributionUVX:
		return i.uvxPlan(agent), nil
	case DistributionNPM:
		return i.npmPlan(agent, node), nil
	default:
		return i.unavailablePlan(agent, failf(StepCatalog, "%w: %s", ErrNotNPM, agent.ID)), nil
	}
}

// npmPlan monta o plano de instalação por pacote npm.
func (i *Installer) npmPlan(agent acpregistry.Agent, runtime acp.NodeRuntime) Plan {
	spec, _, version, err := pinnedSpec(agent)
	if err != nil {
		return i.unavailablePlan(agent, err)
	}

	dir := i.agentVersionDir(agent.ID, version)
	plan := Plan{
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      version,
		Distribution: DistributionNPM,
		Origin:       spec,
		Dir:          dir,
		RunArgs:      slices.Clone(agent.Distribution.NPX.Args),
		Runtime:      requiredRuntime(runtime),
	}
	i.describeInstalled(&plan, agent.ID)

	switch {
	case i.root == "":
		plan.Reason = "não foi possível descobrir o diretório de dados do app para instalar o agente"
	case !plan.Runtime.Found:
		plan.Reason = ErrRuntimeMissing.Error()
	case plan.Installed != nil:
		plan.Reason = ErrAlreadyInstalled.Error()
	default:
		command := i.npmFor(runtime).Describe(dir, spec)
		if command == "" {
			plan.Reason = ErrNoNPM.Error()
			break
		}
		plan.CanInstall = true
		plan.InstallCommand = command
	}
	i.describeUpdate(&plan, agent)
	return plan
}

// uvxPlan monta o plano de instalação por pacote do uv (Fase 9).
func (i *Installer) uvxPlan(agent acpregistry.Agent) Plan {
	spec, _, version, err := pinnedUVSpec(agent)
	if err != nil {
		return i.unavailablePlan(agent, err)
	}
	uv := i.uvRuntime()
	dir := i.agentVersionDir(agent.ID, version)
	plan := Plan{
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      version,
		Distribution: DistributionUVX,
		Origin:       spec,
		Dir:          dir,
		RunArgs:      slices.Clone(agent.Distribution.UVX.Args),
		Runtime:      requiredUVRuntime(uv),
	}
	i.describeInstalled(&plan, agent.ID)

	switch {
	case i.root == "":
		plan.Reason = "não foi possível descobrir o diretório de dados do app para instalar o agente"
	case !plan.Runtime.Found:
		plan.Reason = ErrRuntimeMissingUV.Error()
	case plan.Installed != nil:
		plan.Reason = ErrAlreadyInstalled.Error()
	default:
		command := i.uvFor(uv).Describe(dir, spec)
		if command == "" {
			plan.Reason = ErrNoUV.Error()
			break
		}
		plan.CanInstall = true
		plan.InstallCommand = command
	}
	i.describeUpdate(&plan, agent)
	return plan
}

// describeInstalled põe no plano a instalação que existe deste agente, em
// qualquer versão (D10).
//
// Não é a instalação do diretório desta versão: essa desapareceria no dia em
// que o registro publicasse a seguinte, e o app ofereceria instalar do zero o
// que ele mesmo pôs ali — sobre uma instalação que continua funcionando.
func (i *Installer) describeInstalled(plan *Plan, agentID string) {
	if installed, ok := i.Installed(agentID); ok {
		plan.Installed = &installed
	}
}

// describeUpdate decide se este plano oferece atualizar, e por que não oferece
// quando não oferece (D10).
//
// Atualizar exige tudo o que instalar exige, e mais uma coisa: a versão nova
// não pode valer menos do que a instalada. Se o agente parou de publicar digest
// entre uma e outra, o app mantém o que está no disco e explica — aceitar a
// troca faria do aviso de atualização um caminho para contornar o D4.
func (i *Installer) describeUpdate(plan *Plan, agent acpregistry.Agent) {
	if plan.Installed == nil || plan.Version == "" || plan.Installed.Version == plan.Version {
		return
	}
	plan.Update = true
	switch {
	case plan.Reason != "" && plan.Reason != ErrAlreadyInstalled.Error():
		// O que impede instalar impede atualizar: sem Node, sem npm e sem
		// diretório de dados não há como pôr a versão nova ao lado da velha. O
		// único motivo que não vale aqui é justamente o de já estar instalado.
		plan.UpdateReason = plan.Reason
	case wouldDropVerification(agent, plan.Distribution, *plan.Installed):
		plan.UpdateReason = ErrVerificationWouldDrop.Error()
	default:
		plan.CanUpdate = true
	}
}

// wouldDropVerification diz se atualizar trocaria um artefato conferido por um
// que o app não tem como conferir (D10).
//
// Instalação que já não era conferida não perde nada, e pacote npm é conferido
// pelo próprio npm em qualquer versão. O caso que existe é o do binário: o
// mesmo agente publica `sha256` numa versão e não publica na seguinte, e é essa
// troca que a atualização recusa.
func wouldDropVerification(agent acpregistry.Agent, distribution string, installed Installation) bool {
	if !Verified(installed) || distribution != DistributionBinary {
		return false
	}
	target, _, err := binaryTarget(agent)
	if err != nil {
		// Sem alvo para esta plataforma não há atualização nenhuma a fazer, e
		// o motivo disso é outro. Dizer que a verificação cairia inventaria uma
		// comparação com um artefato que não existe.
		return false
	}
	return target.SHA256 == ""
}

// npmFor é o npm da instalação que está acontecendo.
//
// O npm padrão é preguiçoso porque quem instalou o Node depois de abrir o app não
// deveria ter de reabri-lo. Mas dentro de uma instalação o runtime já foi
// procurado e conferido, e deixá-lo procurar de novo abriria espaço para o npm
// falhar por causa de um Node que não é o que passou na conferência de instantes
// atrás — e ainda pagaria a procura duas vezes.
func (i *Installer) npmFor(runtime acp.NodeRuntime) NPM {
	if _, lazy := i.npm.(lazyNPM); lazy {
		return NewNPM(runtime)
	}
	return i.npm
}

// uvFor é o uv da instalação que está acontecendo — o mesmo cuidado do npmFor.
func (i *Installer) uvFor(runtime acp.UVRuntime) UV {
	if _, lazy := i.uv.(lazyUV); lazy {
		return NewUV(runtime)
	}
	return i.uv
}

// unavailablePlan é o item que o app não sabe instalar, com o motivo dito em
// texto em vez de um botão cinza sem explicação (D7).
//
// A distribuição só é declarada quando o agente de fato publica por um caminho
// conhecido. Dizer `npm` num agente só-uvx (ou o contrário) daria um plano que
// se contradiz.
func (i *Installer) unavailablePlan(agent acpregistry.Agent, err error) Plan {
	distribution := ""
	switch {
	case agent.Distribution.NPX != nil:
		distribution = DistributionNPM
	case agent.Distribution.UVX != nil:
		distribution = DistributionUVX
	case len(agent.Distribution.Binary) > 0:
		distribution = DistributionBinary
	}
	return Plan{
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      agent.Version,
		Distribution: distribution,
		// Aqui o runtime não é pré-requisito de nada: a recusa é da
		// distribuição, e marcar o Node/uv como exigido faria a tela dizer
		// "instale o runtime" no lugar do motivo pelo qual o app não sabe
		// instalar este agente.
		Runtime: runtimeStatus(i.runtime()),
		Reason:  acp.SanitizeLabel(err.Error()),
	}
}

// Install instala o agente pedido e só volta com sucesso depois de o comando
// resolvido responder `initialize` (D8).
//
// Instalar é ação pedida (D3): quem chama já mostrou o que vai ser baixado e
// recebeu o consentimento. Este método não pergunta nada.
// As recusas são conferidas aqui, e não pelo Plan: o plano existe para a tela e
// devolve o motivo em texto, e um texto não diz a quem chamou *qual* recusa
// aconteceu. Repetir as perguntas é o que faz `errors.Is` valer para quem
// programa contra este pacote.
// `confirmed` é o plano que quem chamou mostrou e teve aceito. O zero valor
// aceita o que o app escolher agora.
func (i *Installer) Install(ctx context.Context, agentID string, confirmed Confirmed) (Installation, error) {
	agent, err := i.agent(ctx, agentID)
	if err != nil {
		return Installation{}, err
	}
	// Instalar por cima de qualquer versão é recusado, e não só por cima da
	// mesma: pôr a nova ao lado sem tirar a anterior é atualizar, e atualizar
	// tem um passo no meio que este caminho não dá — repontar o provider antes
	// de a versão velha sumir (D10).
	if existing, ok := i.Installed(agent.ID); ok {
		return Installation{}, failf(StepPrepare, "%w: %s %s", ErrAlreadyInstalled, agent.Name, existing.Version)
	}
	runtime := i.runtime()
	return i.installAgent(ctx, agent, i.distributionFor(agent, runtime), runtime, confirmed)
}

// Update instala a versão que o catálogo publica ao lado da que está em uso e
// devolve as duas (D10).
//
// Ela não remove a anterior. Entre a nova responder `initialize` e a velha sair
// do disco existe um passo que este pacote não faz — repontar o provider —, e
// apagar antes dele deixaria o provider apontando para um diretório que acabou
// de sumir. Quem chama remove com RemoveVersion depois de repontar; uma
// atualização que falha no meio deixa de pé o que funcionava.
//
// As recusas são repetidas aqui, e não deduzidas do plano: o plano existe para
// a tela e devolve o motivo em texto, e um texto não diz a quem chamou *qual*
// recusa aconteceu.
func (i *Installer) Update(ctx context.Context, agentID string, confirmed Confirmed) (Updated, error) {
	agent, err := i.agent(ctx, agentID)
	if err != nil {
		return Updated{}, err
	}
	previous, ok := i.Installed(agent.ID)
	if !ok {
		return Updated{}, failf(StepPrepare, "%w: %s", ErrNotInstalled, acp.SanitizeLabel(agentID))
	}
	runtime := i.runtime()
	distribution := i.distributionFor(agent, runtime)
	version, err := plannedVersion(agent, distribution)
	if err != nil {
		return Updated{}, err
	}
	if version == previous.Version {
		return Updated{}, failf(StepCatalog, "%w: %s %s", ErrNoUpdate, agent.Name, previous.Version)
	}
	if wouldDropVerification(agent, distribution, previous) {
		return Updated{}, failf(StepCatalog, "%w: %s %s", ErrVerificationWouldDrop, agent.Name, version)
	}
	// A distribuição e o runtime já foram resolvidos nas checagens acima. Passar
	// os dois adiante é o que impede a instalação de escolher outro caminho se o
	// Node aparecer ou sumir entre a conferência e o download.
	installation, err := i.installAgent(ctx, agent, distribution, runtime, confirmed)
	if err != nil {
		return Updated{}, err
	}
	return Updated{Installed: installation, Previous: previous}, nil
}

// installAgent instala pelo caminho já escolhido. Instalar e atualizar passam
// pelos mesmos passos — é a mesma versão sendo baixada, conferida e registrada
// —, e o que os separa é o que acontece em volta. Quem chama resolve runtime e
// distribuição uma vez; resolver de novo aqui poderia trocar o caminho no meio
// da atualização.
func (i *Installer) installAgent(
	ctx context.Context,
	agent acpregistry.Agent,
	distribution string,
	runtime acp.NodeRuntime,
	confirmed Confirmed,
) (Installation, error) {
	switch distribution {
	case DistributionBinary:
		return i.installFromBinary(ctx, agent, confirmed)
	case DistributionUVX:
		return i.installFromUVX(ctx, agent, confirmed)
	case DistributionNPM:
		return i.installFromNPM(ctx, agent, runtime, confirmed)
	default:
		return Installation{}, failf(StepCatalog, "%w: %s", ErrNotNPM, agent.ID)
	}
}

// plannedVersion é a versão que seria instalada agora por este caminho. Ela é o
// que a comparação do D10 usa: o campo `version` do item não vale para o pacote
// npm/uv, onde quem fixa a versão pode ser o próprio nome do pacote.
func plannedVersion(agent acpregistry.Agent, distribution string) (string, error) {
	switch distribution {
	case DistributionBinary:
		version := sanitizeVersion(agent.Version)
		if version == "" {
			return "", failf(StepCatalog, "%w: %s", ErrUnpinnedVersion, acp.SanitizeLabel(agent.ID))
		}
		return version, nil
	case DistributionUVX:
		_, _, version, err := pinnedUVSpec(agent)
		return version, err
	default:
		_, _, version, err := pinnedSpec(agent)
		return version, err
	}
}

// installFromNPM instala o agente que é distribuído como pacote.
func (i *Installer) installFromNPM(
	ctx context.Context,
	agent acpregistry.Agent,
	runtime acp.NodeRuntime,
	confirmed Confirmed,
) (Installation, error) {
	spec, name, version, err := pinnedSpec(agent)
	if err != nil {
		return Installation{}, err
	}
	if err := confirmed.check(Confirmed{Distribution: DistributionNPM, Origin: spec}); err != nil {
		return Installation{}, err
	}
	dir := i.agentVersionDir(agent.ID, version)
	if dir == "" {
		return Installation{}, failf(StepPrepare,
			"não foi possível montar o diretório de instalação do agente %s versão %s",
			acp.SanitizeLabel(agent.ID), acp.SanitizeLabel(version))
	}
	if !runtime.Found {
		return Installation{}, failf(StepRuntime, "%w; procurei em: %s", ErrRuntimeMissing, describePaths(runtime.Searched))
	}
	if _, _, ok := runtime.NPMCommand(); !ok {
		return Installation{}, failf(StepRuntime, "%w (Node em %s)", ErrNoNPM, runtime.Node)
	}
	if existing, ok := i.installationAt(dir); ok {
		return Installation{}, failf(StepPrepare, "%w: %s %s", ErrAlreadyInstalled, agent.Name, existing.Version)
	}
	return i.run(ctx, agent, dir, func(ctx context.Context) (Installation, error) {
		return i.install(ctx, agent, spec, name, version, dir, runtime)
	})
}

// installFromUVX instala o agente distribuído por pacote do uv (Fase 9).
func (i *Installer) installFromUVX(
	ctx context.Context,
	agent acpregistry.Agent,
	confirmed Confirmed,
) (Installation, error) {
	spec, name, version, err := pinnedUVSpec(agent)
	if err != nil {
		return Installation{}, err
	}
	if err := confirmed.check(Confirmed{Distribution: DistributionUVX, Origin: spec}); err != nil {
		return Installation{}, err
	}
	dir := i.agentVersionDir(agent.ID, version)
	if dir == "" {
		return Installation{}, failf(StepPrepare,
			"não foi possível montar o diretório de instalação do agente %s versão %s",
			acp.SanitizeLabel(agent.ID), acp.SanitizeLabel(version))
	}
	uv := i.uvRuntime()
	if !uv.Found {
		return Installation{}, failf(StepRuntime, "%w; procurei em: %s", ErrRuntimeMissingUV, describePaths(uv.Searched))
	}
	if existing, ok := i.installationAt(dir); ok {
		return Installation{}, failf(StepPrepare, "%w: %s %s", ErrAlreadyInstalled, agent.Name, existing.Version)
	}
	return i.run(ctx, agent, dir, func(ctx context.Context) (Installation, error) {
		return i.installUV(ctx, agent, spec, name, version, dir, uv)
	})
}

// run é o ciclo de vida comum às duas distribuições: uma instalação por agente
// de cada vez, limpeza do que ficou pela metade e o marco de desfecho.
//
// Ele existe uma vez só porque as regras são as mesmas independentemente de o
// que chegou do outro lado ter vindo do npm ou de um archive — e porque duas
// cópias do mesmo cuidado viram, com o tempo, duas políticas de limpeza
// diferentes.
func (i *Installer) run(
	ctx context.Context,
	agent acpregistry.Agent,
	dir string,
	install func(context.Context) (Installation, error),
) (Installation, error) {
	ctx, done, err := i.begin(ctx, agent.ID)
	if err != nil {
		return Installation{}, err
	}
	defer done()

	installation, err := install(ctx)
	if err != nil {
		// Instalação interrompida não deixa meio agente no disco (D13). O mesmo
		// vale para a que falhou: um diretório com metade de um pacote seria
		// lido como instalação na próxima abertura.
		i.discard(ctx, dir)
		// Só o cancelamento é cancelamento. Contexto encerrado por prazo é falha,
		// e anunciá-lo como decisão de quem clicou esconderia o que aconteceu de
		// quem não clicou em nada.
		i.emit(ctx, failureProgress(agent, errors.Is(ctx.Err(), context.Canceled), err))
		return Installation{}, err
	}
	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageDone})
	return installation, nil
}

// install é a instalação em si, sem a guarda de concorrência e sem a limpeza:
// quem chama cuida das duas, para todo desfecho ruim passar pelo mesmo lugar.
func (i *Installer) install(
	ctx context.Context,
	agent acpregistry.Agent,
	spec, name, version, dir string,
	runtime acp.NodeRuntime,
) (Installation, error) {
	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageStarted})

	if err := prepareDir(dir); err != nil {
		return Installation{}, failf(StepPrepare, "não foi possível preparar %s: %w", dir, err)
	}

	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageInstalling})
	if err := i.npmFor(runtime).Install(ctx, dir, spec); err != nil {
		return Installation{}, failf(StepInstall, "%w", err)
	}

	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageVerifying})
	pkg, err := acp.NPMEntryPoint(dir, name)
	if err != nil {
		return Installation{}, failf(StepResolve, "%w", err)
	}
	// O par `node` + ponto de entrada, e não o `.cmd` que o npm liga: o app
	// precisa conseguir encerrar o processo do agente (D8, AEP-0084 D15).
	command := runtime.Node
	args := append([]string{pkg.EntryPoint}, agent.Distribution.NPX.Args...)

	if err := i.handshake(ctx, command, args); err != nil {
		return Installation{}, failf(StepVerify, "o agente instalado não respondeu ao handshake do protocolo: %w", err)
	}

	installation := Installation{
		Schema:       installationSchema,
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      version,
		Distribution: DistributionNPM,
		Target:       spec,
		Command:      command,
		Args:         args,
		InstalledAt:  i.now().UTC(),
		Dir:          dir,
	}
	if err := writeInstallation(dir, installation); err != nil {
		return Installation{}, failf(StepRecord, "%w", err)
	}
	logging.Infof(ctx, component, "agente %s instalado em %s", agent.ID, dir)
	return installation, nil
}

// installUV é a instalação por uv, sem a guarda de concorrência e sem a limpeza.
func (i *Installer) installUV(
	ctx context.Context,
	agent acpregistry.Agent,
	spec, name, version, dir string,
	runtime acp.UVRuntime,
) (Installation, error) {
	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageStarted})

	if err := prepareDir(dir); err != nil {
		return Installation{}, failf(StepPrepare, "não foi possível preparar %s: %w", dir, err)
	}

	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageInstalling})
	if err := i.uvFor(runtime).Install(ctx, dir, spec); err != nil {
		return Installation{}, failf(StepInstall, "%w", err)
	}

	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageVerifying})
	script, python, err := acp.UVEntryPoint(dir, name)
	if err != nil {
		return Installation{}, failf(StepResolve, "%w", err)
	}
	// No Windows o lançador gerado é um `.exe` spawnável: usá-lo direto. Nos
	// demais casos o comando é o Python do venv + o script — o espelho do par
	// `node` + ponto de entrada do npm —, e nunca um `.cmd`/`.bat` (D8,
	// AEP-0084 D15).
	command := python
	args := append([]string{script}, agent.Distribution.UVX.Args...)
	if acp.Spawnable(script) && strings.EqualFold(filepath.Ext(script), ".exe") {
		command = script
		args = slices.Clone(agent.Distribution.UVX.Args)
	}

	if err := i.handshake(ctx, command, args); err != nil {
		return Installation{}, failf(StepVerify, "o agente instalado não respondeu ao handshake do protocolo: %w", err)
	}

	installation := Installation{
		Schema:       installationSchema,
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      version,
		Distribution: DistributionUVX,
		Target:       spec,
		Command:      command,
		Args:         args,
		InstalledAt:  i.now().UTC(),
		Dir:          dir,
	}
	if err := writeInstallation(dir, installation); err != nil {
		return Installation{}, failf(StepRecord, "%w", err)
	}
	logging.Infof(ctx, component, "agente %s instalado em %s", agent.ID, dir)
	return installation, nil
}

// begin registra a instalação em voo e devolve o contexto cancelável dela.
//
// O cancelamento é o do pedido, guardado para Cancel poder acioná-lo de outra
// chamada: quem clicou em cancelar não é quem está esperando pela instalação.
func (i *Installer) begin(ctx context.Context, agentID string) (context.Context, func(), error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.running[agentID]; ok {
		return nil, nil, failf(StepPrepare, "%w: %s", ErrInstalling, agentID)
	}
	ctx, cancel := context.WithCancel(ctx)
	i.running[agentID] = cancel
	return ctx, func() {
		i.mu.Lock()
		delete(i.running, agentID)
		i.mu.Unlock()
		cancel()
	}, nil
}

// Cancel interrompe a instalação em voo do agente. Falso quer dizer que não
// havia nenhuma — o que acontece quando ela acabou de terminar, e não é erro.
func (i *Installer) Cancel(agentID string) bool {
	i.mu.Lock()
	cancel, ok := i.running[agentID]
	i.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// Installing diz se há instalação em voo do agente, para a tela saber que o
// botão de cancelar tem o que cancelar.
func (i *Installer) Installing(agentID string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	_, ok := i.running[agentID]
	return ok
}

// Remove apaga o diretório do agente (D5).
//
// O que a remoção não faz é apagar o provider: ele é configuração de quem o
// criou, e sumir com ele por causa de um clique em "remover agente" destruiria
// escolha alheia. O provider fica com um comando que não existe, e o health do
// AEP-0084 D12 já sabe dizer isso.
func (i *Installer) Remove(ctx context.Context, agentID string) error {
	dir, err := i.agentDir(agentID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotInstalled, agentID)
		}
		return fmt.Errorf("não foi possível conferir %s: %w", dir, err)
	}
	if err := removeTree(dir); err != nil {
		return fmt.Errorf("não foi possível remover %s: %w", dir, err)
	}
	logging.Infof(ctx, component, "instalação do agente %s removida de %s", agentID, dir)
	return nil
}

// RemoveVersion apaga uma versão só, deixando as outras (D10).
//
// É o último passo da atualização: a versão nova já respondeu ao handshake e o
// provider já aponta para ela, e o que sai é o diretório que ninguém usa mais.
// Ela não é o Remove com um argumento a mais — o Remove apaga o agente inteiro,
// e usá-lo aqui levaria junto a versão que acabou de subir.
func (i *Installer) RemoveVersion(ctx context.Context, agentID, version string) error {
	dir := i.agentVersionDir(agentID, version)
	if dir == "" {
		return fmt.Errorf("não foi possível montar o diretório da versão %s do agente %s",
			acp.SanitizeLabel(version), acp.SanitizeLabel(agentID))
	}
	// O registro tem de descrever aquele diretório, como em qualquer leitura
	// (D9): apagar por caminho montado a partir de dois textos externos sem
	// conferir o que está lá dentro é apagar o que a régua deixar passar.
	if _, ok := i.installationAt(dir); !ok {
		return fmt.Errorf("%w: %s %s", ErrNotInstalled, acp.SanitizeLabel(agentID), acp.SanitizeLabel(version))
	}
	if err := removeTree(dir); err != nil {
		return fmt.Errorf("não foi possível remover %s: %w", dir, err)
	}
	logging.Infof(ctx, component, "versão %s do agente %s removida de %s", version, agentID, dir)
	return nil
}

// Installed devolve a instalação do agente, quando há uma.
//
// Mais de uma versão pode morar sob `<id>/`: é isso que permite baixar a nova ao
// lado da que está em uso (D10). A escolhida é a instalada por último, e não a
// primeira que o diretório listar — a listagem é alfabética, e por ela a `10.0.0`
// viria antes da `2.0.0`. Empate de carimbo fica com a primeira, que é estável
// porque `os.ReadDir` devolve ordenado por nome.
func (i *Installer) Installed(agentID string) (Installation, bool) {
	var newest Installation
	found := false
	for _, installation := range i.Installations(agentID) {
		if !found || installation.InstalledAt.After(newest.InstalledAt) {
			newest, found = installation, true
		}
	}
	return newest, found
}

// Installations são todas as versões deste agente que estão no disco, na ordem
// em que o diretório as lista.
//
// Mais de uma é o estado normal no meio de uma atualização, e pode sobrar de uma
// que não conseguiu limpar a anterior — a remoção é o último passo, e ela é
// adiada quando o agente está em conversa (D10). Quem quer só a que vale usa
// Installed; quem quer varrer o que ficou para trás usa esta.
func (i *Installer) Installations(agentID string) []Installation {
	dir, err := i.agentDir(agentID)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]Installation, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if installation, ok := i.installationAt(filepath.Join(dir, entry.Name())); ok {
			out = append(out, installation)
		}
	}
	return out
}

// List devolve o que o app instalou, ordenado por identificador para a tela ter
// uma ordem estável.
//
// Diretório sem `installed.json` legível não entra: ou é resíduo de uma
// instalação interrompida junto com o app, ou não foi este app que o escreveu.
// Nos dois casos, tratá-lo como instalação faria a tela oferecer usar um agente
// que não existe.
func (i *Installer) List() []Installation {
	if i.root == "" {
		return nil
	}
	entries, err := os.ReadDir(i.root)
	if err != nil {
		return nil
	}
	out := make([]Installation, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if installation, ok := i.Installed(entry.Name()); ok {
			out = append(out, installation)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].AgentID < out[b].AgentID })
	return out
}

// agent acha o agente no catálogo servido.
func (i *Installer) agent(ctx context.Context, agentID string) (acpregistry.Agent, error) {
	if i.source == nil {
		return acpregistry.Agent{}, failf(StepCatalog, "o catálogo do registro ACP não está disponível")
	}
	catalog := i.source.Catalog(ctx)
	for _, agent := range catalog.Agents {
		if agent.ID == agentID {
			return agent, nil
		}
	}
	if catalog.Reason != "" {
		// O catálogo vazio tem motivo próprio, e ele é mais útil do que dizer
		// que o agente não existe: quem está sem rede na primeira execução não
		// tem catálogo nenhum, e o agente pode muito bem estar no registro.
		return acpregistry.Agent{}, failf(StepCatalog, "%w: %s", ErrNotInCatalog, catalog.Reason)
	}
	return acpregistry.Agent{}, failf(StepCatalog, "%w: %s", ErrNotInCatalog, acp.SanitizeLabel(agentID))
}

// agentDir é `<root>/<id>`, com o identificador conferido antes de virar
// caminho: ele vem do catálogo, que é dado externo (D9).
func (i *Installer) agentDir(agentID string) (string, error) {
	if i.root == "" {
		return "", errors.New("não foi possível descobrir o diretório de dados do app")
	}
	if !safePathSegment(agentID) {
		return "", fmt.Errorf("identificador de agente inválido: %q", acp.SanitizeLabel(agentID))
	}
	return filepath.Join(i.root, agentID), nil
}

// agentVersionDir é `<root>/<id>/<versão>` (D5). A versão entra no caminho
// porque ela permite baixar a nova ao lado da que está em uso (D10) e porque
// remover passa a ser apagar um diretório.
func (i *Installer) agentVersionDir(agentID, version string) string {
	dir, err := i.agentDir(agentID)
	if err != nil || !safePathSegment(version) {
		return ""
	}
	return filepath.Join(dir, version)
}

// installationAt lê o `installed.json` de um diretório de versão e confere que
// ele descreve aquele diretório.
//
// O arquivo é dado externo. Ele vive no disco de quem usa o app, e nada impede
// que tenha sido copiado de outra máquina, editado à mão ou deixado para trás
// por um layout antigo. Aceitá-lo por ser JSON legível faria a tela oferecer
// "usar o comando instalado" para um comando que este app não instalou.
func (i *Installer) installationAt(dir string) (Installation, bool) {
	if dir == "" {
		return Installation{}, false
	}
	installation, err := readInstallation(dir)
	if err != nil {
		return Installation{}, false
	}
	// O registro tem de falar do diretório em que está, que é `<id>/<versão>`
	// (D5). Divergência é registro de outro lugar, e ela desalinharia a
	// comparação que decide se existe versão nova a oferecer (D10).
	if installation.AgentID != filepath.Base(filepath.Dir(dir)) || installation.Version != filepath.Base(dir) {
		logging.Warnf(context.Background(), component,
			"o registro em %s descreve %s@%s e foi ignorado", dir, installation.AgentID, installation.Version)
		return Installation{}, false
	}
	// E tem de executar algo que esta instalação trouxe. A conferência é sobre o
	// conjunto, e não sobre o comando: no npm quem mora aqui dentro é o ponto de
	// entrada, enquanto o `node` é da máquina e fica fora.
	if !runsFromDir(dir, installation) {
		logging.Warnf(context.Background(), component,
			"o registro em %s não aponta para nada instalado ali e foi ignorado", dir)
		return Installation{}, false
	}
	return installation, true
}

// runsFromDir diz se o comando registrado executa algo de dentro do diretório da
// instalação. É a guarda de caminho do D9 aplicada na leitura: sem ela, um
// `installed.json` trocado apontaria o provider para qualquer executável da
// máquina, e o app o subiria achando que subiu o agente que instalou.
func runsFromDir(dir string, installation Installation) bool {
	// O prefixo é resolvido antes da comparação: em macOS `/var` é link para
	// `/private/var`, e comparar um lado resolvido com o outro cru recusaria
	// instalação legítima.
	root := dir
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		root = resolved
	}
	for _, part := range append([]string{installation.Command}, installation.Args...) {
		if !filepath.IsAbs(part) {
			continue
		}
		// O caminho estar textualmente dentro não basta: um link dentro da
		// instalação pode levar para fora dela, e quem escreveu o registro
		// também pode ter posto o link ali. O que se executa é o destino, e é
		// sobre ele que a guarda vale — a mesma conclusão a que a resolução do
		// ponto de entrada já tinha chegado.
		real, err := filepath.EvalSymlinks(part)
		if err != nil || !acp.WithinDir(root, real) {
			continue
		}
		if info, err := os.Stat(real); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// discard apaga o diretório de uma instalação que não terminou.
//
// O contexto de quem pediu pode já estar cancelado — é justamente o caso do
// cancelamento —, então a limpeza não depende dele. Ela é só remoção de arquivo,
// e não vale deixar resíduo por causa de um contexto morto.
func (i *Installer) discard(ctx context.Context, dir string) {
	if dir == "" {
		return
	}
	if err := removeTree(dir); err != nil {
		logging.Warnf(ctx, component, "não foi possível limpar a instalação interrompida em %s: %v", dir, err)
	}
}

// emit entrega um marco a quem escuta, isolado por pânico: quem escuta é código
// de fora, e um defeito dele não pode virar falha de instalação.
func (i *Installer) emit(ctx context.Context, progress Progress) {
	if i.progress == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf(ctx, component, "quem escuta o progresso da instalação quebrou: %v", r)
		}
	}()
	i.progress(progress)
}

// failureProgress monta o marco de desfecho ruim, separando o que a pessoa
// pediu do que deu errado: cancelar é decisão, e anunciá-lo como falha diria que
// algo quebrou quando nada quebrou.
func failureProgress(agent acpregistry.Agent, cancelled bool, err error) Progress {
	if cancelled {
		return Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageCancelled}
	}
	return Progress{
		AgentID: agent.ID,
		Agent:   agent.Name,
		Stage:   StageFailed,
		Step:    StepOf(err),
		Reason:  acp.SanitizeLabel(err.Error()),
	}
}

// prepareDir deixa o diretório da instalação vazio e pronto para receber o
// pacote. Ele pode existir com resíduo de uma tentativa que o app não conseguiu
// limpar — máquina desligada no meio —, e instalar por cima disso misturaria
// duas tentativas.
func prepareDir(dir string) error {
	if dir == "" {
		return errors.New("sem diretório de destino")
	}
	if err := removeTree(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

// removeTree apaga a árvore, repetindo em caso de falha passageira. No Windows,
// o processo do npm que acabou de ser morto pode ainda segurar um arquivo por um
// instante, e antivírus e indexador fazem o mesmo; em POSIX isso não acontece.
func removeTree(dir string) error {
	const attempts = 5
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return err
}

// maxInstalledBytes é o teto do `installed.json`.
//
// O arquivo que o app escreve tem algumas centenas de bytes, e ele é dado
// externo: um adulterado de um gigabyte iria inteiro para a memória só por
// alguém ter aberto a tela que lista o que está instalado. O teto é folgado o
// bastante para nunca alcançar um registro de verdade.
const maxInstalledBytes = 1 << 20

// readInstallation lê o `installed.json` de um diretório de instalação.
func readInstallation(dir string) (Installation, error) {
	data, err := readAtMost(filepath.Join(dir, installedFileName), maxInstalledBytes, "um registro de instalação")
	if err != nil {
		return Installation{}, err
	}
	var installation Installation
	if err := json.Unmarshal(data, &installation); err != nil {
		return Installation{}, err
	}
	if installation.Schema != installationSchema {
		return Installation{}, fmt.Errorf("esquema %d desconhecido em %s", installation.Schema, dir)
	}
	if installation.Command == "" {
		return Installation{}, fmt.Errorf("registro sem comando em %s", dir)
	}
	installation.Dir = dir
	if installation.Args == nil {
		installation.Args = []string{}
	}
	return installation, nil
}

// readAtMost lê o arquivo recusando o que passar do teto. O byte de folga do
// LimitReader é o que permite saber que passou sem ter lido o excesso.
//
// O que é dito pelo chamador, porque a recusa vai para o log de quem precisa
// achar o arquivo: dizer o teto sem dizer de que arquivo se trata deixa a
// mensagem apontando para o lugar errado quando há mais de um.
func readAtMost(path string, limit int64, what string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s passa de %d bytes e não é %s", path, limit, what)
	}
	return data, nil
}

// writeInstallation grava o `installed.json` (D5).
//
// Grava no temporário e renomeia. O registro é o que declara a instalação
// existente, e o app pode cair no meio da escrita: um JSON pela metade faria o
// agente instalado sumir da tela na abertura seguinte, porque registro ilegível
// não conta como instalação. O rename é o que troca um arquivo inteiro por
// outro, e é o que o cache do registro já faz pelo mesmo motivo.
func writeInstallation(dir string, installation Installation) error {
	data, err := json.MarshalIndent(installation, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar o registro da instalação: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".installed-*.tmp")
	if err != nil {
		return fmt.Errorf("erro ao criar o arquivo temporário do registro da instalação: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao gravar o registro da instalação: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao finalizar o registro da instalação: %w", err)
	}
	if err := replaceFile(tmpName, filepath.Join(dir, installedFileName)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao substituir o registro da instalação: %w", err)
	}
	return nil
}

// replaceFile põe o temporário no lugar do arquivo final. A repetição é do
// Windows: um antivírus ou o indexador podem estar com o arquivo aberto no
// instante da troca, e a espera curta resolve — é a mesma disciplina do
// removeTree, e pelo mesmo motivo.
func replaceFile(tmpName, path string) error {
	const attempts = 5
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		if err = os.Rename(tmpName, path); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return err
}

// safePathSegment diz se o texto pode ser um segmento de caminho. É a guarda de
// quem monta `~/.assistente/agents/<id>/<versão>/` a partir de dado externo (D9).
func safePathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." || len(segment) > 64 {
		return false
	}
	if segment != filepath.Base(segment) || segment != filepath.Clean(segment) {
		return false
	}
	// Espaço e caractere de controle não entram: o Windows come o espaço do fim
	// do nome de diretório, e daí o caminho que o app grava no `installed.json`
	// deixa de ser o que existe no disco. E um identificador com quebra de linha
	// no meio é ilegível justamente onde ele mais precisa ser lido — na mensagem
	// que explica por que a instalação não deu certo.
	for _, r := range segment {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return !containsAny(segment, `/\:*?"<>|`) && segment[0] != '.'
}

func containsAny(s, chars string) bool {
	for _, r := range s {
		for _, bad := range chars {
			if r == bad {
				return true
			}
		}
	}
	return false
}

// describePaths junta os lugares consultados para caber numa frase. Sem eles,
// "não encontrei o Node" não é verificável por quem vai instalar o Node.
func describePaths(paths []string) string {
	if len(paths) == 0 {
		return "PATH"
	}
	return "PATH, " + joinLimited(paths, 6)
}

func joinLimited(items []string, limit int) string {
	if len(items) > limit {
		items = items[:limit]
	}
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ", "
		}
		out += item
	}
	return out
}
