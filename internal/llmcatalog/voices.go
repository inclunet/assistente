package llmcatalog

// TTSSelection describes the voice selection contract for one exact exposure.
// An empty mode means unknown; it must not be inferred from the model ID.
type TTSSelection struct {
	SelectionMode string    `json:"selection_mode"`
	Voices        VoiceList `json:"voices"`
}

// ResolveTTSSelection returns catalog metadata without probing synthesis or
// modifying the snapshot. Missing metadata leaves manual configuration possible.
func (d Document) ResolveTTSSelection(serverProfileID, apiFormat, modelID string) TTSSelection {
	result := TTSSelection{Voices: VoiceList{Completeness: "unknown", Items: []Voice{}}}
	if serverProfileID == "" || apiFormat == "" || modelID == "" {
		return result
	}
	for _, exposure := range d.Exposures {
		if exposure.ServerProfileID != serverProfileID || exposure.APIFormat != apiFormat || exposure.ExposedModelID != modelID {
			continue
		}
		result.SelectionMode = exposure.SelectionMode
		if exposure.Voices != nil {
			result.Voices.Completeness = exposure.Voices.Completeness
			result.Voices.Items = cloneSlice(exposure.Voices.Items)
		}
		return result
	}
	return result
}
