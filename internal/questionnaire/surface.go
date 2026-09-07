package questionnaire

import (
	"context"
	"errors"
	"strings"
	"time"
)

// A pergunta que o backend faz não pressupõe uma tela. O mesmo pipeline de
// envio atende o app desktop, os canais (Telegram, Signal, Slack), jobs
// agendados, subagentes e a CLI — e um diálogo emitido para o Wails numa
// conversa de canal fica vinte minutos esperando alguém que não está lá,
// enquanto o turno do agente de código não anda (AEP-0084 D9, Fase 5).
//
// Surface diz de onde veio a conversa e Router leva a pergunta para lá. O
// contrato é o mesmo dos diálogos de sempre — RequestPayload entra, Response
// volta —, e é por isso que ele serve às confirmações que já existem (shell,
// HTTP mutável, edição de arquivo) sem que nenhuma delas precise saber onde a
// pessoa está.

// SurfaceKind é o tipo da superfície de origem da conversa.
type SurfaceKind string

const (
	// SurfaceNone é a conversa sem interlocutor: job agendado, subagente, CLI
	// não interativa. É o zero-value de propósito — quem não souber dizer de
	// onde a conversa veio cai no desfecho seguro, que é não esperar por
	// ninguém.
	SurfaceNone SurfaceKind = ""
	// SurfaceDesktop é a tela do app, onde o questionário de sempre abre.
	SurfaceDesktop SurfaceKind = "desktop"
	// SurfaceChannel é a conversa de um mensageiro, onde a pergunta vira
	// mensagem.
	SurfaceChannel SurfaceKind = "channel"
)

// ChannelTimeout é o prazo de uma pergunta feita em canal: minutos, não os
// vinte do desktop (AEP-0084 D9). Na tela a pessoa está diante do diálogo;
// numa mensagem ela pode estar com o telefone no bolso, e o turno do agente
// fica parado até alguém responder. O prazo cabe com folga dentro do teto que
// o transporte do agente impõe a quem decide, para que ele nunca corte antes
// da chance de responder.
const ChannelTimeout = 3 * time.Minute

// Surface descreve a superfície de origem da conversa: onde perguntar, quem
// pode responder e o que uma resposta dali tem direito de decidir.
type Surface struct {
	// Kind diz que superfície é. O zero-value é SurfaceNone.
	Kind SurfaceKind
	// ConversationID é a conversa do app dona da pergunta. Vai preenchido
	// mesmo em SurfaceNone: é por ela que o aviso do que foi negado encontra
	// a conversa.
	ConversationID string
	// Channel é o mensageiro de origem, quando Kind é SurfaceChannel.
	Channel string
	// ContactID é quem conversa do outro lado do canal — o dono daquela
	// conversa e o único de quem uma resposta vale (AEP-0084 D9).
	ContactID string
}

// DesktopSurface é a conversa que está numa tela do app.
func DesktopSurface(conversationID string) Surface {
	return Surface{Kind: SurfaceDesktop, ConversationID: strings.TrimSpace(conversationID)}
}

// ChannelSurface é a conversa de um mensageiro. Sem canal ou sem contato não
// há para onde mandar a pergunta nem de quem aceitar a resposta, e o que sobra
// é uma conversa sem interlocutor — que nega na hora em vez de esperar.
func ChannelSurface(conversationID, channel, contactID string) Surface {
	surface := Surface{
		Kind:           SurfaceChannel,
		ConversationID: strings.TrimSpace(conversationID),
		Channel:        strings.TrimSpace(channel),
		ContactID:      strings.TrimSpace(contactID),
	}
	if surface.ConversationID == "" || surface.Channel == "" || surface.ContactID == "" {
		return NoSurface(conversationID)
	}
	return surface
}

// NoSurface é a conversa sem ninguém para responder.
func NoSurface(conversationID string) Surface {
	return Surface{Kind: SurfaceNone, ConversationID: strings.TrimSpace(conversationID)}
}

