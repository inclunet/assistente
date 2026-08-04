// Package acp fala o Agent Client Protocol (ACP) com um agente de código
// externo — Cursor CLI, Claude Code — que roda como subprocesso e conversa por
// JSON-RPC sobre stdio.
//
// O pacote é o transporte do AEP-0084 e para por aí: não conhece conversas,
// perfis nem eventos do app. Quem traduz um turno ACP para o barramento de
// providers é o provider de chat, em cima destas interfaces.
//
// O SDK de terceiros (github.com/coder/acp-go-sdk) fica confinado aqui, atrás
// das interfaces Client e Session. A implementação usa a conexão de baixo nível
// do SDK em vez do cliente pronto por um motivo concreto: o cliente pronto só
// entrega ao app os métodos que ele tipou e as extensões com prefixo "_". As do
// Cursor são "cursor/*" e seriam respondidas automaticamente como "método não
// encontrado", sem o app nunca ver o pedido.
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Erros que o chamador precisa distinguir. O resto vem embrulhado com contexto.
var (
	// ErrNotAuthenticated indica que o agente está instalado mas sem login.
	// É um estado diagnosticável, com instrução própria — não uma falha de
	// conexão (AEP-0084 D12).
	ErrNotAuthenticated = errors.New("agente ACP não autenticado")

	// ErrSessionLost indica que o processo do agente caiu e levou junto a
	// memória da sessão. Quem recebe isso precisa retomar ou recriar a sessão,
	// avisando a pessoa de que o agente perdeu o contexto (AEP-0084 D4).
	ErrSessionLost = errors.New("sessão ACP perdida: o processo do agente caiu")

	// ErrClosed indica uso de um cliente já encerrado.
	ErrClosed = errors.New("cliente ACP encerrado")

	// ErrSessionClosed indica uso de uma sessão encerrada de propósito. É
	// diferente de ErrSessionLost: aqui ninguém perdeu nada, a conversa acabou.
	ErrSessionClosed = errors.New("sessão ACP encerrada")

	// ErrCancelNotConfirmed indica que o cancelamento foi enviado mas o agente
	// não confirmou o fim do turno dentro do prazo. Importa porque um agente de
	// código que não confirmou pode ainda estar editando arquivo (AEP-0084 D10).
	ErrCancelNotConfirmed = errors.New("agente não confirmou o cancelamento do turno")
)

// Config descreve como iniciar e identificar o agente.
type Config struct {
	// Command e Args são o binário e os argumentos do agente em modo ACP.
	// No Windows o Cursor não expõe um .exe: o comando costuma ser o wrapper
	// PowerShell ou o par node.exe + index.js da versão instalada.
	Command string
	Args    []string

	// Env são variáveis extras; o ambiente do processo pai é sempre herdado.
	Env map[string]string

	// WorkDir é o diretório de trabalho do processo do agente.
	WorkDir string

	// ClientName e ClientVersion identificam o app no handshake.
	ClientName    string
	ClientVersion string

	// OnConfigOptions é avisado quando o agente conta o estado das opções de uma
	// sessão — o que ele faz ao trocar de modelo sozinho, inclusive entre turnos
	// (AEP-0084 D6). É um canal próprio, e não o sink do turno, justamente por
	// isso: o sink só existe entre o começo e o fim de um turno, e a pessoa
	// precisa saber com quem está falando também fora dele.
	//
	// Roda na goroutine de entrega do transporte, então precisa retornar rápido
	// e não pode conversar com o agente: enquanto ela não volta, o protocolo
	// fica parado.
	OnConfigOptions func(sessionID string, options []ConfigOption)
}

func (c Config) validate() error {
	if strings.TrimSpace(c.Command) == "" {
		return errors.New("comando do agente ACP não informado")
	}
	return nil
}

