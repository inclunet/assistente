//go:build windows

package ossession

import "context"

// Probe lê uma vez o estado da sessão interativa atual. Não cria watcher nem
// recursos nativos persistentes; estado desconhecido e erros permanecem
// fail-closed para os chamadores.
func Probe(ctx context.Context) (State, error) {
	return probeWith(ctx, ensureSupportedWindowsVersion, currentSessionID, querySessionState)
}
