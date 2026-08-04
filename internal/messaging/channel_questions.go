package messaging

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"assistente/internal/channels"
	"assistente/internal/logging"
	"assistente/internal/questionnaire"
)

const channelQuestionComponent = "messaging.channel-questions"

// ChannelQuestions leva ao contato de um canal a pergunta que o backend faria
// na tela e espera a resposta pelo mesmo caminho por onde as mensagens dele
// entram (AEP-0084 D9, Fase 5).
//
// Não há fluxo novo de mensagem em nenhuma das duas pontas (AEP-0040): a
// pergunta sai pelo mensageiro registrado no gateway, como o código de
// pareamento já sai, e a resposta é lida em handleIncoming, o único ponto por
// onde mensagem de canal entra. O que existe aqui é a decisão de que aquela
// mensagem responde a uma pergunta pendente em vez de começar um turno novo.
type ChannelQuestions struct {
	mu sync.Mutex
	// pending guarda no máximo uma pergunta por conversa: a resposta é um
	// número, e duas perguntas abertas na mesma conversa fariam o "1" valer
	// para as duas.
	pending map[string]*channelQuestion
	// send é o envio pelo mensageiro do canal, emprestado do gateway.
	send func(ctx context.Context, channel, chatID, text string) error
	// timeout é o prazo da pergunta. Vazio usa questionnaire.ChannelTimeout;
	// campo, e não constante direta, para que o teste não espere minutos.
	timeout time.Duration
}

var _ questionnaire.ChannelAsker = (*ChannelQuestions)(nil)

// Erros que a pergunta em canal devolve a quem a fez. Nenhum deles é "o agente
// ficou sem resposta": quem chamou traduz cada um no desfecho negativo que o
// método dele aceita.
// Os três primeiros embrulham questionnaire.ErrAskerUnavailable porque são a
// mesma coisa para quem perguntou: a pergunta não chegou a aparecer, e o
// desfecho negativo sai agora. Só o prazo estourado é diferente — ali a
// pergunta apareceu e ninguém decidiu —, e é essa diferença que muda o aviso
// que a conversa recebe.
var (
	// ErrChannelQuestionUnsupported é o diálogo que não cabe numa mensagem:
	// mais de uma decisão, nenhuma decisão, ou uma que exige texto livre. Numa
	// tela isso se resolve com campos; aqui a resposta é um número.
	ErrChannelQuestionUnsupported = fmt.Errorf(
		"este diálogo não cabe numa mensagem de canal: %w", questionnaire.ErrAskerUnavailable)
	// ErrChannelQuestionBusy é a conversa que já tem uma pergunta esperando
	// resposta.
	ErrChannelQuestionBusy = fmt.Errorf(
		"já há uma pergunta pendente nesta conversa: %w", questionnaire.ErrAskerUnavailable)
	// ErrChannelQuestionUndeliverable é a pergunta que não chegou ao canal.
	ErrChannelQuestionUndeliverable = fmt.Errorf(
		"a pergunta não pôde ser enviada ao canal: %w", questionnaire.ErrAskerUnavailable)
	// ErrChannelQuestionExpired é o prazo curto estourado sem resposta.
	ErrChannelQuestionExpired = errors.New("prazo da pergunta no canal esgotado")
)

// AnswerResult diz o que o gateway deve fazer com a mensagem que acabou de
// chegar.
type AnswerResult int

const (
	// AnswerNotPending é a mensagem comum: não havia pergunta esperando, e ela
	// segue o fluxo normal de turno.
	AnswerNotPending AnswerResult = iota
	// AnswerDelivered é a resposta entregue a quem perguntou.
	AnswerDelivered
	// AnswerIgnored é a mensagem que chegou com uma pergunta pendente e não a
	// respondeu: veio de quem não é dono da conversa, ou não nomeou nenhuma
	// opção. Ela não vira turno — mandá-la ao modelo cancelaria (barge-in) o
	// turno que está justamente esperando a decisão.
	AnswerIgnored
)

