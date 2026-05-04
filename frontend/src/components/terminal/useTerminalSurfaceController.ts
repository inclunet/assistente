import { useEffect, useRef } from 'react';
import { useTerminalStore } from '../../store/terminalStore';
import { useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';

function isWorkspaceTabActive(tabId: string): boolean {
  return useWorkspaceStore.getState().workspace?.activeTabId === tabId;
}

export function useTerminalSurfaceController(tab: WorkspaceTab, isActive: boolean) {
  const updateWorkspaceTab = useWorkspaceStore((state) => state.updateTab);
  const isWsInitialized = useWorkspaceStore((state) => state.isInitialized);
  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  const sessionId = (tab.state?.sessionId as string) || '';
  useEffect(() => {
    if (!isWsInitialized || !isActive || tab.type !== 'terminal') return;

    const syncKey = `${tab.id}:${sessionId}`;

    const store = useTerminalStore.getState();
    if (sessionId) {
      const exists = store.sessions.some((session) => session.id === sessionId);
      if (exists) {
        if (!store.historyBySession[sessionId]) {
          void store.loadHistory(sessionId);
        }
        lastSyncedRef.current = syncKey;
        return;
      }

      if (lastSyncedRef.current === syncKey) return;

      store.loadSessions().then(() => {
        if (!isWorkspaceTabActive(tab.id)) return;

        const reloaded = useTerminalStore.getState();
        if (reloaded.sessions.some((session) => session.id === sessionId)) {
          if (!reloaded.historyBySession[sessionId]) {
            void reloaded.loadHistory(sessionId);
          }
          lastSyncedRef.current = syncKey;
        } else {
          void recoverStaleSession(tab.id);
        }
      }).catch((error: unknown) => {
        if (!isWorkspaceTabActive(tab.id)) return;

        console.error('[TerminalSurfaceController] Erro ao carregar sessões:', error);
        void recoverStaleSession(tab.id);
      });
      return;
    }

    if (!creatingRef.current) {
      void createSessionForTab(tab.id);
    }
  }, [isActive, isWsInitialized, sessionId, tab.id, tab.type]);

  async function recoverStaleSession(tabId: string) {
    creatingRef.current = true;
    try {
      const newSessionId = await useTerminalStore.getState().createSession();
      if (newSessionId) {
        await updateWorkspaceTab(tabId, { state: { sessionId: newSessionId } });
        lastSyncedRef.current = `${tabId}:${newSessionId}`;
      }
    } catch (error) {
      console.error('[TerminalSurfaceController] Erro ao recuperar sessão obsoleta:', error);
    } finally {
      creatingRef.current = false;
    }
  }

  async function createSessionForTab(tabId: string) {
    creatingRef.current = true;
    try {
      const newSessionId = await useTerminalStore.getState().createSession();
      if (newSessionId) {
        await updateWorkspaceTab(tabId, { state: { sessionId: newSessionId } });
        lastSyncedRef.current = `${tabId}:${newSessionId}`;
      }
    } catch (error) {
      console.error('[TerminalSurfaceController] Erro ao criar sessão:', error);
    } finally {
      creatingRef.current = false;
    }
  }
}
