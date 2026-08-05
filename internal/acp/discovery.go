package acp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"assistente/internal/logging"

	sdk "github.com/coder/acp-go-sdk"
)

// Categorias reservadas do ACP para as opções que o app expõe. Ficam aqui, e
// não em quem exibe, porque é este pacote que decide o que sabe representar.
const (
	// CategoryModel é a opção que escolhe o modelo do turno.
	CategoryModel = string(sdk.SessionConfigOptionCategoryModel)
	// CategoryMode é a opção que escolhe o modo do agente (agent, plan, ask).
	CategoryMode = modeCategory
)

// OptionByCategory acha a opção de uma categoria no conjunto que o agente
// mandou. Quem precisa de modelo ou de modo procura por categoria — nunca pelo
// identificador, que é escolha do agente e varia entre implementações.
func OptionByCategory(options []ConfigOption, category string) (ConfigOption, bool) {
	for _, option := range options {
		if option.Category == category {
			return option, true
		}
	}
	return ConfigOption{}, false
}

// ModelValues são os valores selecionáveis da opção de modelo, na ordem em que o
// agente os ofereceu. Vazio quando o agente não expõe escolha de modelo: quem
// lista modelos precisa distinguir "não há escolha" de "falhou".
func ModelValues(options []ConfigOption) []string {
	option, ok := OptionByCategory(options, CategoryModel)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(option.Values))
	for _, value := range option.Values {
		if id := strings.TrimSpace(value.Value); id != "" {
			values = append(values, id)
		}
	}
	return values
}

// Offers diz se um valor está entre os que a opção oferece. Mandar ao agente um
// modelo que ele não tem faria o turno inteiro falhar por causa de uma escolha
// antiga guardada no perfil.
//
// A comparação ignora espaço nas pontas porque é aparado que o valor chega à
// escolha da pessoa: ModelValues entrega a lista sem os espaços, e é o que ela
// escolheu que fica no perfil. Comparar cru diria que o agente não oferece
// justamente o modelo que ele acabou de listar.
func (o ConfigOption) Offers(value string) bool {
	wanted := strings.TrimSpace(value)
	for _, candidate := range o.Values {
		if strings.TrimSpace(candidate.Value) == wanted {
			return true
		}
	}
	return false
}

// IsCurrent diz se a opção já está no valor pedido, pela mesma comparação
// aparada de Offers: um espaço a mais na resposta do agente não é troca de
// modelo, e trataria como troca um pedido que não muda nada.
func (o ConfigOption) IsCurrent(value string) bool {
	return strings.TrimSpace(o.CurrentValue) == strings.TrimSpace(value)
}

// discovery é a sessão de descoberta de um processo e o que ela produziu
// (AEP-0084 D6). A consulta de modelos acontece fora de qualquer conversa — na
// tela de configurações —, mas no ACP as opções pertencem a uma sessão. A saída
// é uma sessão sem prompt, aberta na mesma conexão de todo mundo: o processo do
// agente nunca é efêmero.
//
// É uma só por processo, reaproveitada, porque ela é barata justamente por
// nunca receber um turno. O que não pode acontecer é deixar rastro de sessões
// abandonadas no agente.
type discovery struct {
	// gen conta as invalidações e é atômico de propósito: invalidar acontece na
	// goroutine que entrega as notificações do agente, e ela não pode esperar
	// por um lock que uma consulta em andamento está segurando. O SDK só
	// devolve a resposta de um pedido depois que as notificações que vieram
	// antes dela foram processadas — esperar aqui penduraria as duas pontas.
	gen atomic.Uint64

	// mu é segurada durante o session/new de propósito: duas telas abrindo ao
	// mesmo tempo devem compartilhar uma sessão de descoberta, não abrir duas.
	mu      sync.Mutex
	session *session
	cached  []ConfigOption
	// cachedGen é a geração em que o cache foi lido. Comparar com gen é o que
	// distingue "ninguém invalidou" de "já não vale" sem precisar de um bool
	// que a goroutine de entrega teria de escrever sob trava.
	cachedGen uint64
	// valid separa a lista vazia da falta de leitura: um agente que não oferece
	// opção nenhuma respondeu isso, e reperguntar a cada render seria bater no
	// agente por uma resposta que ele já deu.
	valid bool
}

