package app

import (
	"context"

	"assistente/internal/connstatus"
	"assistente/internal/database"
)

// startConnectionMonitor inicia (ou reinicia) o monitor de status de conexão
// para o usuário autenticado. O loop roda em um contexto próprio cancelável,
// independente do timeout agregado do reload, e é encerrado no logout/shutdown.
//
// Reaproveita providers.Service.CheckHealth (que por sua vez reaproveita
// ProbeConnection) — não há lógica de teste de conexão duplicada aqui.
func (a *App) startConnectionMonitor(userID string) {
	if a.providerSvc == nil || a.emitter == nil || a.profileManager == nil {
		return
	}

	a.connMu.Lock()
	if a.connCancel != nil {
		a.connCancel()
		a.connCancel = nil
	}
	ctx, cancel := context.WithCancel(database.WithUserID(context.Background(), userID))
	a.connCancel = cancel

	check := func(c context.Context) connstatus.Snapshot {
		profile, err := a.profileManager.GetActive()
		if err != nil || profile == nil {
			return connstatus.Snapshot{
				State:     connstatus.StateOffline,
				Error:     "perfil ativo não encontrado",
				ErrorType: "profile_missing",
			}
		}
		res := a.providerSvc.CheckHealth(c, profile)
		return connstatus.Snapshot{
			State:        string(res.State),
			ProviderID:   res.ProviderID,
			ProviderName: res.ProviderName,
			Model:        res.Model,
			LatencyMs:    res.LatencyMs,
			Error:        res.Error,
			ErrorType:    res.ErrorType,
		}
	}

	// Reaproveita o intervalo padrão do pacote (connstatus trata <=0 como
	// default). Um intervalo configurável por usuário fica como follow-up.
	monitor := connstatus.New(check, a.emitter, connstatus.DefaultInterval)
	a.connMonitor = monitor
	a.connMu.Unlock()

	go monitor.Run(ctx)
}

// stopConnectionMonitor encerra o monitor de status de conexão, se ativo.
func (a *App) stopConnectionMonitor() {
	a.connMu.Lock()
	if a.connCancel != nil {
		a.connCancel()
		a.connCancel = nil
	}
	a.connMonitor = nil
	a.connMu.Unlock()
}
