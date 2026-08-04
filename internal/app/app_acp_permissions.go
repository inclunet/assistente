package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/acptrust"
	"assistente/internal/core/ports"
	"assistente/internal/logging"
	"assistente/internal/questionnaire"
)

const acpPermissionComponent = "app.acp-permissions"

// Chaves dos itens do questionário: a ação que se lê e a escolha que se faz.
const (
	permissionActionID = "action"
	permissionAnswerID = "decision"
)

// permissionTextNamespace é o assunto deste diálogo nas chaves de tradução
// (AEP-0085 D7). O texto pronto em pt-BR continua viajando como fallback: é ele
// que aparece se a chave faltar num locale, e é ele que serve às superfícies
// que não traduzem nada.
const permissionTextNamespace = "app.questionnaire.agentPermission."

func permissionTextKey(field string) string {
	return permissionTextNamespace + field
}

// acpRequestHandler responde ao que o agente de código pergunta ao app
// (AEP-0084 D9): o pedido de permissão para agir na máquina, tratado aqui, e as
// extensões bloqueantes do Cursor, em app_acp_extensions.go.
//
// As dependências entram como função porque este handler nasce antes do resto
// do app: o serviço de agentes é criado cedo, e o questionário só existe
// depois. Guardar o valor aqui congelaria um nulo.
type acpRequestHandler struct {
	// owner diz de quem é o turno em voo daquela sessão do agente.
	owner func(sessionID string) (acp.TurnOwner, bool)
	// questions é o questionário do desktop, o mesmo mecanismo acessível que
	// shell, HTTP mutável e confirmação de edição já usam.
	questions func() *questionnaire.Manager
	// notices leva à conversa o aviso do que foi negado sem decisão de
	// ninguém.
	notices func() ports.Emitter
	// trust guarda o que a pessoa já autorizou para sempre naquele perfil.
	trust func() *acptrust.Store
	// activeProfile resolve o perfil corrente quando o turno não nomeia um,
	// que é o caso do desktop. Mesmo acordo das allowlists de rede.
	activeProfile func() string
}

var _ acp.RequestHandler = (*acpRequestHandler)(nil)

