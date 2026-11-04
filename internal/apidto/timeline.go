package apidto

// Contrato compartilhado da timeline canônica (AEP-0116 e AEP-0088 D5).
// IDs são opacos; revisões e sequências são strings decimais. A ordem pertence
// ao backend. Invocações contêm somente resumos, sem argumentos/resultados integrais.

type CanonicalItem struct {
	ExecutionID             string                     `json:"executionId,omitempty"`
	RecoveryReference       *TimelineRecoveryReference `json:"recoveryReference,omitempty"`
	ItemID                  string                     `json:"itemId"`
	ConversationID          string                     `json:"conversationId"`
	ScopeParentID           *string                    `json:"scopeParentId"`
	Kind                    string                     `json:"kind"`
	TurnID                  *string                    `json:"turnId"`
	ReplyToItemID           string                     `json:"replyToItemId"`
	MessageIDs              []string                   `json:"messageIds"`
	RepresentativeMessageID *string                    `json:"representativeMessageId"`
	Position                string                     `json:"position"`
	Revision                string                     `json:"revision"`
	State                   string                     `json:"state"`
	Durability              string                     `json:"durability"`
	Segments                []CanonicalSegment         `json:"segments"`
	Metadata                CanonicalMetadata          `json:"metadata"`
}

// Identifica os bytes de uma execução, não concede autorização nem confirma
// persistência. O cliente conserva a referência inteira sem interpretar seus IDs.
type TimelineRecoveryReference struct {
	ProjectionID string `json:"projectionId"`
	Incarnation  string `json:"incarnation"`
	TurnID       string `json:"turnId"`
	ExecutionID  string `json:"executionId"`
}

// Estado observado da execução; não é recibo de bytes nem permissão de descarte.
// unknown significa prova insuficiente, inclusive após recriação da projeção.
type TimelineRecoveryStatus struct {
	ConversationID string                    `json:"conversationId"`
	Reference      TimelineRecoveryReference `json:"reference"`
	State          string                    `json:"state"` // active, retained, persisted, unknown
}

type TimelineRecoveryPageRequest struct {
	Reference TimelineRecoveryReference `json:"reference"`
	Cursor    string                    `json:"cursor"`
}

// Página de buffers em memória. Found=false não confirma descarte. A versão
// identifica bytes observados, não garante retenção futura nem transferência.
type TimelineRecoveryPage struct {
	ConversationID string                    `json:"conversationId"`
	Reference      TimelineRecoveryReference `json:"reference"`
	Found          bool                      `json:"found"`
	ContentVersion string                    `json:"contentVersion"`
	Chunks         []TimelineRecoveryChunk   `json:"chunks"`
	NextCursor     string                    `json:"nextCursor"`
}

// ByteOffset refere-se aos bytes UTF-8 do buffer identificado por iteração,
// geração, parte e kind. O leitor não deve reinterpretá-lo como índice UTF-16.
type TimelineRecoveryChunk struct {
	Iteration  int    `json:"iteration"`
	Generation string `json:"generation"`
	Part       int    `json:"part"`
	Kind       string `json:"kind"`
	ByteOffset int    `json:"byteOffset"`
	Text       string `json:"text"`
}

type CanonicalSegment struct {
	SegmentID   string                `json:"segmentId"`
	Kind        string                `json:"kind"`
	Text        string                `json:"text,omitempty"`
	Invocations []TurnSegmentToolCall `json:"invocations,omitempty"`
}