// newChannelQuestions monta o mecanismo sobre o envio do gateway.
func newChannelQuestions(send func(ctx context.Context, channel, chatID, text string) error) *ChannelQuestions {
	return &ChannelQuestions{pending: map[string]*channelQuestion{}, send: send}
}

// channelQuestion é uma pergunta aberta numa conversa de canal.
type channelQuestion struct {
	conversationID string
	channel        string
	// contactID é o dono da conversa: só a resposta dele vale (AEP-0084 D9).
	contactID string
	chatID    string
	form      channelForm
	answer    chan questionnaire.Response
}

// AskOnChannel manda a pergunta para a conversa do canal e espera a resposta
// até o prazo curto. Qualquer desfecho volta como resposta ou erro — nunca
// como espera indefinida, porque do outro lado há um turno parado.
func (q *ChannelQuestions) AskOnChannel(ctx context.Context, surface questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	if q == nil || q.send == nil {
		return questionnaire.Response{}, questionnaire.ErrAskerUnavailable
	}
	if surface.Kind != questionnaire.SurfaceChannel {
		return questionnaire.Response{}, questionnaire.ErrNoInterlocutor
	}

	timeout := payload.Timeout
	if timeout <= 0 {
		timeout = q.deadline()
	}
	form, err := channelFormOf(payload, timeout)
	if err != nil {
		return questionnaire.Response{}, err
	}

	question := &channelQuestion{
		conversationID: surface.ConversationID,
		channel:        surface.Channel,
		contactID:      surface.ContactID,
		// O destino de saída pode não ser o contato: no Slack a conversa é com
		// a pessoa e a mensagem vai para o canal dela. Quem responde continua
		// sendo só o contato.
		chatID: channels.GetReplyChatID(surface.Channel, surface.ContactID),
		form:   form,
		answer: make(chan questionnaire.Response, 1),
	}
	if !q.register(question) {
		return questionnaire.Response{}, ErrChannelQuestionBusy
	}
	defer q.forget(question)

	if err := q.send(ctx, question.channel, question.chatID, form.text); err != nil {
		// Sem a mensagem no canal não há pergunta: ninguém tem o que
		// responder, e esperar o prazo só atrasaria o desfecho negativo.
		return questionnaire.Response{}, fmt.Errorf("%w: %v", ErrChannelQuestionUndeliverable, err)
	}
	logging.Infof(ctx, channelQuestionComponent,
		"[Canal] pergunta enviada ao canal %q (conversa %s), prazo de %s", question.channel, question.conversationID, timeout)

	clock := time.NewTimer(timeout)
	defer clock.Stop()
	select {
	case resp := <-question.answer:
		return resp, nil
	case <-clock.C:
		// A resposta pode ter entrado no instante em que o prazo virou. Ela vale:
		// quem decidiu dentro do prazo não pode receber "acabou o tempo" por
		// causa de um empate de relógio.
		if resp, answered := q.expire(question); answered {
			return resp, nil
		}
		// Prazo curto estourado: a pessoa precisa saber que a pergunta não vale
		// mais, senão responde para o vazio e fica esperando o efeito de uma
		// decisão que já foi tomada sem ela.
		q.tell(ctx, question, channelExpiredNotice)
		return questionnaire.Response{}, ErrChannelQuestionExpired
	case <-ctx.Done():
		// Turno cancelado, conversa encerrada ou app saindo. Nada expirou, e
		// avisar de prazo aqui contaria uma coisa pela outra.
		return questionnaire.Response{}, ctx.Err()
	}
}

