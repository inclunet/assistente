// Package acpinstall instala, pelo catálogo do registro ACP, os agentes
// distribuídos por npm, binário ou uvx, num diretório do próprio app
// (AEP-0086 Fases 3, 4 e 9).
//
// O desenho segue quatro decisões do AEP, e elas explicam por que o pacote tem
// as peças que tem:
//
//   - D6: `npx`/`uvx` não são instalação. O pacote é instalado uma vez — com
//     `npm install --prefix` ou `uv venv` + `uv pip install` — na versão que o
//     registro fixa, e o que fica guardado é o comando resolvido — nenhum
//     turno spawna `npx` nem `uvx`.
//   - D5: a instalação mora em `~/.assistente/agents/<id>/<versão>/`, com um
//     `installed.json` que diz o que o app fez, e remover é apagar o diretório.
//   - D8: o comando de spawn é resolvido pelo app, e a instalação só é
//     declarada concluída depois de o comando resolvido responder `initialize`.
//   - D13: progresso e erro são percebidos por leitor de telas — marcos
//     anunciados, erro que nomeia a etapa, cancelamento que não deixa resíduo.
//
// A execução do npm/uv e o handshake entram por interface porque o teste os
// substitui: o CI não baixa pacote da rede nem sobe agente de verdade.
package acpinstall

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
)

// Tipos de distribuição gravados no `installed.json` e mostrados na tela.
const (
	// DistributionNPM é a instalação por pacote npm (Fase 3).
	DistributionNPM = "npm"

	// DistributionBinary é a instalação por artefato binário (Fase 4).
	DistributionBinary = "binary"

	// DistributionUVX é a instalação por pacote do uv (Fase 9).
	DistributionUVX = "uvx"
)

// Nomes de pré-requisito de runtime, como aparecem em texto (D7). O app não
// instala runtime; ele nomeia o que falta.
const (
	RuntimeNode = "Node.js"
	RuntimeUV   = "uv"
)

// Confirmed é o plano que quem pediu a instalação mostrou e teve aceito (D3).
//
// Ele existe porque o consentimento é sobre um artefato concreto, e o que seria
// instalado pode mudar entre mostrar e clicar: o Node aparece quando alguém
// termina de instalá-lo em outra janela, e o catálogo é revalidado a cada meia
// hora — o registro republica versão e digest sozinho. Instalar assim mesmo
// baixaria coisa que ninguém viu.
//
// Campo vazio não é exigência. O zero valor aceita o que o app escolher agora, e
// é como quem programa contra este pacote pede "instale do jeito que der".
type Confirmed struct {
	// Distribution é o caminho de instalação que estava à vista.
	Distribution string

	// Origin é o pacote com a versão, ou a URL do artefato.
	Origin string

	// SHA256 é o digest publicado que a tela mostrou.
	SHA256 string

	// AcceptUnverified é a resposta à pergunta que o artefato sem digest
	// publicado exige (D4).
	//
	// É o único campo cujo zero valor recusa em vez de aceitar, e a exceção é o
	// ponto: os outros descrevem o que estava à vista, e omiti-los quer dizer
	// "não confiro isso". Este descreve uma pergunta feita, e omiti-lo quer
	// dizer que ela não foi feita. Um padrão permissivo aqui seria o
	// interruptor global que o D4 proíbe, escrito na assinatura da função.
	AcceptUnverified bool
}

// check confere o que foi confirmado contra o que será feito agora.
//
// A divergência é dita pelo campo, e não pelos dois valores: a frase vai para a
// tela e para um leitor de telas, e duas URLs de artefato lidas em sequência
// não ajudam ninguém a decidir se confirma de novo.
func (c Confirmed) check(current Confirmed) error {
	switch {
	case c.Distribution != "" && c.Distribution != current.Distribution:
		return failf(StepCatalog, "%w: o modo de instalação deixou de ser %s", ErrPlanChanged, c.Distribution)
	case c.Origin != "" && c.Origin != current.Origin:
		return failf(StepCatalog, "%w: a origem do que seria baixado mudou", ErrPlanChanged)
	case c.SHA256 != "" && c.SHA256 != current.SHA256:
		return failf(StepCatalog, "%w: o digest publicado para este artefato mudou", ErrPlanChanged)
	}
	return nil
}

