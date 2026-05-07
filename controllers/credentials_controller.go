package controllers

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

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

// CredentialsControllerConfig agrupa dependências do CredentialsController.
type CredentialsControllerConfig struct {
	CredMgr *credentials.Manager
}

// CredentialsController expõe operações de gerenciamento de credenciais via Wails.
type CredentialsController struct {
	credMgr *credentials.Manager
}

// NewCredentialsController cria um CredentialsController com as dependências fornecidas.
func NewCredentialsController(cfg CredentialsControllerConfig) *CredentialsController {
	return &CredentialsController{
		credMgr: cfg.CredMgr,
	}
}

// ListCredentials retorna credenciais registradas (sem valores sensíveis).
func (c *CredentialsController) ListCredentials() ([]CredentialSummary, error) {
	return c.ListCredentialsWithContext(context.Background())
}

func (c *CredentialsController) ListCredentialsWithContext(ctx context.Context) ([]CredentialSummary, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	if c.credMgr == nil {
		return []CredentialSummary{}, nil
	}

	list, err := c.credMgr.ListVisibleCredentialsWithContext(ctx)
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
func (c *CredentialsController) UpsertCredential(input CredentialInput) error {
	return c.UpsertCredentialWithContext(context.Background(), input)
}

func (c *CredentialsController) UpsertCredentialWithContext(ctx context.Context, input CredentialInput) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	if c.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	if !c.credMgr.CanPersist() {
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

	return c.credMgr.RegisterPatternWithContext(ctx, pattern, auth)
}

// DeleteCredential remove uma credencial pelo padrão.
func (c *CredentialsController) DeleteCredential(pattern string) error {
	return c.DeleteCredentialWithContext(context.Background(), pattern)
}

func (c *CredentialsController) DeleteCredentialWithContext(ctx context.Context, pattern string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	if c.credMgr == nil {
		return fmt.Errorf("credential manager não inicializado")
	}
	if credentials.IsManagedPattern(pattern) {
		return fmt.Errorf("credencial '%s' é gerenciada pelo sistema e não pode ser removida manualmente", pattern)
	}
	return c.credMgr.DeletePattern(ctx, pattern)
}

// ListExternalSources lista fontes externas disponíveis para autocomplete.
// prefix deve ser "keyring://" ou "env://".
func (c *CredentialsController) ListExternalSources(prefix string) ([]ExternalSourceSuggestion, error) {
	switch prefix {
	case "keyring://":
		return c.listKeyringEntries()
	case "env://":
		return c.listEnvVars()
	default:
		return []ExternalSourceSuggestion{}, nil
	}
}

func (c *CredentialsController) listEnvVars() ([]ExternalSourceSuggestion, error) {
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

func (c *CredentialsController) listKeyringEntries() ([]ExternalSourceSuggestion, error) {
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
