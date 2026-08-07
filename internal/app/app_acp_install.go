package app

import (
	"context"
	"errors"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/acpregistry"
)

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

	// Origin é a origem: o nome completo do pacote com a versão.
	Origin string `json:"origin"`

	// Dir é onde a instalação vai morar. Fica à vista porque o app está
	// escrevendo no disco de alguém.
	Dir string `json:"dir"`

	// InstallCommand é a linha de comando que será executada para instalar.
	InstallCommand string `json:"install_command,omitempty"`

	// RunArgs são os argumentos que o registro manda passar ao agente depois do
	// ponto de entrada.
	RunArgs []string `json:"run_args,omitempty"`

	// Runtime é o pré-requisito nesta máquina.
	Runtime ACPRuntimeStatus `json:"runtime"`

	// CanInstall diz se dá para instalar agora.
	CanInstall bool `json:"can_install"`

	// Reason é por que não dá, quando não dá. O botão indisponível vem sempre
	// com o motivo à vista.
	Reason string `json:"reason,omitempty"`

	// Installed é a instalação que já existe, quando existe.
	Installed *ACPInstallation `json:"installed,omitempty"`

	// Installing diz que há instalação em voo deste agente, para a tela saber
	// que o botão de cancelar tem o que cancelar.
	Installing bool `json:"installing"`
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
// Preguiçoso de propósito: montá-las no startup faria toda abertura do app
// tocar o disco do cache por causa de uma tela que talvez ninguém abra. O
// instalador, porém, não monta serviço do registro próprio: ele usa o mesmo que
// a tela do catálogo consulta (`a.acpRegistry`, nascido no initACP). Com dois, o
// "atualizar catálogo" da tela traria a versão nova para a lista e deixaria o
// instalador planejando com o índice velho que ele guarda em memória — dois
// números diferentes para o mesmo agente, na mesma tela.
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
func (a *App) acpInstallHandshake(ctx context.Context, command string, args []string) error {
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
		return ACPInstallPlan{}, err
	}
	return a.acpInstallPlan(ctx, agentID)
}

// ACPAgentInstallPlanForKind é o plano do agente que corresponde a um tipo de
// provider do app (`cursor`, `claude-code`).
//
// Existe porque o formulário do provedor sabe o tipo que está configurando, e
// não o identificador do registro — os dois conjuntos de identificadores foram
// escolhidos em momentos diferentes, e o mapeamento entre eles vive num lugar só
// (D11). Tipo sem correspondente no catálogo devolve um plano vazio, e não erro:
// configurar comando e argumentos à mão continua sendo caminho válido.
func (a *App) ACPAgentInstallPlanForKind(kind string) (ACPInstallPlan, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return ACPInstallPlan{}, err
	}
	agentID := acpinstall.RegistryIDForKind(kind)
	if agentID == "" {
		return ACPInstallPlan{}, nil
	}
	return a.acpInstallPlan(ctx, agentID)
}

// acpInstallPlan é o plano em si, igual para as duas portas de entrada.
//
// Agente que não está no catálogo servido não é erro de tela: o catálogo é dado
// externo e pode não ter carregado — a primeira execução offline não tem catálogo
// nenhum (D2). O desfecho é um plano que não oferece instalação, com o motivo em
// texto, que é o que o D7 pede para qualquer indisponibilidade.
func (a *App) acpInstallPlan(ctx context.Context, agentID string) (ACPInstallPlan, error) {
	installer := a.acpCatalogServices().installer
	plan, err := installer.Plan(ctx, agentID)
	if err != nil {
		return ACPInstallPlan{
			AgentID: agentID,
			Runtime: runtimeStatusDTO(acp.FindNodeRuntime()),
			Reason:  acp.SanitizeLabel(err.Error()),
		}, nil
	}
	return installPlanDTO(plan, installer.Installing(agentID)), nil
}

// InstallACPAgent instala o agente do catálogo e só volta com sucesso depois de
// o comando resolvido responder `initialize` (D8).
//
// Instalar é ação pedida (D3): quem chama já mostrou o que vai ser baixado e
// recebeu o consentimento. Este método não pergunta nada — e é por isso que o
// diálogo de confirmação é obrigação da tela, não uma gentileza dela.
func (a *App) InstallACPAgent(agentID string) (ACPInstallation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return ACPInstallation{}, err
	}
	installation, err := a.acpCatalogServices().installer.Install(ctx, agentID)
	if err != nil {
		return ACPInstallation{}, err
	}
	return installationDTO(installation), nil
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

// RemoveACPAgent apaga o diretório do agente instalado (D5).
//
// O que a remoção não faz é apagar o provider: ele é configuração de quem o
// criou, e sumir com ele por causa de um clique em "remover agente" destruiria
// escolha alheia. O provider fica com um comando que não existe, e o health do
// AEP-0084 D12 já sabe dizer isso — não há estado novo a inventar.
func (a *App) RemoveACPAgent(agentID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
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
		Dir:            plan.Dir,
		InstallCommand: plan.InstallCommand,
		RunArgs:        plan.RunArgs,
		Runtime: ACPRuntimeStatus{
			Name:     plan.Runtime.Name,
			Found:    plan.Runtime.Found,
			Path:     plan.Runtime.Path,
			Version:  plan.Runtime.Version,
			Searched: plan.Runtime.Searched,
		},
		CanInstall: plan.CanInstall,
		Reason:     plan.Reason,
		Installing: installing,
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
		Command:      installation.Command,
		Args:         installation.Args,
		Dir:          installation.Dir,
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