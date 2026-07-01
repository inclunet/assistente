package app

import (
	"assistente/internal/logging"
	"context"
	"os"

	"assistente/controllers"
	"assistente/internal/credentials"
	"assistente/internal/database"
)

// ============================================================================
// Credential Management
// ============================================================================

// CredentialSummary é alias de controllers.CredentialSummary para o frontend Wails.
type CredentialSummary = controllers.CredentialSummary

// CredentialInput é alias de controllers.CredentialInput para o frontend Wails.
type CredentialInput = controllers.CredentialInput

// ExternalSourceSuggestion é alias de controllers.ExternalSourceSuggestion para o frontend Wails.
type ExternalSourceSuggestion = controllers.ExternalSourceSuggestion

// initCredentialManager inicializa o gerenciador de credenciais com persistência
func (a *App) initCredentialManager() {
	a.credStore = credentials.NewDBStore()
	persist := true

	var dek []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Errorf(context.Background(), "app.app-credentials", "[Credentials] Panic ao acessar keychain (go-keyring): %v", r)
				persist = false
				dek = nil
			}
		}()
		var err error
		dek, err = credentials.LoadDEKFromKeychain()
		if err != nil {
			if !credentials.IsKeychainNotFound(err) {
				logging.Errorf(context.Background(), "app.app-credentials", "[Credentials] Erro ao acessar keychain: %v", err)
			}
			persist = false
			dek = nil
		}
	}()

	a.credMgr = credentials.NewManagerWithStore(dek, a.credStore, persist)
	// initCredentialManager roda em OnStartup, antes de qualquer login.
	// Sem sessão, só instance secrets (refresh-token-pepper, signing
	// key, etc.) precisam estar em memória. As credenciais user-scoped
	// entram pós-Login via adoptLegacyDataForUser → LoadUserCredentials.
	if err := a.credMgr.LoadInstanceSecrets(a.internalBootstrapCtx()); err != nil {
		logging.Errorf(context.Background(), "app.app-credentials", "[Credentials] Erro ao carregar instance secrets: %v", err)
	}
	a.handleVaultIntegrityOnBoot()
	a.registerEnvCredentials(a.internalBootstrapCtx(), a.credMgr)
}

// handleVaultIntegrityOnBoot reage ao status de integridade do vault
// que foi calculado em LoadInstanceSecrets. Política atual (AEP-0061):
//
//   - Se há credenciais ilegíveis (cifradas com DEK que não bate com
//     a do keychain), faz purge automático após log explícito. Decisão
//     arquitetural: o usuário escolheu a política `auto_purge` quando
//     adotamos o AEP — manter creds ilegíveis no banco só causa
//     confusão e ainda esbarra em validações user-scope. A UI mostra
//     o histórico via `App.GetVaultIntegrityStatus`.
//   - Se há divergência DEK_keychain ↔ DEK_wraps (não só órfãs, mas
//     wrap embrulhando outra DEK), apenas LOGA e mantém o estado
//     bloqueado para escritas; recovery exige ação explícita do
//     usuário (UnlockOverwriteKeychain ou setup nova senha).
func (a *App) handleVaultIntegrityOnBoot() {
	if a.credMgr == nil {
		return
	}
	status := a.credMgr.IntegrityStatus()
	if !status.OK {
		logging.Infof(context.Background(), "app.app-credentials", "[Credentials] vault integrity: NOT OK — %s (keychain=%s wraps=%s)", status.Reason, status.KeychainDekID, status.WrapsDekID)
	}
	if len(status.UnreadableCredentialIDs) == 0 {
		return
	}
	logging.Infof(context.Background(), "app.app-credentials", "[Credentials] %d credenciais ilegíveis encontradas (cifradas com DEK divergente da atual): %v — removendo automaticamente", len(status.UnreadableCredentialIDs), status.UnreadableCredentialIDs)
	removed, err := a.credMgr.PurgeUnreadableCredentials(a.internalBootstrapCtx())
	if err != nil {
		logging.Errorf(context.Background(), "app.app-credentials", "[Credentials] erro ao purgar credenciais ilegíveis: %v", err)
		return
	}
	logging.Infof(context.Background(), "app.app-credentials", "[Credentials] %d credenciais ilegíveis removidas. Reemita as credenciais correspondentes via UI/wizard.", removed)
}

// GetVaultIntegrityStatus expõe o status de integridade do vault
// (DEK_keychain ↔ DEK_wraps) para a UI. Frontend usa para mostrar
// banner quando há divergência ou credenciais ilegíveis recém
// purgadas.
func (a *App) GetVaultIntegrityStatus() credentials.VaultIntegrityStatus {
	if a.credMgr == nil {
		return credentials.VaultIntegrityStatus{}
	}
	return a.credMgr.IntegrityStatus()
}

