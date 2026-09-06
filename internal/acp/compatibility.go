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
// Os valores são estruturados, estáveis e de baixa cardinalidade para poderem
// ser contados nos logs. Identificador de sessão, comando, argumentos e paths
// locais ficam de fora: não são necessários para a métrica e exporiam detalhes
// da instalação.
func recordCompatibility(
	feature string,
	attrs ...slog.Attr,
) {
	fields := []any{
		slog.String("compatibility_feature", feature),
	}
	for _, attr := range attrs {
		fields = append(fields, attr)
	}
	// Deliberadamente não herda o contexto do turno: Logger acrescentaria IDs
	// de conversa, turno, perfil e tool, que não pertencem a esta métrica.
	ctx := context.Background()
	logging.Logger(ctx, logComponent).InfoContext(ctx, "compatibilidade ACP utilizada", fields...)
}