// TryAnswer decide o que uma mensagem recém-chegada é: a resposta de uma
// pergunta pendente, uma mensagem que não a responde, ou um turno comum.
//
// A recusa da resposta de terceiro é decisão daqui, e não efeito colateral de
// não encontrar a pergunta: a pergunta guarda de quem ela é, e quem não for
// aquele contato não decide por ele (AEP-0084 D9).
func (q *ChannelQuestions) TryAnswer(ctx context.Context, conversationID, contactID, text string) AnswerResult {
	if q == nil {
		return AnswerNotPending
	}
	q.mu.Lock()
	question, ok := q.pending[strings.TrimSpace(conversationID)]
	if !ok {
		q.mu.Unlock()
		return AnswerNotPending
	}
	if question.contactID != strings.TrimSpace(contactID) {
		q.mu.Unlock()
		// Nada é respondido a quem não é dono: dizer que há uma autorização
		// pendente já contaria a um terceiro o que o agente está tentando
		// fazer na máquina de outra pessoa.
		logging.Warnf(ctx, channelQuestionComponent,
			"[Canal] resposta ignorada: quem escreveu não é o dono da conversa %s no canal %q",
			question.conversationID, question.channel)
		return AnswerIgnored
	}
	value, chosen := question.form.choose(text)
	if !chosen {
		q.mu.Unlock()
		q.tell(ctx, question, question.form.retry)
		return AnswerIgnored
	}
	// Sai da lista e entrega sob a mesma tomada da trava: a pergunta foi
	// respondida, uma segunda mensagem não pode reabri-la, e quem estiver
	// desistindo por prazo estourado encontra a decisão já no canal em vez de
	// concluir que ninguém respondeu. O canal tem espaço para uma resposta e só
	// uma pode chegar aqui, então o envio não bloqueia.
	delete(q.pending, question.conversationID)
	question.answer <- questionnaire.Response{Answers: map[string]any{question.form.questionID: value}}
	q.mu.Unlock()

	logging.Infof(ctx, channelQuestionComponent,
		"[Canal] pergunta da conversa %s respondida pelo dono do canal %q", question.conversationID, question.channel)
	return AnswerDelivered
}

// PendingCount é quantas perguntas esperam resposta agora.
func (q *ChannelQuestions) PendingCount() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *ChannelQuestions) deadline() time.Duration {
	if q.timeout > 0 {
		return q.timeout
	}
	return questionnaire.ChannelTimeout
}

func (q *ChannelQuestions) register(question *channelQuestion) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending == nil {
		q.pending = map[string]*channelQuestion{}
	}
	if _, busy := q.pending[question.conversationID]; busy {
		return false
	}
	q.pending[question.conversationID] = question
	return true
}

// expire fecha a pergunta por prazo e confere, na mesma tomada da trava, se uma
// resposta não entrou junto com o estouro. Sem essa conferência a decisão de
// quem respondeu no último segundo se perderia, e a pessoa receberia o aviso de
// prazo depois de ter decidido.
func (q *ChannelQuestions) expire(question *channelQuestion) (questionnaire.Response, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if current, ok := q.pending[question.conversationID]; ok && current == question {
		delete(q.pending, question.conversationID)
	}
	select {
	case resp := <-question.answer:
		return resp, true
	default:
		return questionnaire.Response{}, false
	}
}

// forget tira a pergunta da lista se a que estiver lá ainda for esta. A
// conferência importa porque a pergunta respondida já saiu, e a conversa pode
// ter aberto outra desde então.
func (q *ChannelQuestions) forget(question *channelQuestion) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if current, ok := q.pending[question.conversationID]; ok && current == question {
		delete(q.pending, question.conversationID)
	}
}

// tell manda um recado curto ao canal. Falhar aqui não muda o desfecho da
// pergunta: ela já foi decidida (ou expirou), e o que se perde é a explicação.
func (q *ChannelQuestions) tell(ctx context.Context, question *channelQuestion, text string) {
	if q.send == nil || text == "" {
		return
	}
	// Contexto próprio: o aviso de prazo estourado costuma sair justamente
	// quando o contexto de quem perguntou está morrendo, e sem isso a pessoa
	// nunca saberia por que a ação foi negada.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelNoticeTimeout)
	defer cancel()
	if err := q.send(sendCtx, question.channel, question.chatID, text); err != nil {
		logging.Warnf(ctx, channelQuestionComponent,
			"[Canal] aviso sobre a pergunta da conversa %s não pôde ser enviado: %v", question.conversationID, err)
	}
}

