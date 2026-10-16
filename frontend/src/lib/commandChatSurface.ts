import type { SurfaceContext } from './chatSurface';

export type ChatCommandExecutionStatus = 'idle' | 'queued' | 'running';

export interface ChatCommandSurfaceSource {
  readonly tabId: string;
  readonly conversationId: string;
  readonly sessionKey: string;
  readonly sessionLoaded: boolean;
  readonly executionStatus: ChatCommandExecutionStatus;
}

/**
 * Monta somente os fatos de conversa que podem ser usados pela superfície de
 * comandos. O leitor é deliberadamente puro: a página relê os stores e passa
 * um snapshot novo a cada leitura do getter registrado pelo hook.
 */
export function readChatCommandSurface(
  source: ChatCommandSurfaceSource | null | undefined,
): SurfaceContext | null {
  if (!source || !source.tabId || !source.conversationId || !source.sessionKey || !source.sessionLoaded) {
    return null;
  }

  const resourceIdentity = `${source.tabId}:${source.conversationId}:${source.sessionKey}`;
  const factsVersion = `loaded=${source.sessionLoaded ? '1' : '0'};execution=${source.executionStatus}`;

  return {
    surfaceType: 'chat',
    surfaceId: source.tabId,
    snapshotVersion: `chat-command-v1:${resourceIdentity}:${factsVersion}`,
    metadata: {
      conversationId: source.conversationId,
      sessionKey: source.sessionKey,
      sessionLoaded: source.sessionLoaded,
      executionStatus: source.executionStatus,
      resourceIdentity,
    },
  };
}