// Origem do digest guardado no `installed.json` (D4).
const (
	// DigestVerified é o digest que o registro publicou e que bateu com o
	// artefato que chegou.
	DigestVerified = "verified"

	// DigestObserved é o digest calculado do arquivo na falta de um publicado.
	// Ele não atesta procedência; serve para perceber que a mesma versão mudou
	// de conteúdo depois.
	DigestObserved = "observed"
)

// Verified diz se o artefato que está no disco foi conferido contra um digest
// publicado (D4).
//
// Quem dispensa a pergunta é o pacote de gerenciador (`npm` e `uvx`): ali quem
// confere é o próprio npm/uv contra o registro dele, o campo é naturalmente
// vazio, e uma ressalva seria alarme nos agentes de pacote. Todo o resto
// responde pelo digest, e qualquer coisa que não seja "conferido" conta como
// não conferido — inclusive o campo vazio e a distribuição que este app não
// escreveu.
//
// Conferido é o registro que diz qual digest conferiu. Um que se declare
// `verified` com o campo vazio não descreve nenhuma conferência, e a instalação
// binária que o app faz sempre grava os dois.
func Verified(installation Installation) bool {
	switch installation.Distribution {
	case DistributionNPM, DistributionUVX:
		return true
	}
	return installation.SHA256Origin == DigestVerified && installation.SHA256 != ""
}

// Stage é o marco da instalação. São marcos, e não bytes: anunciar percentual
// continuamente atropelaria qualquer outra leitura em curso (D13, AEP-0058).
type Stage string

const (
	// StageStarted é o começo, depois de a confirmação ter sido dada.
	StageStarted Stage = "started"
	// StageInstalling é o npm baixando e escrevendo o pacote.
	StageInstalling Stage = "installing"
	// StageVerifying é o app conferindo que o que ele instalou fala ACP (D8).
	StageVerifying Stage = "verifying"
	// StageDone é a instalação concluída, com handshake bem-sucedido.
	StageDone Stage = "done"
	// StageFailed é a instalação que não deu, com etapa e motivo.
	StageFailed Stage = "failed"
	// StageCancelled é a instalação interrompida por quem a pediu, já limpa.
	StageCancelled Stage = "cancelled"
)

// Step é a etapa em que a instalação estava quando falhou. Rede indisponível,
// runtime ausente, comando não resolvido e agente que não fala ACP são desfechos
// diferentes com ações diferentes, e "falha na instalação" não é acionável
// (D13).
type Step string

const (
	// StepCatalog é a procura do agente no catálogo.
	StepCatalog Step = "catalog"
	// StepRuntime é a procura do Node.
	StepRuntime Step = "runtime"
	// StepPrepare é a preparação do diretório da instalação.
	StepPrepare Step = "prepare"
	// StepInstall é o `npm install`.
	StepInstall Step = "install"
	// StepDownload é o download do artefato binário, com a conferência do
	// digest. Falhar aqui é diferente de falhar ao abrir o arquivo: uma coisa
	// pede olhar a rede, a outra pede olhar o que o registro publicou.
	StepDownload Step = "download"
	// StepExtract é a abertura do artefato no diretório da instalação.
	StepExtract Step = "extract"
	// StepResolve é a resolução do ponto de entrada e do comando.
	StepResolve Step = "resolve"
	// StepVerify é o handshake que prova que o comando resolvido fala ACP.
	StepVerify Step = "verify"
	// StepRecord é a gravação do `installed.json`.
	StepRecord Step = "record"
)

// Progress é um marco a anunciar e a escrever na tela (D13).
type Progress struct {
	// AgentID é o identificador do agente no registro. Todo marco o carrega:
	// duas instalações podem estar em voo, e um progresso sem dono não diz de
	// quem ele fala.
	AgentID string

	// Agent é o nome do agente, já saneado, para a frase da tela.
	Agent string

	// Stage é o marco.
	Stage Stage

	// Step é a etapa que falhou. Só vem em StageFailed.
	Step Step

	// Reason é o motivo em texto, já saneado. Só vem em StageFailed, e ele
	// carrega a mensagem original do npm quando é dela que se trata: quem vai
	// resolver um proxy corporativo ou um Node velho demais precisa do texto
	// que o npm escreveu, e não de uma tradução genérica.
	Reason string
}

