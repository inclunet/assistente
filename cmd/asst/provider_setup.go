package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"assistente/internal/apidto"
	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
	"assistente/internal/profiles"
	"assistente/internal/providers"

	"github.com/google/uuid"
)

// providerSetupBackend mantém sondagem e persistência no serviço compartilhado.
type providerSetupBackend interface {
	GetLLMCatalog() (apidto.LLMCatalogResponse, error)
	RefreshLLMCatalog() (apidto.LLMCatalogResponse, error)
	ListModelsRaw(apidto.TestLLMProviderRequest) ([]string, error)
	CreateLLMProvider(apidto.CreateLLMProviderRequest) (map[string]interface{}, error)
	SetDefaultProvider(string) error
	GetCredentialForURL(string) (*apidto.CredentialSummary, error)
	GetActiveProfileAndSlug() (*profiles.ActiveProfile, error)
	UpdateProfile(string, profiles.Profile) error
}

type providerPrompt struct {
	reader *bufio.Reader
	out    io.Writer
}

func (p providerPrompt) text(label, initial string, required bool) (string, error) {
	if initial != "" {
		label += " [" + initial + "]"
	}
	if _, err := fmt.Fprint(p.out, label+": "); err != nil {
		return "", err
	}
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("entrada interrompida: %w", err)
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = initial
	}
	if required && value == "" {
		return "", fmt.Errorf("%s é obrigatório", label)
	}
	return value, nil
}

func (p providerPrompt) choice(label string, options []string, initial string) (string, error) {
	for i, option := range options {
		if _, err := fmt.Fprintf(p.out, "  %d. %s\n", i+1, option); err != nil {
			return "", err
		}
	}
	value, err := p.text(label, initial, true)
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(value); err == nil && n > 0 && n <= len(options) {
		return options[n-1], nil
	}
	for _, option := range options {
		if value == option {
			return value, nil
		}
	}
	return "", fmt.Errorf("escolha inválida: %s", value)
}

func (p providerPrompt) field(server *llmcatalog.Server, key, label, initial string, required bool) (string, error) {
	if server == nil {
		return p.text(label, initial, required)
	}
	policy, exists := server.Fields[key]
	if !exists || policy.Policy == "fixed" || policy.Policy == "hidden" {
		return initial, nil
	}
	if policy.Policy == "choice" {
		options := make([]string, 0, len(policy.Choices))
		for _, v := range policy.Choices {
			if s, ok := v.(string); ok {
				options = append(options, s)
			}
		}
		return p.choice(label, options, initial)
	}
	return p.text(label, initial, required || policy.Required)
}

