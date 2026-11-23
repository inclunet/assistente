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
        if (!store.historyBySession[sessionId] || store.partialHistoryBySession?.[sessionId]) {
          void store.loadHistory(sessionId);
        }
        return;
      }

      // Uma listagem pode ser descartada e substituída por outra após um
      // evento de lifecycle. Observe a sessão até a resposta vigente chegar.
      let observing = true;
      const hydrate = () => {
        if (!observing || !isWorkspaceTabActive(tab.id)) return;
        const reloaded = useTerminalStore.getState();
        if (!reloaded.sessions.some((session) => session.id === sessionId)) return;
        observing = false;
        unsubscribe();
        if (!reloaded.historyBySession[sessionId] || reloaded.partialHistoryBySession?.[sessionId]) void reloaded.loadHistory(sessionId);
      };
      const unsubscribe = useTerminalStore.subscribe((next, previous) => {
        if (next.sessions !== previous.sessions) hydrate();
        if (previous.isLoadingSessions && !next.isLoadingSessions) {
          // Um evento de ciclo de vida pode iniciar outra consulta no mesmo despacho.
          queueMicrotask(() => {
            if (!useTerminalStore.getState().isLoadingSessions) unsubscribe();
          });
        }
      });
      void store.loadSessions().then((published) => {
        hydrate();
        if (published || useTerminalStore.getState().sessionListFailed) unsubscribe();
      }).catch((error: unknown) => {
        unsubscribe();
        if (isWorkspaceTabActive(tab.id)) {
          logger.error('[TerminalSurfaceController] Erro ao carregar sessões:', error);
        }
      });
      return () => { observing = false; unsubscribe(); };
    }
  }, [isActive, isWsInitialized, sessionId, tab.id, tab.type]);
}
