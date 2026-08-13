package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/acpregistry"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/logging"
	"assistente/internal/providers"
)

// acpInstallComponent nomeia este arquivo no log.
const acpInstallComponent = "app.app-acp-install"

// ACPInstallProgressEvent é o nome do evento de progresso da instalação. Ele
// carrega o identificador do agente porque duas instalações podem estar em voo, e
// um progresso sem dono não diz de quem ele fala.
const ACPInstallProgressEvent = "acp:install:progress"

// ACPRuntimeStatus é o pré-requisito de runtime nesta máquina, para a tela dizer
// em texto o que falta (AEP-0086 D7). O app não instala runtime: instalar o Node
// é um link e uma frase, não uma automação.
type ACPRuntimeStatus struct {
	// Name é o nome do runtime, para a frase da tela.
	Name string `json:"name"`

	// Required diz se esta instalação depende dele. Artefato binário sobe sem
	// runtime nenhum, e a tela só bloqueia por falta de Node quando o caminho
	// escolhido de fato o usa.
	Required bool `json:"required"`

	// Found diz se ele está aqui.
	Found bool `json:"found"`

	// Path é o executável encontrado.
	Path string `json:"path,omitempty"`

	// Version é a versão quando o caminho a revela. Descobri-la de outro jeito
	// exigiria executar o runtime, e a procura não executa nada.
	Version string `json:"version,omitempty"`

	// Searched são os lugares consultados, para "não encontrado" ser
	// verificável em vez de ser só uma negativa.
	Searched []string `json:"searched,omitempty"`
}

// ACPInstallation é um agente que o app instalou (D5).
type ACPInstallation struct {
	AgentID      string   `json:"agent_id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Distribution string   `json:"distribution"`
	Target       string   `json:"target"`
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	Dir          string   `json:"dir"`

	// Env são as variáveis do alvo binário gravadas na instalação (configuração
	// do registro, não segredo). A tela as aplica em ACPEnv ao montar o provedor.
	Env map[string]string `json:"env,omitempty"`

	// SHA256 e SHA256Origin são o digest do artefato instalado e o que ele
	// vale: `verified` foi conferido contra o que o registro publicou;
	// `observed` é só o que chegou, e a tela continua dizendo isso depois de
	// instalado (D4). Pacote npm não tem nenhum dos dois — quem confere ali é o
	// próprio npm.
	SHA256       string `json:"sha256,omitempty"`
	SHA256Origin string `json:"sha256_origin,omitempty"`

	// DiskBytes é o tamanho ocupado no disco pelo diretório da instalação.
	// Zero omite o campo na tela.
	DiskBytes int64 `json:"disk_bytes,omitempty"`

	// InstalledAt é a data da instalação em RFC 3339, para a tela formatá-la no
	// idioma de quem lê.
	InstalledAt string `json:"installed_at"`
}

// ACPInstallPlan é o que a tela mostra antes de baixar qualquer byte (D3) e o
// que ela precisa para oferecer, ou não oferecer, a instalação (D7).
type ACPInstallPlan struct {
	// AgentID e Name identificam o agente. Os dois aparecem porque o
	// identificador do registro e o nome não são a mesma coisa.
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`

	// Version é a versão que será instalada — a que o registro fixa (D6).
	Version string `json:"version"`

	// Distribution é o tipo de distribuição.
	Distribution string `json:"distribution"`

	// Origin é a origem: o nome completo do pacote com a versão, ou a URL do
	// artefato que será baixado.
	Origin string `json:"origin"`

	// Bytes é o tamanho do download quando o servidor informa, sem baixar o
	// arquivo. Zero omite o campo na tela (D3).
	Bytes int64 `json:"bytes,omitempty"`

	// Target é o alvo de plataforma do artefato, quando a distribuição é
	// binária. O mesmo agente publica arquivos diferentes por plataforma, e
	// qual deles vem é parte de "o que vai ser baixado".
	Target string `json:"target,omitempty"`

	// SHA256 é o digest publicado que será conferido contra o download.
	SHA256 string `json:"sha256,omitempty"`

	// Unverified diz que este artefato não publica digest, e que instalar é
	// baixar um arquivo que o app não tem como conferir. A tela nomeia essa
	// ausência em texto e pede uma confirmação própria por causa dele (D4).
	Unverified bool `json:"unverified,omitempty"`

	// Dir é onde a instalação vai morar. Fica à vista porque o app está
	// escrevendo no disco de alguém.
	Dir string `json:"dir"`

	// InstallCommand é a linha de comando que será executada para instalar.
	InstallCommand string `json:"install_command,omitempty"`

	// RunArgs são os argumentos que o registro manda passar ao agente depois do
	// ponto de entrada. Sem `omitempty`: a lista vazia é resposta, e o DTO a
	// preenche justamente para a tela não ter de distinguir "sem argumentos" de
	// "campo ausente".
	RunArgs []string `json:"run_args"`

	// Runtime é o pré-requisito nesta máquina.
	Runtime ACPRuntimeStatus `json:"runtime"`

	// CanInstall diz se dá para instalar agora.
	CanInstall bool `json:"can_install"`

	// Reason é por que não dá, quando não dá. O botão indisponível vem sempre
	// com o motivo à vista.
	Reason string `json:"reason,omitempty"`

	// Installed é a instalação que já existe, quando existe — em qualquer
	// versão, e não só na que o catálogo publica agora.
	Installed *ACPInstallation `json:"installed,omitempty"`

	// Update diz que a versão instalada não é a que o catálogo publica, e que
	// o que esta linha oferece é atualizar (D10). Nada acontece sozinho: o
	// aviso é texto, e a atualização é pedida.
	Update bool `json:"update"`

	// CanUpdate diz se dá para atualizar agora, e UpdateReason é por que não
	// dá, quando não dá. O botão indisponível vem sempre com o motivo à vista.
	CanUpdate    bool   `json:"can_update"`
	UpdateReason string `json:"update_reason,omitempty"`

	// Installing diz que há instalação em voo deste agente, para a tela saber
	// que o botão de cancelar tem o que cancelar.
	Installing bool `json:"installing"`
}

