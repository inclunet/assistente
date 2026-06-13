// Tipos do aviso de inicialização parcial do runtime user-scoped pós-login
// (issue #250). Espelham o payload tipado emitido pelo backend em
// `internal/app/app_auth.go` (RuntimePartialInitPayload).

/** Nome do evento Wails emitido quando o reload pós-login termina com falhas. */
export const RUNTIME_PARTIAL_INIT_EVENT = 'runtime:partial-init';

/** Identificadores estáveis dos subsistemas reportados pelo backend. */
export const RUNTIME_SUBSYSTEMS = ['mcp', 'jobs', 'tool_invocations', 'timeout'] as const;

/**
 * Falha de um subsistema user-scoped durante o reload pós-login. Carrega só o
 * identificador estável do subsistema (usado para traduzir o aviso). A
 * mensagem de erro fica apenas nos logs do backend — não é emitida ao
 * frontend, evitando vazar detalhes internos (review PR #278).
 */
export interface RuntimeSubsystemFailure {
  subsystem: string;
}

/** Payload do evento `runtime:partial-init` recebido do backend. */
export interface RuntimePartialInitPayload {
  subsystems: RuntimeSubsystemFailure[];
}
