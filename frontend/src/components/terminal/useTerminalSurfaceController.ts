import { logger } from '../../utils/logger';
import { useEffect } from 'react';
import { useTerminalStore } from '../../store/terminalStore';
import { useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';

function isWorkspaceTabActive(tabId: string): boolean {
  return useWorkspaceStore.getState().workspace?.activeTabId === tabId;
}

export function useTerminalSurfaceController(tab: WorkspaceTab, isActive: boolean) {
  const isWsInitialized = useWorkspaceStore((state) => state.isInitialized);

  const sessionId = (tab.state?.sessionId as string) || '';
  useEffect(() => {
    if (!isWsInitialized || !isActive || tab.type !== 'terminal') return;

    const store = useTerminalStore.getState();
    if (sessionId) {
      const exists = store.sessions.some((session) => session.id === sessionId);
      if (exists) {
        if (!store.historyBySession[sessionId]) {
          void store.loadHistory(sessionId);
        }
        return;
      }

      void store.loadSessions().then(() => {
        if (!isWorkspaceTabActive(tab.id)) return;

        const reloaded = useTerminalStore.getState();
        if (reloaded.sessions.some((session) => session.id === sessionId)) {
          if (!reloaded.historyBySession[sessionId]) {
            void reloaded.loadHistory(sessionId);
          }
        }
      }).catch((error: unknown) => {
        if (isWorkspaceTabActive(tab.id)) {
          logger.error('[TerminalSurfaceController] Erro ao carregar sessões:', error);
        }
      });
      return;
    }
  }, [isActive, isWsInitialized, sessionId, tab.id, tab.type]);
}