// ACPInstallConfirmation é o plano que a tela mostrou e teve aceito (D3). Ela
// volta com o pedido de instalação para o backend recusar o que mudou desde a
// confirmação, em vez de baixar um artefato que ninguém viu.
type ACPInstallConfirmation struct {
	Distribution string `json:"distribution,omitempty"`
	Origin       string `json:"origin,omitempty"`
	SHA256       string `json:"sha256,omitempty"`

	// AcceptUnverified é a resposta ao diálogo do artefato sem digest (D4).
	// Falso recusa a instalação desse tipo de artefato em vez de deixá-la
	// passar: a pergunta é feita a cada instalação, e não existe preferência
	// que a desligue.
	AcceptUnverified bool `json:"accept_unverified,omitempty"`
}

// ACPInstallProgress é um marco da instalação (D13). Marcos, e não bytes:
// anunciar percentual continuamente atropelaria qualquer outra leitura em curso.
type ACPInstallProgress struct {
	AgentID string `json:"agent_id"`
	Agent   string `json:"agent,omitempty"`

	// Stage é o marco: `started`, `installing`, `verifying`, `done`, `failed` ou
	// `cancelled`.
	Stage string `json:"stage"`

	// Step é a etapa que falhou; só vem em `failed`. Erro que não nomeia a etapa
	// não é acionável.
	Step string `json:"step,omitempty"`

	// Reason é o motivo em texto; só vem em `failed`. Ele carrega a mensagem
	// original do npm quando é dela que se trata.
	Reason string `json:"reason,omitempty"`
}

// acpCatalog reúne as duas peças do catálogo do registro ACP: o serviço que lê e
// cacheia o índice (Fase 1) e o instalador (Fase 3).
//
// Elas moram juntas porque compartilham o serviço do registro: duas instâncias
// dele revalidariam a CDN em dobro e escreveriam no mesmo arquivo de cache. Quem
// precisar do catálogo para exibi-lo usa `registry` daqui em vez de montar outro.
type acpCatalog struct {
	registry  *acpregistry.Service
	installer *acpinstall.Installer
}

