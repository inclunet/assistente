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

func (t *OpenDeepLinkTool) Description() string {
	return "Opens a deep link in the application. Use to navigate, open tabs, or trigger actions. Supported URIs: assistente://conversation/{id}, assistente://conversation/new?message=..., assistente://tasklist/{id}, assistente://editor/{id}, assistente://terminal/{id}, assistente://navigate/{route}"
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