// Client é uma conexão com um agente ACP. Um cliente corresponde a um processo
// e multiplexa várias sessões nele (AEP-0084 D3). O processo sobe no primeiro
// uso, seja qual for, e é reaproveitado.
type Client interface {
	// NewSession abre uma sessão nova com o diretório de trabalho informado.
	NewSession(ctx context.Context, cwd string) (Session, error)

	// LoadSession retoma uma sessão que o agente já conhece. Só funciona quando
	// Capabilities().LoadSession é verdadeiro.
	LoadSession(ctx context.Context, sessionID, cwd string) (Session, error)

	// Capabilities devolve o que o agente anunciou no handshake, iniciando o
	// processo se ainda não estiver de pé. Serve também como health check.
	Capabilities(ctx context.Context) (Capabilities, error)

	// CloseSession encerra no agente uma sessão que o app abandonou sem ter o
	// objeto dela em mãos — um registro antigo que não pôde ser retomado, por
	// exemplo. Não sobe processo: sem agente de pé a sessão já morreu com ele.
	// Quando o agente não anuncia o método, não há o que fazer e a sessão vive
	// até o processo acabar.
	CloseSession(ctx context.Context, sessionID string) error

	// Options devolve as opções que o agente oferece — modelos, modos —, lidas
	// de uma sessão de descoberta sem prompt na mesma conexão de todo mundo
	// (AEP-0084 D6). O resultado é guardado por processo, para a tela de
	// configurações não bater no agente a cada render.
	Options(ctx context.Context, cwd string) ([]ConfigOption, error)

	// InvalidateOptions descarta o que foi descoberto, para a próxima consulta
	// perguntar de novo ao agente.
	InvalidateOptions()

	// Call é a saída para métodos que este pacote não tipa: extensões do agente
	// e seletores legados que os SDKs vão deixando de tipar. Sem ela, o app
	// ficaria refém do que o SDK resolveu cobrir (AEP-0084 D2).
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)

	// Close encerra o processo do agente e invalida as sessões.
	Close() error
}

// Session é uma conversa do lado do agente. O histórico vive nela, não no app:
// cada turno envia só a mensagem nova (AEP-0084 D4).
type Session interface {
	// ID é o identificador que o agente atribuiu à sessão, exatamente como ele
	// mandou. É chave de roteamento — volta para o agente em todo pedido e é
	// por ele que as atualizações são encontradas —, então não passa pela
	// limpeza que o identificador de ferramenta recebe: alterá-lo aqui seria
	// falar de uma sessão que o agente não conhece. Quem for exibi-lo ou usá-lo
	// como chave de UI limpa na hora (AEP-0084 D11); em mensagem e log ele sai
	// citado, senão uma quebra de linha dele forja linha de log.
	ID() string

	// Prompt envia um turno e entrega o que o agente produz ao sink, em ordem.
	// Só um turno por sessão corre de cada vez: uma chamada concorrente espera
	// a anterior terminar (AEP-0084 D10).
	//
	// Cancelar o ctx não apenas desiste da espera — manda o agente parar e
	// aguarda a confirmação, porque um turno abandonado continuaria editando
	// arquivos e rodando comandos.
	//
	// Todo erro devolvido é um *PromptError, que diz se o agente chegou a
	// aceitar o pedido. Quem for retentar precisa olhar esse campo.
	Prompt(ctx context.Context, content []Content, sink UpdateSink) (StopReason, error)

	// Close solta a sessão no app e, quando o agente suporta, também nele.
	Close(ctx context.Context) error

	// Cancel interrompe o turno em andamento sem encerrar a sessão.
	Cancel(ctx context.Context) error

	// ConfigOptions devolve as opções conhecidas (modelo, modo) da sessão.
	ConfigOptions() []ConfigOption

	// SetConfigOption troca uma opção e devolve o estado completo resultante:
	// mudar de modelo pode mudar as opções dependentes.
	//
	// O estado devolvido cobre as opções que este pacote sabe representar. Uma
	// troca em identificador fora dessa lista chega ao agente e vale lá, mas
	// não aparece aqui — não há nome nem valores para desenhar um seletor que
	// ninguém modelou. Quem dirige uma opção dessas guarda o valor por conta.
	SetConfigOption(ctx context.Context, id, value string) ([]ConfigOption, error)
}

// Capabilities é o que o agente anunciou no handshake.
type Capabilities struct {
	// AgentName e AgentVersion identificam o agente, quando ele se apresenta.
	AgentName    string
	AgentVersion string

	// LoadSession indica suporte a retomar sessões por ID.
	LoadSession bool

	// CloseSession indica suporte a encerrar sessões pelo protocolo. Sem ele,
	// o app só consegue esquecer a sessão do lado de cá.
	CloseSession bool

	// PromptImage, PromptAudio e PromptEmbeddedContext dizem que tipo de anexo
	// o agente aceita no prompt. Anexo não aceito não pode ser descartado em
	// silêncio (AEP-0084 D4).
	PromptImage           bool
	PromptAudio           bool
	PromptEmbeddedContext bool

	// AuthMethods são os métodos de login que o agente oferece. O app não
	// dispara login sozinho: um fluxo OAuth abrindo navegador sem a pessoa
	// pedir seria pior do que o erro.
	AuthMethods []AuthMethod
}

// AuthMethod é um método de autenticação anunciado pelo agente.
type AuthMethod struct {
	ID          string
	Name        string
	Description string
	Kind        AuthKind
}

// AuthKind diz quem conduz a autenticação, o que muda a instrução dada à
// pessoa: o agente resolvendo sozinho vira "rode o login no terminal",
// enquanto uma variável de ambiente viraria um pedido de credencial.
type AuthKind string