// channelNoticeTimeout limita o envio de um recado sobre a pergunta. Ele é
// curto porque ninguém está esperando por ele: a decisão já saiu.
const channelNoticeTimeout = 10 * time.Second

// channelExpiredNotice conta que o prazo acabou e o que aconteceu por causa
// disso. Diz o desfecho, e não só o prazo: "expirou" sem "foi negado" deixaria
// a pessoa esperando que a ação ainda pudesse acontecer.
const channelExpiredNotice = "O prazo para responder terminou e o pedido foi negado. " +
	"Se ainda quiser autorizar, peça de novo ao assistente."

// channelForm é o diálogo pronto para virar mensagem: o texto que a pessoa lê,
// o recado de quando ela responde outra coisa, e o mapa de volta do número
// escolhido para o valor que o diálogo esperava.
type channelForm struct {
	text  string
	retry string
	// questionID é o item do diálogo que a resposta preenche — o mesmo que
	// quem perguntou lê em Response.Answers.
	questionID string
	// values é o que cada número devolve: o texto estável da opção
	// (Text.String(), o mesmo valor que a tela devolve) ou o booleano da
	// pergunta de sim/não.
	values []any
}

// choose lê a resposta da pessoa. Só o número da opção conta: aceitar o rótulo
// escrito à mão faria uma resposta parecida com duas opções decidir por
// aproximação, e aqui a diferença entre duas opções é autorizar ou negar.
func (f channelForm) choose(reply string) (any, bool) {
	digits := strings.TrimRight(strings.TrimSpace(reply), ".)-")
	digits = strings.TrimSpace(digits)
	if digits == "" {
		return nil, false
	}
	number, err := strconv.Atoi(digits)
	if err != nil || number < 1 || number > len(f.values) {
		return nil, false
	}
	return f.values[number-1], true
}

// Tetos do texto que sai para o canal. A mensagem vai para um app de terceiro,
// onde ela fica gravada no histórico de conversa; e as plataformas têm limite
// de tamanho — o do Telegram é o mais apertado (4096 caracteres). O que é
// cortado é sempre o bloco de conteúdo, nunca a lista de opções: sem ela a
// pessoa não tem como responder.
const (
	channelMessageBudget = 3000
	channelContentBudget = 1200
	channelContentFloor  = 200
)

// channelTruncatedMark diz que o texto não veio inteiro e onde ele está
// completo, para que ninguém autorize com base num pedaço achando que era tudo.
const channelTruncatedMark = "\n[…] texto cortado; o pedido completo está no app."

// channelFormOf traduz o diálogo para uma mensagem de canal. Devolve
// ErrChannelQuestionUnsupported quando a decisão não cabe num número: sem
// pergunta respondível, com mais de uma, ou com um campo obrigatório de texto
// livre. Fingir que caberia faria a pessoa responder uma coisa e o app
// entender outra.
func channelFormOf(payload questionnaire.RequestPayload, timeout time.Duration) (channelForm, error) {
	answerable, err := answerableQuestion(payload.Questions)
	if err != nil {
		return channelForm{}, err
	}
	labels, values := channelChoices(answerable)
	if len(values) == 0 {
		return channelForm{}, ErrChannelQuestionUnsupported
	}

	form := channelForm{questionID: answerable.ID, values: values}
	form.text = renderChannelQuestion(payload, answerable, labels, timeout)
	form.retry = fmt.Sprintf(
		"Não entendi a resposta. Responda com o número da opção, de 1 a %d.", len(values))
	return form, nil
}

