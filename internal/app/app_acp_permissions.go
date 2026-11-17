package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/acptrust"
	"assistente/internal/authorization"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/logging"
	"assistente/internal/questionnaire"
	"assistente/internal/tools/invocationctx"
)

const acpPermissionComponent = "app.acp-permissions"

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
	// surfaces roteia a pergunta para a superfície de origem da conversa: o
	// questionário do desktop ou o canal de onde a conversa veio (AEP-0084
	// Fase 5). É o mesmo roteador que as confirmações do app usam, e ele já
	// resolve as duas pontas na hora do uso.
	surfaces *questionnaire.Router
	// origin descobre de qual canal a conversa veio, para o turno que não tem
	// tela. Sem isso o pedido de um canal não teria onde ser perguntado.
	origin func(acp.TurnOwner) questionnaire.Surface
	// notices leva à conversa o aviso do que foi negado sem decisão de
	// ninguém.
	notices func() ports.Emitter
	// trust guarda concessões locais vinculadas à origem capturada do turno.
	trust func() *acptrust.Store
}

var _ acp.RequestHandler = (*acpRequestHandler)(nil)

// RequestPermission leva o pedido do agente a quem pode decidir. Sem decisão
// — ninguém para perguntar, prazo estourado, diálogo recusado — devolve a
// escolha vazia, e o transporte responde ao agente a recusa pontual que ele
// mesmo ofereceu. Nunca deixa o pedido sem resposta: um turno pendurado é pior
// do que uma ação negada.
func (h *acpRequestHandler) RequestPermission(ctx context.Context, req acp.PermissionRequest) acp.PermissionOutcome {
	owner, known := h.turnOwner(req.SessionID)
	surface := h.surfaceOf(owner, known)
	if !surface.HasInterlocutor() {
		h.notifyConversation(owner, ports.ChatNoticeKindPermissionNoWatcher, req.ToolCall.Kind)
		return acp.PermissionOutcome{}
	}
	// Só uma resposta pontual do protocolo pode representar a concessão local.
	approval := ""
	rejection := ""
	for _, option := range req.Options {
		if option.Kind == "reject_once" && strings.TrimSpace(option.ID) != "" {
			rejection = option.ID
			break
		}
	}
	for _, option := range req.Options {
		if option.Kind == "allow_once" && strings.TrimSpace(option.ID) != "" {
			approval = option.ID
			break
		}
	}
	if approval == "" {
		h.notifyConversation(owner, ports.ChatNoticeKindPermissionUnavailable, req.ToolCall.Kind)
		return acp.PermissionOutcome{}
	}
	ctx = database.WithUserID(ctx, owner.UserID)
	ctx = invocationctx.With(ctx, invocationctx.InvocationContext{ConversationID: owner.ConversationID, ProfileSlug: owner.ProfileSlug, Source: "acp"})
	ctx = invocationctx.WithWorkspace(ctx, owner.Workspace)
	store := h.trustStore()
	if store != nil {
		ctx = store.Capture(ctx)
	}
	resource := acptrust.Resource{Kind: acp.ToolKind(req.ToolCall.Kind), AgentID: owner.AgentID}
	scopes := []authorization.Scope{authorization.ScopeOnce}
	if surface.AllowsPersistentAuthorization() && store != nil && owner.AgentID != "" {
		var err error
		scopes, err = store.AvailableScopes(ctx)
		if err != nil {
			h.notifyConversation(owner, ports.ChatNoticeKindPermissionUnavailable, req.ToolCall.Kind)
			return acp.PermissionOutcome{}
		}
	}
	actions := questionnaire.AuthorizationActions{Scopes: scopes, PersistentShortcut: authorization.ScopeProfile}
	explicitDenial := false
	action := acp.SanitizeContent(req.ToolCall.Title)
	if strings.TrimSpace(action) == "" {
		action = "(o agente não descreveu a ação)"
	}
	flow := authorization.Flow[acptrust.Entry]{
		Evaluate: func(ctx context.Context) (authorization.Decision, error) {
			if surface.AllowsPersistentAuthorization() && store != nil {
				return store.Match(ctx, resource)
			}
			return authorization.Decision{Effect: "ask"}, nil
		},
		Ask: func(ctx context.Context) (authorization.Consent[acptrust.Entry], error) {
			response, err := h.askOnSurface(ctx, surface, questionnaire.RequestPayload{
				Kind:        questionnaire.KindDecision,
				Title:       questionnaire.Keyed(permissionTextKey("title"), "O agente pede permissão"),
				Description: questionnaire.Keyed(permissionTextKey("description."+permissionKindKey(resource.Kind)), permissionDescription(resource.Kind)),
				Hint:        questionnaire.KeyedWith(permissionTextKey("localScope"), map[string]any{"agent": owner.AgentID, "kind": resource.Kind}, fmt.Sprintf("Agente: %s. Ao lembrar, a autorização vale para toda a classe %s no escopo escolhido, até revogar. Não autoriza ferramentas nativas do aplicativo.", owner.AgentID, resource.Kind)),
				Body:        action,
				BodyLabel:   questionnaire.Keyed(permissionTextKey("bodyLabel"), "Ação solicitada"),
				AllowCancel: true, Actions: actions.Actions(),
			})
			if err != nil {
				return authorization.Consent[acptrust.Entry]{}, err
			}
			choice, err := actions.Parse(response)
			explicitDenial = err == nil && !response.Cancelled && choice.Effect == "deny"
			return authorization.Consent[acptrust.Entry]{Effect: choice.Effect, Scope: choice.Scope, Value: acptrust.Entry{Kind: resource.Kind, AgentID: resource.AgentID, Scope: choice.Scope}}, err
		},
		PersistenceFailed: func(ctx context.Context, _ authorization.Request, err error) {
			logging.Warnf(ctx, acpPermissionComponent, "Falha ao lembrar autorização ACP: %v", err)
			h.notifyConversation(owner, ports.ChatNoticeKindPermissionAlwaysNotSaved, resource.Kind)
		},
	}
	if store != nil {
		flow.Remember = func(ctx context.Context, choice authorization.Consent[acptrust.Entry]) error {
			err := store.Grant(ctx, choice.Value)
			if err == nil {
				h.notifyConversation(owner, ports.ChatNoticeKindPermissionAlwaysAllowed, resource.Kind)
			}
			return err
		}
	}
	decision, err := authorization.Resolve(ctx, authorization.Request{Domain: "acp", Operation: resource.Kind, Resource: resource.Kind, AvailableScopes: scopes}, flow)
	if err != nil {
		logging.Warnf(ctx, acpPermissionComponent, "Falha na autorização ACP (%s): %v", permissionLogSummary(req.ToolCall), err)
		if notice := permissionFailureNotice(ctx, err); notice != "" {
			h.notifyConversation(owner, notice, resource.Kind)
		}
		return acp.PermissionOutcome{}
	}
	if decision.Effect != "allow" {
		if explicitDenial {
			return acp.PermissionOutcome{OptionID: rejection}
		}
		return acp.PermissionOutcome{}
	}
	return acp.PermissionOutcome{OptionID: approval}
}

