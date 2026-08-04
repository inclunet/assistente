package acp

// SameDir diz se dois caminhos apontam para o mesmo diretório, com a mesma
// regra que decide se a sessão de pé ainda serve. Exportada para que quem
// compara diretórios na tela chegue à mesma conclusão que a montagem da sessão:
// dizer "recriação pendente" onde a sessão não vai ser recriada é anunciar uma
// perda de memória que não vai acontecer.
func SameDir(a, b string) bool { return sameDir(a, b) }

// ConversationWorkDir é o diretório em que o agente desta conversa trabalha
// agora — o escolhido para ela, ou o do app quando não houve escolha
// (AEP-0084 D5).
//
// Resolvido do mesmo jeito que na montagem da sessão, e não lido cru: é este
// caminho que a barra da conversa mostra, e mostrar "./projeto" onde o agente
// recebeu "/casa/ana/projeto" faria a pessoa conferir o alcance dele por um
// texto que não é o que valeu.
//
// Nada é aberto aqui: saber onde o agente age não pode fazer nascer um processo.
func (m *Manager) ConversationWorkDir(conversationID string) (string, error) {
	return m.dirFor(conversationID)
}

// WorkDir é o diretório do app — o workspace ativo —, que é onde uma conversa
// sem escolha própria coloca o agente.
func (m *Manager) WorkDir() (string, error) {
	return m.currentDir()
}

// ConversationSessionDir é o diretório da sessão que esta conversa tem de pé,
// ou vazio quando ela não tem nenhuma. É por ele que se sabe que a escolha nova
// ainda não valeu: a sessão em pé continua falando dos arquivos de antes, e o
// diretório novo só vale quando ela for recriada.
func (m *Manager) ConversationSessionDir(conversationID string) string {
	conv := m.lookup(conversationID)
	if conv == nil {
		return ""
	}
	conv.mu.Lock()
	defer conv.mu.Unlock()
	if conv.active == nil || conv.active.session == nil {
		return ""
	}
	return conv.active.dir
}