// answerableQuestion acha o item que a pessoa decide. Os demais só aparecem:
// bloco de conteúdo é o que ela lê para decidir, e campo opcional de texto
// livre não tem como ser preenchido por mensagem — deixá-lo de fora é melhor
// do que recusar o diálogo inteiro por causa dele.
func answerableQuestion(questions []questionnaire.Question) (questionnaire.Question, error) {
	var found questionnaire.Question
	answered := 0
	for _, question := range questions {
		switch question.Type {
		case "single_choice", "boolean":
			answered++
			found = question
		default:
			if question.Required {
				// Obrigatório e não decidível por número: a resposta que a
				// pessoa mandasse não preencheria este campo, e quem perguntou
				// receberia um diálogo respondido pela metade.
				return questionnaire.Question{}, ErrChannelQuestionUnsupported
			}
		}
	}
	if answered != 1 {
		// Nenhuma decisão não é pergunta; mais de uma não cabe num número só.
		return questionnaire.Question{}, ErrChannelQuestionUnsupported
	}
	return found, nil
}

// channelChoices monta as opções numeradas e o que cada número devolve.
//
// O valor é o mesmo que a tela devolveria: o texto estável da opção, que é o
// fallback do Text (AEP-0085 D5), ou o booleano do sim/não. É isso que faz o
// código de quem perguntou não precisar saber por onde a resposta veio.
func channelChoices(question questionnaire.Question) ([]string, []any) {
	if question.Type == "boolean" {
		return []string{channelYesLabel, channelNoLabel}, []any{true, false}
	}
	labels := make([]string, 0, len(question.Options))
	values := make([]any, 0, len(question.Options))
	for _, option := range question.Options {
		label := sanitizeChannelText(option.String())
		if label == "" {
			continue
		}
		labels = append(labels, label)
		// O valor volta como o backend o mandou, sem o saneamento da exibição:
		// é por ele que quem perguntou reencontra a escolha.
		values = append(values, option.String())
	}
	return labels, values
}

// Rótulos do sim/não numa superfície sem camada de tradução. O texto pronto é
// o que vale aqui (AEP-0085 Fase 5): o frontend nunca vê esta mensagem, e não
// há idioma de interface do outro lado de um mensageiro. No dia em que houver
// idioma por contato, é esta camada que passa a traduzir.
const (
	channelYesLabel = "Sim"
	channelNoLabel  = "Não"
)

// renderChannelQuestion escreve a mensagem que a pessoa lê. A ordem é a do
// diálogo na tela: o que está em jogo, o que foi pedido, as opções e o prazo.
func renderChannelQuestion(payload questionnaire.RequestPayload, answerable questionnaire.Question, labels []string, timeout time.Duration) string {
	head := channelSections(payload, answerable)
	tail := channelDecision(answerable, labels, timeout)

	// O bloco de conteúdo é o único de tamanho imprevisível; o orçamento que
	// resta depois do texto fixo é dele. A conta é em runas, a mesma unidade do
	// corte e a que as plataformas usam para limitar mensagem.
	fixed := utf8.RuneCountInString(tail)
	blocks := 0
	for _, section := range head {
		fixed += utf8.RuneCountInString(section.text)
		if section.content != "" {
			blocks++
		}
	}
	budget := channelContentBudget
	if blocks > 0 {
		if room := (channelMessageBudget - fixed) / blocks; room < budget {
			budget = max(room, channelContentFloor)
		}
	}

	var b strings.Builder
	for _, section := range head {
		b.WriteString(section.text)
		if section.content != "" {
			b.WriteString(truncateChannelText(section.content, budget))
		}
	}
	b.WriteString(tail)
	return b.String()
}

// channelSection é um trecho da mensagem. O conteúdo vem separado porque é o
// único que pode ser cortado.
type channelSection struct {
	text    string
	content string
}

