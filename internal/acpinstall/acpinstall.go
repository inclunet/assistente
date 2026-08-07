// Package acpinstall instala, pelo catálogo do registro ACP, os agentes
// distribuídos por npm, num diretório do próprio app (AEP-0086 Fase 3).
//
// O desenho segue quatro decisões do AEP, e elas explicam por que o pacote tem
// as peças que tem:
//
//   - D6: `npx` não é instalação. O pacote é instalado uma vez, com
//     `npm install --prefix`, na versão que o registro fixa, e o que fica
//     guardado é o par `node` + ponto de entrada — nenhum turno spawna `npx`.
//   - D5: a instalação mora em `~/.assistente/agents/<id>/<versão>/`, com um
//     `installed.json` que diz o que o app fez, e remover é apagar o diretório.
//   - D8: o comando de spawn é resolvido pelo app, e a instalação só é
//     declarada concluída depois de o comando resolvido responder `initialize`.
//   - D13: progresso e erro são percebidos por leitor de telas — marcos
//     anunciados, erro que nomeia a etapa, cancelamento que não deixa resíduo.
//
// A execução do npm e o handshake entram por interface porque o teste os
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

// DistributionNPM é o tipo de distribuição que esta fase instala. Ele fica no
// `installed.json` e na tela: quem olha uma instalação precisa saber de onde ela
// veio, e a Fase 4 acrescenta a dela ao lado desta.
const DistributionNPM = "npm"

// RuntimeNode é o nome do pré-requisito, como ele aparece em texto (D7). O app
// não instala runtime; ele nomeia o que falta.
const RuntimeNode = "Node.js"

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
	// npm. Instalar binário é a Fase 4, e `uvx` é a Fase 9.
	ErrNotNPM = errors.New("este agente não é distribuído por npm")

	// ErrRuntimeMissing é a falta do Node. Não é defeito do app nem do agente:
	// é pré-requisito ausente, e o app não instala runtime (D7).
	ErrRuntimeMissing = errors.New("o Node.js não foi encontrado nesta máquina")

	// ErrNoNPM é o Node encontrado sem npm ao lado. Acontece em instalação
	// mínima do Node, e a saída é a mesma: dizer o que falta.
	ErrNoNPM = errors.New("o npm não foi encontrado junto do Node.js desta máquina")

	// ErrUnpinnedVersion é o pacote sem versão fixada e sem versão publicada no
	// item. Instalar `latest` faria a instalação não ser reproduzível e tiraria
	// o sentido do aviso de atualização (D6, D10).
	ErrUnpinnedVersion = errors.New("o catálogo não fixa a versão deste pacote")

	// ErrAlreadyInstalled é o agente que já está instalado nesta versão.
	// Reinstalar por cima apagaria uma instalação que funciona para tentar
	// montar outra igual.
	ErrAlreadyInstalled = errors.New("este agente já está instalado")

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

	// Origin é a origem: o nome completo do pacote com a versão. É o que
	// responde "de onde isso vem" no diálogo de confirmação (D3).
	Origin string `json:"origin"`

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

	// Installed é a instalação que já existe, quando existe. É o que faz a tela
	// oferecer remover em vez de instalar de novo.
	Installed *Installation `json:"installed,omitempty"`
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

	// Target é o alvo instalado: nesta fase, o pacote com a versão. É o
	// equivalente npm do alvo de plataforma que a distribuição binária tem, e o
	// que permite dizer exatamente o que foi baixado.
	//
	// Não há digest a guardar aqui: o registro não publica digest para pacote
	// npm, e a integridade do que o npm baixa é conferida pelo próprio npm
	// contra o `integrity` do registro dele. Digest de artefato é assunto da
	// distribuição binária (D4, Fase 4).
	Target string `json:"target"`

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
