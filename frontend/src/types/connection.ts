// Tipos do indicador de status de conexão com a API LLM (Issue #38).
// Espelham o payload tipado emitido pelo backend em `internal/connstatus`.

/** Nome do evento Wails emitido a cada ciclo de health check. */
export const CONNECTION_STATUS_EVENT = 'llm:connection-status';

/**
 * Estado da conexão com o provider/API ativo.
 * - `online`: endpoint acessível e autenticado
 * - `offline`: endpoint inacessível ou autenticação rejeitada
 * - `unauthenticated`: agente de código de pé, mas sem login (AEP-0084 D12).
 *   Estado próprio porque a saída é outra: não se conserta endereço nem
 *   credencial no app, roda-se o login do CLI do agente
 * - `checking`: sondagem em andamento (mostrado como "reconectando")
 * - `unknown`: ainda não houve verificação (estado inicial da UI)
 */
export type ConnectionState = 'online' | 'offline' | 'unauthenticated' | 'checking' | 'unknown';

/** Payload do evento `llm:connection-status` recebido do backend. */
export interface ConnectionStatusPayload {
  state: ConnectionState;
  providerId: string;
  providerName: string;
  model?: string;
  latencyMs: number;
  avgLatencyMs: number;
  error?: string;
  errorType?: string;
  timestamp: number;
}
