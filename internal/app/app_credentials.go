package app

import (
	"context"
	"log"
	"os"
	"strings"

	"assistente/controllers"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/providers"
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
				log.Printf("[Credentials] Panic ao acessar keychain (go-keyring): %v", r)
				persist = false
				dek = nil
			}
		}()
		var err error
		dek, err = credentials.LoadDEKFromKeychain()
		if err != nil {
			if !credentials.IsKeychainNotFound(err) {
				log.Printf("[Credentials] Erro ao acessar keychain: %v", err)
			}
			persist = false
			dek = nil
		}
	}()

	a.credMgr = credentials.NewManagerWithStore(dek, a.credStore, persist)
	if err := a.credMgr.LoadFromStore(context.Background()); err != nil {
		log.Printf("[Credentials] Erro ao carregar credenciais persistidas: %v", err)
	}
	// initCredentialManager roda em OnStartup, antes de qualquer login;
	// internalBootstrapCtx é o caminho legítimo aqui. registerEnvCredentials
	// já guarda explicitamente com UserIDFromContext, então sem sessão
	// vira no-op (o reload pós-login carrega as envs depois).
	a.registerEnvCredentials(a.internalBootstrapCtx(), a.credMgr)
}

// migrateLegacyConfig detecta config.json com campos legados e migra para o
// novo sistema. Recebe o ctx já autenticado do caller — Major G do
// re-review do AEP-0052: a versão anterior chamava requireAuthenticatedContext
// internamente e, se falhasse, "pulava silenciosamente". Como a função SÓ é
// chamada de dentro de reloadUserScopedRuntime (que já validou auth no topo),
// um ctx sem userID aqui é bug do caller — engolir o erro mascarava a
// regressão. Agora o ctx é passado explicitamente; falha em
// RegisterPatternWithContext gera log de ERROR e a função retorna sem
// migrar, mas o invariante (caller validou auth) é deixado a cargo do caller.
//
// Migração:
// 1. Se APIKey existir → registra como credencial no credentials.Manager
// 2. Se APIKey existir → garante que provider default está usando as credenciais
// 3. Limpa campos legados do config.json
func (a *App) migrateLegacyConfig(ctx context.Context) {
	if a.settingsSvc == nil {
		return
	}
	cfg, err := a.settingsSvc.GetConfig()
	if err != nil {
		return
	}

	needsMigration := false
	migratedFields := []string{}

	if cfg.APIKey != "" {
		needsMigration = true
		migratedFields = append(migratedFields, "APIKey")

		baseURL := cfg.APIBaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}

		pattern := ""
		if extractedHost, hostErr := providers.ExtractHostname(baseURL); hostErr == nil && extractedHost != "" {
			pattern = extractedHost
		} else if strings.Contains(baseURL, "anthropic") {
			pattern = "api.anthropic.com"
		} else if strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") {
			pattern = ""
		} else {
			pattern = "api.openai.com"
		}

		if pattern != "" {
			authCfg := &credentials.AuthConfig{
				Type:  "bearer",
				Token: cfg.APIKey,
			}
			if err := a.credMgr.RegisterPatternWithContext(ctx, pattern, authCfg); err != nil {
				log.Printf("[Migration] ERRO ao registrar credencial do config.json: %v (ctx pode estar sem userID — verifique caller)", err)
			} else {
				log.Printf("[Migration] ✓ APIKey migrado para credentials.Manager (pattern: %s)", pattern)
			}
		}
	}

	// Verificar outros campos legados
	if cfg.APIBaseURL != "" && cfg.APIBaseURL != "https://api.openai.com/v1" {
		migratedFields = append(migratedFields, "APIBaseURL")
	}
	if cfg.DefaultModel != "" && cfg.DefaultModel != "gpt-4o-mini" {
		migratedFields = append(migratedFields, "DefaultModel")
	}
	if cfg.ResponseTimeout != 0 && cfg.ResponseTimeout != 180 {
		migratedFields = append(migratedFields, "ResponseTimeout")
	}
	if cfg.ActiveProfile != "" && cfg.ActiveProfile != "padrao" {
		migratedFields = append(migratedFields, "ActiveProfile")
	}

	if needsMigration {
		log.Printf("[Migration] Config.json legado detectado — campos migrados: %v", migratedFields)
		log.Printf("[Migration] Novas configurações devem ser feitas via Perfis e Provider Registry")
		log.Printf("[Migration] Os campos legados em config.json não serão mais usados")
	}
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

	if err := a.credMgr.LoadFromStore(context.Background()); err != nil {
		log.Printf("[Credentials] Erro ao carregar credenciais persistidas: %v", err)
	}
	// configureCredentialManager pode rodar pré-login (carrega DEK do
	// keychain antes de qualquer sessão). internalBootstrapCtx é o caminho
	// legítimo aqui; registerEnvCredentials já guarda com UserIDFromContext.
	a.registerEnvCredentials(a.internalBootstrapCtx(), a.credMgr)
}

// HasMasterKey verifica se uma master key (senha mestre) já foi configurada no banco.
func (a *App) HasMasterKey() bool {
	store := credentials.NewDBStore()
	has, err := store.HasKeyWrap(context.Background(), credentials.KeyWrapKindMaster)
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
