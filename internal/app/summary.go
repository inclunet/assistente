package app

// summary.go — thin adapter: delega toda a lógica de sumarização para internal/summarization.
//
// checkAndTriggerSummarization é chamada após cada resposta do LLM para verificar
// se a conversa precisa ser resumida (em background, sem bloquear o usuário).
func (a *App) checkAndTriggerSummarization(conversationID uint) {
	a.summarySvc.CheckAndTriggerSummarization(conversationID)
}
