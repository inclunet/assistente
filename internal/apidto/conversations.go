package apidto

// ConversationSummaryInfo é o resumo rolling de uma conversa (borda Wails).
type ConversationSummaryInfo struct {
	Summary               string `json:"summary"`
	SummaryUpToMessageID  string `json:"summary_up_to_message_id"`
	SummarizingInProgress bool   `json:"summarizing_in_progress"`
}
