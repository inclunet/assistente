package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/acpregistry"
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

// ACPInstallProgress é um marco da instalação (D13). Marcos, e não bytes:
// anunciar percentual continuamente atropelaria qualquer outra leitura em curso.
//
// Payload de evento tipado à mão no frontend (não atravessa assinatura Wails).
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
