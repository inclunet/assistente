package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/logging"
	"assistente/internal/questionnaire"
)

const acpPermissionComponent = "app.acp-permissions"

// permissionAnswerID é a chave da escolha dentro do questionário.
const permissionAnswerID = "decision"

// acpRequestHandler responde ao que o agente de código pergunta ao app
// (AEP-0084 D9). Hoje isso é o pedido de permissão para agir na máquina; as
// extensões bloqueantes do Cursor ainda não são tratadas, e o transporte
// responde por elas que o método não existe.
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
}

var _ acp.RequestHandler = (*acpRequestHandler)(nil)

// RequestPermission leva o pedido do agente a quem pode decidir. Sem decisão
// — ninguém para perguntar, prazo estourado, diálogo recusado — devolve a
// escolha vazia, e o transporte responde ao agente a recusa pontual que ele
// mesmo ofereceu. Nunca deixa o pedido sem resposta: um turno pendurado é pior
// do que uma ação negada.
func (h *acpRequestHandler) RequestPermission(ctx context.Context, req acp.PermissionRequest) acp.PermissionOutcome {
	action := acp.SanitizeLabel(req.ToolCall.Title)

	owner, ok := h.turnOwner(req.SessionID)
	if !ok || !owner.Interactive {
		// Turno de canal, job, subagente ou CLI: não há tela onde perguntar.
		// Esperar aqui penduraria o agente até o teto do transporte.
		logging.Infof(ctx, acpPermissionComponent,
			"[ACP] permissão negada na hora, sem ninguém a quem perguntar (sessão %q, conversa %q): %s",
			req.SessionID, owner.ConversationID, action)
		return acp.PermissionOutcome{}
	}

	choices := permissionChoicesFrom(req.Options)
	if len(choices) == 0 {
		// Pedido sem opção nenhuma: não há o que oferecer à pessoa, e inventar
		// uma resposta seria decidir por ela.
		logging.Warnf(ctx, acpPermissionComponent, "[ACP] pedido de permissão sem opções: %s", action)
		return acp.PermissionOutcome{}
	}

	manager := h.questionnaireManager()
	if manager == nil {
		logging.Warnf(ctx, acpPermissionComponent, "[ACP] permissão negada: o questionário não está disponível")
		return acp.PermissionOutcome{}
	}

	// Sem prazo próprio: vale o do questionário, que é o do desktop e cabe
	// dentro do teto que o transporte impõe ao handler. Um prazo maior que o
	// teto tiraria da pessoa a chance de responder (AEP-0084 D9).
	resp, err := manager.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       "O agente pede permissão",
		Description: permissionDescription(action, req.ToolCall.Kind),
		AllowCancel: true,
		SubmitLabel: "Confirmar",
		CancelLabel: "Negar",
		Questions: []questionnaire.Question{{
			ID:        permissionAnswerID,
			Type:      "single_choice",
			Prompt:    "O que o agente pode fazer?",
			Options:   choices.labels(),
			Required:  true,
			AutoFocus: true,
		}},
	})
	if err != nil {
		// Prazo estourado, turno cancelado ou app encerrando. Todos viram a
		// mesma coisa para o agente: ninguém autorizou.
		logging.Infof(ctx, acpPermissionComponent,
			"[ACP] permissão sem resposta na conversa %q (%s): %v", owner.ConversationID, action, err)
		return acp.PermissionOutcome{}
	}
	if resp.Cancelled {
		return acp.PermissionOutcome{}
	}

	label, _ := resp.Answers[permissionAnswerID].(string)
	id, ok := choices.optionID(label)
	if !ok {
		logging.Warnf(ctx, acpPermissionComponent, "[ACP] resposta de permissão fora das opções oferecidas; negando")
		return acp.PermissionOutcome{}
	}
	return acp.PermissionOutcome{OptionID: id}
}

// HandleCustom ainda não trata as extensões bloqueantes do Cursor. Devolver
// que não foram tratadas faz o transporte responder "método não encontrado",
// que desbloqueia o agente sem fingir suporte.
func (h *acpRequestHandler) HandleCustom(context.Context, string, json.RawMessage) (any, bool) {
	return nil, false
}

// CustomFallback não tem desfecho a oferecer enquanto nenhuma extensão é
// tratada aqui.
func (h *acpRequestHandler) CustomFallback(string) (any, bool) {
	return nil, false
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

// permissionDescription monta o texto do diálogo. Quem autoriza precisa ver o
// que está autorizando, e o que o agente manda é dado não confiável: o título
// costuma ser a linha de comando literal (AEP-0084 D9/D11).
func permissionDescription(action, kind string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "(o agente não descreveu a ação)"
	}
	if kind = strings.TrimSpace(acp.SanitizeLabel(kind)); kind != "" {
		return fmt.Sprintf("O agente quer executar uma ação do tipo %q:\n\n%s", kind, action)
	}
	return fmt.Sprintf("O agente quer executar:\n\n%s", action)
}

// permissionChoice é uma opção do pedido já pronta para a tela: o rótulo que
// aparece e o identificador que volta ao agente.
type permissionChoice struct {
	id    string
	label string
}

type permissionChoices []permissionChoice

func (c permissionChoices) labels() []string {
	out := make([]string, 0, len(c))
	for _, choice := range c {
		out = append(out, choice.label)
	}
	return out
}

// optionID reencontra a opção pelo rótulo escolhido. É por rótulo porque é o
// que o questionário devolve — daí os rótulos precisarem ser distintos.
func (c permissionChoices) optionID(label string) (string, bool) {
	for _, choice := range c {
		if choice.label == label {
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
		if seen[label] {
			// O identificador desempata o rótulo, e por isso vira texto na
			// tela: passa pelo mesmo saneamento do resto do que o agente
			// manda. O que volta a ele continua sendo o identificador cru —
			// o transporte só aceita a opção escrita como foi oferecida.
			label = fmt.Sprintf("%s (%s)", label, acp.SanitizeLabel(option.ID))
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, permissionChoice{id: option.ID, label: label})
	}
	return out
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