// HasInterlocutor diz se existe alguém a quem perguntar. Sem interlocutor a
// pergunta não é feita: o desfecho negativo sai na hora, porque esperar por
// quem não existe é pendurar o turno até o prazo estourar.
func (s Surface) HasInterlocutor() bool {
	return s.Kind == SurfaceDesktop || s.Kind == SurfaceChannel
}

// AllowsPersistentAuthorization diz se uma decisão vinda desta superfície pode
// virar autorização permanente. Só o desktop pode: autorizar para sempre por
// mensagem de texto amplia execução silenciosa futura a partir de um canal
// remoto, e o ganho não paga o risco (AEP-0084 D9). Pelo mesmo motivo é só ali
// que a autorização já concedida é consultada — senão um canal colheria o sim
// que alguém deu na tela.
func (s Surface) AllowsPersistentAuthorization() bool {
	return s.Kind == SurfaceDesktop
}

var (
	// ErrNoInterlocutor é a pergunta que não foi feita porque não havia
	// superfície onde fazê-la.
	ErrNoInterlocutor = errors.New("a conversa não tem superfície onde perguntar")
	// ErrAskerUnavailable é a superfície que existe mas não tem como
	// apresentar a pergunta agora — questionário fora do ar, canal sem
	// mecanismo de pergunta ligado. Diferente de ninguém ter respondido: aqui
	// a pergunta nem chegou a aparecer.
	ErrAskerUnavailable = errors.New("a superfície de origem não tem como apresentar a pergunta")
)

// ChannelAsker leva a pergunta à conversa de um mensageiro e devolve a
// resposta no mesmo formato do diálogo de tela. Quem implementa vive junto do
// canal, porque a pergunta sai pelo caminho de envio que já existe e a
// resposta entra pelo caminho de recebimento que já existe (AEP-0040).
type ChannelAsker interface {
	AskOnChannel(ctx context.Context, surface Surface, payload RequestPayload) (Response, error)
}

// Router escolhe a superfície onde a pergunta é feita. As dependências entram
// como função porque quem pergunta nasce antes delas: o serviço de agentes de
// código é criado cedo, e o questionário e o gateway de canais só existem
// depois — guardar o valor congelaria um nulo.
type Router struct {
	desktop func() *Manager
	channel func() ChannelAsker
}

// NewRouter monta o roteador sobre o questionário do desktop e o mecanismo de
// pergunta em canal. Qualquer um dos dois pode faltar: a superfície que não
// tiver como perguntar devolve ErrAskerUnavailable, e quem chamou responde o
// desfecho negativo do método dele.
func NewRouter(desktop func() *Manager, channel func() ChannelAsker) *Router {
	return &Router{desktop: desktop, channel: channel}
}

// Ask faz a pergunta na superfície de origem da conversa. Nunca bloqueia
// esperando quem não existe: sem interlocutor devolve ErrNoInterlocutor na
// hora, que é o que deixa quem chamou responder ao agente (ou à tool) o
// desfecho negativo em vez de pendurar o turno.
func (r *Router) Ask(ctx context.Context, surface Surface, payload RequestPayload) (Response, error) {
	if r == nil {
		return Response{}, ErrAskerUnavailable
	}
	switch surface.Kind {
	case SurfaceDesktop:
		manager := r.desktopManager()
		if manager == nil {
			return Response{}, ErrAskerUnavailable
		}
		return manager.RequestQuestionnaire(ctx, payload)
	case SurfaceChannel:
		asker := r.channelAsker()
		if asker == nil {
			return Response{}, ErrAskerUnavailable
		}
		return asker.AskOnChannel(ctx, surface, payload)
	default:
		return Response{}, ErrNoInterlocutor
	}
}

func (r *Router) desktopManager() *Manager {
	if r.desktop == nil {
		return nil
	}
	return r.desktop()
}

func (r *Router) channelAsker() ChannelAsker {
	if r.channel == nil {
		return nil
	}
	return r.channel()
}
