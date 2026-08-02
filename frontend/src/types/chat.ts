/**
 * Origem de uma ferramenta nos eventos de chat (AEP-0039, AEP-0084 D7).
 * `acp_agent` marca ferramentas que um agente externo rodou por conta própria.
 */
export type ToolOrigin = 'builtin' | 'mcp_bridge' | 'mcp_native' | 'acp_agent';

const APP_TOOL_ORIGINS: ReadonlySet<string> = new Set<ToolOrigin>(['builtin', 'mcp_bridge', 'mcp_native']);

/**
 * Diz se a ferramenta do evento foi executada pelo app. Quem age por nome de
 * ferramenta — recarregar o documento, fechar o modal — só pode agir quando a
 * resposta é sim: as do agente externo são informativas e uma edição dele não
 * passou pelo fluxo de aprovação do editor (AEP-0084 D7).
 */
export function isAppToolEvent(origin?: string): boolean {
  return !origin || APP_TOOL_ORIGINS.has(origin);
}

/**
 * Status de uma tool call em execução durante streaming.
 * Mantido fora dos componentes para que stores e serviços não dependam da UI.
 */
export interface ToolCallStatus {
  name: string;
  callId: string;
  args?: string;
  status: 'running' | 'done' | 'error';
  summary?: string;
  origin?: ToolOrigin;
}
