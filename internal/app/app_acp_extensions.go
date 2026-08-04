package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/core/ports"
	"assistente/internal/logging"
	"assistente/internal/questionnaire"
)

const acpExtensionComponent = "app.acp-extensions"

// As extensões bloqueantes do Cursor. Elas não são do padrão ACP, mas param o
// turno do mesmo jeito que um pedido de permissão: o agente fica esperando a
// resposta do app para continuar (AEP-0084 D9).
const (
	methodAskQuestion = "cursor/ask_question"
	methodCreatePlan  = "cursor/create_plan"
)

// Desfechos que cada método aceita. Não são intercambiáveis: mandar um
// identificador de opção de permissão numa pergunta de múltipla escolha, ou
// "rejected" onde se espera "skipped", é erro de protocolo — o agente lê a
// resposta errada como decisão de verdade.
const (
	askOutcomeAnswered  = "answered"
	askOutcomeSkipped   = "skipped"
	planOutcomeAccepted = "accepted"
	planOutcomeRejected = "rejected"
)

// Chaves dos itens do questionário.
const (
	askPromptPrefix = "pergunta-"
	askAnswerPrefix = "resposta-"
	planContentID   = "plano"
	planAnswerID    = "decisao"
)

// Rótulos da decisão sobre o plano. São fixos, e não vindos do agente: ele
// manda o plano, não as opções. Continuam sendo o valor estável da escolha —
// é por eles que createPlan reencontra a decisão (AEP-0085 D5).
const (
	planApproveLabel = "Aprovar o plano"
	planRejectLabel  = "Recusar o plano"
)

// Assuntos destes dois diálogos nas chaves de tradução (AEP-0085 D7). São
// separados porque são diálogos diferentes: um pergunta, o outro submete um
// plano à aprovação, e nenhum texto serve aos dois.
const (
	askTextNamespace  = "app.questionnaire.agentQuestion."
	planTextNamespace = "app.questionnaire.agentPlan."
)

func askTextKey(field string) string {
	return askTextNamespace + field
}

func planTextKey(field string) string {
	return planTextNamespace + field
}

// O que dizemos ao agente quando o desfecho não foi decisão de ninguém. Ele
// costuma repetir esse texto para a pessoa, então ele explica o que houve em
// vez de só negar.
const (
	reasonUnreadable   = "O app não entendeu o pedido."
	reasonNoWatcher    = "Não havia ninguém na tela para responder."
	reasonUnavailable  = "O app não conseguiu apresentar o pedido."
	reasonNoAnswer     = "Ninguém respondeu dentro do prazo."
	reasonDismissed    = "A pessoa preferiu não responder."
	reasonNothingTaken = "A pessoa não escolheu nenhuma opção."
	reasonUndecided    = "O app não conseguiu decidir."
	reasonPlanRefused  = "A pessoa recusou o plano."
)

// HandleCustom traduz as extensões bloqueantes do Cursor para o questionário —
// o mesmo mecanismo do shell, da allowlist de rede e da confirmação de edição,
// que já é navegável por teclado e lido por leitor de telas (AEP-0084 D9).
//
// Método que não é tratado aqui devolve handled=false, e o transporte responde
// "método não encontrado": é o que desbloqueia o agente sem fingir suporte.
func (h *acpRequestHandler) HandleCustom(ctx context.Context, req acp.CustomRequest) (any, bool) {
	switch req.Method {
	case methodAskQuestion:
		return h.askQuestion(ctx, req), true
	case methodCreatePlan:
		return h.createPlan(ctx, req), true
	default:
		return nil, false
	}
}

// CustomFallback é o desfecho negativo de cada extensão para quando ninguém
// decidiu: o teto de tempo do transporte estourou ou o handler quebrou. Cada
// método tem o seu — "skipped" na pergunta, "rejected" no plano —, porque um
// erro genérico faria o agente concluir que o app falhou em vez de entender
// que a resposta foi não (AEP-0084 D9).
func (h *acpRequestHandler) CustomFallback(method string) (any, bool) {
	switch method {
	case methodAskQuestion:
		return askSkipped(reasonUndecided), true
	case methodCreatePlan:
		return planRejected(reasonUndecided), true
	default:
		return nil, false
	}
}