// acpCatalogServices monta as peças na primeira chamada.
//
// Preguiçoso porque o instalador só interessa a quem for instalar, e nada no
// startup depende dele. O que não muda de lugar é o serviço do registro: ele
// nasce no initACP porque a tela de provedores o consulta assim que abre, e
// nascer não custa nada — o cache do disco só é lido na primeira consulta.
//
// O instalador, então, não monta serviço próprio: usa o mesmo `a.acpRegistry`. Com
// dois, o "atualizar catálogo" da tela traria a versão nova para a lista e
// deixaria o instalador planejando com o índice velho que ele guarda em
// memória — dois números diferentes para o mesmo agente, na mesma tela.
func (a *App) acpCatalogServices() *acpCatalog {
	a.acpCatalogOnce.Do(func() {
		if a.acpRegistry == nil {
			a.acpRegistry = acpregistry.New(acpregistry.Config{})
		}
		a.acpCatalogSvc = &acpCatalog{
			registry: a.acpRegistry,
			installer: acpinstall.New(acpinstall.Config{
				Source:     a.acpRegistry,
				Handshake:  a.acpInstallHandshake,
				OnProgress: a.emitACPInstallProgress,
			}),
		}
	})
	return a.acpCatalogSvc
}

// acpInstallHandshake confere que o comando resolvido fala ACP (D8).
//
// A instalação só é declarada concluída depois disto: um provider salvo que
// nunca sobe é pior do que uma instalação que falhou, porque o erro aparece
// muito depois, na primeira conversa, longe de quem poderia consertá-lo.
//
// Falta de login é sucesso. O agente subiu, se apresentou e o comando está
// certo; autenticar é assunto do próprio agente (AEP-0084 D12), e recusar a
// instalação por isso mandaria a pessoa reinstalar o que já está no lugar.
func (a *App) acpInstallHandshake(ctx context.Context, command string, args []string, env map[string]string) error {
	if a.acpMgr == nil {
		return errors.New("o serviço de agentes de código não está disponível para conferir a instalação")
	}
	report := a.acpMgr.ProbeCandidate(ctx, acp.ProviderSpec{
		// A conferência não pertence a provider nenhum: o identificador existe
		// só para o log dizer de que sondagem se está falando.
		ID:      "acp-install",
		Name:    "conferência da instalação",
		Command: command,
		Args:    args,
		// Env do alvo binário (VT_ACP_* etc.): sem ele o handshake do vtcode
		// falharia antes de o provedor nascer com o ACPEnv certo.
		Env: env,
	})
	switch report.State {
	case acp.HealthOnline, acp.HealthUnauthenticated:
		return nil
	default:
		if report.Error != "" {
			return errors.New(report.Error)
		}
		return errors.New("o agente instalado não respondeu ao handshake do protocolo")
	}
}

// emitACPInstallProgress manda o marco para a tela.
func (a *App) emitACPInstallProgress(progress acpinstall.Progress) {
	if a.emitter == nil {
		return
	}
	a.emitter.Emit(ACPInstallProgressEvent, ACPInstallProgress{
		AgentID: progress.AgentID,
		Agent:   progress.Agent,
		Stage:   string(progress.Stage),
		Step:    string(progress.Step),
		Reason:  progress.Reason,
	})
}

// ACPAgentInstallPlan diz o que será instalado do agente pedido e se dá para
// instalar agora.
//
// Exige sessão como o resto da API de provedores: a tela que chama só existe
// depois do login.
func (a *App) ACPAgentInstallPlan(agentID string) (ACPInstallPlan, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return emptyInstallPlan(), err
	}
	return a.acpInstallPlan(ctx, agentID)
}

