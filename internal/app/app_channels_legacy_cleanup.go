package app

import (
	"fmt"

	"assistente/internal/channels"
	"assistente/internal/contacts"
)

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
	DryRun     bool                            `json:"dryRun"`
	Eligible   []CleanupLegacyChannelJSONItem  `json:"eligible"`
	Removed    []string                        `json:"removed"`
	BackedUpTo string                          `json:"backedUpTo,omitempty"`
	Skipped    []CleanupLegacyChannelJSONItem  `json:"skipped"`
	Errors     []string                        `json:"errors"`
	Warnings   []string                        `json:"warnings"`
}

// CleanupLegacyChannelJSON lista (dry-run) ou remove JSON legado de canais/contatos.
// Nunca é chamado pelo import pós-login automático — apenas via UI opt-in.
func (a *App) CleanupLegacyChannelJSON(opts CleanupLegacyChannelJSONOptions) (CleanupLegacyChannelJSONResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return CleanupLegacyChannelJSONResult{}, err
	}
	if !channels.UsingDatabase() || !contacts.UsingDatabase() {
		return CleanupLegacyChannelJSONResult{}, fmt.Errorf("channels/contacts DB não habilitado; cleanup legado indisponível")
	}
	result, err := channels.CleanupLegacyJSONFiles(ctx, channels.LegacyCleanupOptions{
		Confirm:         opts.Confirm,
		NoBackup:        opts.NoBackup,
		ContactsUsingDB: contacts.UsingDatabase(),
	})
	return mapLegacyCleanupResult(result), err
}

func mapLegacyCleanupResult(in channels.LegacyCleanupResult) CleanupLegacyChannelJSONResult {
	out := CleanupLegacyChannelJSONResult{
		DryRun:     in.DryRun,
		Removed:    append([]string(nil), in.Removed...),
		BackedUpTo: in.BackedUpTo,
		Errors:     append([]string(nil), in.Errors...),
		Warnings:   append([]string(nil), in.Warnings...),
	}
	for _, item := range in.Eligible {
		out.Eligible = append(out.Eligible, CleanupLegacyChannelJSONItem{
			Path: item.Path, Kind: item.Kind, Slug: item.Slug, Reason: item.Reason,
		})
	}
	for _, item := range in.Skipped {
		out.Skipped = append(out.Skipped, CleanupLegacyChannelJSONItem{
			Path: item.Path, Kind: item.Kind, Slug: item.Slug, Reason: item.Reason,
		})
	}
	return out
}