// askQuestionRequest é a pergunta bloqueante como o Cursor a manda. Repare que
// ela não traz sessionId: quem descobre a conversa dona do pedido é o
// transporte, pelo turno em voo.
type askQuestionRequest struct {
	ToolCallID string        `json:"toolCallId"`
	Title      string        `json:"title"`
	Questions  []askQuestion `json:"questions"`
}

type askQuestion struct {
	ID      string        `json:"id"`
	Prompt  string        `json:"prompt"`
	Options []askChoiceIn `json:"options"`
	// AllowMultiple é a pergunta que aceita mais de uma resposta.
	AllowMultiple bool `json:"allowMultiple"`
}

type askChoiceIn struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type askResponse struct {
	Outcome askOutcome `json:"outcome"`
}

type askOutcome struct {
	Outcome string      `json:"outcome"`
	Answers []askAnswer `json:"answers,omitempty"`
	Reason  string      `json:"reason,omitempty"`
}

type askAnswer struct {
	QuestionID        string   `json:"questionId"`
	SelectedOptionIDs []string `json:"selectedOptionIds"`
}

func askSkipped(reason string) askResponse {
	return askResponse{Outcome: askOutcome{Outcome: askOutcomeSkipped, Reason: reason}}
}

func askAnswered(answers []askAnswer) askResponse {
	return askResponse{Outcome: askOutcome{Outcome: askOutcomeAnswered, Answers: answers}}
}

// askQuestion leva a pergunta do agente a quem pode respondê-la. Todo caminho
// daqui devolve um desfecho que o método aceita: sem resposta o agente segue o
// turno com o que já sabe, que é ruim, mas melhor do que ficar pendurado.
func (h *acpRequestHandler) askQuestion(ctx context.Context, req acp.CustomRequest) any {
	var pedido askQuestionRequest
	if err := json.Unmarshal(req.Params, &pedido); err != nil {
		logging.Warnf(ctx, acpExtensionComponent, "[ACP] pergunta do agente ilegível: %v", err)
		return askSkipped(reasonUnreadable)
	}
	registro := askLogSummary(req.SessionID, pedido)

	owner, ok := h.turnOwner(req.SessionID)
	if !ok {
		// O transporte não soube dizer de quem é o pedido: sem turno em voo,
		// ou com mais de um. Não é o mesmo que não haver ninguém na tela — e o
		// agente costuma repetir o motivo à pessoa, então ele diz o que houve.
		// Sem conversa dona também não há a quem avisar.
		logging.Infof(ctx, acpExtensionComponent,
			"[ACP] pergunta pulada: o pedido não pôde ser atribuído a nenhuma conversa (%s)", registro)
		return askSkipped(reasonUndecided)
	}
	surface := h.surfaceOf(owner, ok)
	if !surface.HasInterlocutor() {
		// Job agendado, subagente, CLI, e conversa de canal que já não se sabe
		// de onde veio: não há onde perguntar, e esperar aqui penduraria o
		// agente até o teto do transporte. Mesma regra do pedido de permissão
		// (AEP-0084 D9).
		logging.Infof(ctx, acpExtensionComponent,
			"[ACP] pergunta pulada na hora, sem ninguém a quem perguntar (conversa %q): %s",
			owner.ConversationID, registro)
		h.notifyConversation(owner, ports.ChatNoticeKindQuestionNoWatcher, "")
		return askSkipped(reasonNoWatcher)
	}

	itens, respostas := askDialogFrom(pedido)
	if len(respostas) == 0 {
		// Pergunta sem opção nenhuma: não há o que oferecer à pessoa, e inventar
		// uma resposta seria decidir por ela.
		logging.Warnf(ctx, acpExtensionComponent, "[ACP] pergunta não pôde ser apresentada: %s", registro)
		h.notifyConversation(owner, ports.ChatNoticeKindQuestionUnavailable, "")
		return askSkipped(reasonUnavailable)
	}

	// Sem prazo próprio: vale o da superfície de origem, e os dois cabem dentro
	// do teto que o transporte impõe ao handler. Um prazo maior que o teto
	// tiraria de quem responde a chance de fazê-lo (AEP-0084 D9).
	resp, err := h.askOnSurface(ctx, surface, questionnaire.RequestPayload{
		Title:       questionnaire.Keyed(askTextKey("title"), "O agente tem uma pergunta"),
		Description: askDescriptionText(pedido.Title),
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed(askTextKey("submit"), "Responder"),
		CancelLabel: questionnaire.Keyed(askTextKey("cancel"), "Pular a pergunta"),
		Questions:   itens,
	})
	if err != nil {
		// Prazo estourado, turno cancelado, pergunta que a superfície não soube
		// apresentar, app encerrando. Para o agente é tudo a mesma coisa —
		// ninguém respondeu —, mas o motivo que ele repete à pessoa e o aviso
		// que fica na conversa mudam com a causa.
		logging.Infof(ctx, acpExtensionComponent,
			"[ACP] pergunta sem resposta na conversa %q (%s): %v", owner.ConversationID, registro, err)
		causa := undecidedCauseOf(ctx, err)
		// Turno cancelado não vira aviso: foi a própria pessoa que desistiu, e
		// o diálogo já saiu da tela dizendo isso.
		if notice := askFailureNotice(causa); notice != "" {
			h.notifyConversation(owner, notice, "")
		}
		return askSkipped(undecidedReason(causa))
	}
	if resp.Cancelled {
		return askSkipped(reasonDismissed)
	}

	answers := askAnswersFrom(resp, respostas)
	if len(answers) == 0 {
		return askSkipped(reasonNothingTaken)
	}
	return askAnswered(answers)
}