// emptyInstallPlan é o plano que não oferece nada, com as listas que o DTO
// promete sempre presentes. Pelo literal zerado, `run_args` sairia como `null`, e
// a tela teria de distinguir "sem argumentos" de "campo ausente" — que é
// justamente o que o contrato existe para evitar.
func emptyInstallPlan() ACPInstallPlan {
	return ACPInstallPlan{RunArgs: []string{}}
}

// acpInstallPlan é o plano em si.
//
// Agente que não está no catálogo servido não é erro de tela: o catálogo é dado
// externo e pode não ter carregado — a primeira execução offline não tem catálogo
// nenhum (D2). O desfecho é um plano que não oferece instalação, com o motivo em
// texto, que é o que o D7 pede para qualquer indisponibilidade.
func (a *App) acpInstallPlan(ctx context.Context, agentID string) (ACPInstallPlan, error) {
	installer := a.acpCatalogServices().installer
	plan, err := installer.Plan(ctx, agentID)
	if err != nil {
		unavailable := emptyInstallPlan()
		unavailable.AgentID = agentID
		unavailable.Runtime = runtimeStatusDTO(acp.FindNodeRuntime())
		unavailable.Reason = acp.SanitizeLabel(err.Error())
		return unavailable, nil
	}
	return installPlanDTO(plan, installer.Installing(agentID)), nil
}

// InstallACPAgent instala o agente do catálogo e só volta com sucesso depois de
// o comando resolvido responder `initialize` (D8).
//
// Instalar é ação pedida (D3): quem chama já mostrou o que vai ser baixado e
// recebeu o consentimento. Este método não pergunta nada — e é por isso que o
// diálogo de confirmação é obrigação da tela, não uma gentileza dela.
// `confirmed` é o plano que a tela mostrou no diálogo. Ele viaja de volta
// porque o que seria instalado depende da máquina e do catálogo, e os dois
// mudam entre mostrar e confirmar: instalar assim mesmo baixaria coisa que
// ninguém viu. Campos vazios aceitam o que o app escolher agora.
func (a *App) InstallACPAgent(agentID string, confirmed ACPInstallConfirmation) (ACPInstallation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return ACPInstallation{}, err
	}
	installation, err := a.acpCatalogServices().installer.Install(ctx, agentID, acpinstall.Confirmed{
		Distribution:     confirmed.Distribution,
		Origin:           confirmed.Origin,
		SHA256:           confirmed.SHA256,
		AcceptUnverified: confirmed.AcceptUnverified,
	})
	if err != nil {
		return ACPInstallation{}, err
	}
	return installationDTO(installation), nil
}

// UpdateACPAgent troca a instalação deste agente pela versão que o catálogo
// publica (AEP-0086 D10).
//
// A ordem é a que o D10 fixa, e ela existe para uma atualização que falha deixar
// de pé o que funcionava: instalar a versão nova ao lado da que está em uso,
// conferir o handshake, repontar os provedores que apontavam para a antiga e só
// então apagá-la. Nenhum passo pode trocar de lugar — repontar antes de o
// handshake passar deixaria provedores apontando para um agente que não sobe, e
// apagar antes de repontar os deixaria apontando para um diretório que sumiu.
//
// Como a instalação, atualizar é ação pedida (D3): quem chama já mostrou o que
// vai ser baixado e recebeu o consentimento.
func (a *App) UpdateACPAgent(agentID string, confirmed ACPInstallConfirmation) (ACPInstallation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return ACPInstallation{}, err
	}
	installer := a.acpCatalogServices().installer
	// Os provedores são levantados antes, sobre o que ainda está no lugar:
	// depois de a nova subir, é o comando antigo que diz quais deles eram
	// daquela instalação, e ele continua gravado neles — mas o diretório de onde
	// ele vem é o da versão que vai sair.
	//
	// Todas as versões no disco, e não só a corrente: mais de uma é o estado
	// normal depois de uma atualização que não pôde limpar a anterior, e um
	// provedor pode ter ficado apontando para qualquer uma delas. É o mesmo
	// conjunto que a limpeza vai apagar no fim.
	pointing := a.acpProvidersFrom(installer.Installations(agentID))
	if err := a.refuseUpdateDuringTurn(pointing); err != nil {
		return ACPInstallation{}, err
	}

	updated, err := installer.Update(ctx, agentID, acpinstall.Confirmed{
		Distribution:     confirmed.Distribution,
		Origin:           confirmed.Origin,
		SHA256:           confirmed.SHA256,
		AcceptUnverified: confirmed.AcceptUnverified,
	})
	if err != nil {
		return ACPInstallation{}, err
	}

	a.repointACPProviders(ctx, pointing, updated.Installed)
	a.removeSupersededVersions(ctx, agentID, updated.Installed.Version, pointing)
	return installationDTO(updated.Installed), nil
}

