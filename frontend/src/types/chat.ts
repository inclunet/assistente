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
}
