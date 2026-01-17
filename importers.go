package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ==================== Import API (OpenAPI/Postman) ====================

// OpenAPIImportResult representa o resultado da importação OpenAPI para a UI
type OpenAPIImportResult struct {
	DisplayName    string             `json:"display_name"`
	Description    string             `json:"description"`
	BaseURL        string             `json:"base_url"`
	AuthType       string             `json:"auth_type"`
	AuthConfig     map[string]string  `json:"auth_config"`
	DefaultHeaders map[string]string  `json:"default_headers"`
	Endpoints      []ImportedEndpoint `json:"endpoints"`
}

// ImportedEndpoint representa um endpoint importado
type ImportedEndpoint struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Method           string                 `json:"method"`
	PathTemplate     string                 `json:"path_template"`
	QueryTemplate    string                 `json:"query_template"`
	HeadersJSON      string                 `json:"headers_json"`
	BodyTemplate     string                 `json:"body_template"`
	Parameters       map[string]interface{} `json:"parameters"`
	ResponseTemplate string                 `json:"response_template"`
}

// ParseOpenAPISpec analisa uma especificação OpenAPI/Swagger e retorna os dados para preview
func (a *App) ParseOpenAPISpec(content string) (*OpenAPIImportResult, error) {
	result, err := a.agentManager.ParseOpenAPISpec(content)
	if err != nil {
		return nil, err
	}

	// Converte para o tipo exportado (main package)
	endpoints := make([]ImportedEndpoint, 0, len(result.Endpoints))
	for _, ep := range result.Endpoints {
		endpoints = append(endpoints, ImportedEndpoint{
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       ep.Parameters,
			ResponseTemplate: ep.ResponseTemplate,
		})
	}

	return &OpenAPIImportResult{
		DisplayName:    result.DisplayName,
		Description:    result.Description,
		BaseURL:        result.BaseURL,
		AuthType:       result.AuthType,
		AuthConfig:     result.AuthConfig,
		DefaultHeaders: result.DefaultHeaders,
		Endpoints:      endpoints,
	}, nil
}

// ParsePostmanCollection analisa uma coleção Postman e retorna os dados para preview
func (a *App) ParsePostmanCollection(content string) (*OpenAPIImportResult, error) {
	result, err := a.agentManager.ParsePostmanCollection(content)
	if err != nil {
		return nil, err
	}

	// Converte para o tipo exportado (main package)
	endpoints := make([]ImportedEndpoint, 0, len(result.Endpoints))
	for _, ep := range result.Endpoints {
		endpoints = append(endpoints, ImportedEndpoint{
			Name:             ep.Name,
			Description:      ep.Description,
			Method:           ep.Method,
			PathTemplate:     ep.PathTemplate,
			QueryTemplate:    ep.QueryTemplate,
			HeadersJSON:      ep.HeadersJSON,
			BodyTemplate:     ep.BodyTemplate,
			Parameters:       ep.Parameters,
			ResponseTemplate: ep.ResponseTemplate,
		})
	}

	return &OpenAPIImportResult{
		DisplayName:    result.DisplayName,
		Description:    result.Description,
		BaseURL:        result.BaseURL,
		AuthType:       result.AuthType,
		AuthConfig:     result.AuthConfig,
		DefaultHeaders: result.DefaultHeaders,
		Endpoints:      endpoints,
	}, nil
}

// ImportOpenAPIToHTTPAgent importa uma especificação OpenAPI como um novo HTTP Agent
func (a *App) ImportOpenAPIToHTTPAgent(content, name string) (*HTTPAgentFullConfig, error) {
	// Parse a especificação
	parsed, err := a.ParseOpenAPISpec(content)
	if err != nil {
		return nil, err
	}

	// Usa o nome fornecido ou gera a partir do título
	agentName := name
	if agentName == "" {
		agentName = sanitizeAgentName(parsed.DisplayName)
	}

	// Converte configs para JSON
	authConfigJSON, _ := json.Marshal(parsed.AuthConfig)
	headersJSON, _ := json.Marshal(parsed.DefaultHeaders)

	// Cria o HTTP Agent
	agent, err := a.CreateHTTPAgentFull(
		agentName,
		parsed.DisplayName,
		parsed.Description,
		"gpt-4o-mini",
		"",
		true,
		parsed.BaseURL,
		parsed.AuthType,
		string(authConfigJSON),
		string(headersJSON),
		30,
		3,
	)
	if err != nil {
		return nil, err
	}

	// Cria os endpoints
	for _, ep := range parsed.Endpoints {
		paramsJSON, _ := json.Marshal(ep.Parameters)
		_, err := a.CreateHTTPEndpoint(
			agent.HTTPAgentID,
			ep.Name,
			ep.Description,
			ep.Method,
			ep.PathTemplate,
			ep.QueryTemplate,
			ep.HeadersJSON,
			ep.BodyTemplate,
			string(paramsJSON),
			ep.ResponseTemplate,
		)
		if err != nil {
			// Log erro mas continua
			fmt.Printf("Erro ao criar endpoint %s: %v\n", ep.Name, err)
		}
	}

	// Recarrega para ter os endpoints
	return a.GetHTTPAgentFull(agent.ID)
}

// ImportPostmanToHTTPAgent importa uma coleção Postman como um novo HTTP Agent
func (a *App) ImportPostmanToHTTPAgent(content, name string) (*HTTPAgentFullConfig, error) {
	// Parse a coleção
	parsed, err := a.ParsePostmanCollection(content)
	if err != nil {
		return nil, err
	}

	// Usa o nome fornecido ou gera a partir do título
	agentName := name
	if agentName == "" {
		agentName = sanitizeAgentName(parsed.DisplayName)
	}

	// Converte configs para JSON
	authConfigJSON, _ := json.Marshal(parsed.AuthConfig)
	headersJSON, _ := json.Marshal(parsed.DefaultHeaders)

	// Cria o HTTP Agent
	agent, err := a.CreateHTTPAgentFull(
		agentName,
		parsed.DisplayName,
		parsed.Description,
		"gpt-4o-mini",
		"",
		true,
		parsed.BaseURL,
		parsed.AuthType,
		string(authConfigJSON),
		string(headersJSON),
		30,
		3,
	)
	if err != nil {
		return nil, err
	}

	// Cria os endpoints
	for _, ep := range parsed.Endpoints {
		paramsJSON, _ := json.Marshal(ep.Parameters)
		_, err := a.CreateHTTPEndpoint(
			agent.HTTPAgentID,
			ep.Name,
			ep.Description,
			ep.Method,
			ep.PathTemplate,
			ep.QueryTemplate,
			ep.HeadersJSON,
			ep.BodyTemplate,
			string(paramsJSON),
			ep.ResponseTemplate,
		)
		if err != nil {
			fmt.Printf("Erro ao criar endpoint %s: %v\n", ep.Name, err)
		}
	}

	// Recarrega para ter os endpoints
	return a.GetHTTPAgentFull(agent.ID)
}

// sanitizeAgentName converte um nome para formato de ID válido
func sanitizeAgentName(name string) string {
	result := strings.ToLower(name)
	replacer := strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		"/", "_",
	)
	result = replacer.Replace(result)

	// Remove caracteres inválidos
	var sb strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		}
	}
	result = sb.String()

	// Remove underscores duplicados
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}

	return strings.Trim(result, "_")
}