// askFailureNotice é o aviso que a conversa recebe quando a pergunta do agente
// acabou sem resposta. Vazio quer dizer não avisar.
func askFailureNotice(causa undecidedCause) string {
	switch causa {
	case causeCancelled:
		return ""
	case causeNoInterlocutor:
		return ports.ChatNoticeKindQuestionNoWatcher
	case causeUnavailable:
		return ports.ChatNoticeKindQuestionUnavailable
	default:
		return ports.ChatNoticeKindQuestionTimeout
	}
}

// planFailureNotice é o mesmo para o plano proposto.
func planFailureNotice(causa undecidedCause) string {
	switch causa {
	case causeCancelled:
		return ""
	case causeNoInterlocutor:
		return ports.ChatNoticeKindPlanNoWatcher
	case causeUnavailable:
		return ports.ChatNoticeKindPlanUnavailable
	default:
		return ports.ChatNoticeKindPlanTimeout
	}
}

// undecidedReason é o que o agente repete à pessoa. Ele erra ao dizer que
// ninguém respondeu a tempo se a pergunta nunca apareceu — quem lê acharia que
// ela foi ignorada.
func undecidedReason(causa undecidedCause) string {
	switch causa {
	case causeNoInterlocutor:
		return reasonNoWatcher
	case causeUnavailable:
		return reasonUnavailable
	default:
		return reasonNoAnswer
	}
}

// askChoice é uma opção da pergunta pronta para a tela: o rótulo que aparece e
// o identificador que volta ao agente.
type askChoice struct {
	id    string
	label string
}

// askItem liga uma pergunta do agente ao item do questionário que a responde.
type askItem struct {
	questionID string
	answerID   string
	choices    []askChoice
}

// askDialogFrom monta o diálogo e o mapa de volta. Cada pergunta vira dois
// itens: o texto do agente em bloco de leitura, inteiro, e a escolha. É o
// mesmo formato do pedido de permissão, e pelo mesmo motivo — o que a pessoa
// lê para decidir não pode aparecer cortado.
func askDialogFrom(pedido askQuestionRequest) ([]questionnaire.Question, []askItem) {
	// A numeração conta o que a pessoa vai ver, e não o que o agente mandou:
	// com uma pergunta descartada no meio, "Pergunta 2 de 3" numa tela de duas
	// faria quem ouve procurar a que não está lá.
	perguntas := askAnswerableFrom(pedido.Questions)
	itens := make([]questionnaire.Question, 0, len(perguntas)*2)
	respostas := make([]askItem, 0, len(perguntas))
	for i, pergunta := range perguntas {
		answerID := fmt.Sprintf("%s%d", askAnswerPrefix, i)
		itens = append(itens,
			questionnaire.Question{
				// O texto da pergunta é do agente: vai como conteúdo de bloco,
				// sem chave de tradução (AEP-0085 D6).
				ID:      fmt.Sprintf("%s%d", askPromptPrefix, i),
				Type:    "readonly_code",
				Prompt:  askPromptLabelText(i, len(perguntas)),
				Content: askPromptContent(pergunta.Prompt),
			},
			questionnaire.Question{
				ID:   answerID,
				Type: askChoiceType(pergunta.AllowMultiple),
				// Rótulo que o agente mandou é texto, nunca chave de tradução:
				// traduzir o que vem de fora exibiria o texto de outro lugar do
				// app no lugar da opção que ele ofereceu (AEP-0085).
				Prompt:  askChoicePromptText(pergunta.AllowMultiple),
				Options: questionnaire.PlainTexts(askLabels(pergunta.choices)),
				// A múltipla escolha aceita nenhuma marcada — exigir uma
				// obrigaria a inventar resposta para sair do diálogo.
				Required:  !pergunta.AllowMultiple,
				AutoFocus: i == 0,
			})
		respostas = append(respostas, askItem{
			questionID: pergunta.ID,
			answerID:   answerID,
			choices:    pergunta.choices,
		})
	}
	return itens, respostas
}