const (
	AuthKindAgent    AuthKind = "agent"
	AuthKindEnvVar   AuthKind = "env_var"
	AuthKindTerminal AuthKind = "terminal"
)

// Content é um bloco da mensagem enviada ao agente.
type Content struct {
	// Text é o conteúdo textual do bloco.
	Text string
	// ImageData é uma imagem em base64 e ImageMIME o tipo dela. Quando
	// preenchidos, o bloco é de imagem e Text é ignorado.
	ImageData string
	ImageMIME string
}

// TextContent cria um bloco de texto.
func TextContent(text string) Content { return Content{Text: text} }

// ImageContent cria um bloco de imagem a partir de dados em base64.
func ImageContent(base64Data, mimeType string) Content {
	return Content{ImageData: base64Data, ImageMIME: mimeType}
}

// StopReason é o motivo pelo qual o agente encerrou o turno.
type StopReason string

const (
	StopEndTurn         StopReason = "end_turn"
	StopMaxTokens       StopReason = "max_tokens"
	StopMaxTurnRequests StopReason = "max_turn_requests"
	StopRefusal         StopReason = "refusal"
	StopCancelled       StopReason = "cancelled"
)

// UpdateKind identifica o que chegou do agente durante o turno.
type UpdateKind string

const (
	// UpdateText é um pedaço da resposta.
	UpdateText UpdateKind = "text"
	// UpdateThought é um pedaço do raciocínio.
	UpdateThought UpdateKind = "thought"
	// UpdateToolStart anuncia uma ferramenta que o agente começou a usar.
	UpdateToolStart UpdateKind = "tool_start"
	// UpdateToolProgress atualiza uma ferramenta já anunciada. Só os campos
	// preenchidos mudaram; os vazios continuam com o valor anterior.
	UpdateToolProgress UpdateKind = "tool_progress"
	// UpdateConfigOptions traz o conjunto completo de opções da sessão, seja
	// porque o app pediu, seja porque o agente trocou de modelo sozinho.
	UpdateConfigOptions UpdateKind = "config_options"
	// UpdateMode informa o modo corrente (agent, plan, ask).
	UpdateMode UpdateKind = "mode"
	// UpdateTitle é o título que o agente gerou para a conversa.
	UpdateTitle UpdateKind = "title"
)

// Update é um evento do turno. Tipos de notificação que este pacote ainda não
// mapeia são descartados no transporte, e não chegam ao sink.
type Update struct {
	Kind UpdateKind

	// Text vale para UpdateText e UpdateThought.
	Text string

	// Tool vale para UpdateToolStart e UpdateToolProgress.
	Tool *ToolCall

	// ConfigOptions vale para UpdateConfigOptions.
	ConfigOptions []ConfigOption

	// Mode vale para UpdateMode e Title para UpdateTitle.
	Mode  string
	Title string
}

// UpdateSink recebe os eventos de um turno, na ordem em que o agente os
// emitiu. É chamado por uma goroutine só, então não precisa de sincronização
// interna, mas precisa retornar rápido: bloquear aqui segura o protocolo.
//
// Ele roda enquanto a sessão está entregando, então não pode chamar Prompt na
// mesma sessão — isso esperaria pela vez de um turno que é justamente o que
// está entregando. Ler a sessão (ID, ConfigOptions) e encerrá-la são seguros.
type UpdateSink func(Update)

// ToolCall descreve uma ferramenta que o agente está usando. São ferramentas
// dele, não do app: informam a UI e o leitor de telas, e nunca viram execução
// do lado de cá (AEP-0084 D7).
type ToolCall struct {
	// ID identifica a chamada dentro da sessão, já normalizado.
	ID string
	// Kind é a categoria (read, edit, execute, search…), enumerável e
	// traduzível — ao contrário do título.
	Kind string
	// Title é legível e vem do agente: pode ser a linha de comando literal.
	// É dado não confiável e precisa ser saneado antes de virar UI ou anúncio.
	Title string
	// Status é pending, in_progress, completed ou failed.
	Status string
}

// ConfigOption é uma opção de configuração da sessão: modelo, modo, nível de
// raciocínio. O agente sempre manda o conjunto completo, porque mudar uma pode
// mudar as outras.
type ConfigOption struct {
	ID           string
	Name         string
	Category     string
	CurrentValue string
	Values       []ConfigValue
}

// ConfigValue é um valor selecionável de uma ConfigOption.
type ConfigValue struct {
	Value string
	Name  string
}

// PermissionRequest é o agente pedindo autorização para agir na máquina.
type PermissionRequest struct {
	SessionID string
	ToolCall  ToolCall
	Options   []PermissionOption
}

// PermissionOption é uma resposta possível ao pedido, com o rótulo que o
// próprio agente mandou.
type PermissionOption struct {
	ID   string
	Name string
	// Kind é allow_once, allow_always, reject_once ou reject_always.
	Kind string
}

