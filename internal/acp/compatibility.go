package acp

import (
	"context"
	"log/slog"

	"assistente/internal/logging"
)

const (
	compatibilityLegacyModelsPayload = "legacy_models_payload"
	compatibilityLegacySelector      = "legacy_session_selector"
)

// recordCompatibility registra o uso efetivo de um contrato ACP anterior.
// Esses eventos são a evidência necessária para decidir uma futura expiração:
// presença do código ou de testes não diz se um agente real ainda depende dele.
//
// Os valores são estruturados e estáveis para poderem ser contados nos logs.
// O identificador da sessão e a descrição do agente passam pelo slog, que
// escapa quebras de linha vindas do protocolo antes de escrever.
func recordCompatibility(
	ctx context.Context,
	cfg Config,
	feature string,
	sessionID string,
	attrs ...slog.Attr,
) {
	fields := []any{
		slog.String("compatibility_feature", feature),
		slog.String("agent", describeAgent(cfg)),
	}
	if sessionID != "" {
		fields = append(fields, slog.String("session_id", sessionID))
	}
	for _, attr := range attrs {
		fields = append(fields, attr)
	}
	logging.Logger(ctx, logComponent).InfoContext(ctx, "compatibilidade ACP utilizada", fields...)
}