// undecidedCause é por que um diálogo do agente acabou sem decisão de ninguém.
// A classificação é a mesma para permissão, pergunta e plano; o que muda é o
// aviso e o desfecho que cada método aceita.
type undecidedCause int

const (
	// causeCancelled é o turno interrompido antes da resposta: quase sempre
	// porque a pessoa cancelou, mas também quando a conversa fecha ou o app
	// encerra. O que os une é que ninguém está mais esperando a decisão.
	causeCancelled undecidedCause = iota
	// causeNoInterlocutor é a conversa que não tinha onde perguntar.
	causeNoInterlocutor
	// causeUnavailable é a superfície que não conseguiu apresentar a pergunta:
	// diálogo fora do ar, pergunta que não cabe numa mensagem, canal que não
	// entregou. A pergunta não chegou a aparecer.
	causeUnavailable
	// causeNoAnswer é a pergunta que apareceu e ninguém decidiu no prazo.
	causeNoAnswer
)

func undecidedCauseOf(ctx context.Context, err error) undecidedCause {
	switch {
	case turnCancelled(ctx, err):
		return causeCancelled
	case errors.Is(err, questionnaire.ErrNoInterlocutor):
		return causeNoInterlocutor
	case errors.Is(err, questionnaire.ErrAskerUnavailable):
		return causeUnavailable
	default:
		return causeNoAnswer
	}
}

// permissionFailureNotice escolhe o que contar à conversa quando o pedido acabou
// sem decisão de ninguém. Vazio quer dizer não avisar.
//
// Turno cancelado não vira aviso: foi a própria pessoa que desistiu, e o diálogo
// já saiu da tela dizendo isso. Avisar de novo seria cobrar explicação de quem
// acabou de dar uma. Dizer "ninguém respondeu a tempo" numa pergunta que nunca
// apareceu culparia quem não a viu.
func permissionFailureNotice(ctx context.Context, err error) string {
	switch undecidedCauseOf(ctx, err) {
	case causeCancelled:
		return ""
	case causeNoInterlocutor:
		return ports.ChatNoticeKindPermissionNoWatcher
	case causeUnavailable:
		return ports.ChatNoticeKindPermissionUnavailable
	default:
		return ports.ChatNoticeKindPermissionTimeout
	}
}

// surfaceOf diz onde perguntar sobre este turno. Turno com tela pergunta na
// tela; sem ela, a conversa de canal pergunta por mensagem, e o resto — job,
// subagente, CLI — não tem a quem perguntar (AEP-0084 Fase 5).
func (h *acpRequestHandler) surfaceOf(owner acp.TurnOwner, known bool) questionnaire.Surface {
	if !known {
		// Sessão sem turno anotado: não se sabe nem de que conversa é o pedido.
		return questionnaire.Surface{}
	}
	if owner.Interactive {
		return questionnaire.DesktopSurface(owner.ConversationID)
	}
	if h == nil || h.origin == nil {
		return questionnaire.NoSurface(owner.ConversationID)
	}
	return h.origin(owner)
}

// askOnSurface leva o diálogo à superfície de origem. Sem roteador não há como
// perguntar, e o erro é o mesmo de uma superfície que não sabe apresentar a
// pergunta: ninguém decidiu.
func (h *acpRequestHandler) askOnSurface(ctx context.Context, surface questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	if h == nil {
		return questionnaire.Response{}, questionnaire.ErrAskerUnavailable
	}
	return h.surfaces.Ask(ctx, surface, payload)
}

func (h *acpRequestHandler) turnOwner(sessionID string) (acp.TurnOwner, bool) {
	if h == nil || h.owner == nil {
		return acp.TurnOwner{}, false
	}
	return h.owner(sessionID)
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

func (h *acpRequestHandler) trustStore() *acptrust.Store {
	if h == nil || h.trust == nil {
		return nil
	}
	return h.trust()
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