// Failure é uma falha que nomeia a etapa (D13).
type Failure struct {
	Step Step
	Err  error
}

func (f *Failure) Error() string {
	if f == nil || f.Err == nil {
		return ""
	}
	return f.Err.Error()
}

func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Err
}

// failf monta uma falha de etapa.
func failf(step Step, format string, args ...any) *Failure {
	return &Failure{Step: step, Err: fmt.Errorf(format, args...)}
}

// StepOf diz em que etapa um erro de instalação aconteceu. Erro que não é deste
// pacote não tem etapa, e quem exibe cai no texto.
func StepOf(err error) Step {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Step
	}
	return ""
}

var (
	// ErrNotInCatalog é o agente que não está no catálogo servido. Ele acontece
	// de verdade: o catálogo é dado externo, e o item que estava na tela pode
	// não estar mais no índice revalidado (D9).
	ErrNotInCatalog = errors.New("este agente não está no catálogo do registro ACP")

	// ErrNotNPM é o agente que existe no catálogo mas não é distribuído por
	// npm. Continua existindo para quem pergunta pelo caminho npm; Plan/Install
	// de `uvx` não caem nele.
	ErrNotNPM = errors.New("este agente não é distribuído por npm")

	// ErrNotUVX é o agente que não é distribuído por uvx.
	ErrNotUVX = errors.New("este agente não é distribuído por uvx")

	// ErrNotBinary é o agente que não publica artefato binário.
	ErrNotBinary = errors.New("este agente não é distribuído como binário")

	// ErrPlanChanged é a instalação pedida por um plano que deixou de valer. O
	// que será instalado depende da máquina e do catálogo, e os dois mudam:
	// quem confirmou baixar um arquivo com determinado digest não consentiu com
	// outra coisa.
	ErrPlanChanged = errors.New("o que seria instalado mudou desde a confirmação; confira de novo antes de baixar")

	// ErrUnverifiedNotAccepted é o artefato sem digest publicado pedido sem a
	// confirmação reforçada que ele exige (D4). Oito dos 17 agentes com binário
	// estão neste conjunto, e o Cursor é um deles: não é caso raro, é metade do
	// catálogo — daí o caminho existir, e daí a pergunta ser obrigatória.
	ErrUnverifiedNotAccepted = errors.New("este agente não publica verificação de integridade, e instalá-lo exige a confirmação de quem vai usá-lo")

	// ErrCommandNotResolved é o artefato aberto de que não saiu um comando que
	// este app consiga executar (D8). O `.cmd` do Cursor no Windows é o caso
	// que o AEP-0084 D15 recusa de propósito.
	ErrCommandNotResolved = errors.New("não foi possível resolver um comando executável no que foi instalado")

	// ErrRuntimeMissing é a falta do Node. Não é defeito do app nem do agente:
	// é pré-requisito ausente, e o app não instala runtime (D7).
	ErrRuntimeMissing = errors.New("o Node.js não foi encontrado nesta máquina")

	// ErrRuntimeMissingUV é a falta do uv — o mesmo tratamento do D7, com o
	// nome do runtime certo (não Node.js).
	ErrRuntimeMissingUV = errors.New("o uv não foi encontrado nesta máquina")

	// ErrNoNPM é o Node encontrado sem npm ao lado. Acontece em instalação
	// mínima do Node, e a saída é a mesma: dizer o que falta.
	ErrNoNPM = errors.New("o npm não foi encontrado junto do Node.js desta máquina")

	// ErrNoUV é a falta do executável do uv quando a instalação precisaria dele.
	ErrNoUV = errors.New("o uv não foi encontrado nesta máquina")

	// ErrUnpinnedVersion é o pacote sem versão fixada e sem versão publicada no
	// item. Instalar `latest` faria a instalação não ser reproduzível e tiraria
	// o sentido do aviso de atualização (D6, D10).
	ErrUnpinnedVersion = errors.New("o catálogo não fixa a versão deste pacote")

	// ErrAlreadyInstalled é o agente que já está instalado. Reinstalar por cima
	// apagaria uma instalação que funciona para tentar montar outra igual, e
	// instalar ao lado sem tirar a anterior é atualizar — que tem caminho
	// próprio, com repontar o provider no meio (D10).
	ErrAlreadyInstalled = errors.New("este agente já está instalado")

	// ErrNoUpdate é a atualização pedida para quem já está na versão do
	// catálogo. Ela acontece de verdade: o índice é revalidado sozinho, e a
	// versão nova pode ter sido instalada por outra janela entre o aviso e o
	// clique.
	ErrNoUpdate = errors.New("este agente já está na versão que o catálogo publica")

	// ErrVerificationWouldDrop é a atualização que trocaria um artefato
	// conferido por um que o app não tem como conferir (D10). Aceitá-la faria
	// do aviso de versão nova um caminho para contornar o D4 — bastaria o
	// agente parar de publicar digest numa versão para a instalação conferida
	// virar não conferida sem ninguém decidir isso.
	ErrVerificationWouldDrop = errors.New(
		"a versão nova deste agente não publica verificação de integridade, e a instalada foi conferida")

	// ErrInstalling é a segunda instalação do mesmo agente ao mesmo tempo. Duas
	// escrevendo no mesmo diretório deixariam meio agente no disco.
	ErrInstalling = errors.New("já há uma instalação deste agente em andamento")

	// ErrNotInstalled é a remoção de um agente que o app não instalou.
	ErrNotInstalled = errors.New("este agente não foi instalado pelo app")
)