// Options devolve as opções que o agente oferece, do cache quando ele vale e da
// sessão de descoberta quando não (AEP-0084 D6).
func (c *client) Options(ctx context.Context, cwd string) ([]ConfigOption, error) {
	cn, err := c.ensureConn(ctx)
	if err != nil {
		return nil, err
	}
	dir, err := absoluteDir(cwd)
	if err != nil {
		return nil, err
	}
	return cn.options(ctx, dir)
}

// InvalidateOptions descarta o que foi descoberto. A próxima consulta pergunta
// ao agente de novo: é o que a pessoa pede ao recarregar a lista na tela, e o
// que um config_option_update significa (AEP-0084 D6).
func (c *client) InvalidateOptions() {
	c.mu.Lock()
	cn := c.conn
	c.mu.Unlock()
	if cn == nil {
		return
	}
	cn.invalidateOptions()
}

func (c *conn) invalidateOptions() {
	c.disco.gen.Add(1)
}

// announceOptions conta a quem cuida do app que as opções de uma sessão mudaram.
// O aviso é isolado por pânico pelo mesmo motivo que a entrega ao sink: quem
// escuta é código de fora, e um defeito dele não pode virar falha ao tratar a
// notificação do agente.
func (c *conn) announceOptions(sessionID string, options []ConfigOption) {
	if c.cfg.OnConfigOptions == nil || len(options) == 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf(context.Background(), logComponent,
				"[ACP] quem escuta as opções da sessão %q quebrou: %v", sessionID, r)
		}
	}()
	c.cfg.OnConfigOptions(sessionID, options)
}

// announceCommands conta a quem cuida do app quais comandos uma sessão passou a
// oferecer. Isolado por pânico pelo mesmo motivo do anúncio das opções.
//
// Ao contrário das opções, a lista vazia é anunciada: ela diz que o agente
// deixou de oferecer comandos, e calar isso deixaria na tela uma lista que não
// vale mais.
func (c *conn) announceCommands(sessionID string, commands []Command) {
	if c.cfg.OnCommands == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf(context.Background(), logComponent,
				"[ACP] quem escuta os comandos da sessão %q quebrou: %v", sessionID, r)
		}
	}()
	c.cfg.OnCommands(sessionID, commands)
}

// options lê as opções do agente. O cache é por processo, e por isso não precisa
// de invalidação quando o processo cai: a conexão nova traz um cache vazio, e a
// sessão que produziu a lista anterior morreu com a antiga.
func (c *conn) options(ctx context.Context, dir string) ([]ConfigOption, error) {
	c.disco.mu.Lock()
	defer c.disco.mu.Unlock()

	generation := c.disco.gen.Load()
	if c.disco.valid && c.disco.cachedGen == generation {
		return copyOptions(c.disco.cached), nil
	}

	sess, err := c.discoverySession(ctx, dir)
	if err != nil {
		return nil, err
	}
	options := sess.ConfigOptions()
	// A geração é a do início da leitura: uma invalidação que chegou enquanto o
	// agente respondia fala de um estado mais novo do que este, e guardá-lo como
	// atual esconderia a mudança até a próxima.
	c.disco.cached = options
	c.disco.cachedGen = generation
	c.disco.valid = true
	return copyOptions(options), nil
}

// discoverySession devolve a sessão de descoberta pronta para ser lida. Com o
// cache inválido, o que se quer é a resposta atual do agente, e a única forma de
// obtê-la é uma sessão nova — a anterior só saberia o que ouviu.
//
// A anterior é encerrada pelo método do protocolo antes disso, quando o agente
// anuncia suportá-lo. Quando não anuncia (o caso do Cursor de hoje), abrir outra
// deixaria a antiga pendurada nele: aí a sessão de sempre é reaproveitada, e o
// que ela sabe é a melhor resposta disponível sem sujar o agente.
func (c *conn) discoverySession(ctx context.Context, dir string) (*session, error) {
	if current := c.disco.session; current != nil && !current.isClosed() {
		if !c.caps.CloseSession {
			return current, nil
		}
		if err := current.Close(ctx); err != nil {
			logging.Debugf(ctx, logComponent,
				"[ACP] sessão de descoberta do agente %s não foi encerrada: %v", describeAgent(c.cfg), err)
		}
	}
	c.disco.session = nil

	sess, err := c.openSession(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("descobrir as opções do agente %s: %w", describeAgent(c.cfg), err)
	}
	c.disco.session = sess
	return sess, nil
}