// askAnswerable é a pergunta que sobrou com algo a oferecer, junto das opções
// já prontas para a tela.
type askAnswerable struct {
	askQuestion
	choices []askChoice
}

// askAnswerableFrom descarta a pergunta sem opção nenhuma: não há o que
// oferecer, e um item de escolha vazio na tela só atrapalharia quem navega por
// teclado.
func askAnswerableFrom(questions []askQuestion) []askAnswerable {
	out := make([]askAnswerable, 0, len(questions))
	for _, pergunta := range questions {
		escolhas := askChoicesFrom(pergunta.Options)
		if len(escolhas) == 0 {
			continue
		}
		out = append(out, askAnswerable{askQuestion: pergunta, choices: escolhas})
	}
	return out
}

// askChoicesFrom prepara as opções que o agente ofereceu. Os rótulos são dele,
// então passam pelo saneamento; o que não tiver identificador fica de fora,
// porque não haveria o que responder ao escolhê-lo. Rótulo repetido ganha
// desempate pelo mesmo motivo do pedido de permissão: a escolha volta da tela
// como texto, e dois rótulos iguais fariam a resposta apontar sempre para o
// primeiro.
func askChoicesFrom(options []askChoiceIn) []askChoice {
	out := make([]askChoice, 0, len(options))
	seen := make(map[string]bool, len(options))
	for _, option := range options {
		if strings.TrimSpace(option.ID) == "" {
			continue
		}
		label := acp.SanitizeLabel(option.Label)
		if label == "" {
			label = "Opção sem nome"
		}
		label = distinctLabel(seen, label, option.ID)
		seen[label] = true
		out = append(out, askChoice{id: option.ID, label: label})
	}
	return out
}

func askLabels(choices []askChoice) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		out = append(out, choice.label)
	}
	return out
}

func askChoiceType(multiple bool) string {
	if multiple {
		return "multiple_choice"
	}
	return "single_choice"
}

func askChoicePrompt(multiple bool) string {
	if multiple {
		return "Sua resposta (pode marcar mais de uma)"
	}
	return "Sua resposta"
}

// askChoicePromptText nomeia o item de escolha. Cada forma da pergunta tem a
// sua chave: a de múltipla escolha avisa que dá para marcar mais de uma, e é
// disso que depende quem só ouve o rótulo antes de responder.
func askChoicePromptText(multiple bool) questionnaire.Text {
	if multiple {
		return questionnaire.Keyed(askTextKey("answerPromptMultiple"), askChoicePrompt(true))
	}
	return questionnaire.Keyed(askTextKey("answerPrompt"), askChoicePrompt(false))
}

// askPromptLabel nomeia o bloco de leitura. Numerar só faz sentido quando há
// mais de uma pergunta; com uma só, o número seria ruído para quem ouve.
func askPromptLabel(index, total int) string {
	if total <= 1 {
		return "Pergunta do agente"
	}
	return fmt.Sprintf("Pergunta %d de %d", index+1, total)
}

// askPromptLabelText leva o nome do bloco traduzível. A numeração vai em
// valores interpolados, e não na chave: a posição é número, não assunto — e o
// fallback já vai com ela no lugar (AEP-0085 D3).
//
// Os nomes evitam os reservados do i18next (count, context, lng), que mudariam
// pluralização, contexto ou idioma da tradução.
func askPromptLabelText(index, total int) questionnaire.Text {
	if total <= 1 {
		return questionnaire.Keyed(askTextKey("promptLabel"), askPromptLabel(index, total))
	}
	return questionnaire.KeyedWith(
		askTextKey("promptLabelNumbered"),
		map[string]any{"position": index + 1, "total": total},
		askPromptLabel(index, total),
	)
}

