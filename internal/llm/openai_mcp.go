package llm

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
)

// mcpErrorLogMaxLen limita o texto de erro logado para falhas de MCP nativo,
// evitando linhas de log gigantes quando o servidor devolve um corpo extenso.
const mcpErrorLogMaxLen = 800

// truncateMCPError normaliza e limita, de forma segura para UTF-8, o texto de
// erro devolvido por um servidor MCP nativo, anexando um marcador quando o corte
// ocorre. Mantém a mensagem curta o suficiente para uma linha de log.
func truncateMCPError(errText string) string {
	errText = strings.TrimSpace(errText)
	if len(errText) <= mcpErrorLogMaxLen {
		return errText
	}
	truncated := errText[:mcpErrorLogMaxLen]
	// Recuar até um limite de rune válido para não cortar um caractere multibyte.
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "… (truncado)"
}

// mcpFailureLogFields monta os campos de diagnóstico de uma falha de MCP nativo
// (server_label, nome da tool e o erro truncado) para inclusão em logs de ERRO.
// Assim, falhas server-side (ex.: cloudId ausente/inválido no Atlassian) passam a
// explicar a causa sem exigir correlação manual entre o itemID e o output item.
func mcpFailureLogFields(serverLabel, name, errText string) string {
	return fmt.Sprintf("server=%q tool=%q error=%q", serverLabel, name, truncateMCPError(errText))
}

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
	// Failed marca que recebemos response.mcp_call.failed. O ERRO único e
	// correlacionado da falha sai em response.output_item.done (que carrega o
	// texto do erro) ou, se esse item nunca vier, no fallback de fim de stream —
	// nunca no próprio evento .failed, que geraria log duplicado.
	Failed bool
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
		logging.Infof(context.Background(), "llm.openai-mcp", "[OpenAIProvider] MCP native tool: label=%q url=%q hasAuth=%v allowedTools=%d",
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