// CatalogSource é o mínimo que este pacote precisa do serviço do registro
// (`internal/acpregistry`), que é quem sabe de cache, revalidação e saneamento.
type CatalogSource interface {
	Catalog(ctx context.Context) acpregistry.Catalog
}

// NPM executa o npm. É interface porque o teste substitui a execução: o CI não
// pode depender de baixar pacote do registro npm.
type NPM interface {
	// Install roda `npm install --prefix <prefix> <spec>`. O erro devolvido
	// carrega o que o npm escreveu, porque é dessa mensagem que quem vai
	// resolver o problema precisa.
	Install(ctx context.Context, prefix, spec string) error

	// Describe é a linha de comando que Install vai executar, para o diálogo de
	// confirmação mostrar o que será executado antes de qualquer byte ser
	// baixado (D3).
	Describe(prefix, spec string) string
}

// UV executa o uv. É interface pelo mesmo motivo do NPM: o CI não baixa pacote
// do PyPI.
type UV interface {
	// Install cria o venv em dir e instala a spec pinada nele.
	Install(ctx context.Context, dir, spec string) error

	// Describe é a linha de comando que Install vai executar (D3).
	Describe(dir, spec string) string
}

// Handshake sobe o comando resolvido e confere que ele fala ACP (D8). Devolver
// nil é dizer que o agente respondeu `initialize`.
//
// Falta de login é sucesso: o agente subiu, se apresentou e o comando está
// certo. Autenticar é assunto do próprio agente (AEP-0084 D12), e recusar a
// instalação por isso mandaria a pessoa reinstalar o que já está no lugar.
type Handshake func(ctx context.Context, command string, args []string) error

// RuntimeStatus é o pré-requisito de runtime nesta máquina, dito em texto (D7).
type RuntimeStatus struct {
	// Name é o nome do runtime, para a frase da tela.
	Name string `json:"name"`

	// Required diz se esta instalação depende dele. Artefato binário sobe sem
	// runtime nenhum, e bloquear a instalação por falta de Node ali seria negar
	// o download por um pré-requisito que não existe — justamente para os sete
	// agentes que não têm alternativa npm.
	Required bool `json:"required"`

	// Found diz se ele está aqui.
	Found bool `json:"found"`

	// Path é o executável encontrado.
	Path string `json:"path,omitempty"`

	// Version é a versão quando o caminho a revela. Descobri-la de outro jeito
	// exigiria executar o runtime, e a procura não executa nada (D7).
	Version string `json:"version,omitempty"`

	// Searched são os lugares consultados, para "não encontrado" ser
	// verificável em vez de ser só uma negativa.
	Searched []string `json:"searched,omitempty"`
}

