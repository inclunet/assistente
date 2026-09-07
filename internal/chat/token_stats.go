package chat

import "assistente/internal/apidto"

// TokenStats e ToolUsageBreakdown vivem em apidto (borda Wails, AEP-0088 D5).
// Aliases preservam o domínio chat sem duplicar o contrato tipado.
type TokenStats = apidto.TokenStats
type ToolUsageBreakdown = apidto.ToolUsageBreakdown
