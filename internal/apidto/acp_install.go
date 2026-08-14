package apidto

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

// ACPInstallation é um agente que o app instalou (AEP-0086 D5).
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