// Plan é o que a tela mostra antes de baixar qualquer byte (D3) e o que ela
// precisa para oferecer, ou não oferecer, a instalação (D7).
type Plan struct {
	// AgentID e Name identificam o agente. Os dois aparecem porque o
	// identificador do registro e o nome não são a mesma coisa, e é pelo
	// identificador que a instalação é pedida.
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`

	// Version é a versão que será instalada — a que o registro fixa (D6).
	Version string `json:"version"`

	// Distribution é o tipo de distribuição.
	Distribution string `json:"distribution"`

	// Origin é a origem: o nome completo do pacote com a versão, ou a URL do
	// artefato. É o que responde "de onde isso vem" no diálogo de confirmação
	// (D3).
	Origin string `json:"origin"`

	// Bytes é o tamanho do download quando o servidor informa (Content-Length
	// ou Content-Range), sem baixar o arquivo. Zero omite o campo na tela
	// (D3: "o tamanho quando o servidor informa").
	Bytes int64 `json:"bytes,omitempty"`

	// Target é o alvo de plataforma que será baixado, quando a distribuição é
	// binária. Ele fica à vista porque o mesmo agente publica arquivos
	// diferentes por plataforma, e qual deles vem é parte de "o que vai ser
	// baixado".
	Target string `json:"target,omitempty"`

	// SHA256 é o digest publicado para o alvo. Vazio quer dizer que o registro
	// não publica um.
	SHA256 string `json:"sha256,omitempty"`

	// Unverified diz que instalar isto é baixar um arquivo que o app não tem
	// como conferir (D4).
	//
	// É campo próprio, e não a ausência do `sha256`, porque quem lê o plano
	// precisa da afirmação e não da dedução: a tela tem de mostrar uma frase
	// que nomeia a ausência, e "o campo veio vazio" é fácil demais de esquecer
	// de perguntar. A instalação exige a resposta a essa pergunta, e ela é
	// pedida a cada vez — não há interruptor que a desligue.
	Unverified bool `json:"unverified,omitempty"`

	// Dir é onde a instalação vai morar (D5). Fica à vista porque o app está
	// escrevendo no disco de alguém.
	Dir string `json:"dir"`

	// InstallCommand é a linha de comando que será executada para instalar. É a
	// parte mais literal do "o que vai ser baixado à vista": não é uma descrição
	// do que o app faria, é o que ele vai rodar.
	InstallCommand string `json:"install_command,omitempty"`

	// RunArgs são os argumentos que o registro manda passar ao agente depois do
	// ponto de entrada, na ordem publicada (D6).
	RunArgs []string `json:"run_args,omitempty"`

	// Runtime é o pré-requisito nesta máquina.
	Runtime RuntimeStatus `json:"runtime"`

	// CanInstall diz se dá para instalar agora.
	CanInstall bool `json:"can_install"`

	// Reason é por que não dá, quando não dá. O botão indisponível vem sempre
	// com o motivo à vista: botão cinza sem explicação é o pior desfecho para
	// quem navega por teclado (D7).
	Reason string `json:"reason,omitempty"`

	// Installed é a instalação que já existe, quando existe — em qualquer
	// versão, e não só na que o catálogo publica agora. É o que faz a tela
	// oferecer remover em vez de instalar de novo.
	//
	// Ela ser de qualquer versão é o que permite dizer que há uma nova: uma
	// instalação que só contasse quando batesse com o catálogo desapareceria da
	// tela no dia em que o registro publicasse a versão seguinte, e o app
	// ofereceria instalar do zero o que ele mesmo pôs ali (D10).
	Installed *Installation `json:"installed,omitempty"`

	// Update diz que a versão instalada não é a que o catálogo publica, e que
	// o que esta linha oferece é atualizar. Nada acontece sozinho: o aviso é
	// texto, e a atualização é pedida (D10).
	Update bool `json:"update,omitempty"`

	// CanUpdate diz se dá para atualizar agora, e UpdateReason é por que não
	// dá, quando não dá. São campos separados dos de instalação porque as
	// respostas divergem: o agente que já está instalado nunca pode ser
	// instalado, e é justamente ele que pode ser atualizado.
	CanUpdate    bool   `json:"can_update,omitempty"`
	UpdateReason string `json:"update_reason,omitempty"`
}

// Updated é o desfecho de uma atualização: a versão que subiu e a que estava no
// lugar (D10).
//
// As duas voltam porque a anterior ainda está no disco quando esta função
// retorna. Entre instalar a nova e apagar a velha existe um passo que este
// pacote não faz — repontar o provider —, e apagar antes dele deixaria o
// provider apontando para um diretório que acabou de sumir.
type Updated struct {
	// Installed é a versão nova, já com handshake conferido (D8).
	Installed Installation

	// Previous é a que estava instalada, ainda no disco.
	Previous Installation
}

// Installation é o que o app instalou, e é o conteúdo do `installed.json` (D5).
// Sem esse registro, o app teria que reconstruir por adivinhação, a cada
// abertura, o que ele mesmo escreveu.
type Installation struct {
	// Schema versiona este arquivo. Registro de esquema desconhecido é tratado
	// como instalação que não é do app: mexer nele às cegas seria pior.
	Schema int `json:"schema"`

	// AgentID é o identificador do agente no registro.
	AgentID string `json:"agent_id"`

	// Name é o nome do agente no catálogo, já saneado.
	Name string `json:"name"`

	// Version é a versão instalada.
	Version string `json:"version"`

	// Distribution é o tipo de distribuição — `npm` nesta fase.
	Distribution string `json:"distribution"`

	// Target é o alvo instalado. O conteúdo varia com a distribuição: no npm é
	// o pacote com a versão, no binário é o alvo de plataforma. Nos dois casos
	// é o que permite dizer exatamente o que foi baixado.
	Target string `json:"target"`

	// SHA256 é o digest do artefato instalado, e SHA256Origin diz o que ele
	// vale: `verified` é o digest que o registro publicou e que bateu com o que
	// chegou; `observed` é o que foi calculado do arquivo na falta de um
	// publicado, e serve para perceber que a mesma versão mudou depois (D4).
	//
	// Nenhum dos dois existe para pacote npm: o registro não publica digest de
	// pacote, e a integridade do que o npm baixa é conferida pelo próprio npm
	// contra o `integrity` do registro dele.
	SHA256       string `json:"sha256,omitempty"`
	SHA256Origin string `json:"sha256_origin,omitempty"`

	// Command e Args são o comando resolvido, no formato que o provider guarda.
	// Eles ficam gravados porque não são recalculados a cada turno (D8).
	Command string   `json:"command"`
	Args    []string `json:"args"`

	// InstalledAt é quando isto foi instalado.
	InstalledAt time.Time `json:"installed_at"`

	// Dir é onde a instalação está. Não é gravado no arquivo — ele é o
	// diretório que contém o arquivo, e guardá-lo faria um caminho velho
	// sobreviver a uma mudança de home.
	Dir string `json:"-"`

	// DiskBytes é o tamanho ocupado no disco pelo diretório da instalação.
	// Calculado na leitura (walk), não gravado no `installed.json` — gravá-lo
	// envelheceria com arquivos que o agente cria depois de instalado.
	DiskBytes int64 `json:"-"`
}

// installationSchema é o esquema corrente do `installed.json`.
const installationSchema = 1

// versionRule é a forma que uma versão pode ter para virar nome de diretório em
// `~/.assistente/agents/<id>/<versão>/` (D5). A régua é apertada porque o valor
// vem do catálogo, que é dado externo: recusar aqui é mais barato do que
// confiar em quem monta o caminho depois.
var versionRule = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]*$`)