// removeSupersededVersions apaga as versões que ninguém mais sobe.
//
// É o último passo da atualização, e ele varre em vez de apagar só a anterior:
// uma atualização passada pode ter deixado a sua para trás, e insistir na
// remoção mais tarde é mais barato do que acumular versões que ninguém executa.
//
// A conversa é conferida de novo aqui, e não só na entrada: baixar e conferir um
// agente leva tempo, e um turno pode ter começado nesse meio. O processo em voo
// é o da versão antiga, e apagar o diretório dele é puxar o programa debaixo de
// uma edição em curso — quando isso acontece, a limpeza fica para a próxima.
//
// Não remover não desfaz a atualização: o provedor já aponta para a versão nova,
// e o que fica para trás é disco ocupado. Devolver erro aqui faria a tela dizer
// que a atualização não deu quando ela deu.
func (a *App) removeSupersededVersions(
	ctx context.Context,
	agentID, keep string,
	usando []*llm.ProviderConfig,
) {
	if err := a.refuseUpdateDuringTurn(usando); err != nil {
		logging.Warnf(ctx, acpInstallComponent,
			"as versões anteriores do agente %s ficaram no disco: %v", agentID, err)
		return
	}
	installer := a.acpCatalogServices().installer
	for _, installation := range installer.Installations(agentID) {
		if installation.Version == keep {
			continue
		}
		if err := installer.RemoveVersion(ctx, agentID, installation.Version); err != nil {
			logging.Warnf(ctx, acpInstallComponent,
				"a versão %s do agente %s ficou no disco depois da atualização: %v",
				installation.Version, agentID, err)
		}
	}
}

// refuseUpdateDuringTurn recusa a atualização enquanto algum destes provedores
// tem turno em voo (D10).
//
// Um turno em voo está com o processo antigo, que edita arquivos: trocar o
// binário debaixo dele é trocar o programa no meio de uma edição. A recusa é
// dita com o motivo, e não enfileirada em silêncio — enfileirar faria a
// atualização acontecer quando ninguém mais estivesse olhando.
func (a *App) refuseUpdateDuringTurn(usando []*llm.ProviderConfig) error {
	if a.acpMgr == nil {
		return nil
	}
	for _, provider := range usando {
		if a.acpMgr.TurnInFlight(provider.ID) {
			return fmt.Errorf(
				"o provedor %q está no meio de uma conversa com este agente; espere o turno terminar para atualizar",
				provider.Name)
		}
	}
	return nil
}

// acpProvidersFrom são os provedores que sobem alguma destas instalações.
//
// A pergunta é pelo diretório, e não pelo `acp_agent_id`: o identificador diz de
// que agente do catálogo aquele provedor é, e um provedor pode muito bem apontar
// para o mesmo agente instalado por fora, à mão. Repontar esse seria reescrever
// escolha alheia — e é a mesma disciplina da detecção automática, que também não
// sobrescreve comando configurado (AEP-0084 Fase 3).
func (a *App) acpProvidersFrom(installations []acpinstall.Installation) []*llm.ProviderConfig {
	if a.llmRegistry == nil || len(installations) == 0 {
		return nil
	}
	dirs := make([]string, 0, len(installations))
	for _, installation := range installations {
		if installation.Dir != "" {
			// O diretório vazio deixaria tudo "dentro" dele, e a atualização
			// sairia repontando a máquina inteira.
			dirs = append(dirs, installation.Dir)
		}
	}
	var out []*llm.ProviderConfig
	for _, provider := range a.llmRegistry.List() {
		if provider == nil || !provider.IsACP() {
			continue
		}
		if runsFromAny(dirs, provider) {
			out = append(out, provider)
		}
	}
	return out
}

