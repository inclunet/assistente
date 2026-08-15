import { useTerminalStore } from '../store/terminalStore';
import { type TabType, useWorkspaceStore } from '../store/workspaceStore';

/**
 * Cria uma aba e os recursos de domínio exigidos por ela.
 *
 * Uma ação explícita "nova aba de terminal" cria uma sessão nova e grava seu
 * sessionId na aba. Abrir uma sessão existente continua usando addTab com estado
 * explícito (por exemplo, no fluxo de deep link).
 */
export async function createWorkspaceTab(type: TabType, title: string): Promise<string> {
  const workspaceStore = useWorkspaceStore.getState();
  if (type !== 'terminal') {
    return workspaceStore.addTab(type, title);
  }

  const terminalStore = useTerminalStore.getState();
  const sessionId = await terminalStore.createSession();
  if (!sessionId) {
    throw new Error('não foi possível criar a sessão de terminal');
  }

  const sessionsLoaded = await terminalStore.loadSessions();
  if (!sessionsLoaded) {
    await terminalStore.closeSession(sessionId);
    throw new Error('não foi possível confirmar a sessão de terminal criada');
  }

  try {
    return await workspaceStore.addTab(type, title, { sessionId });
  } catch (error) {
    await terminalStore.closeSession(sessionId);
    throw error;
  }
}