// askPromptContent é o texto da pergunta como a pessoa vai lê-lo: saneado, mas
// inteiro. O corte de rótulo esconderia o fim de uma pergunta longa, e é
// justamente ali que costuma estar a condição que muda a resposta.
func askPromptContent(prompt string) string {
	if content := acp.SanitizeContent(prompt); content != "" {
		return content
	}
	return "(o agente não escreveu a pergunta)"
}

// askDescription abre o diálogo dizendo o que está em jogo. O assunto vem do
// agente e entra como rótulo — curto, saneado e numa linha só.
func askDescription(title string) string {
	const base = "O agente parou o turno para perguntar. Sem resposta ele segue assim mesmo, e o que vier depois pode não ser o que você queria."
	if assunto := acp.SanitizeLabel(title); assunto != "" {
		return fmt.Sprintf("%s Assunto: %q.", base, assunto)
	}
	return base
}

// askDescriptionText leva a descrição traduzível. O assunto é texto do agente:
// entra como valor interpolado, nunca na chave, porque chave é decisão do app
// (AEP-0085 D6). Sem assunto a frase é outra, e por isso tem chave própria —
// uma só deixaria "Assunto:" vazio na tela de quem traduz.
func askDescriptionText(title string) questionnaire.Text {
	assunto := acp.SanitizeLabel(title)
	if assunto == "" {
		return questionnaire.Keyed(askTextKey("description"), askDescription(title))
	}
	return questionnaire.KeyedWith(
		askTextKey("descriptionSubject"),
		map[string]any{"subject": assunto},
		askDescription(title),
	)
}

// askAnswersFrom traduz o que a tela devolveu nos identificadores que o agente
// entende. Pergunta sem nada escolhido fica de fora: mandar uma resposta vazia
// seria dizer que a pessoa escolheu, quando ela só passou por cima.
func askAnswersFrom(resp questionnaire.Response, itens []askItem) []askAnswer {
	out := make([]askAnswer, 0, len(itens))
	for _, item := range itens {
		selecionados := selectedOptionIDs(resp.Answers[item.answerID], item.choices)
		if len(selecionados) == 0 {
			continue
		}
		out = append(out, askAnswer{QuestionID: item.questionID, SelectedOptionIDs: selecionados})
	}
	return out
}

// selectedOptionIDs reencontra as opções pelos rótulos escolhidos. O que volta
// ao agente é o identificador cru, como ele o mandou: aparado ou saneado, não
// bateria com nenhuma opção do lado dele.
func selectedOptionIDs(answer any, choices []askChoice) []string {
	var out []string
	for _, label := range chosenLabels(answer) {
		for _, choice := range choices {
			if choice.label == label {
				out = append(out, choice.id)
				break
			}
		}
	}
	return out
}

// chosenLabels lê a resposta da tela nas formas que ela chega: texto na
// escolha única, lista na múltipla — e a lista vem como []any depois de
// atravessar o JSON da ponte.
func chosenLabels(answer any) []string {
	switch value := answer.(type) {
	case string:
		return []string{value}
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if label, ok := item.(string); ok {
				out = append(out, label)
			}
		}
		return out
	default:
		return nil
	}
}

// createPlanRequest é o plano que o agente montou e submete à aprovação.
type createPlanRequest struct {
	ToolCallID string      `json:"toolCallId"`
	Name       string      `json:"name"`
	Overview   string      `json:"overview"`
	Plan       string      `json:"plan"`
	Todos      []planTodo  `json:"todos"`
	IsProject  bool        `json:"isProject"`
	Phases     []planPhase `json:"phases"`
}

type planTodo struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type planPhase struct {
	Name  string     `json:"name"`
	Todos []planTodo `json:"todos"`
}

type planResponse struct {
	Outcome planOutcome `json:"outcome"`
}

type planOutcome struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
	// PlanURI é onde o plano foi gravado. O app não grava plano nenhum, então
	// vai vazio: o agente completa com o arquivo dele quando tem um.
	PlanURI string `json:"planUri,omitempty"`
}

func planRejected(reason string) planResponse {
	return planResponse{Outcome: planOutcome{Outcome: planOutcomeRejected, Reason: reason}}
}

