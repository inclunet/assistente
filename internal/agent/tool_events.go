package agent

import (
	"assistente/internal/core/ports"
	"assistente/internal/events"
)

// Tool origin constants (AEP-0039 Fase 1).
const (
	OriginBuiltin   = "builtin"
	OriginMCPBridge = "mcp_bridge"
	OriginMCPNative = "mcp_native"
)

// EmitToolStart emits a chat:tool_start event with the Origin field populated.
// All tool_start emissions MUST go through this helper (AEP-0039).
func EmitToolStart(emitter events.Emitter, ev ports.ToolStartEvent) {
	// Backward compat: keep Native in sync with Origin
	if ev.Origin == OriginMCPNative {
		ev.Native = true
	}
	emitter.Emit("chat:tool_start", ev)
}

// EmitToolEnd emits a chat:tool_end event with the Origin field populated.
// All tool_end emissions MUST go through this helper (AEP-0039).
func EmitToolEnd(emitter events.Emitter, ev ports.ToolEndEvent) {
	// Backward compat: keep Native in sync with Origin
	if ev.Origin == OriginMCPNative {
		ev.Native = true
	}
	emitter.Emit("chat:tool_end", ev)
}
