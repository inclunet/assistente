package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"assistente/internal/config"
	"assistente/internal/credentials"
	"assistente/internal/providers"
)

// ============================================================================
// Credential Management
// ============================================================================

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
	a.registerEnvCredentials(a.credMgr)
}

// migrateLegacyConfig detecta config.json com campos legados e migra para novo sistema
// Migração:
// 1. Se APIKey existir → registra como credencial no credentials.Manager
// 2. Se APIKey existir → garante que provider default está usando as credenciais
// 3. Limpa campos legados do config.json
func (a *App) migrateLegacyConfig() {
	cfg, err := config.Load()
	if err != nil {
		// Sem config, sem migração necessária
		return
	}

	needsMigration := false
	migratedFields := []string{}

	// Verificar se tem APIKey (campo principal legado)
	if cfg.APIKey != "" {
		needsMigration = true
		migratedFields = append(migratedFields, "APIKey")

		// Extrair domínio do BaseURL
		baseURL := cfg.APIBaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}

		// Determinar pattern baseado no baseURL
		pattern := ""
		if extractedHost, hostErr := providers.ExtractHostname(baseURL); hostErr == nil && extractedHost != "" {
			pattern = extractedHost
		} else if strings.Contains(baseURL, "anthropic") {
			pattern = "api.anthropic.com"
		} else if strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") {
			pattern = "" // local, sem pattern
		} else {
			pattern = "api.openai.com" // fallback para OpenAI
		}

		// Registrar credencial no credentials.Manager
		if pattern != "" {
			authCfg := &credentials.AuthConfig{
				Type:  "bearer",
				Token: cfg.APIKey,
			}
			if err := a.credMgr.RegisterPatternWithContext(a.ctx, pattern, authCfg); err != nil {
				log.Printf("[Migration] Erro ao registrar credencial do config.json: %v", err)
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

func (a *App) registerEnvCredentials(credMgr *credentials.Manager) {
	if credMgr == nil {
		return
	}

	// GITHUB_TOKEN -> *.github.com, github.com
	if ghToken := os.Getenv("GITHUB_TOKEN"); ghToken != "" {
		_ = credMgr.RegisterPattern("*.github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
		_ = credMgr.RegisterPattern("github.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: ghToken,
		})
	}

	// GITLAB_TOKEN -> *.gitlab.com, gitlab.com
	if glToken := os.Getenv("GITLAB_TOKEN"); glToken != "" {
		_ = credMgr.RegisterPattern("*.gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
		_ = credMgr.RegisterPattern("gitlab.com", &credentials.AuthConfig{
			Type:  "bearer",
			Token: glToken,
		})
	}

	// BITBUCKET_TOKEN -> *.bitbucket.org, bitbucket.org
	if bbToken := os.Getenv("BITBUCKET_TOKEN"); bbToken != "" {
		_ = credMgr.RegisterPattern("*.bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
		_ = credMgr.RegisterPattern("bitbucket.org", &credentials.AuthConfig{
			Type:  "bearer",
			Token: bbToken,
		})
	}

	// API genérica - GENERIC_API_KEY para qualquer host (fallback)
	if apiKey := os.Getenv("GENERIC_API_KEY"); apiKey != "" {
		_ = credMgr.RegisterPattern("*", &credentials.AuthConfig{
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
	a.registerEnvCredentials(a.credMgr)
}

func (a *App) resolveCredentialRef(ref string) string {
	if ref == "" || a.credMgr == nil {
		return ""
	}
	auth, err := a.credMgr.GetByPattern(ref)
	if err != nil {
		log.Printf("[Credentials] Erro ao resolver referência %s: %v", ref, err)
		return ""
	}
	return credentials.ResolveSecretFromAuth(auth)
}

// ============================================================================
// Credential UI API
// ============================================================================

// CredentialSummary descreve uma credencial para exibição (sem dados sensíveis).
type CredentialSummary struct {
	Pattern string `json:"pattern"`
	Type    string `json:"type"`
	Masked  string `json:"masked"`
	Managed bool   `json:"managed"`
}

// CredentialInput descreve a entrada para criar/atualizar credenciais.
type CredentialInput struct {
	Pattern     string `json:"pattern"`
	Type        string `json:"type"`
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"headerName,omitempty"`
	HeaderValue string `json:"headerValue,omitempty"`
}

// ExternalSourceSuggestion representa uma sugestão de fonte externa para autocomplete.
type ExternalSourceSuggestion struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ListCredentials retorna credenciais registradas (sem valores sensíveis).
func (a *App) ListCredentials() ([]CredentialSummary, error) {
	if a.credMgr == nil {
		return []CredentialSummary{}, nil
	}

	list, err := a.credMgr.ListCredentials()
	if err != nil {
		return nil, err
	}

	result := make([]CredentialSummary, 0, len(list))
	for _, entry := range list {
		if entry.Auth == nil {
			continue
		}
		result = append(result, CredentialSummary{
			Pattern: entry.Pattern,
			Type:    entry.Auth.Type,
			Masked:  credentials.SummarizeAuth(entry.Auth),
			Managed: credentials.IsManagedPattern(entry.Pattern),
		})
	}

	return result, nil
}

// UpsertCredential cria ou atualiza uma credencial no credential manager.
func (a *App) UpsertCredential(input CredentialInput) error {
	if a.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	if !a.credMgr.CanPersist() {
		return fmt.Errorf("cofre de credenciais indisponível: configure a senha mestre")
	}

	pattern := strings.TrimSpace(input.Pattern)
	if pattern == "" {
		return fmt.Errorf("pattern é obrigatório")
	}

	if credentials.IsManagedPattern(pattern) {
		return fmt.Errorf("credencial '%s' é gerenciada pelo sistema e não pode ser editada manualmente", pattern)
	}

	auth := &credentials.AuthConfig{Type: strings.TrimSpace(input.Type)}
	switch auth.Type {
	case "bearer", "oauth2", "secret":
		if strings.TrimSpace(input.Token) == "" {
			return fmt.Errorf("token é obrigatório")
		}
		auth.Token = input.Token
	case "basic":
		if strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.Password) == "" {
			return fmt.Errorf("usuário e senha são obrigatórios")
		}
		auth.Username = input.Username
		auth.Password = input.Password
	case "custom":
		if strings.TrimSpace(input.HeaderName) == "" || strings.TrimSpace(input.HeaderValue) == "" {
			return fmt.Errorf("header e valor são obrigatórios")
		}
		auth.Headers = map[string]string{input.HeaderName: input.HeaderValue}
	default:
		return fmt.Errorf("tipo de credencial inválido")
	}

	return a.credMgr.RegisterPatternWithContext(context.Background(), pattern, auth)
}

// DeleteCredential remove uma credencial pelo padrão.
func (a *App) DeleteCredential(pattern string) error {
	if a.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	if credentials.IsManagedPattern(pattern) {
		return fmt.Errorf("credencial '%s' é gerenciada pelo sistema e não pode ser removida manualmente", pattern)
	}
	return a.credMgr.DeletePattern(context.Background(), pattern)
}

// ListExternalSources lista fontes externas disponíveis para autocomplete.
// prefix deve ser "keyring://" ou "env://".
func (a *App) ListExternalSources(prefix string) ([]ExternalSourceSuggestion, error) {
	switch prefix {
	case "keyring://":
		return a.listKeyringEntries()
	case "env://":
		return a.listEnvVars()
	default:
		return []ExternalSourceSuggestion{}, nil
	}
}

func (a *App) listEnvVars() ([]ExternalSourceSuggestion, error) {
	envs := os.Environ()
	suggestions := make([]ExternalSourceSuggestion, 0, len(envs))

	skipPrefixes := []string{"PROCESSOR_", "SYSTEM", "WINDOWS", "COMMON"}
	skipExact := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"TEMP": true, "TMP": true, "OS": true,
		"HOMEDRIVE": true, "HOMEPATH": true,
		"USERDOMAIN": true, "USERNAME": true,
		"LOCALAPPDATA": true, "APPDATA": true,
		"PROGRAMFILES": true, "PROGRAMDATA": true,
		"WINDIR": true, "SYSTEMROOT": true,
		"COMPUTERNAME": true, "NUMBER_OF_PROCESSORS": true,
		"PROGRAMFILES(X86)": true, "PSMODULEPATH": true,
		"PUBLIC": true, "SESSIONNAME": true,
		"USERPROFILE": true, "ALLUSERSPROFILE": true,
	}

	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		name := parts[0]
		upper := strings.ToUpper(name)

		if skipExact[upper] {
			continue
		}
		skip := false
		for _, p := range skipPrefixes {
			if strings.HasPrefix(upper, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		suggestions = append(suggestions, ExternalSourceSuggestion{
			Value: "env://" + name,
			Label: name,
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Label < suggestions[j].Label
	})
	return suggestions, nil
}

func (a *App) listKeyringEntries() ([]ExternalSourceSuggestion, error) {
	entries, err := credentials.ListKeyringEntries()
	if err != nil {
		return nil, err
	}

	suggestions := make([]ExternalSourceSuggestion, 0, len(entries))
	for _, e := range entries {
		ref := "keyring://" + e.Target
		suggestions = append(suggestions, ExternalSourceSuggestion{
			Value: ref,
			Label: e.Target,
		})
	}
	return suggestions, nil
}