// RequestPermission leva o pedido do agente a quem pode decidir. Sem decisão
// — ninguém para perguntar, prazo estourado, diálogo recusado — devolve a
// escolha vazia, e o transporte responde ao agente a recusa pontual que ele
// mesmo ofereceu. Nunca deixa o pedido sem resposta: um turno pendurado é pior
// do que uma ação negada.
func (h *acpRequestHandler) RequestPermission(ctx context.Context, req acp.PermissionRequest) acp.PermissionOutcome {
	// A ação vai inteira para a tela, e não pelo saneamento de rótulo: o corte
	// que serve a um anúncio esconderia o fim de uma linha de comando longa,
	// e é justamente o fim dela que costuma mudar o que ela faz.
	action := acp.SanitizeContent(req.ToolCall.Title)
	if action == "" {
		action = "(o agente não descreveu a ação)"
	}
	registro := permissionLogSummary(req.ToolCall)

	owner, ok := h.turnOwner(req.SessionID)
	if !ok || !owner.Interactive {
		// Turno de canal, job, subagente ou CLI: não há tela onde perguntar.
		// Esperar aqui penduraria o agente até o teto do transporte.
		logging.Infof(ctx, acpPermissionComponent,
			"[ACP] permissão negada na hora, sem ninguém a quem perguntar (sessão %q, conversa %q): %s",
			req.SessionID, owner.ConversationID, registro)
		h.notifyConversation(owner, ports.ChatNoticeKindPermissionNoWatcher, req.ToolCall.Kind)
		return acp.PermissionOutcome{}
	}

	choices := permissionChoicesFrom(req.Options)
	if len(choices) == 0 {
		// Pedido sem opção nenhuma: não há o que oferecer à pessoa, e inventar
		// uma resposta seria decidir por ela.
		logging.Warnf(ctx, acpPermissionComponent, "[ACP] pedido de permissão sem opções: %s", registro)
		h.notifyConversation(owner, ports.ChatNoticeKindPermissionUnavailable, req.ToolCall.Kind)
		return acp.PermissionOutcome{}
	}

	kind := acp.ToolKind(req.ToolCall.Kind)
	profile := h.profileOf(owner)

	// Autorização permanente é o "permitir sempre" de antes: perguntar de novo
	// seria ignorar o que a pessoa já respondeu. Só se chega aqui com turno de
	// desktop — a negativa acima vem primeiro de propósito, para que um canal
	// remoto não colha o sim que alguém deu na tela (AEP-0084 D9).
	if h.alreadyAllowed(profile, kind) {
		if id, ok := choices.approval(); ok {
			logging.Infof(ctx, acpPermissionComponent,
				"[ACP] permissão concedida pelo que o perfil %q já autorizou (%s)", profile, registro)
			return acp.PermissionOutcome{OptionID: id}
		}
		// O agente não ofereceu como dizer sim. Não há o que responder no
		// lugar dele: a pergunta segue para a tela.
		logging.Warnf(ctx, acpPermissionComponent,
			"[ACP] o perfil %q autoriza esta classe, mas o pedido não trouxe opção de permitir: %s", profile, registro)
	}

	manager := h.questionnaireManager()
	if manager == nil {
		logging.Warnf(ctx, acpPermissionComponent, "[ACP] permissão negada: o questionário não está disponível")
		h.notifyConversation(owner, ports.ChatNoticeKindPermissionUnavailable, req.ToolCall.Kind)
		return acp.PermissionOutcome{}
	}

	// Sem prazo próprio: vale o do questionário, que é o do desktop e cabe
	// dentro do teto que o transporte impõe ao handler. Um prazo maior que o
	// teto tiraria da pessoa a chance de responder (AEP-0084 D9).
	resp, err := manager.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       questionnaire.Keyed(permissionTextKey("title"), "O agente pede permissão"),
		Description: permissionDescriptionText(choices, kind),
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed(permissionTextKey("submit"), "Confirmar"),
		CancelLabel: questionnaire.Keyed(permissionTextKey("cancel"), "Negar"),
		Questions: []questionnaire.Question{
			{
				// A ação vai inteira, em bloco: é o que a pessoa lê para
				// decidir, e um resumo faria autorizar o que não apareceu na
				// tela. É o mesmo formato da confirmação de edição e da
				// autorização de rede. O conteúdo do bloco é do agente: vai
				// como texto, sem chave de tradução (AEP-0085 D6).
				ID:      permissionActionID,
				Type:    "readonly_code",
				Prompt:  questionnaire.Keyed(permissionTextKey("actionPrompt"), "Ação pedida"),
				Content: action,
			},
			{
				ID:   permissionAnswerID,
				Type: "single_choice",
				// Rótulo que o agente mandou é texto, nunca chave de tradução:
				// traduzir o que vem de fora exibiria o texto de outro lugar do
				// app no lugar da opção que ele ofereceu (AEP-0085).
				Prompt:    questionnaire.Keyed(permissionTextKey("choicePrompt"), "O que o agente pode fazer?"),
				Options:   questionnaire.PlainTexts(choices.labels()),
				Required:  true,
				AutoFocus: true,
			},
		},
	})
	if err != nil {
		// Prazo estourado, turno cancelado ou app encerrando. Todos viram a
		// mesma coisa para o agente: ninguém autorizou.
		logging.Infof(ctx, acpPermissionComponent,
			"[ACP] permissão sem resposta na conversa %q (%s): %v", owner.ConversationID, registro, err)
		// Turno cancelado não vira aviso: foi a própria pessoa que desistiu, e
		// o diálogo já saiu da tela dizendo isso. Avisar de novo seria cobrar
		// explicação de quem acabou de dar uma.
		if !turnCancelled(ctx, err) {
			h.notifyConversation(owner, ports.ChatNoticeKindPermissionTimeout, req.ToolCall.Kind)
		}
		return acp.PermissionOutcome{}
	}
	if resp.Cancelled {
		return acp.PermissionOutcome{}
	}

	label, _ := resp.Answers[permissionAnswerID].(string)
	choice, ok := choices.byLabel(label)
	if !ok {
		logging.Warnf(ctx, acpPermissionComponent, "[ACP] resposta de permissão fora das opções oferecidas; negando")
		return acp.PermissionOutcome{}
	}
	if choice.always() {
		h.rememberAlways(ctx, owner, profile, kind, registro)
	}
	return acp.PermissionOutcome{OptionID: choice.id}
}

func (h *acpRequestHandler) turnOwner(sessionID string) (acp.TurnOwner, bool) {
	if h == nil || h.owner == nil {
		return acp.TurnOwner{}, false
	}
	return h.owner(sessionID)
}