// sanitizeVersion aceita a versão como ela é ou recusa por inteiro. Consertar
// uma versão tirando caractere daria outra versão, e a versão é o que diz o que
// está instalado.
func sanitizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > 64 || strings.Contains(version, "..") {
		return ""
	}
	if !versionRule.MatchString(version) {
		return ""
	}
	return version
}

// splitPackageSpec separa o nome do pacote da versão fixada. O `@` do escopo não
// é separador de versão: `@agentclientprotocol/codex-acp@1.1.9` é o pacote
// `@agentclientprotocol/codex-acp` na versão `1.1.9`.
func splitPackageSpec(spec string) (name, version string) {
	spec = strings.TrimSpace(spec)
	if i := strings.LastIndex(spec, "@"); i > 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}

// pinnedSpec devolve o pacote com a versão fixada e a versão isolada.
//
// A versão instalada é a que o registro fixa, e não `latest` (D6). O índice pina
// todas as entradas `npx` de hoje; quando o nome do pacote não trouxer a versão,
// a que vale é a que o item publica — que é o registro fixando a versão do mesmo
// jeito, por outro campo. Sem nenhuma das duas não há instalação reproduzível, e
// o desfecho é recusar em vez de aceitar o que o npm resolver na hora.
func pinnedSpec(agent acpregistry.Agent) (spec, name, version string, err error) {
	if agent.Distribution.NPX == nil {
		return "", "", "", failf(StepCatalog, "%w: %s", ErrNotNPM, agent.ID)
	}
	name, version = splitPackageSpec(agent.Distribution.NPX.Package)
	if name == "" {
		return "", "", "", failf(StepCatalog, "o catálogo não diz qual pacote npm instalar para o agente %s", agent.ID)
	}
	if version == "" {
		version = agent.Version
	}
	version = sanitizeVersion(version)
	if version == "" {
		return "", "", "", failf(StepCatalog, "%w: %s", ErrUnpinnedVersion, name)
	}
	return name + "@" + version, name, version, nil
}