// runsFromAny diz se o comando deste provedor sai de dentro de algum destes
// diretórios. O argumento entra na conta junto com o comando: no pacote npm quem
// mora ali dentro é o ponto de entrada, enquanto o `node` é da máquina e fica
// fora.
//
// A pergunta aqui é de identidade — de quem é este provedor —, e não a guarda de
// execução que o registro da instalação aplica ao ser lido: um caminho que já
// não existe continua dizendo de que instalação aquele provedor era, e é
// justamente ele que precisa ser repontado.
func runsFromAny(dirs []string, provider *llm.ProviderConfig) bool {
	for _, part := range append([]string{provider.ACPCommand}, provider.ACPArgs...) {
		if !filepath.IsAbs(part) {
			continue
		}
		part = resolvedPath(part)
		for _, dir := range dirs {
			if acp.WithinDir(resolvedPath(dir), part) {
				return true
			}
		}
	}
	return false
}

// resolvedPath segue os links do caminho quando dá. Em macOS `/var` é link para
// `/private/var`, e comparar um lado resolvido com o outro cru não reconheceria
// a própria instalação. O que não existe mais fica como está.
func resolvedPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// repointACPProviders põe os provedores da instalação antiga na nova.
//
// Um provedor que não aceita a troca não derruba a atualização: a versão nova
// está no disco e responde ao protocolo, e voltar atrás por causa de um
// provedor deixaria os outros pela metade. O que fica é o aviso no log e um
// provedor apontando para um comando que a remoção logo a seguir vai apagar —
// que é o estado que o health do AEP-0084 D12 já sabe explicar.
func (a *App) repointACPProviders(
	ctx context.Context,
	usando []*llm.ProviderConfig,
	installation acpinstall.Installation,
) {
	if a.providerSvc == nil {
		return
	}
	args := slices.Clone(installation.Args)
	// O env do artefato novo substitui o que veio da instalação anterior no
	// ACPEnv. ACPCredentialEnv do cofre não entra aqui — é configuração à parte.
	env := maps.Clone(installation.Env)
	for _, provider := range usando {
		if _, err := a.providerSvc.Update(ctx, provider.ID, providers.UpdateRequest{
			ACPCommand: installation.Command,
			ACPArgs:    &args,
			ACPEnv:     &env,
		}); err != nil {
			logging.Warnf(ctx, acpInstallComponent,
				"o provedor %s continuou apontando para a versão anterior do agente %s: %v",
				provider.ID, installation.AgentID, err)
			continue
		}
		logging.Infof(ctx, acpInstallComponent,
			"o provedor %s passou a subir a versão %s do agente %s",
			provider.ID, installation.Version, installation.AgentID)
	}
}

// CancelACPAgentInstall interrompe a instalação em voo. Cancelar limpa o que foi
// escrito: uma instalação interrompida não deixa meio agente no disco (D13).
//
// Não haver o que cancelar não é erro: a instalação pode ter terminado entre o
// clique e a chamada.
func (a *App) CancelACPAgentInstall(agentID string) error {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return err
	}
	a.acpCatalogServices().installer.Cancel(agentID)
	return nil
}

// CanRemoveACPAgent diz se o agente foi instalado pelo app e ficou sem nenhum
// provedor que o use. A instalação é da máquina, portanto a referência também é
// verificada entre todos os usuários antes de a tela oferecer desinstalação.
func (a *App) CanRemoveACPAgent(agentID string) (bool, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return false, err
	}
	inUse, err := database.HasAnyLLMProviderForACPAgent(ctx, agentID)
	if err != nil || inUse {
		return false, err
	}
	for _, installation := range a.acpCatalogServices().installer.List() {
		if installation.AgentID == agentID {
			return true, nil
		}
	}
	return false, nil
}

