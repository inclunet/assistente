package speech

import "assistente/internal/llmcatalog"

// Resolve the current catalog on each request. Unknown metadata preserves the
// user's explicit configuration; it never infers a mode from a model name.
func catalogSelectionMode(snapshot func() llmcatalog.Snapshot, serverProfileID, apiFormat string, configured TTSSelectionMode) func(string) TTSSelectionMode {
	return func(modelID string) TTSSelectionMode {
		if snapshot != nil {
			mode := snapshot().Document.ResolveTTSSelection(serverProfileID, apiFormat, modelID).SelectionMode
			if mode != "" {
				return TTSSelectionMode(mode)
			}
		}
		return configured
	}
}
