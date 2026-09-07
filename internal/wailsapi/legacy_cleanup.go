package wailsapi

import (
	"assistente/internal/apidto"
	"assistente/internal/channels"
	"assistente/internal/contacts"
	"context"
	"fmt"
	"sync"
)

// LegacyCleanup é o bind Wails do cleanup opt-in de JSON legado de canais/contatos
// (AEP-0088 / AEP-0083). Sem controller: chama channels.CleanupLegacyJSONFiles.
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type LegacyCleanup struct {
	mu      sync.RWMutex
	session Session
}

// NewLegacyCleanup cria o bind vazio; AttachLegacyCleanup preenche a session no startup.
func NewLegacyCleanup() *LegacyCleanup {
	return &LegacyCleanup{}
}

// AttachLegacyCleanup associa Session após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachLegacyCleanup(api *LegacyCleanup, session Session) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
}

func (api *LegacyCleanup) deps() (Session, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil {
		return nil, ErrLegacyCleanupNotWired
	}
	return api.session, nil
}

// CleanupLegacyChannelJSON lista (dry-run) ou remove JSON legado de canais/contatos.
// Nunca é chamado pelo import pós-login automático — apenas via UI opt-in.
func (api *LegacyCleanup) CleanupLegacyChannelJSON(opts apidto.CleanupLegacyChannelJSONOptions) (apidto.CleanupLegacyChannelJSONResult, error) {
	session, err := api.deps()
	if err != nil {
		return apidto.CleanupLegacyChannelJSONResult{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.CleanupLegacyChannelJSONResult, error) {
		if !channels.UsingDatabase() || !contacts.UsingDatabase() {
			return apidto.CleanupLegacyChannelJSONResult{}, fmt.Errorf("channels/contacts DB não habilitado; cleanup legado indisponível")
		}
		result, err := channels.CleanupLegacyJSONFiles(ctx, channels.LegacyCleanupOptions{
			Confirm:         opts.Confirm,
			NoBackup:        opts.NoBackup,
			ContactsUsingDB: contacts.UsingDatabase(),
		})
		return mapLegacyCleanupResult(result), err
	})
}

func mapLegacyCleanupResult(in channels.LegacyCleanupResult) apidto.CleanupLegacyChannelJSONResult {
	out := apidto.CleanupLegacyChannelJSONResult{
		DryRun:     in.DryRun,
		Removed:    append([]string(nil), in.Removed...),
		BackedUpTo: in.BackedUpTo,
		Errors:     append([]string(nil), in.Errors...),
		Warnings:   append([]string(nil), in.Warnings...),
	}
	for _, item := range in.Eligible {
		out.Eligible = append(out.Eligible, apidto.CleanupLegacyChannelJSONItem{
			Path: item.Path, Kind: item.Kind, Slug: item.Slug, Reason: item.Reason,
		})
	}
	for _, item := range in.Skipped {
		out.Skipped = append(out.Skipped, apidto.CleanupLegacyChannelJSONItem{
			Path: item.Path, Kind: item.Kind, Slug: item.Slug, Reason: item.Reason,
		})
	}
	return out
}
