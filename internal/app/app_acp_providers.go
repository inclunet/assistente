package app

import (
	"errors"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
)

// ACPAgentSetup é o que a tela de provedores precisa para configurar um agente
// de código: onde ele está nesta máquina e sobre qual diretório ele vai agir.
//
// Os dois lados vêm juntos de propósito. Configurar um agente é decidir o
// comando e saber onde ele mexe em arquivos; separá-los em duas chamadas só
// faria a tela pedir em dois passos o que ela sempre mostra junto.
type ACPAgentSetup struct {
	// Found diz se há agente instalado. Falso não é erro: é o estado que a tela
	// precisa explicar, com o que fazer para resolver (AEP-0084 Fase 3).
	Found bool `json:"found"`

	// Detectable diz se este app sabe procurar o agente no disco. Ele existe
	// porque o catálogo tem 38 agentes e a detecção escrita à mão cobre dois
	// (AEP-0086 D1): sem separar as duas coisas, a tela diria "não encontrado"
	// sobre uma procura que não aconteceu, e mandaria instalar o que talvez já
	// esteja instalado. Falso não é erro nem defeito — é o caso comum.
	Detectable bool `json:"detectable"`

	// Command e Args sobem o agente em modo ACP, já no formato que o provider
	// guarda.
	Command string   `json:"command"`
	Args    []string `json:"args"`

	// Version e Source dizem qual instalação respondeu, quando dá para saber.
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`

	// LoginCommand é o que autenticar este agente exige rodar no terminal,
	// quando o login não é o próprio comando que sobe o ACP. É o caso do Claude
	// Code, cujo ACP vem de um adaptador npm sem login nenhum: quem autentica é
	// o CLI `claude`. Vazio quer dizer que o login sai do comando configurado,
	// com outro subcomando — o caso do Cursor, que a tela já sabe montar.
	LoginCommand string `json:"login_command,omitempty"`

	// Searched são os lugares consultados. Só interessa quando não se achou
	// nada, e é o que transforma "não encontrado" em algo verificável.
	Searched []string `json:"searched,omitempty"`

	// WorkDir é o diretório sobre o qual o agente age (AEP-0084 D5): o
	// workspace ativo, ou o diretório de onde o app foi iniciado quando não há
	// workspace. Fica visível porque é onde o agente vai editar arquivos, e
	// esconder isso seria esconder o alcance do que a pessoa está autorizando.
	WorkDir string `json:"work_dir,omitempty"`
}

// ACPAgentHealth é o resultado de testar um agente de código: se ele sobe, se
// atende e, quando não atende, por quê.
//
// São três estados porque a saída de cada um é diferente (AEP-0084 D12):
// `online` dá para usar, `unauthenticated` pede o login do CLI e `offline` pede
// conferir comando e instalação. Tratar falta de login como erro de conexão
// mandaria a pessoa arrumar o que já está certo.
type ACPAgentHealth struct {
	// State é `online`, `unauthenticated` ou `offline`.
	State string `json:"state"`

	// AgentName e AgentVersion são como o agente se apresentou. Servem de prova
	// de que se falou com o programa esperado.
	AgentName    string `json:"agent_name,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`

	// LoginMethods são os métodos de login anunciados pelo agente, para a tela
	// dizer qual autenticação está em falta. Só vêm preenchidos quando importam.
	LoginMethods []ACPLoginMethod `json:"login_methods,omitempty"`

	// LoginCommand é o comando de autenticação que o próprio agente informou,
	// já montado como linha para copiar. Ele tem precedência sobre tudo o que o
	// app sabe ou deduz sobre este login (AEP-0084 Fase 10): quem escreveu o
	// agente sabe como se autentica nele. Vazio é o agente que não disse nada.
	LoginCommand string `json:"login_command,omitempty"`

	// WorkDir é o diretório com que a sonda abriu a sessão: o mesmo que um turno
	// usaria (AEP-0084 D5).
	WorkDir string `json:"work_dir,omitempty"`

	// LatencyMs é quanto a sondagem levou, do spawn à sessão.
	LatencyMs int64 `json:"latency_ms"`

	// Error é o motivo técnico, já achatado em uma linha. Complementa a
	// instrução da tela; não a substitui.
	Error string `json:"error,omitempty"`
}

// ACPLoginMethod é um método de autenticação anunciado pelo agente.
type ACPLoginMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	// Command é a linha que autentica por este método, quando o agente a
	// informou. Ela é para ser mostrada e copiada, nunca executada pelo app:
	// é texto de terceiro (AEP-0084 D11).
	Command string `json:"command,omitempty"`
}

// TestACPAgent testa um comando de agente antes de ele virar provider: sobe o
// agente, faz o handshake e tenta abrir uma sessão.
//
// O processo desta sondagem é descartável — a configuração pode nem ser salva, e
// deixar um agente de pé por clique em "testar" acumularia processos.
func (a *App) TestACPAgent(command string, args []string) (ACPAgentHealth, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return ACPAgentHealth{}, err
	}
	if a.acpMgr == nil {
		return ACPAgentHealth{}, errors.New("serviço de agentes de código não inicializado")
	}

	report := a.acpMgr.ProbeCandidate(ctx, acp.ProviderSpec{
		// A sondagem não pertence a provider nenhum: o identificador existe só
		// para o log dizer de que agente se está falando.
		ID:      "acp-test",
		Name:    "teste de configuração",
		Command: command,
		Args:    args,
	})

	health := ACPAgentHealth{
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
			health.LoginMethods = append(health.LoginMethods, ACPLoginMethod{
				// Nome e descrição já vêm saneados do relatório. O ID chega
				// intacto de lá porque lá ele é identificador de protocolo, mas
				// daqui para a frente ele é rótulo de última instância — a tela
				// mostra o nome ou, na falta dele, o ID —, e como rótulo ele passa
				// pelo mesmo tratamento.
				ID:          acp.SanitizeLabel(method.ID),
				Name:        method.Name,
				Description: method.Description,
				Command:     method.Login.CommandLine(command, args),
			})
		}
	}
	return health, nil
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
func (a *App) DetectACPAgent(agentID string) (ACPAgentSetup, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return ACPAgentSetup{}, err
	}

	setup := ACPAgentSetup{Args: []string{}}
	// O diretório não depende de procura nenhuma: ele é o workspace ativo do
	// app, e é sobre ele que o agente vai agir seja qual for o caminho pelo qual
	// o comando chegou aos campos. Falha aqui também não invalida a detecção:
	// sem diretório a tela deixa de mostrar um dado, e o resto continua valendo.
	if dir, err := a.acpWorkDir(); err == nil {
		setup.WorkDir = dir
	}

	kind, detectable := acpregistry.DetectableKind(agentID)
	if !detectable {
		return setup, nil
	}

	install, err := acp.DetectAgent(kind)
	if err != nil {
		return ACPAgentSetup{}, err
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
}
