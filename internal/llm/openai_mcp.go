package llm

import (
	"log"
	"strings"

	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
)

// pendingMCPCall acumula o estado de um item mcp_call (MCP nativo) durante o
// streaming da Responses API, keyed por item_id.
type pendingMCPCall struct {
	ID          string
	Name        string
	ServerLabel string
	Args        strings.Builder
	// Completed marca que recebemos response.mcp_call.completed mas ainda não
	// finalizamos via response.output_item.done. Usado pelo fallback pós-stream.
	Completed bool
}

// buildNativeMCPTools converte os MCP servers configurados em tools type:"mcp"
// para a Responses API. Cada server vira uma tool com headers de auth, headers
// custom e allowed tools preservados. Retorna nil quando não há servers.
func buildNativeMCPTools(mcpServers []MCPServerConfig) []responses.ToolUnionParam {
	if len(mcpServers) == 0 {
		return nil
	}
	respTools := make([]responses.ToolUnionParam, 0, len(mcpServers))
	for _, srv := range mcpServers {
		mcpTool := responses.ToolParamOfMcp(srv.Name, srv.URL)
		mcpTool.OfMcp.RequireApproval = responses.ToolMcpRequireApprovalUnionParam{
			OfMcpToolApprovalSetting: param.NewOpt(string(responses.ToolMcpRequireApprovalMcpToolApprovalSettingNever)),
		}
		if srv.AuthToken != "" {
			mcpTool.OfMcp.Headers = map[string]string{
				"Authorization": "Bearer " + srv.AuthToken,
			}
		}
		for k, v := range srv.Headers {
			if mcpTool.OfMcp.Headers == nil {
				mcpTool.OfMcp.Headers = make(map[string]string)
			}
			mcpTool.OfMcp.Headers[k] = v
		}
		if len(srv.AllowedTools) > 0 {
			mcpTool.OfMcp.AllowedTools = responses.ToolMcpAllowedToolsUnionParam{
				OfMcpAllowedTools: srv.AllowedTools,
			}
		}
		respTools = append(respTools, mcpTool)
		log.Printf("[OpenAIProvider] MCP native tool: label=%q url=%q hasAuth=%v allowedTools=%d",
			srv.Name, srv.URL, srv.AuthToken != "", len(srv.AllowedTools))
	}
	return respTools
}

// flushPendingCompletedMCPCalls emite eventos de conclusão (IsCompleted) para os
// mcp_call sinalizados como concluídos via response.mcp_call.completed mas que
// nunca receberam response.output_item.done (que os removeria do mapa). Sem isto,
// endpoints/proxies que omitem output_item.done fariam a tool nativa aparecer
// "rodando" no streaming e sumir do histórico (nada persistido). O output não está
// disponível neste caminho; preservamos ao menos a chamada e seus argumentos.
// Retorna true se emitiu ao menos um evento.
func flushPendingCompletedMCPCalls(active map[string]*pendingMCPCall, handler StreamHandler) bool {
	emitted := false
	for itemID, mc := range active {
		if mc == nil || !mc.Completed {
			continue
		}
		emitted = true
		handler.OnMCPToolEvent(MCPToolEvent{
			ID:          mc.ID,
			Name:        mc.Name,
			ServerLabel: mc.ServerLabel,
			Arguments:   mc.Args.String(),
			IsCompleted: true,
		})
		delete(active, itemID)
	}
	return emitted
}