// configureProvider é o único formulário terminal para setup e providers add.
// O segredo permanece no draft até CreateLLMProvider confirmar a transação.
func configureProvider(svc providerSetupBackend, out io.Writer, reader *bufio.Reader, readPwd passwordReader, makeDefault bool) error {
	p := providerPrompt{reader, out}
	snapshot, err := svc.GetLLMCatalog()
	if err != nil {
		return fmt.Errorf("catálogo: %w", err)
	}
	if len(snapshot.Document.Servers) == 0 {
		refresh, err := p.text("Catálogo indisponível. Tentar atualizar? (s/N)", "n", true)
		if err != nil {
			return err
		}
		if strings.EqualFold(refresh, "s") || strings.EqualFold(refresh, "sim") {
			snapshot, err = svc.RefreshLLMCatalog()
			if err != nil {
				return err
			}
		}
	}
	var servers []llmcatalog.Server
	var templates []*llm.ProviderConfig
	var names []string
	for _, server := range snapshot.Document.Servers {
		template, err := providers.TemplateFromCatalogServer(server)
		if err != nil {
			continue
		}
		servers = append(servers, server)
		templates = append(templates, template)
		names = append(names, server.Name+" ("+server.ID+")")
	}
	names = append(names, "Configuração manual")
	selected, err := p.choice("Provedor", names, "")
	if err != nil {
		return err
	}
	var server *llmcatalog.Server
	config := &llm.ProviderConfig{Type: llm.ProviderCustom, APIFormat: llm.APIFormatOpenAI, AuthMode: llm.AuthModeRequired}
	for i := range servers {
		if names[i] == selected {
			server, config = &servers[i], templates[i]
			break
		}
	}
	if server != nil {
		for _, link := range []string{server.SignupURL, server.APIKeyURL} {
			if link != "" {
				if _, err := fmt.Fprintln(out, link); err != nil {
					return err
				}
			}
		}
	}
	req := apidto.CreateLLMProviderRequest{ID: uuid.NewString(), Type: string(config.Type), ServerProfileID: config.ServerProfileID}
	req.Name, err = p.text("Nome da conexão", config.Name, true)
	if err != nil {
		return err
	}
	req.BaseURL, err = p.field(server, "base_url", "URL base", config.BaseURL, true)
	if err != nil {
		return err
	}
	if server == nil {
		req.APIFormat, err = p.choice("Formato da API", []string{"openai", "openai_responses", "anthropic", "google"}, "openai")
	} else {
		req.APIFormat, err = p.field(server, "api_format", "Formato da API", string(config.APIFormat), true)
	}
	if err != nil {
		return err
	}
	var authMode string
	if server == nil {
		authMode, err = p.choice("Autenticação", []string{"required", "optional", "none"}, "required")
	} else {
		mode := providers.CatalogFieldDefault(*server, "auth.mode", server.Auth.Mode)
		mode, err = p.field(server, "auth.mode", "Autentica\u00e7\u00e3o", mode, true)
		if err != nil {
			return err
		}
		parsed, parseErr := providers.CatalogAuthMode(mode)
		if parseErr != nil {
			return parseErr
		}
		authMode = string(parsed)
	}
	if err != nil {
		return err
	}
	req.AuthMode = authMode
	if req.APIFormat == "google" && authMode != "required" {
		return fmt.Errorf("google exige autenticação obrigatória")
	}
	if authMode != "none" {
		key, err := readPwd("API key: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(key) == "" && authMode == "required" {
			return fmt.Errorf("API key não pode ser vazia")
		}
		if key != "" {
			summary, err := svc.GetCredentialForURL(req.BaseURL)
			if err != nil {
				return err
			}
			if summary == nil || summary.Pattern == "" {
				return fmt.Errorf("não foi possível determinar a referência da credencial")
			}
			req.Credential = &apidto.CredentialInput{Pattern: summary.Pattern, Source: "static", Type: "bearer", Token: key}
			if req.APIFormat == string(llm.APIFormatAnthropic) {
				req.Credential.Type = "custom"
				req.Credential.Token = ""
				req.Credential.HeaderName, req.Credential.HeaderValue = "x-api-key", key
			}
		}
		if llm.RequiresInsecureHTTPAuthorization(req.BaseURL) {
			answer, err := p.text("HTTP sem TLS em "+req.BaseURL+" pode expor a credencial. Autorizar esta URL? (s/N)", "n", true)
			if err != nil {
				return err
			}
			if !strings.EqualFold(answer, "s") && !strings.EqualFold(answer, "sim") {
				return fmt.Errorf("configuração cancelada")
			}
			req.InsecureHTTPAuthorizedURL = req.BaseURL
		}
	}
	reasoning := string(config.ReasoningContentMode)
	if reasoning == "" {
		reasoning = "disabled"
	}
	req.ReasoningContentMode, err = p.choice("Reenvio de reasoning_content com ferramentas", []string{"disabled", "replay_with_tools"}, reasoning)
	if err != nil {
		return err
	}
	models, probeErr := svc.ListModelsRaw(apidto.TestLLMProviderRequest{Type: req.Type, APIFormat: req.APIFormat, BaseURL: req.BaseURL, Credential: req.Credential, AuthMode: req.AuthMode, InsecureHTTPAuthorizedURL: req.InsecureHTTPAuthorizedURL})
	if probeErr != nil {
		if _, err := fmt.Fprintf(out, "Não foi possível listar os modelos: %v\n", probeErr); err != nil {
			return err
		}
		answer, err := p.text("Continuar com modelo manual? (s/N)", "n", true)
		if err != nil {
			return err
		}
		if !strings.EqualFold(answer, "s") && !strings.EqualFold(answer, "sim") {
			return fmt.Errorf("configuração cancelada")
		}
	}
	for _, model := range models {
		if _, err := fmt.Fprintln(out, "  "+model); err != nil {
			return err
		}
	}
	req.DefaultModel, err = p.field(server, "default_model_id", "Modelo (ID exato)", config.DefaultModel, true)
	if err != nil {
		return err
	}
	// Sem política declarada, o modelo continua selecionável conforme a UI.
	if server != nil {
		if _, ok := server.Fields["default_model_id"]; !ok {
			req.DefaultModel, err = p.text("Modelo (ID exato)", config.DefaultModel, true)
		}
	}
	if err != nil {
		return err
	}
	created, err := svc.CreateLLMProvider(req)
	if err != nil {
		return fmt.Errorf("erro ao criar provedor: %w", err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		return fmt.Errorf("cadastro não retornou o ID da conexão")
	}
	if makeDefault {
		if err := svc.SetDefaultProvider(id); err != nil {
			return fmt.Errorf("provedor %s criado, mas não foi possível defini-lo como padrão: %w", id, err)
		}
		active, err := svc.GetActiveProfileAndSlug()
		if err == nil && (active == nil || active.Profile == nil || active.Slug == "") {
			err = fmt.Errorf("nenhum perfil ativo")
		}
		if err == nil {
			profile := *active.Profile
			profile.Chat.LLMProvider = profiles.DefaultProviderSentinel
			profile.Chat.Model = profiles.DefaultProviderSentinel
			err = svc.UpdateProfile(active.Slug, profile)
		}
		if err != nil {
			return fmt.Errorf("provedor %s criado e definido como padrão, mas o perfil ativo não foi atualizado: %w", id, err)
		}
	}
	_, err = fmt.Fprintf(out, "Provedor '%s' criado com sucesso (%s). Modelo: %s\n", req.Name, id, req.DefaultModel)
	return err
}
