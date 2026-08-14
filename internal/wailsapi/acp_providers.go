package wailsapi

import (
	"assistente/internal/acp"
	"assistente/internal/acpregistry"
	"assistente/internal/apidto"
	"context"
	"sync"
)

// ACPProviders é o bind Wails do domínio acp_providers — detect/test de agentes
// antes de virarem provider (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type ACPProviders struct {
	mu      sync.RWMutex
	session Session
	mgr     *acp.Manager
	workDir func() (string, error)
}

// NewACPProviders cria o bind vazio; AttachACPProviders preenche deps no startup.
func NewACPProviders() *ACPProviders {
	return &ACPProviders{}
}

// AttachACPProviders associa Session, Manager ACP e workDir após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachACPProviders(api *ACPProviders, session Session, mgr *acp.Manager, workDir func() (string, error)) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.mgr = mgr
	api.workDir = workDir
}

func (api *ACPProviders) deps() (Session, *acp.Manager, func() (string, error), error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.mgr == nil {
		return nil, nil, nil, ErrACPProvidersNotWired
	}
	return api.session, api.mgr, api.workDir, nil
}

// TestACPAgent testa um comando de agente antes de ele virar provider: sobe o
// agente, faz o handshake e tenta abrir uma sessão.
//
// O processo desta sondagem é descartável — a configuração pode nem ser salva, e
// deixar um agente de pé por clique em "testar" acumularia processos.
func (api *ACPProviders) TestACPAgent(command string, args []string) (apidto.ACPAgentHealth, error) {
	session, mgr, _, err := api.deps()
	if err != nil {
		return apidto.ACPAgentHealth{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ACPAgentHealth, error) {
		report := mgr.ProbeCandidate(ctx, acp.ProviderSpec{
			// A sondagem não pertence a provider nenhum: o identificador existe só
			// para o log dizer de que agente se está falando.
			ID:      "acp-test",
			Name:    "teste de configuração",
			Command: command,
			Args:    args,
		})

		health := apidto.ACPAgentHealth{
			State:        string(report.State),
			AgentName:    report.AgentName,
			AgentVersion: report.AgentVersion,
			WorkDir:      report.WorkDir,
			LatencyMs:    report.Latency.Milliseconds(),
			Error:        report.Error,
		}
		// Os métodos de login só interessam quando é o login que falta: em provider
		// saudável eles seriam ruído, e o Cursor anuncia o dele sempre.
		if report.Unauthenticated() {
			// A linha vem montada sobre o comando testado porque a variante de
			// terminal do protocolo descreve o login como argumentos do próprio
			// agente — sem o programa, aquilo seria uma linha solta.
			health.LoginCommand = acp.LoginCommandFrom(report.AuthMethods, command, args)
			for _, method := range report.AuthMethods {
				health.LoginMethods = append(health.LoginMethods, apidto.ACPLoginMethod{
					// Nome e descrição já vêm saneados do relatório. O ID chega
					// intacto de lá porque lá ele é identificador de protocolo, mas
					// daqui para a frente ele é rótulo de última instância — a tela
					// mostra o nome ou, na falta dele, o ID —, e como rótulo ele passa
					// pelo mesmo tratamento.
					ID:          acp.SanitizeLabel(method.ID),
					Name:        method.Name,
					Description: method.Description,
					Command:     method.Login.CommandLine(command, args),
					// As variáveis vêm junto porque é aqui que a tela pode
					// oferecer ligar a passagem de credencial: o estado em que
					// falta login é exatamente aquele em que alguém quer fazê-lo
					// (AEP-0086 D12).
					EnvVars:            authEnvVarsDTO(method.EnvVars),
					CredentialProvider: method.CredentialProvider,
				})
			}
		}
		return health, nil
	})
}

func authEnvVarsDTO(vars []acp.AuthEnvVar) []apidto.ACPAuthEnvVar {
	if len(vars) == 0 {
		return nil
	}
	out := make([]apidto.ACPAuthEnvVar, 0, len(vars))
	for _, v := range vars {
		out = append(out, apidto.ACPAuthEnvVar{
			Name:     v.Name,
			Label:    v.Label,
			Optional: v.Optional,
			Secret:   v.Secret,
		})
	}
	return out
}

// DetectACPAgent procura na máquina o agente de código pedido e devolve, junto,
// o diretório sobre o qual ele vai agir. O agente é nomeado pelo `id` do
// registro ACP (AEP-0086 D11).
//
// Não saber procurar aquele agente é resposta, e não erro: são 38 no catálogo e
// dois com detecção escrita à mão (D1), então a pergunta é feita para todos e a
// maioria das respostas é "o app não olha aqui". Devolvê-la como erro faria a
// tela tratar o caso comum como anomalia.
//
// Exige sessão como o resto da API de provedores: a tela que chama só existe
// depois do login, e um sondador de sistema de arquivos aberto antes disso é
// superfície que não precisa existir.
func (api *ACPProviders) DetectACPAgent(agentID string) (apidto.ACPAgentSetup, error) {
	session, _, workDir, err := api.deps()
	if err != nil {
		return apidto.ACPAgentSetup{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ACPAgentSetup, error) {
		_ = ctx
		setup := apidto.ACPAgentSetup{Args: []string{}}
		// O diretório não depende de procura nenhuma: ele é o workspace ativo do
		// app, e é sobre ele que o agente vai agir seja qual for o caminho pelo
		// qual o comando chegou aos campos. Falha aqui também não invalida a
		// detecção: sem diretório a tela deixa de mostrar um dado, e o resto
		// continua valendo.
		if workDir != nil {
			if dir, err := workDir(); err == nil {
				setup.WorkDir = dir
			}
		}

		kind, detectable := acpregistry.DetectableKind(agentID)
		if !detectable {
			return setup, nil
		}

		install, err := acp.DetectAgent(kind)
		if err != nil {
			return apidto.ACPAgentSetup{}, err
		}

		setup.Detectable = true
		setup.Found = install.Found
		setup.Command = install.Command
		setup.Version = install.Version
		setup.Source = install.Source
		setup.Searched = install.Searched
		setup.LoginCommand = install.LoginCommand
		if install.Args != nil {
			// Lista sempre presente: `null` faria a tela distinguir "sem
			// argumentos" de "campo ausente" antes de preencher o formulário.
			setup.Args = install.Args
		}
		return setup, nil
	})
}
