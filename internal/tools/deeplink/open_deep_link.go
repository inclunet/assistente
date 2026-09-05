package deeplink

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

// DeepLinkEmitter é a interface para emitir deep links ao frontend.
type DeepLinkEmitter interface {
	EmitDeepLink(uri string)
}

type openDeepLinkArgs struct {
	URI string `json:"uri"`
}

// OpenDeepLinkTool executa um deep link no frontend via evento Wails.
type OpenDeepLinkTool struct {
	emitter DeepLinkEmitter
}

func NewOpenDeepLink(emitter DeepLinkEmitter) *OpenDeepLinkTool {
	return &OpenDeepLinkTool{emitter: emitter}
}

func (t *OpenDeepLinkTool) Name() string { return "open_deep_link" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *OpenDeepLinkTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "app", Class: "app_tool", Package: "basic", Risk: "read"}
}

func (t *OpenDeepLinkTool) Description() string {
	return `Opens a deep link in the application. Supported URIs:
- Conversations: assistente://conversation/{id}, assistente://conversation/new?message=...&title=..., assistente://conversation/{id}/send?message=...
  Conversation links accept an optional profile={slug} query parameter that forces the target conversation to use a specific profile (works for new, open and send). Add it with "?" when the URI has no query yet, or with "&" when it already has one — e.g. assistente://conversation/{id}?profile=techsupport or assistente://conversation/new?message=...&profile=programacao.
- Tab open: assistente://tasklist/{id}, assistente://editor/{id}, assistente://terminal/{id}
- Tab create: assistente://tasklist/new?title=..., assistente://editor/new?title=..., assistente://editor/open?file=..., assistente://terminal/new?cmd=...
- Navigate: assistente://navigate/{route}. Settings screens are tabs, so the route keeps the settings/ prefix (routes: settings, settings/providers, settings/mcp, settings/skills, settings/channels, settings/contacts, settings/credentials, settings/allowlists, settings/network-allowlist, settings/path-allowlist, settings/appearance, settings/data, settings/restore-defaults, profiles, history, memories, tasklists, help, about, update)
- Resource edit/new: assistente://{resource}/new, assistente://{resource}/edit/{id} (resources: profiles, providers, credentials, allowlists, skills, mcp, channels, tasklists). Profile editing accepts the validated tab=voice query parameter to open voice settings directly: assistente://profiles/edit/{slug}?tab=voice`
}

func (t *OpenDeepLinkTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"uri": {
				"type": "string",
				"description": "Deep link URI starting with assistente://"
			}
		},
		"required": ["uri"],
		"additionalProperties": false
	}`)
}

func (t *OpenDeepLinkTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params openDeepLinkArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}

	uri := strings.TrimSpace(params.URI)
	if uri == "" {
		return tools.ToolResult{Content: "URI cannot be empty", IsError: true}, nil
	}

	if !strings.HasPrefix(uri, "assistente://") {
		return tools.ToolResult{Content: "URI must start with assistente://", IsError: true}, nil
	}

	t.emitter.EmitDeepLink(uri)

	return tools.ToolResult{
		Content: fmt.Sprintf("Deep link executed: %s", uri),
	}, nil
}
