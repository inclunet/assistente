package apidto

const TimelineEventName = "chat:timeline"

const TimelineClosedEventName = "chat:timeline_closed"

// Controle terminal sem conteúdo. Não concede leitura nem renova autorização.
type TimelineClosedEvent struct {
	ConversationID string `json:"conversationId"`
	SurfaceID      string `json:"surfaceId"`
	WindowID       string `json:"windowId"`
}

// TimelineEvent é endereçado à inscrição, não a uma aba implícita. A revisão
// do snapshot permite descartar entregas em trânsito de uma consulta substituída.
type TimelineEvent struct {
	ConversationID   string           `json:"conversationId"`
	SurfaceID        string           `json:"surfaceId"`
	WindowID         string           `json:"windowId"`
	Epoch            string           `json:"epoch"`
	SnapshotRevision string           `json:"snapshotRevision"`
	SyncRequired     bool             `json:"syncRequired"`
	Reason           string           `json:"reason,omitempty"`
	Updates          []TimelineUpdate `json:"updates"`
}