func (h *acpRequestHandler) questionnaireManager() *questionnaire.Manager {
	if h == nil || h.questions == nil {
		return nil
	}
	return h.questions()
}

// turnCancelled diz se o pedido acabou porque o turno foi abortado, e não
// porque o tempo acabou. Olha o contexto do turno além do erro: o erro é a
// via normal, mas depender só dele amarraria esta decisão ao jeito como o
// questionário embrulha a causa — e um dia em que ela se perder no caminho, a
// pessoa que cancelou receberia um "ninguém respondeu a tempo".
//
// Prazo do turno estourado não é cancelamento: o teto que o transporte impõe
// ao handler existe justamente como limite de espera por uma resposta, e ao
// estourar vale o mesmo desfecho do prazo da pergunta (AEP-0084 D9). Para quem
// lê, ninguém respondeu a tempo — que é o que o aviso diz.
func turnCancelled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
}

// notifyConversation conta à conversa o que o app decidiu sobre um pedido de
// permissão: a ação negada sem que ninguém decidisse, ou a autorização que
// passou a valer daqui em diante. O agente costuma seguir o turno dizendo
// apenas que não conseguiu; sem este aviso, a pessoa fica sem saber que houve
// um pedido, muito menos o que aconteceu com ele.
//
// Vai só a classe da ação, nunca o texto do agente: o aviso pode aparecer numa
// conversa que ninguém está olhando agora, e a linha de comando literal
// costuma carregar segredo.
func (h *acpRequestHandler) notifyConversation(owner acp.TurnOwner, kind, action string) {
	if h == nil || h.notices == nil || strings.TrimSpace(owner.ConversationID) == "" {
		return
	}
	emitter := h.notices()
	if emitter == nil {
		return
	}
	emitter.Emit("chat:notice", ports.ChatNoticeEvent{
		ConversationID: owner.ConversationID,
		Kind:           kind,
		// A classe passa pelo conjunto do protocolo: o que o agente inventar
		// vira "other", que quem exibe traduz na frase genérica. Sem isso,
		// texto do agente entraria no meio do aviso.
		Action: acp.ToolKind(action),
	})
}

// profileOf diz de quem é a autorização permanente deste turno. O turno nomeia
// o perfil quando ele foi escolhido na origem (canal, job); no desktop vale o
// perfil corrente, como nas allowlists de rede.
func (h *acpRequestHandler) profileOf(owner acp.TurnOwner) string {
	if slug := strings.TrimSpace(owner.ProfileSlug); slug != "" {
		return slug
	}
	if h == nil || h.activeProfile == nil {
		return ""
	}
	return strings.TrimSpace(h.activeProfile())
}

func (h *acpRequestHandler) trustStore() *acptrust.Store {
	if h == nil || h.trust == nil {
		return nil
	}
	return h.trust()
}

// alreadyAllowed diz se este perfil já resolveu esta classe de ação antes.
// Sem perfil não há autorização a consultar: guardar por "perfil nenhum"
// valeria para todo mundo, que é o oposto do que a pessoa autorizou.
func (h *acpRequestHandler) alreadyAllowed(profile, kind string) bool {
	if profile == "" {
		return false
	}
	store := h.trustStore()
	if store == nil {
		return false
	}
	return store.Allows(profile, kind)
}

// rememberAlways guarda o "permitir sempre" e conta à conversa o que houve.
// Falhar ao gravar não desfaz a autorização desta vez — a pessoa disse sim, e o
// agente vai agir —, mas a próxima volta a perguntar, que é o lado seguro de
// não conseguir lembrar. Nos dois casos o aviso fica na conversa: a escolha
// vale além deste turno, e o diálogo que a recebeu já saiu da tela.
func (h *acpRequestHandler) rememberAlways(ctx context.Context, owner acp.TurnOwner, profile, kind, registro string) {
	store := h.trustStore()
	if store == nil || profile == "" {
		logging.Warnf(ctx, acpPermissionComponent,
			"[ACP] permissão dada para sempre não pôde ser guardada (perfil %q): %s", profile, registro)
		h.notifyConversation(owner, ports.ChatNoticeKindPermissionAlwaysNotSaved, kind)
		return
	}
	if err := store.Allow(profile, kind); err != nil {
		logging.Warnf(ctx, acpPermissionComponent,
			"[ACP] erro ao guardar a autorização permanente do perfil %q: %v", profile, err)
		h.notifyConversation(owner, ports.ChatNoticeKindPermissionAlwaysNotSaved, kind)
		return
	}
	logging.Infof(ctx, acpPermissionComponent,
		"[ACP] o perfil %q passa a autorizar %q sem perguntar (%s)", profile, kind, registro)
	h.notifyConversation(owner, ports.ChatNoticeKindPermissionAlwaysAllowed, kind)
}