func planAccepted() planResponse {
	return planResponse{Outcome: planOutcome{Outcome: planOutcomeAccepted}}
}

// createPlan leva o plano do agente a quem pode aprová-lo. Sem decisão o plano
// é recusado: aprovar em silêncio um plano que ninguém leu é o oposto do que a
// pergunta existe para fazer (AEP-0084 D9).
func (h *acpRequestHandler) createPlan(ctx context.Context, req acp.CustomRequest) any {
	var pedido createPlanRequest
	if err := json.Unmarshal(req.Params, &pedido); err != nil {
		logging.Warnf(ctx, acpExtensionComponent, "[ACP] plano do agente ilegível: %v", err)
		return planRejected(reasonUnreadable)
	}
	registro := planLogSummary(req.SessionID, pedido)

	owner, ok := h.turnOwner(req.SessionID)
	if !ok {
		logging.Infof(ctx, acpExtensionComponent,
			"[ACP] plano recusado: o pedido não pôde ser atribuído a nenhuma conversa (%s)", registro)
		return planRejected(reasonUndecided)
	}
	surface := h.surfaceOf(owner, ok)
	if !surface.HasInterlocutor() {
		logging.Infof(ctx, acpExtensionComponent,
			"[ACP] plano recusado na hora, sem ninguém a quem apresentá-lo (conversa %q): %s",
			owner.ConversationID, registro)
		h.notifyConversation(owner, ports.ChatNoticeKindPlanNoWatcher, "")
		return planRejected(reasonNoWatcher)
	}

	resp, err := h.askOnSurface(ctx, surface, questionnaire.RequestPayload{
		Title:       questionnaire.Keyed(planTextKey("title"), "O agente propôs um plano"),
		Description: planDescriptionText(pedido),
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed(planTextKey("submit"), "Confirmar"),
		CancelLabel: questionnaire.Keyed(planTextKey("cancel"), "Recusar"),
		Questions: []questionnaire.Question{
			{
				// O plano vai inteiro, em bloco: é o que a pessoa lê para
				// decidir, e um resumo faria aprovar o que não apareceu na
				// tela. O bloco é texto do agente, sem chave (AEP-0085 D6).
				ID:      planContentID,
				Type:    "readonly_code",
				Prompt:  questionnaire.Keyed(planTextKey("contentPrompt"), "Plano proposto"),
				Content: planContent(pedido),
			},
			{
				ID:     planAnswerID,
				Type:   "single_choice",
				Prompt: questionnaire.Keyed(planTextKey("choicePrompt"), "O agente pode seguir este plano?"),
				// Aqui a opção é do app, e não do agente: ganha chave. O valor
				// que volta em Answers continua sendo o fallback, que é como
				// createPlan reencontra a escolha (AEP-0085 D5).
				Options: []questionnaire.Text{
					questionnaire.Keyed(planTextKey("approve"), planApproveLabel),
					questionnaire.Keyed(planTextKey("reject"), planRejectLabel),
				},
				Required:  true,
				AutoFocus: true,
			},
		},
	})
	if err != nil {
		logging.Infof(ctx, acpExtensionComponent,
			"[ACP] plano sem decisão na conversa %q (%s): %v", owner.ConversationID, registro, err)
		causa := undecidedCauseOf(ctx, err)
		if notice := planFailureNotice(causa); notice != "" {
			h.notifyConversation(owner, notice, "")
		}
		return planRejected(undecidedReason(causa))
	}
	// Sair pelo botão de cancelar é recusar: ele se chama "Recusar", e é a
	// saída de quem leu o plano e não quis. Dizer ao agente que o pedido foi
	// "dispensado" o faria contar à pessoa uma coisa diferente da que ela fez.
	if resp.Cancelled {
		return planRejected(reasonPlanRefused)
	}
	if escolha, _ := resp.Answers[planAnswerID].(string); escolha == planApproveLabel {
		return planAccepted()
	}
	return planRejected(reasonPlanRefused)
}

