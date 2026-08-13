// Package wailsapi — allowlist de métodos sem WithUser (AEP-0088 Fase 2).
//
// Domínios migrados (ex.: Tokens) autenticam só via WithUser. Métodos que
// intencionalmente NÃO usam auth ficam no *App e devem aparecer nesta lista.
// Novos binds autenticados não entram aqui.
package wailsapi

// UnauthenticatedAppMethods são métodos públicos do *App (borda Wails) que
// operam sem requireAuthenticatedContext / WithUser. Login/status precisam
// rodar pré-sessão; demais entradas exigem revisão explícita ao alterar.
var UnauthenticatedAppMethods = []string{
	"Login",
	"Logout",
	"RefreshAuth",
	"GetAuthStatus",
	"GetAuthUser",
	"SetupVault",
	"UnlockVault",
	"CreateAdminUser",
	// Vault/credentials pré-sessão (permanecem no *App; CRUD migrou para wailsapi.Credentials).
	"HasMasterKey",
	"SetupMasterPassword",
	"GetVaultIntegrityStatus",
	"CanPersistCredentials",
}
