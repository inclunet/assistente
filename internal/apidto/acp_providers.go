package apidto

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

	// Command é a linha que autentica por este método. Ela nasce do que o
	// agente informou: ou a linha inteira, quando ele publica o programa, ou
	// os argumentos dele completados com o comando configurado, que é como a
	// variante de terminal do protocolo descreve o login. Vazio é o agente que
	// não informou nada, ou informou de um jeito que não se completa com o
	// comando desta máquina.
	//
	// Ela é para ser mostrada e copiada, nunca executada pelo app: o que veio
	// do agente é texto de terceiro (AEP-0084 D11).
	Command string `json:"command,omitempty"`

	// EnvVars são as variáveis de ambiente que este método pede, nomeadas pelo
	// próprio agente (AEP-0086 D12). É o que a tela oferece para preencher no
	// lugar de perguntar qual variável recebe a credencial.
	EnvVars []ACPAuthEnvVar `json:"env_vars,omitempty"`

	// CredentialProvider é o emissor da credencial, quando o agente o nomeia
	// (`openai`, por exemplo). Serve para sugerir a entrada do cofre que
	// combina; a escolha continua sendo de quem configura.
	CredentialProvider string `json:"credential_provider,omitempty"`
}

// ACPAuthEnvVar é uma variável de ambiente pedida por um método de
// autenticação do agente.
type ACPAuthEnvVar struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	// Optional diz que o agente sobe sem ela; Secret, que o valor é segredo —
	// e só o que é segredo tem razão de vir do cofre.
	Optional bool `json:"optional,omitempty"`
	Secret   bool `json:"secret,omitempty"`
}
