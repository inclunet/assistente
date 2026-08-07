package acpinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
	"assistente/internal/configdir"
	"assistente/internal/logging"
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

// Config monta o instalador.
type Config struct {
	// Root é onde as instalações moram. Vazio usa `~/.assistente/agents` (D5).
	Root string

	// Source é o catálogo do registro.
	Source CatalogSource

	// NPM executa o npm. Vazio usa o npm da instalação de Node encontrada na
	// máquina, procurada no momento do uso.
	NPM NPM

	// Runtime procura o Node. Vazio usa a procura de `internal/acp`.
	Runtime func() acp.NodeRuntime

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
	root      string
	source    CatalogSource
	npm       NPM
	runtime   func() acp.NodeRuntime
	handshake Handshake
	progress  func(Progress)
	now       func() time.Time

	mu      sync.Mutex
	running map[string]context.CancelFunc
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
	clock := cfg.Now
	if clock == nil {
		clock = time.Now
	}
	npm := cfg.NPM
	if npm == nil {
		npm = lazyNPM{lookup: lookup}
	}
	handshake := cfg.Handshake
	if handshake == nil {
		handshake = HandshakeUnsupported
	}
	return &Installer{
		root:      root,
		source:    cfg.Source,
		npm:       npm,
		runtime:   lookup,
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
	spec, _, version, err := pinnedSpec(agent)
	if err != nil {
		// Agente do catálogo que esta fase não sabe instalar não é erro da tela:
		// é um item com estado próprio, e o motivo fica em texto.
		return i.unavailablePlan(agent, err), nil
	}

	dir := i.agentVersionDir(agent.ID, version)
	// Uma procura só para o plano inteiro: a linha de comando que ele mostra tem
	// de ser a do Node que ele diz ter encontrado, e não a de uma segunda
	// procura feita alguns microssegundos depois.
	runtime := i.runtime()
	plan := Plan{
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      version,
		Distribution: DistributionNPM,
		Origin:       spec,
		Dir:          dir,
		RunArgs:      slices.Clone(agent.Distribution.NPX.Args),
		Runtime:      runtimeStatus(runtime),
	}
	if installed, ok := i.installationAt(dir); ok {
		plan.Installed = &installed
	}

	switch {
	case i.root == "":
		plan.Reason = "não foi possível descobrir o diretório de dados do app para instalar o agente"
	case !plan.Runtime.Found:
		// Sem Node não se oferece instalação, e o motivo fica em texto (D7). O
		// app não instala Node: instalar o runtime é um link e uma frase.
		plan.Reason = ErrRuntimeMissing.Error()
	case plan.Installed != nil:
		plan.Reason = ErrAlreadyInstalled.Error()
	default:
		// Node encontrado não garante npm: o `npm-cli.js` pode não estar ao lado
		// dele. Sem a linha de comando não há o que mostrar na confirmação, e
		// oferecer o botão levaria a um diálogo que promete executar nada e a uma
		// instalação que falha depois de aceita.
		command := i.npmFor(runtime).Describe(dir, spec)
		if command == "" {
			plan.Reason = ErrNoNPM.Error()
			break
		}
		plan.CanInstall = true
		plan.InstallCommand = command
	}
	return plan, nil
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

// unavailablePlan é o item que o app não sabe instalar, com o motivo dito em
// texto em vez de um botão cinza sem explicação (D7).
//
// A distribuição só é declarada quando o agente de fato publica por npm. Boa
// parte dos itens indisponíveis chega aqui justamente por não publicar, e dizer
// `npm` neles daria um plano que se contradiz: a distribuição afirmando uma
// coisa e o motivo, logo abaixo, dizendo a contrária.
func (i *Installer) unavailablePlan(agent acpregistry.Agent, err error) Plan {
	distribution := ""
	if agent.Distribution.NPX != nil {
		distribution = DistributionNPM
	}
	return Plan{
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      agent.Version,
		Distribution: distribution,
		Runtime:      runtimeStatus(i.runtime()),
		Reason:       acp.SanitizeLabel(err.Error()),
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
func (i *Installer) Install(ctx context.Context, agentID string) (Installation, error) {
	agent, err := i.agent(ctx, agentID)
	if err != nil {
		return Installation{}, err
	}
	spec, name, version, err := pinnedSpec(agent)
	if err != nil {
		return Installation{}, err
	}
	dir := i.agentVersionDir(agent.ID, version)
	if dir == "" {
		return Installation{}, failf(StepPrepare,
			"não foi possível montar o diretório de instalação do agente %s versão %s",
			acp.SanitizeLabel(agentID), acp.SanitizeLabel(version))
	}
	runtime := i.runtime()
	if !runtime.Found {
		return Installation{}, failf(StepRuntime, "%w; procurei em: %s", ErrRuntimeMissing, describePaths(runtime.Searched))
	}
	if _, _, ok := runtime.NPMCommand(); !ok {
		return Installation{}, failf(StepRuntime, "%w (Node em %s)", ErrNoNPM, runtime.Node)
	}
	if existing, ok := i.installationAt(dir); ok {
		return Installation{}, failf(StepPrepare, "%w: %s %s", ErrAlreadyInstalled, agent.Name, existing.Version)
	}

	ctx, done, err := i.begin(ctx, agentID)
	if err != nil {
		return Installation{}, err
	}
	defer done()

	installation, err := i.install(ctx, agent, spec, name, version, dir, runtime)
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

// Installed devolve a instalação do agente, quando há uma.
//
// Mais de uma versão pode morar sob `<id>/`: é isso que permite baixar a nova ao
// lado da que está em uso (D10). A escolhida é a instalada por último, e não a
// primeira que o diretório listar — a listagem é alfabética, e por ela a `10.0.0`
// viria antes da `2.0.0`. Empate de carimbo fica com a primeira, que é estável
// porque `os.ReadDir` devolve ordenado por nome.
func (i *Installer) Installed(agentID string) (Installation, bool) {
	dir, err := i.agentDir(agentID)
	if err != nil {
		return Installation{}, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Installation{}, false
	}
	var newest Installation
	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		installation, ok := i.installationAt(filepath.Join(dir, entry.Name()))
		if !ok {
			continue
		}
		if !found || installation.InstalledAt.After(newest.InstalledAt) {
			newest, found = installation, true
		}
	}
	return newest, found
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

// readInstallation lê o `installed.json` de um diretório de instalação.
func readInstallation(dir string) (Installation, error) {
	data, err := os.ReadFile(filepath.Join(dir, installedFileName))
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

// writeInstallation grava o `installed.json` (D5).
func writeInstallation(dir string, installation Installation) error {
	data, err := json.MarshalIndent(installation, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar o registro da instalação: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, installedFileName), data, 0o644); err != nil {
		return fmt.Errorf("erro ao gravar o registro da instalação: %w", err)
	}
	return nil
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