// alwaysWarning explica o alcance do "permitir sempre" antes de alguém
// escolhê-lo. A autorização vale para a classe inteira da ação, e não só para
// o comando que está na tela: sem dizer isso, quem autorizasse um comando
// inofensivo estaria liberando todos os outros da mesma classe sem saber.
func alwaysWarning(choices permissionChoices, kind string) string {
	if !choices.hasAlways() {
		return ""
	}
	if kind == "" || kind == acp.ToolKindOther {
		return " Se escolher permitir sempre, este perfil passa a autorizar sem perguntar qualquer ação que o agente não classifique, até você revogar."
	}
	return fmt.Sprintf(" Se escolher permitir sempre, este perfil passa a autorizar qualquer ação da classe %q sem perguntar, até você revogar.", kind)
}

// permissionLogSummary descreve o pedido para o log sem levar junto o que o
// agente escreveu. O título costuma ser a linha de comando literal, e linha de
// comando carrega segredo em flag e em variável de ambiente — é o mesmo motivo
// pelo qual o shell do app não registra o comando cru. Na tela o texto
// integral aparece, porque quem autoriza precisa ver o que está autorizando;
// no log ele ficaria guardado sem que ninguém tenha pedido.
func permissionLogSummary(call acp.ToolCall) string {
	kind := acp.SanitizeLabel(call.Kind)
	if kind == "" {
		kind = "ação sem classe"
	}
	return fmt.Sprintf("%s, chamada %q", kind, acp.SanitizeLabel(call.ID))
}

// permissionDescriptionText é a descrição do diálogo pronta para a tela: a
// frase de abertura e, quando o agente ofereceu autorizar para sempre, o aviso
// do que esse sempre abrange. As duas moram num campo só, então é a chave que
// carrega as duas variações — não há onde exibir dois textos ali.
//
// A classe entra na chave, e não como valor interpolado: o código do protocolo
// é inglês, e "o agente quer execute" continuaria em inglês em qualquer idioma
// (AEP-0085 D6). O fallback vai com o texto de sempre, já montado em pt-BR.
func permissionDescriptionText(choices permissionChoices, kind string) questionnaire.Text {
	campo := "description"
	if choices.hasAlways() {
		// O aviso do "sempre" muda a frase inteira, e não só a acrescenta: a
		// chave precisa dizer de qual das duas ele fala.
		campo = "descriptionAlways"
	}
	return questionnaire.Keyed(
		permissionTextKey(campo+"."+permissionKindKey(kind)),
		permissionDescription(kind)+alwaysWarning(choices, kind),
	)
}

// permissionKindKey é o pedaço da chave que diz de que classe a frase fala. O
// que não estiver no conjunto do protocolo cai em "other", a frase genérica.
func permissionKindKey(kind string) string {
	switch normalized := acp.ToolKind(kind); normalized {
	case "switch_mode":
		// O código do protocolo tem underscore e a chave do locale não: o
		// conjunto equivalente do lado da tela (agentPermissions.action.*) já
		// nomeia as classes assim, e duas grafias fariam a mesma classe
		// aparecer com dois nomes nos arquivos de tradução.
		return "switchMode"
	default:
		return normalized
	}
}

// permissionDescription abre o diálogo dizendo a classe da ação, que é
// enumerável — ao contrário do que o agente escreve, que vai no bloco abaixo
// dela. A classe vem pelo conjunto do protocolo: o que o agente inventar vira
// "other", e a frase genérica, para que texto dele não entre aqui (D9/D11).
func permissionDescription(kind string) string {
	if kind == "" || kind == acp.ToolKindOther {
		return "O agente quer executar uma ação na sua máquina. Confira o que ele pede antes de decidir."
	}
	return fmt.Sprintf("O agente quer executar uma ação do tipo %q na sua máquina. Confira o que ele pede antes de decidir.", kind)
}

// Classes de opção do protocolo que interessam à decisão.
const (
	optionAllowOnce   = "allow_once"
	optionAllowAlways = "allow_always"
)