// splitUVPackageSpec separa o nome do pacote da versão fixada pelo `==` do
// PEP 508. Sem `==`, a versão vem do campo do item — o mesmo desenho do npm.
func splitUVPackageSpec(spec string) (name, version string) {
	spec = strings.TrimSpace(spec)
	if name, version, ok := strings.Cut(spec, "=="); ok {
		return strings.TrimSpace(name), strings.TrimSpace(version)
	}
	return spec, ""
}

// pinnedUVSpec devolve o pacote uv com a versão fixada, paralelo a pinnedSpec.
func pinnedUVSpec(agent acpregistry.Agent) (spec, name, version string, err error) {
	if agent.Distribution.UVX == nil {
		return "", "", "", failf(StepCatalog, "%w: %s", ErrNotUVX, agent.ID)
	}
	name, version = splitUVPackageSpec(agent.Distribution.UVX.Package)
	if name == "" {
		return "", "", "", failf(StepCatalog, "o catálogo não diz qual pacote uv instalar para o agente %s", agent.ID)
	}
	if version == "" {
		version = agent.Version
	}
	version = sanitizeVersion(version)
	if version == "" {
		return "", "", "", failf(StepCatalog, "%w: %s", ErrUnpinnedVersion, name)
	}
	return name + "==" + version, name, version, nil
}

// runtimeStatus traduz a procura do Node para o que a tela mostra.
func runtimeStatus(runtime acp.NodeRuntime) RuntimeStatus {
	return RuntimeStatus{
		Name:     RuntimeNode,
		Found:    runtime.Found,
		Path:     runtime.Node,
		Version:  runtime.Version,
		Searched: runtime.Searched,
	}
}

// requiredRuntime é a mesma procura, marcada como pré-requisito. Quem instala
// por npm depende do Node para instalar e para executar; quem baixa artefato
// não depende de nada, e é essa diferença que o campo carrega.
func requiredRuntime(runtime acp.NodeRuntime) RuntimeStatus {
	status := runtimeStatus(runtime)
	status.Required = true
	return status
}

// uvRuntimeStatus traduz a procura do uv para o que a tela mostra.
func uvRuntimeStatus(runtime acp.UVRuntime) RuntimeStatus {
	return RuntimeStatus{
		Name:     RuntimeUV,
		Found:    runtime.Found,
		Path:     runtime.UV,
		Searched: runtime.Searched,
	}
}

// requiredUVRuntime marca o uv como pré-requisito (D7).
func requiredUVRuntime(runtime acp.UVRuntime) RuntimeStatus {
	status := uvRuntimeStatus(runtime)
	status.Required = true
	return status
}
