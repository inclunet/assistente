package acp

import "strings"

// TurnOwner diz de quem é o turno em voo numa sessão do agente. O agente
// pergunta no meio do turno — permissão para editar arquivo, para rodar
// comando — e o pedido chega identificado só pela sessão dele. Sem saber a
// quem aquela sessão está respondendo, a camada que decide não teria como
// perguntar nem como saber se há alguém para perguntar (AEP-0084 D9).
type TurnOwner struct {
	// ConversationID é a conversa do app que pediu este turno.
	ConversationID string

	// Interactive é o turno que tem gente esperando numa tela capaz de
	// responder. Turno de job agendado, de subagente ou de canal não tem tela;
	// o de canal tem a própria conversa, e quem resolve essa superfície precisa
	// de UserID para chegar até ela (AEP-0084 D9, Fase 5).
	Interactive bool

	// UserID é o dono do turno. Ele é anotado aqui porque o pedido de permissão
	// chega depois, por outra goroutine e num contexto do transporte, onde o
	// escopo de usuário já não existe — e sem ele não há como descobrir de que
	// canal veio a conversa sem ler dados de outra pessoa (AEP-0052).
	UserID string

	// ProfileSlug é o perfil que pediu o turno, quando ele foi escolhido
	// explicitamente (canais, jobs). Vazio quer dizer o perfil ativo, e quem
	// precisa do slug resolve isso — é o mesmo acordo do resto do app. As
	// autorizações permanentes valem por perfil, então é por aqui que elas
	// encontram de quem são (AEP-0084 D9).
	ProfileSlug string
}

// BeginTurn anota que o turno desta conversa começou e devolve o encerramento.
// Vale para o turno inteiro, e não só para a chamada ao agente: o pedido de
// permissão chega enquanto o turno corre, por outra goroutine.
//
// Sem sessão montada não há o que anotar — e também não haverá pedido, porque
// nada foi mandado ao agente.
func (c *Conversation) BeginTurn(owner TurnOwner) (end func()) {
	c.mu.Lock()
	sessionID, providerID := "", ""
	if c.active != nil {
		sessionID = c.active.sessionID
		providerID = c.active.providerID
	}
	c.mu.Unlock()

	if strings.TrimSpace(sessionID) == "" {
		return func() {}
	}
	owner.ConversationID = c.id
	token := c.manager.setTurnOwner(sessionID, providerID, owner)
	return func() { c.manager.clearTurnOwner(sessionID, token) }
}

// TurnOwnerOf devolve quem espera o turno em voo naquela sessão do agente.
// Sessão sem turno é pedido fora de hora: ninguém está esperando resposta, e
// não há tela onde perguntar.
func (m *Manager) TurnOwnerOf(sessionID string) (TurnOwner, bool) {
	m.ownersMu.Lock()
	defer m.ownersMu.Unlock()
	entry, ok := m.owners[sessionID]
	if !ok {
		return TurnOwner{}, false
	}
	return entry.owner, true
}

// TurnInFlight diz se alguma conversa está com um turno correndo neste provider
// agora (AEP-0086 D10).
//
// Ela existe para a atualização do agente poder ser recusada durante conversa: o
// turno em voo está com o processo antigo, que edita arquivos, e trocar o
// binário debaixo dele é trocar o programa no meio de uma edição. A recusa é
// dita com o motivo, e não enfileirada em silêncio.
//
// A resposta é do instante em que foi lida, e não vale como reserva: um turno
// pode começar logo depois. É o suficiente para o caso que ela existe para
// pegar — a pessoa que manda uma mensagem e clica em atualizar em seguida —, e o
// que a atualização faz de fato é instalar ao lado, sem tocar no que está de pé.
//
// Quem responde é o registro do turno, e não a sessão montada na conversa: a
// conversa pode ser remontada no meio do turno — troca de diretório, processo
// que caiu — e passar a apontar para outra sessão enquanto o turno anterior
// ainda corre no processo antigo. Procurar pelo que está montado agora diria
// que ninguém está trabalhando justamente quando alguém está.
func (m *Manager) TurnInFlight(providerID string) bool {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return false
	}
	m.ownersMu.Lock()
	defer m.ownersMu.Unlock()
	for _, entry := range m.owners {
		if entry.providerID == providerID {
			return true
		}
	}
	return false
}

// turnRegistration é o dono do turno, o provider que o está atendendo e a marca
// de quem o anotou.
type turnRegistration struct {
	owner      TurnOwner
	providerID string
	token      uint64
}

func (m *Manager) setTurnOwner(sessionID, providerID string, owner TurnOwner) uint64 {
	m.ownersMu.Lock()
	defer m.ownersMu.Unlock()
	if m.owners == nil {
		m.owners = make(map[string]turnRegistration)
	}
	m.ownerToken++
	m.owners[sessionID] = turnRegistration{
		owner:      owner,
		providerID: strings.TrimSpace(providerID),
		token:      m.ownerToken,
	}
	return m.ownerToken
}

// clearTurnOwner esquece o turno, se o que estiver anotado ainda for aquele. A
// marca importa por causa do barge-in: mandar mensagem nova cancela o turno
// anterior, e os dois convivem por um instante na mesma sessão. Sem conferir,
// o turno que está saindo apagaria o dono do que acabou de entrar — e o pedido
// de permissão do novo seria negado como se ninguém estivesse esperando.
func (m *Manager) clearTurnOwner(sessionID string, token uint64) {
	m.ownersMu.Lock()
	defer m.ownersMu.Unlock()
	if entry, ok := m.owners[sessionID]; ok && entry.token == token {
		delete(m.owners, sessionID)
	}
}