func (a *App) registerEnvCredentials(ctx context.Context, credMgr *credentials.Manager) {
	if credMgr == nil {
		return
	}
	if _, ok := database.UserIDFromContext(ctx); !ok {
		return
	}

	// GITHUB_TOKEN -> *.github.com, github.com
	if ghToken := os.Getenv("GITHUB_TOKEN"); ghToken != "" {
		_ = credMgr.RegisterPatternWithContext(ctx, "*.github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
		_ = credMgr.RegisterPatternWithContext(ctx, "github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
	}

	// GITLAB_TOKEN -> *.gitlab.com, gitlab.com
	if glToken := os.Getenv("GITLAB_TOKEN"); glToken != "" {
		_ = credMgr.RegisterPatternWithContext(ctx, "*.gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
		_ = credMgr.RegisterPatternWithContext(ctx, "gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
	}

	// BITBUCKET_TOKEN -> *.bitbucket.org, bitbucket.org
	if bbToken := os.Getenv("BITBUCKET_TOKEN"); bbToken != "" {
		_ = credMgr.RegisterPatternWithContext(ctx, "*.bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
		_ = credMgr.RegisterPatternWithContext(ctx, "bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
	}

	// API genérica - GENERIC_API_KEY para qualquer host (fallback)
	if apiKey := os.Getenv("GENERIC_API_KEY"); apiKey != "" {
		_ = credMgr.RegisterPatternWithContext(ctx, "*", &credentials.AuthConfig{
			Type: "custom",
			Headers: map[string]string{
				"X-API-Key": apiKey,
			},
		})
	}
}

func (a *App) configureCredentialManager(dek []byte, persist bool) {
	if a.credStore == nil {
		a.credStore = credentials.NewDBStore()
	}
	if a.credMgr == nil {
		a.credMgr = credentials.NewManagerWithStore(dek, a.credStore, persist)
	} else {
		a.credMgr.Reset(dek, persist)
	}

	// configureCredentialManager pode rodar pré-login (carrega DEK do
	// keychain antes de qualquer sessão). Só instance secrets entram em
	// memória aqui; user-scoped vem depois via LoadUserCredentials.
	if err := a.credMgr.LoadInstanceSecrets(a.internalBootstrapCtx()); err != nil {
		logging.Errorf(context.Background(), "app.app-credentials", "[Credentials] Erro ao carregar instance secrets: %v", err)
	}
	a.handleVaultIntegrityOnBoot()
	a.registerEnvCredentials(a.internalBootstrapCtx(), a.credMgr)
}

// HasMasterKey verifica se uma master key (senha mestre) já foi configurada no banco.
func (a *App) HasMasterKey() bool {
	store := credentials.NewDBStore()
	has, err := store.HasKeyWrap(a.appContext(), credentials.KeyWrapKindMaster)
	if err != nil {
		return false
	}
	return has
}

// SetupMasterPassword configura a senha mestre pela primeira vez.
// Retorna a recovery key gerada (que o usuário deve guardar).
// Após sucesso, o credential manager é reconfigurado com persistência ativada.
func (a *App) SetupMasterPassword(password string) (string, error) {
	store := credentials.NewDBStore()
	result, err := credentials.SetupMasterKeyAdoptingKeychain(store, password)
	if err != nil {
		return "", err
	}
	a.configureCredentialManager(result.DEK, true)
	return result.RecoveryKey, nil
}

// CanPersistCredentials retorna true se o credential manager está configurado
// com persistência ativada (ou seja, a DEK foi carregada ou configurada).
func (a *App) CanPersistCredentials() bool {
	if a.credMgr == nil {
		return false
	}
	return a.credMgr.CanPersist()
}

// ============================================================================
// Credential UI API
// ============================================================================

// ListCredentials retorna credenciais registradas (sem valores sensíveis).
func (a *App) ListCredentials() ([]CredentialSummary, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.credentialsCtrl.ListCredentialsWithContext(ctx)
}

// UpsertCredential cria ou atualiza uma credencial no credential manager.
func (a *App) UpsertCredential(input CredentialInput) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.credentialsCtrl.UpsertCredentialWithContext(ctx, input)
}

// DeleteCredential remove uma credencial pelo padrão.
func (a *App) DeleteCredential(pattern string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.credentialsCtrl.DeleteCredentialWithContext(ctx, pattern)
}

// ListExternalSources lista fontes externas disponíveis para autocomplete.
func (a *App) ListExternalSources(prefix string) ([]ExternalSourceSuggestion, error) {
	return a.credentialsCtrl.ListExternalSources(prefix)
}
