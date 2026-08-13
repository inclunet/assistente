// Tipos dos eventos de run de sub-agente (AEP-0068 F5).
//
// Espelham à mão o struct `subagent.RunEvent` do backend: payload de evento não
// aparece em assinatura de método exportado do App, então o gerador de bindings
// não o escreve em `@wailsjs/go/models` — importá-lo de lá seria erro.

/** Emitido quando um run de sub-agente começa a executar. */
export const SUBAGENT_RUN_STARTED_EVENT = 'subagent:run-started';

/** Emitido quando um run de sub-agente atinge um estado terminal. */
export const SUBAGENT_RUN_FINISHED_EVENT = 'subagent:run-finished';

/** Status possíveis de um run (espelha o enum de `database.SubAgentRunStatus*`). */
export type SubAgentRunStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'timed_out';

/**
 * Payload dos eventos acima. `conversationId` é a sub-conversa do run e vem
 * sempre preenchido, como exige o contrato de eventos do projeto.
 */
export interface SubAgentRunEvent {
  runId: string;
  conversationId: string;
  parentConversationId?: string;
  title?: string;
  status: string;
  background: boolean;
  error?: string;
}

/** Status em que o run ainda está em andamento — os únicos canceláveis. */
export const ACTIVE_SUBAGENT_RUN_STATUSES: ReadonlySet<string> = new Set<SubAgentRunStatus>([
  'queued',
  'running',
]);

export function isActiveSubAgentRunStatus(status: string | undefined): boolean {
  return status !== undefined && ACTIVE_SUBAGENT_RUN_STATUSES.has(status);
}