type CanonicalMetadata struct {
	Origin           string `json:"origin"`
	Role             string `json:"role"`
	Pinned           bool   `json:"pinned"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
}

type CanonicalOperation struct {
	Kind      string            `json:"kind"`
	ItemID    string            `json:"itemId"`
	Revision  string            `json:"revision"`
	Item      *CanonicalItem    `json:"item,omitempty"`
	SegmentID string            `json:"segmentId,omitempty"`
	Text      string            `json:"text,omitempty"`
	Segment   *CanonicalSegment `json:"segment,omitempty"`
}

type TurnSegmentToolCall struct {
	ExecutionID        string `json:"-"`
	ExecutionDepth     int    `json:"-"`
	InvocationID       string `json:"invocationId,omitempty"`
	ID                 string `json:"callId"`
	Name               string `json:"name"`
	Origin             string `json:"origin,omitempty"`
	ServerLabel        string `json:"serverLabel,omitempty"`
	Status             string `json:"status"`
	Iteration          int    `json:"iteration,omitempty"`
	DurationMs         int64  `json:"durationMs,omitempty"`
	InputPreview       string `json:"inputPreview,omitempty"`
	OutputPreview      string `json:"outputPreview,omitempty"`
	InputBytes         int64  `json:"inputBytes,omitempty"`
	OutputBytes        int64  `json:"outputBytes,omitempty"`
	HasDetails         bool   `json:"hasDetails"`
	ResultAvailability string `json:"resultAvailability"`
	HasSearchResults   bool   `json:"hasSearchResults,omitempty"`
	SearchResultCount  int    `json:"searchResultCount,omitempty"`
	SecurityOutcome    string `json:"securityOutcome,omitempty"`
	// AssistantMessageID é metadado interno de hidratação para associar a
	// invocação à mensagem assistant que representou a iteração.
	AssistantMessageID string `json:"-"`
	ACPTextOffset      *int   `json:"-"`
}

type TimelineWindowMetadata struct {
	RetainedItemIDs  []string `json:"retainedItemIds"`
	RetentionBlocked bool     `json:"retentionBlocked"`
	TotalCount       int      `json:"totalCount"`
	StartIndex       int      `json:"startIndex"`
	EndIndex         int      `json:"endIndex"`
	HasBefore        bool     `json:"hasBefore"`
	HasAfter         bool     `json:"hasAfter"`
	BeforeCursor     string   `json:"beforeCursor"`
	AfterCursor      string   `json:"afterCursor"`
}

type TimelineWindowSplice struct {
	Index       int      `json:"index"`
	DeleteCount int      `json:"deleteCount"`
	InsertIDs   []string `json:"insertIds"`
}

type SubscribedTimelineSnapshot struct {
	ConversationID   string             `json:"conversationId"`
	ScopeParentID    *string            `json:"scopeParentId"`
	RetainedItemIDs  []string           `json:"retainedItemIds"`
	RetentionBlocked bool               `json:"retentionBlocked"`
	Mode             TimelineWindowMode `json:"mode"`
	WindowID         string             `json:"windowId"`
	WindowRevision   string             `json:"windowRevision"`
	Epoch            string             `json:"epoch"`
	Sequence         string             `json:"sequence"`
	Items            []CanonicalItem    `json:"items"`
	OrderedItemIDs   []string           `json:"orderedItemIds"`
	TotalCount       int                `json:"totalCount"`
	StartIndex       int                `json:"startIndex"`
	EndIndex         int                `json:"endIndex"`
	HasBefore        bool               `json:"hasBefore"`
	HasAfter         bool               `json:"hasAfter"`
	BeforeCursor     string             `json:"beforeCursor"`
	AfterCursor      string             `json:"afterCursor"`
}

type TimelineWindowNavigation struct {
	Mode           TimelineWindowMode `json:"mode"`
	Epoch          string             `json:"epoch"`
	WindowRevision string             `json:"windowRevision"`
	TimelineWindowPosition
}

type TimelineWindowPosition struct {
	Cursor       string `json:"cursor"`
	Anchor       string `json:"anchor"`
	AnchorItemID string `json:"anchorItemId"`
	Direction    string `json:"direction"`
	Limit        int    `json:"limit"`
}

type TimelineRetentionRequest struct {
	Epoch          string   `json:"epoch"`
	WindowRevision string   `json:"windowRevision"`
	ItemIDs        []string `json:"itemIds"`
}

type TimelineWindowMode string

const (
	TimelineWindowHistorical TimelineWindowMode = "historical"
	TimelineWindowFollowTail TimelineWindowMode = "followTail"
)

type TimelineRecoveryReservationRequest struct {
	Reference      TimelineRecoveryReference `json:"reference"`
	ContentVersion string                    `json:"contentVersion"`
}

// Reserva de memória da capability, não recibo de persistência. Termina em
// Release, Close, revogação/invalidação da janela ou descarte explícito.
type TimelineRecoveryReservation struct {
	ConversationID string                    `json:"conversationId"`
	Reference      TimelineRecoveryReference `json:"reference"`
	ContentVersion string                    `json:"contentVersion"`
}

// Identidade exata de um DTO publicado; revisão de item não é versão dos buffers.
type TimelineRecoveryItemRequest struct {
	Reference TimelineRecoveryReference `json:"reference"`
	ItemID    string                    `json:"itemId"`
	Epoch     string                    `json:"epoch"`
	Revision  string                    `json:"revision"`
}

type TimelineRecoveryItemReservation struct {
	ConversationID string                      `json:"conversationId"`
	Identity       TimelineRecoveryItemRequest `json:"identity"`
}
