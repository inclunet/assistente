package chat

import "assistente/internal/toolinvocations"

func ToolInvocationSummariesToTurnSegments(summaries map[string][]toolinvocations.Summary) map[string][]TurnSegmentToolCall {
	out := make(map[string][]TurnSegmentToolCall, len(summaries))
	for turnID, calls := range summaries {
		for _, call := range calls {
			out[turnID] = append(out[turnID], TurnSegmentToolCall{
				InvocationID:       call.InvocationID,
				ID:                 call.CallID,
				Name:               call.Name,
				Origin:             call.Origin,
				ServerLabel:        call.ServerLabel,
				Status:             call.Status,
				Iteration:          call.Iteration,
				DurationMs:         call.DurationMs,
				InputPreview:       call.InputPreview,
				OutputPreview:      call.OutputPreview,
				InputBytes:         call.InputBytes,
				OutputBytes:        call.OutputBytes,
				HasDetails:         call.HasDetails,
				ResultAvailability: call.ResultAvailability,
				HasSearchResults:   call.HasSearchResults,
				SearchResultCount:  call.SearchResultCount,
				SecurityOutcome:    call.SecurityOutcome,
				AssistantMessageID: call.AssistantMessageID,
				ACPTextOffset:      call.ACPTextOffset,
			})
		}
	}
	return out
}
