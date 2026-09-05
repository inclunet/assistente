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
	return `Open, focus, or navigate to an Assistente resource by executing one exact assistente:// deep link in the frontend.
Use when: the user explicitly asks to open or navigate to an app resource. Reuse an exact link= or assistente:// URI when available; for example {"uri":"assistente://profiles/edit/programacao?tab=voice"} opens that profile directly on voice settings.
Do not use: to read resource contents, obtain permissions, open external HTTP(S) URLs, or merely offer a clickable reference (return a Markdown deep link instead). Do not invent IDs, slugs, paths, or routes. URL-encode generated path IDs and query values.
Focus and return: conversation and tab links focus an existing matching tab or open one when supported. Navigation changes the user's current view. The validated profile tab is only tab=voice. Caller-aware UI flows can return to the originating workspace surface after profile save/cancel, but this tool's backend event carries no caller context, so a direct profile edit opened here returns to the profiles list.
Supported forms:
- Conversations: assistente://conversation/{id}, assistente://conversation/new?message=...&title=..., assistente://conversation/{id}/send?message=.... Optional profile={slug} applies to new, open, and send; use ? for the first query parameter and & for later ones.
- Existing tabs: assistente://tasklist/{id}, assistente://editor/{id}, assistente://terminal/{id}.
- New tabs: assistente://tasklist/new?title=..., assistente://editor/new?title=..., assistente://editor/open?file=..., assistente://terminal/new?cmd=....
- Pages: assistente://navigate/{route}. The frontend accepts only: empty route, settings, settings/providers, settings/mcp, settings/skills, settings/channels, settings/contacts, settings/credentials, settings/allowlists, settings/network-allowlist, settings/path-allowlist, settings/appearance, settings/data, settings/restore-defaults, profiles, history, memories, tasklists, help, about, update. Settings tabs require the settings/ prefix.
- Resource forms: assistente://{resource}/new and assistente://{resource}/edit/{id}, for profiles, providers, credentials, allowlists, skills, mcp, channels, memories, or tasklists. Only profile edit accepts a tab parameter: assistente://profiles/edit/{slug}?tab=voice.
Risk: executing a link can interrupt the user's focus; new/edit/send links can create or change app state, and conversation send transmits a message. A terminal/new link with cmd runs the provided command through the frontend terminal flow rather than the run_command tool; use it only when the user explicitly asks to run that command. A deep link does not itself grant content access or authorization, and downstream actions keep their own validation. The Go tool requires a non-empty URI after trimming and checks the assistente:// prefix; the frontend parser rejects unsupported navigate routes and invalid required or validated parameter combinations. If unavailable, discover and load open_deep_link with tool_catalog when the profile permits on-demand tools.`
}

func (t *OpenDeepLinkTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"uri": {
				"type": "string",
				"description": "Exact assistente:// URI to execute. Reuse a provided link= value when possible; do not invent IDs, slugs, paths, or routes. URL-encode path IDs and query values. Unsupported navigate routes and invalid required or validated parameter combinations are rejected by the frontend parser."
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
