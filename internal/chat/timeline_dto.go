package chat

import "assistente/internal/apidto"

// Aliases preservam o domínio sem duplicar o contrato do transporte.
type CanonicalItem = apidto.CanonicalItem
type TimelineRecoveryReference = apidto.TimelineRecoveryReference
type CanonicalSegment = apidto.CanonicalSegment
type CanonicalMetadata = apidto.CanonicalMetadata
type CanonicalOperation = apidto.CanonicalOperation
type TurnSegmentToolCall = apidto.TurnSegmentToolCall
type TimelineWindowMetadata = apidto.TimelineWindowMetadata
type TimelineWindowSplice = apidto.TimelineWindowSplice
type SubscribedTimelineSnapshot = apidto.SubscribedTimelineSnapshot
type TimelineWindowNavigation = apidto.TimelineWindowNavigation
type TimelineWindowPosition = apidto.TimelineWindowPosition
type TimelineRetentionRequest = apidto.TimelineRetentionRequest
type TimelineWindowMode = apidto.TimelineWindowMode

const (
	TimelineWindowHistorical = apidto.TimelineWindowHistorical
	TimelineWindowFollowTail = apidto.TimelineWindowFollowTail
)