// channelSections é o cabeçalho da mensagem: título, descrição e os blocos que
// a pessoa lê antes de decidir.
func channelSections(payload questionnaire.RequestPayload, answerable questionnaire.Question) []channelSection {
	sections := make([]channelSection, 0, len(payload.Questions)+2)
	if title := sanitizeChannelText(payload.Title.String()); title != "" {
		sections = append(sections, channelSection{text: title + "\n\n"})
	}
	if description := sanitizeChannelText(payload.Description.String()); description != "" {
		sections = append(sections, channelSection{text: description + "\n\n"})
	}
	for _, question := range payload.Questions {
		if question.ID == answerable.ID {
			continue
		}
		content := sanitizeChannelText(question.Content)
		if content == "" {
			continue
		}
		label := sanitizeChannelText(question.Prompt.String())
		if label == "" {
			sections = append(sections, channelSection{content: content})
		} else {
			sections = append(sections, channelSection{text: label + ":\n", content: content})
		}
		sections = append(sections, channelSection{text: "\n\n"})
	}
	return sections
}

// channelDecision é o fim da mensagem: a pergunta, as opções numeradas e o
// prazo. Nunca é cortado — sem ele não há como responder.
func channelDecision(answerable questionnaire.Question, labels []string, timeout time.Duration) string {
	var b strings.Builder
	if prompt := sanitizeChannelText(answerable.Prompt.String()); prompt != "" {
		b.WriteString(prompt)
		b.WriteString("\n")
	}
	for i, label := range labels {
		fmt.Fprintf(&b, "%d - %s\n", i+1, label)
	}
	fmt.Fprintf(&b, "\nResponda com o número da opção. Sem resposta em %s, o pedido é negado.",
		channelDeadlineText(timeout))
	return b.String()
}

// channelDeadlineText escreve o prazo do jeito que se lê numa mensagem. Em
// minutos quando dá, porque "3m0s" é formato de log, não de conversa.
func channelDeadlineText(timeout time.Duration) string {
	if minutes := int(timeout.Minutes()); minutes >= 1 && timeout == time.Duration(minutes)*time.Minute {
		if minutes == 1 {
			return "1 minuto"
		}
		return fmt.Sprintf("%d minutos", minutes)
	}
	return timeout.String()
}

// sanitizeChannelText tira do texto o que não se lê numa mensagem: caracteres
// de controle e os invisíveis de formatação (categoria Cf do Unicode), que numa
// superfície de terceiro servem para esconder conteúdo de quem está decidindo —
// espaço de largura zero, e também as marcas de direção (U+202E e parentes), com
// as quais um caminho ou uma linha de comando aparece de trás para frente e a
// pessoa autoriza uma coisa lendo outra. Quebra de linha e tabulação ficam: são
// a formatação do bloco que ela precisa ler inteiro.
func sanitizeChannelText(text string) string {
	replaced := strings.ReplaceAll(text, "\r\n", "\n")
	var b strings.Builder
	b.Grow(len(replaced))
	for _, r := range replaced {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == '\r':
			b.WriteRune('\n')
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// truncateChannelText corta o bloco no orçamento dele, contando runas e não
// bytes — cortar no meio de um caractere entregaria texto quebrado a quem
// precisa lê-lo para decidir.
// A marca sai de dentro do orçamento, não por cima dele: ela é parte do bloco
// que vai para a mensagem, e cobrá-la fora estouraria o teto justamente quando
// há vários blocos cortados — cada corte somaria uma marca que ninguém contou.
func truncateChannelText(text string, budget int) string {
	mark := utf8.RuneCountInString(channelTruncatedMark)
	if budget <= mark {
		return channelTruncatedMark
	}
	runes := []rune(text)
	if len(runes) <= budget {
		return text
	}
	return strings.TrimRight(string(runes[:budget-mark]), " \n\t") + channelTruncatedMark
}