// PermissionOutcome é a decisão sobre um pedido de permissão. OptionID vazio
// significa que ninguém escolheu — não havia quem respondesse, o prazo estourou
// ou o turno foi cancelado. Sem decisão, o agente recebe a recusa pontual que
// ele mesmo ofereceu, e só recebe "cancelled" quando o turno acabou ou quando
// recusar pontualmente não estava entre as opções: negar o que ninguém
// autorizou é seguro, e dizer "cancelado" fora de um cancelamento faria o
// agente concluir que a pessoa desistiu do turno inteiro.
type PermissionOutcome struct {
	OptionID string
}

// CustomRequest é um método fora do padrão que o agente mandou ao app: as
// extensões bloqueantes do Cursor e o que mais um agente inventar.
type CustomRequest struct {
	// Method é o nome do método JSON-RPC, como o agente o escreveu.
	Method string

	// SessionID é a conversa do agente a que o pedido pertence, resolvida pelo
	// transporte. Vem do corpo quando o agente o manda; as extensões do Cursor
	// não mandam — cursor/ask_question e cursor/create_plan carregam apenas o
	// toolCallId —, e aí vale o único turno em voo. Vazio quer dizer que não
	// deu para saber de quem é o pedido, e quem trata precisa resolvê-lo sem
	// perguntar a ninguém.
	SessionID string

	// Params é o corpo cru do pedido. É dado não confiável como o resto do que
	// vem do agente (AEP-0084 D11).
	Params json.RawMessage
}

// RequestHandler responde ao que o agente pergunta ao app. Sem ele, todo pedido
// é negado na hora: um turno pendurado é pior do que uma ação negada.
//
// O transporte garante que todo pedido recebe resposta de protocolo, mesmo que
// o handler entre em pânico ou a conexão caia enquanto ele pensa. O que o
// handler não pode fazer é nunca retornar.
type RequestHandler interface {
	// RequestPermission decide se o agente pode executar a ação descrita.
	RequestPermission(ctx context.Context, req PermissionRequest) PermissionOutcome

	// HandleCustom responde métodos fora do padrão, como as extensões
	// bloqueantes do Cursor. Devolver handled=false faz o transporte responder
	// "método não encontrado", que desbloqueia o agente sem fingir suporte.
	HandleCustom(ctx context.Context, req CustomRequest) (result any, handled bool)

	// CustomFallback devolve o desfecho negativo que o método aceita quando
	// ninguém decidiu: o teto de tempo do transporte estourou ou o handler
	// quebrou. É o que o AEP-0084 D9 pede — "skipped" numa pergunta, "rejected"
	// num plano —, porque erro genérico faz o agente concluir que o app falhou
	// em vez de entender que a resposta foi não.
	//
	// Quem monta é quem implementa a extensão, e não o transporte: só essa
	// camada conhece o formato que cada método aceita, e uma resposta de forma
	// errada corre o risco de o agente ler como decisão de verdade. Devolver
	// false deixa o transporte responder erro interno. Precisa responder na
	// hora, sem I/O nem espera — quem já não decidiu é justamente o handler.
	CustomFallback(method string) (result any, ok bool)
}

// denyAll é o handler usado quando nenhum é fornecido.
type denyAll struct{}

func (denyAll) RequestPermission(context.Context, PermissionRequest) PermissionOutcome {
	return PermissionOutcome{}
}

func (denyAll) HandleCustom(context.Context, CustomRequest) (any, bool) {
	return nil, false
}

func (denyAll) CustomFallback(string) (any, bool) {
	return nil, false
}

// normalizeID limpa identificadores vindos do agente antes de virarem chave,
// atributo de DOM ou texto anunciado. Não é hipótese: o Cursor emitiu
// toolCallId com quebra de linha no meio (AEP-0084 D11).
func normalizeID(id string) string {
	return singleLine(id)
}

// singleLine achata um texto para uma linha só: controles viram espaço ou
// somem, e os espaços repetidos colapsam. É o que impede que um valor de fora
// — identificador do agente, comando vindo da configuração — forje uma linha
// de log ou quebre a leitura de uma mensagem de erro.
func singleLine(s string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(strings.Join(strings.Fields(clean), " "))
}

// describeAgent monta o nome do agente para mensagens de erro. O comando e os
// argumentos vêm da configuração do provider, que a pessoa edita e que pode
// chegar importada de outro lugar: passam pela mesma limpeza que o resto do
// que é de fora.
func describeAgent(cfg Config) string {
	if len(cfg.Args) == 0 {
		return singleLine(cfg.Command)
	}
	return singleLine(cfg.Command + " " + strings.Join(cfg.Args, " "))
}
