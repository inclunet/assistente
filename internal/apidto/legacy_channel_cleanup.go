package apidto

// CleanupLegacyChannelJSONOptions controla o cleanup opt-in de JSON legado
// (channels/*.json + contacts.json) após migração DB — AEP-0083.
// Confirm=false (default) é dry-run: lista paths elegíveis sem apagar.
type CleanupLegacyChannelJSONOptions struct {
	Confirm  bool `json:"confirm"`
	NoBackup bool `json:"noBackup"`
}

// CleanupLegacyChannelJSONItem descreve um arquivo legado candidato ou ignorado.
type CleanupLegacyChannelJSONItem struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Slug   string `json:"slug,omitempty"`
	Reason string `json:"reason"`
}

// CleanupLegacyChannelJSONResult é o resultado do dry-run ou da remoção.
type CleanupLegacyChannelJSONResult struct {
	DryRun     bool                           `json:"dryRun"`
	Eligible   []CleanupLegacyChannelJSONItem `json:"eligible"`
	Removed    []string                       `json:"removed"`
	BackedUpTo string                         `json:"backedUpTo,omitempty"`
	Skipped    []CleanupLegacyChannelJSONItem `json:"skipped"`
	Errors     []string                       `json:"errors"`
	Warnings   []string                       `json:"warnings"`
}