// permissionChoice é uma opção do pedido já pronta para a tela: o rótulo que
// aparece, o identificador que volta ao agente e a classe, que diz o que a
// escolha significa para o app.
type permissionChoice struct {
	id    string
	label string
	kind  string
}

// always é a escolha que autoriza daqui em diante, e não só desta vez.
func (c permissionChoice) always() bool {
	return c.kind == optionAllowAlways
}

type permissionChoices []permissionChoice

func (c permissionChoices) labels() []string {
	out := make([]string, 0, len(c))
	for _, choice := range c {
		out = append(out, choice.label)
	}
	return out
}

// byLabel reencontra a opção pelo rótulo escolhido. É por rótulo porque é o
// que o questionário devolve — daí os rótulos precisarem ser distintos.
func (c permissionChoices) byLabel(label string) (permissionChoice, bool) {
	for _, choice := range c {
		if choice.label == label {
			return choice, true
		}
	}
	return permissionChoice{}, false
}

// hasAlways diz se o agente ofereceu autorizar para sempre. Só então o diálogo
// explica o que "sempre" abrange.
func (c permissionChoices) hasAlways() bool {
	for _, choice := range c {
		if choice.always() {
			return true
		}
	}
	return false
}

// approval escolhe como responder "pode" a um pedido que o perfil já autorizou.
// Prefere a permissão pontual: a autorização permanente é do app, e é ele quem
// a repete a cada pedido — dizer "sempre" ao agente todas as vezes o faria
// guardar do lado dele uma decisão que a pessoa pode revogar aqui.
func (c permissionChoices) approval() (string, bool) {
	for _, choice := range c {
		if choice.kind == optionAllowOnce {
			return choice.id, true
		}
	}
	for _, choice := range c {
		if choice.always() {
			return choice.id, true
		}
	}
	return "", false
}

// permissionChoices prepara as opções que o agente ofereceu. Os rótulos são
// dele, então passam pelo saneamento; o que não tiver identificador fica de
// fora, porque não haveria o que responder ao escolhê-lo.
//
// Rótulo repetido ganha o identificador junto: a escolha volta como texto, e
// duas opções com o mesmo nome fariam a resposta apontar sempre para a
// primeira — autorizar sempre no lugar de autorizar uma vez, por exemplo.
func permissionChoicesFrom(options []acp.PermissionOption) permissionChoices {
	out := make(permissionChoices, 0, len(options))
	seen := make(map[string]bool, len(options))
	for _, option := range options {
		if strings.TrimSpace(option.ID) == "" {
			continue
		}
		label := acp.SanitizeLabel(option.Name)
		if label == "" {
			label = permissionKindLabel(option.Kind)
		}
		label = distinctLabel(seen, label, option.ID)
		seen[label] = true
		out = append(out, permissionChoice{
			id:    option.ID,
			label: label,
			kind:  strings.ToLower(strings.TrimSpace(option.Kind)),
		})
	}
	return out
}

// distinctLabel devolve um rótulo que ainda não esteja na lista. O
// identificador é o primeiro desempate por ser o que informa, e vira texto na
// tela: passa pelo saneamento como o resto do que o agente manda. O que volta
// a ele continua sendo o identificador cru — o transporte só aceita a opção
// escrita como foi oferecida.
//
// Quando nem o identificador desempata (dois que saneiam igual, ou que somem
// no saneamento), o número resolve. Deixar a opção de fora seria tirar da
// pessoa uma escolha que o agente ofereceu — inclusive a de autorizar.
func distinctLabel(seen map[string]bool, label, id string) string {
	if !seen[label] {
		return label
	}
	if suffix := acp.SanitizeLabel(id); suffix != "" {
		if candidate := fmt.Sprintf("%s (%s)", label, suffix); !seen[candidate] {
			return candidate
		}
	}
	for n := 2; ; n++ {
		if candidate := fmt.Sprintf("%s (%d)", label, n); !seen[candidate] {
			return candidate
		}
	}
}

// permissionKindLabel nomeia a opção quando o agente não mandou rótulo. A
// classe é enumerável, ao contrário do nome, e é o que sobra para a pessoa
// entender o que está escolhendo.
func permissionKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "allow_once":
		return "Permitir uma vez"
	case "allow_always":
		return "Permitir sempre"
	case "reject_once":
		return "Negar uma vez"
	case "reject_always":
		return "Negar sempre"
	default:
		return "Opção sem nome"
	}
}
