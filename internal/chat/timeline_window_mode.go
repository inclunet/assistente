package chat

import "assistente/internal/database"

// O modo pertence à inscrição e muda somente após Navigate ou Refresh aceito.
type TimelineWindowMode string

const (
	TimelineWindowHistorical TimelineWindowMode = "historical"
	TimelineWindowFollowTail TimelineWindowMode = "followTail"
)

func navigationMode(request TimelineWindowNavigation) (TimelineWindowMode, error) {
	mode := request.Mode
	if mode == "" {
		mode = TimelineWindowHistorical
	}
	if mode != TimelineWindowHistorical && mode != TimelineWindowFollowTail {
		return "", ErrTimelineSubscription
	}
	if mode == TimelineWindowFollowTail && (request.Cursor != "" || request.AnchorItemID != "" || (request.Anchor != "" && request.Anchor != "end") || (request.Direction != "" && request.Direction != "before") || request.Limit <= 0 || request.Limit > database.MaxMessageWindowRows) {
		return "", ErrTimelineSubscription
	}
	return mode, nil
}
