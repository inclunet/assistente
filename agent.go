package main

import (
	"context"

	"assistente/internal/agent"
	"assistente/internal/config"
	"assistente/internal/llm"
)

// ==================== Agentic Loop (thin adapter) ====================

// runAgenticLoop delega para a.agentSvc.RunAgenticLoop.
func (a *App) runAgenticLoop(
	ctx context.Context,
	_ *config.Config,
	messages []llm.Message,
	params llm.ChatParams,
	conversationID uint,
	turnID uint,
	toolDefs []llm.ToolDefinition,
	streamer llm.Streamer,
) {
	a.agentSvc.RunAgenticLoop(ctx, messages, params, conversationID, turnID, toolDefs, streamer,
		func(convID uint, iter int) agent.IterationHandler {
			return agent.NewIterationHandler(a.emitter, convID, iter)
		},
	)
}
