package apidto

type OpenTimelineRequest struct {
	ConversationID string  `json:"conversationId"`
	ScopeParentID  *string `json:"scopeParentId"`
	SurfaceID      string  `json:"surfaceId"`
	TimelineWindowPosition
}

type RefreshTimelineRequest struct {
	WindowRevision string                  `json:"windowRevision"`
	Position       *TimelineWindowPosition `json:"position"`
}

type ReplayTimelineRequest struct {
	Epoch          string `json:"epoch"`
	WindowRevision string `json:"windowRevision"`
	AfterSequence  string `json:"afterSequence"`
}

type TimelineReplay struct {
	ConversationID string           `json:"conversationId"`
	WindowID       string           `json:"windowId"`
	SyncRequired   bool             `json:"syncRequired"`
	Updates        []TimelineUpdate `json:"updates"`
}

// TimelineUpdate expõe somente o lote já roteado à inscrição. Os índices
// internos usados pelo journal não fazem parte do protocolo de transporte.
type TimelineUpdate struct {
	ConversationID string                `json:"conversationId"`
	Epoch          string                `json:"epoch"`
	Sequence       string                `json:"sequence"`
	Operations     []CanonicalOperation  `json:"operations"`
	WindowPatches  []TimelineWindowPatch `json:"windowPatches"`
}

type TimelineWindowPatch struct {
	WindowID         string                  `json:"windowId"`
	PreviousRevision string                  `json:"previousRevision"`
	Revision         string                  `json:"revision"`
	Splices          []TimelineWindowSplice  `json:"splices,omitempty"`
	Items            []CanonicalItem         `json:"items,omitempty"`
	Metadata         *TimelineWindowMetadata `json:"metadata,omitempty"`
	SyncRequired     bool                    `json:"syncRequired,omitempty"`
}