// RemoveACPAgent apaga o diretório de um agente sem provedores (D5).
//
// A guarda é repetida aqui, mesmo que a tela tenha chamado CanRemoveACPAgent:
// outro provedor pode ter sido criado entre a pergunta e a confirmação.
func (a *App) RemoveACPAgent(agentID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	inUse, err := database.HasAnyLLMProviderForACPAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if inUse {
		return errors.New("o agente ainda é usado por outro provedor")
	}
	return a.acpCatalogServices().installer.Remove(ctx, agentID)
}

// ListInstalledACPAgents lista o que o app instalou.
func (a *App) ListInstalledACPAgents() ([]ACPInstallation, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil, err
	}
	installations := a.acpCatalogServices().installer.List()
	out := make([]ACPInstallation, 0, len(installations))
	for _, installation := range installations {
		out = append(out, installationDTO(installation))
	}
	return out, nil
}

// installPlanDTO traduz o plano para o que a tela consome.
func installPlanDTO(plan acpinstall.Plan, installing bool) ACPInstallPlan {
	dto := ACPInstallPlan{
		AgentID:        plan.AgentID,
		Name:           plan.Name,
		Version:        plan.Version,
		Distribution:   plan.Distribution,
		Origin:         plan.Origin,
		Bytes:          plan.Bytes,
		Target:         plan.Target,
		SHA256:         plan.SHA256,
		Unverified:     plan.Unverified,
		Dir:            plan.Dir,
		InstallCommand: plan.InstallCommand,
		RunArgs:        plan.RunArgs,
		Runtime: ACPRuntimeStatus{
			Name:     plan.Runtime.Name,
			Required: plan.Runtime.Required,
			Found:    plan.Runtime.Found,
			Path:     plan.Runtime.Path,
			Version:  plan.Runtime.Version,
			Searched: plan.Runtime.Searched,
		},
		CanInstall:   plan.CanInstall,
		Reason:       plan.Reason,
		Update:       plan.Update,
		CanUpdate:    plan.CanUpdate,
		UpdateReason: plan.UpdateReason,
		Installing:   installing,
	}
	if dto.RunArgs == nil {
		// Lista sempre presente: `null` faria a tela distinguir "sem
		// argumentos" de "campo ausente" antes de exibir o que será executado.
		dto.RunArgs = []string{}
	}
	if plan.Installed != nil {
		installed := installationDTO(*plan.Installed)
		dto.Installed = &installed
	}
	return dto
}

// installationDTO traduz a instalação para o que a tela consome.
func installationDTO(installation acpinstall.Installation) ACPInstallation {
	dto := ACPInstallation{
		AgentID:      installation.AgentID,
		Name:         installation.Name,
		Version:      installation.Version,
		Distribution: installation.Distribution,
		Target:       installation.Target,
		SHA256:       installation.SHA256,
		SHA256Origin: installation.SHA256Origin,
		Command:      installation.Command,
		Args:         installation.Args,
		Env:          maps.Clone(installation.Env),
		Dir:          installation.Dir,
		DiskBytes:    installation.DiskBytes,
	}
	if dto.Args == nil {
		dto.Args = []string{}
	}
	if !installation.InstalledAt.IsZero() {
		dto.InstalledAt = installation.InstalledAt.UTC().Format(time.RFC3339)
	}
	return dto
}

// runtimeStatusDTO traduz a procura do runtime para o que a tela mostra.
func runtimeStatusDTO(runtime acp.NodeRuntime) ACPRuntimeStatus {
	return ACPRuntimeStatus{
		Name:     acpinstall.RuntimeNode,
		Found:    runtime.Found,
		Path:     runtime.Node,
		Version:  runtime.Version,
		Searched: runtime.Searched,
	}
}
