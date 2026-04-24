package llm

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"strings"
	"time"
)

type MCPFailureStage string

const (
	MCPFailureStageListTools MCPFailureStage = "list_tools"
	MCPFailureStageCall      MCPFailureStage = "call"
	MCPFailureStageHandshake MCPFailureStage = "handshake"
	MCPFailureStageUnknown   MCPFailureStage = "unknown"
)

type MCPAttemptFailure struct {
	ServerName  string
	ServerSlug  string
	Stage       MCPFailureStage
	Message     string
	Recoverable bool
	Degradable  bool
}

type mcpStreamAttemptResult struct {
	done       bool
	retry      bool
	mcpFailure *MCPAttemptFailure
}

func maxMCPDegradationRetries(serverCount int) int {
	if serverCount <= 0 {
		return 0
	}
	if serverCount < 3 {
		return serverCount
	}
	return 3
}

func cloneMCPServers(servers []MCPServerConfig) []MCPServerConfig {
	if len(servers) == 0 {
		return nil
	}
	cloned := make([]MCPServerConfig, len(servers))
	copy(cloned, servers)
	return cloned
}

func inferMCPFailure(stage MCPFailureStage, message, rawJSON, fallbackServer string, servers []MCPServerConfig) *MCPAttemptFailure {
	server := fallbackServer
	if server == "" {
		server = extractServerLabelFromRawJSON(rawJSON)
	}
	if server == "" {
		server = matchMCPServerInText(strings.ToLower(message+" "+rawJSON), servers)
	}
	if server == "" && len(servers) == 1 && looksLikeMCPFailure(message+" "+rawJSON) {
		server = servers[0].Name
	}
	if server == "" {
		return nil
	}

	matched, ok := findMCPServer(servers, server)
	if !ok {
		return nil
	}

	fullMessage := strings.TrimSpace(message)
	if fullMessage == "" {
		fullMessage = formatMCPFailureUserMessage(matched.Name, stage)
	}

	return &MCPAttemptFailure{
		ServerName:  matched.Name,
		ServerSlug:  matched.Slug,
		Stage:       stage,
		Message:     strings.TrimSpace(fullMessage),
		Recoverable: looksRecoverableMCPFailure(fullMessage),
		Degradable:  true,
	}
}

func planMCPDegradationRetry(
	ctx context.Context,
	provider string,
	attempt int,
	servers []MCPServerConfig,
	failure *MCPAttemptFailure,
) ([]MCPServerConfig, bool) {
	if failure == nil || !failure.Degradable {
		return nil, false
	}

	remaining, removed, ok := removeMCPServer(servers, failure.ServerName, failure.ServerSlug)
	if !ok {
		return nil, false
	}

	log.Printf("[MCP-DEGRADE] attempt=%d provider=%s server=%s stage=%s action=retry_without_server recoverable=%v",
		attempt, provider, removed.Name, failure.Stage, failure.Recoverable)

	if removed.Recover != nil {
		go func(provider string, removed MCPServerConfig) {
			recoveryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := removed.Recover(recoveryCtx); err != nil {
				log.Printf("[MCP-RECOVER] provider=%s server=%s err=%v", provider, removed.Name, err)
			} else {
				log.Printf("[MCP-RECOVER] provider=%s server=%s err=nil", provider, removed.Name)
			}
		}(provider, removed)
	}

	return remaining, true
}

func removeMCPServer(servers []MCPServerConfig, serverName, serverSlug string) ([]MCPServerConfig, MCPServerConfig, bool) {
	if serverSlug != "" {
		for idx, srv := range servers {
			if srv.Slug == serverSlug {
				remaining := make([]MCPServerConfig, 0, len(servers)-1)
				remaining = append(remaining, servers[:idx]...)
				remaining = append(remaining, servers[idx+1:]...)
				return remaining, srv, true
			}
		}
	}
	for idx, srv := range servers {
		if srv.Name == serverName {
			remaining := make([]MCPServerConfig, 0, len(servers)-1)
			remaining = append(remaining, servers[:idx]...)
			remaining = append(remaining, servers[idx+1:]...)
			return remaining, srv, true
		}
	}
	return nil, MCPServerConfig{}, false
}

func findMCPServer(servers []MCPServerConfig, nameOrSlug string) (MCPServerConfig, bool) {
	for _, srv := range servers {
		if srv.Slug == nameOrSlug {
			return srv, true
		}
	}
	for _, srv := range servers {
		if srv.Name == nameOrSlug {
			return srv, true
		}
	}
	return MCPServerConfig{}, false
}

func formatMCPFailureUserMessage(serverName string, stage MCPFailureStage) string {
	stageText := "durante a comunicação"
	switch stage {
	case MCPFailureStageListTools:
		stageText = "durante a listagem de ferramentas"
	case MCPFailureStageCall:
		stageText = "durante a execução de uma ferramenta"
	case MCPFailureStageHandshake:
		stageText = "durante a conexão"
	}
	return `Falha no servidor MCP "` + serverName + `" ` + stageText + `. Tente novamente ou verifique as credenciais e a conexão.`
}

func extractServerLabelFromRawJSON(raw string) string {
	if raw == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}

	for _, key := range []string{"server_label", "serverLabel", "server_name", "serverName", "name"} {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}

	if item, ok := payload["item"].(map[string]any); ok {
		for _, key := range []string{"server_label", "serverLabel", "server_name", "serverName", "name"} {
			if value, ok := item[key].(string); ok && value != "" {
				return value
			}
		}
	}

	return ""
}

func matchMCPServerInText(text string, servers []MCPServerConfig) string {
	for _, srv := range servers {
		candidates := []string{
			strings.ToLower(srv.Name),
			strings.ToLower(srv.Slug),
			strings.ToLower(hostnameFromURL(srv.URL)),
		}
		for _, candidate := range candidates {
			if candidate != "" && strings.Contains(text, candidate) {
				return srv.Name
			}
		}
	}
	return ""
}

func hostnameFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func looksLikeMCPFailure(text string) bool {
	lower := strings.ToLower(text)
	for _, token := range []string{
		"mcp",
		"connector",
		"tool list",
		"list tools",
		"failed dependency",
		"server unhealthy",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func looksRecoverableMCPFailure(text string) bool {
	lower := strings.ToLower(text)
	for _, token := range []string{
		"auth",
		"authentication",
		"unauthorized",
		"forbidden",
		"token",
		"credential",
		"refresh",
		"expired",
		"timeout",
		"temporar",
		"failed dependency",
		"server unhealthy",
		"unhealthy",
		"connect",
		"dependency",
		"424",
		"5xx",
	} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