// planDescriptionText leva a descrição do plano traduzível. A frase muda com o
// que o pedido traz — plano de projeto, contagem de passos —, e como a descrição
// é um campo só, é a chave que diz qual das quatro frases é. O número de passos
// vai como valor interpolado: número não é assunto, e o fallback já vai com ele
// no lugar (AEP-0085 D3).
func planDescriptionText(pedido createPlanRequest) questionnaire.Text {
	fallback := planDescription(pedido)
	passos := len(pedido.Todos) + phaseTodos(pedido.Phases)
	switch {
	case pedido.IsProject && passos > 0:
		return questionnaire.KeyedWith(planTextKey("descriptionProjectSteps"),
			map[string]any{"steps": passos}, fallback)
	case pedido.IsProject:
		return questionnaire.Keyed(planTextKey("descriptionProject"), fallback)
	case passos > 0:
		return questionnaire.KeyedWith(planTextKey("descriptionSteps"),
			map[string]any{"steps": passos}, fallback)
	default:
		return questionnaire.Keyed(planTextKey("description"), fallback)
	}
}

// planDescription diz o alcance do plano antes de alguém aprová-lo. Só o que é
// enumerável entra aqui; o texto do agente vai no bloco de leitura abaixo.
func planDescription(pedido createPlanRequest) string {
	base := "Aprovar deixa o agente executar os passos abaixo na sua máquina. Leia o plano inteiro antes de decidir."
	if pedido.IsProject {
		base += " O agente marcou este plano como de projeto."
	}
	if total := len(pedido.Todos) + phaseTodos(pedido.Phases); total > 0 {
		base += fmt.Sprintf(" Ele tem %d passo(s).", total)
	}
	return base
}

func phaseTodos(phases []planPhase) int {
	total := 0
	for _, phase := range phases {
		total += len(phase.Todos)
	}
	return total
}

// planContent monta o bloco que a pessoa lê. Cada pedaço vindo do agente é
// saneado por si; o que separa um do outro é texto do app, para que o agente
// não consiga forjar as seções do próprio plano.
func planContent(pedido createPlanRequest) string {
	var b strings.Builder
	appendSection(&b, "", acp.SanitizeContent(pedido.Name))
	appendSection(&b, "Visão geral", acp.SanitizeContent(pedido.Overview))
	appendSection(&b, "Plano", acp.SanitizeContent(pedido.Plan))
	for _, phase := range pedido.Phases {
		titulo := acp.SanitizeLabel(phase.Name)
		if titulo == "" {
			titulo = "Fase sem nome"
		}
		appendSection(&b, "Fase: "+titulo, planTodoLines(phase.Todos))
	}
	appendSection(&b, "Passos", planTodoLines(pedido.Todos))
	if b.Len() == 0 {
		return "(o agente não descreveu o plano)"
	}
	return strings.TrimSpace(b.String())
}

func appendSection(b *strings.Builder, title, body string) {
	if body == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	if title != "" {
		b.WriteString(title)
		b.WriteString(":\n")
	}
	b.WriteString(body)
}

// planTodoLines lista os passos. O conteúdo é o que a pessoa precisa ler
// inteiro; a situação de cada passo é curta e enumerável, e entra como rótulo.
func planTodoLines(todos []planTodo) string {
	linhas := make([]string, 0, len(todos))
	for _, todo := range todos {
		conteudo := acp.SanitizeContent(todo.Content)
		if conteudo == "" {
			continue
		}
		if situacao := acp.SanitizeLabel(todo.Status); situacao != "" {
			linhas = append(linhas, fmt.Sprintf("- [%s] %s", situacao, conteudo))
			continue
		}
		linhas = append(linhas, "- "+conteudo)
	}
	return strings.Join(linhas, "\n")
}

// askLogSummary e planLogSummary descrevem o pedido para o log sem levar junto
// o que o agente escreveu. Vale aqui o mesmo motivo do resumo do pedido de
// permissão: a pergunta e o plano falam do trabalho da pessoa, e o log fica
// guardado sem que ninguém tenha pedido. Na tela o texto integral aparece,
// porque é dele que a decisão depende.
func askLogSummary(sessionID string, pedido askQuestionRequest) string {
	return fmt.Sprintf("sessão %q, chamada %q, %d pergunta(s)",
		acp.SanitizeLabel(sessionID), acp.SanitizeLabel(pedido.ToolCallID), len(pedido.Questions))
}

func planLogSummary(sessionID string, pedido createPlanRequest) string {
	return fmt.Sprintf("sessão %q, chamada %q, %d passo(s)",
		acp.SanitizeLabel(sessionID), acp.SanitizeLabel(pedido.ToolCallID),
		len(pedido.Todos)+phaseTodos(pedido.Phases))
}
