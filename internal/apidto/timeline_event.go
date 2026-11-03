package apidto

const TimelineEventName = "chat:timeline"

// TimelineEvent é endereçado à inscrição, não a uma aba implícita. A revisão
// do snapshot permite descartar entregas em trânsito de uma consulta substituída.
type TimelineEvent struct {
	ConversationID   string           `json:"conversationId"`
	WindowID         string           `json:"windowId"`
	Epoch            string           `json:"epoch"`
	SnapshotRevision string           `json:"snapshotRevision"`
	SyncRequired     bool             `json:"syncRequired"`
	Reason           string           `json:"reason,omitempty"`
	Updates          []TimelineUpdate `json:"updates"`
}
