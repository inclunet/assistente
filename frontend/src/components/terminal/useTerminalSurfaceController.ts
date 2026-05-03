import { useEffect, useRef } from 'react';
import { useTerminalStore } from '../../store/terminalStore';
import { useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';

export function useTerminalSurfaceController(tab: WorkspaceTab, isActive: boolean) {
  const updateWorkspaceTab = useWorkspaceStore((state) => state.updateTab);
  const isWsInitialized = useWorkspaceStore((state) => state.isInitialized);
  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  const sessionId = (tab.state?.sessionId as string) || '';

  useEffect(() => {
    if (!isWsInitialized || !isActive || tab.type !== 'terminal') return;

    const syncKey = `${tab.id}:${sessionId}`;
    if (lastSyncedRef.current === syncKey) return;

    const store = useTerminalStore.getState();
    if (sessionId) {
      const exists = store.sessions.some((session) => session.id === sessionId);
      if (exists) {
        if (store.activeSessionId !== sessionId) {
          store.setActiveSession(sessionId);
        }
        lastSyncedRef.current = syncKey;
        return;
      }

      store.loadSessions().then(() => {
        const reloaded = useTerminalStore.getState();
        if (reloaded.sessions.some((session) => session.id === sessionId)) {
          reloaded.setActiveSession(sessionId);
          lastSyncedRef.current = syncKey;
        } else {
          void recoverStaleSession(tab.id);
        }
      });
      return;
    }

    if (!creatingRef.current) {
      void createSessionForTab(tab.id);
    }
  }, [isActive, isWsInitialized, sessionId, tab.id, tab.type]);

  useEffect(() => () => {
    const currentSessionId = (useWorkspaceStore.getState().workspace?.tabs.find((candidate) => candidate.id === tab.id)?.state?.sessionId as string | undefined)
      || sessionId;
    if (currentSessionId) {
      void useTerminalStore.getState().closeSession(currentSessionId);
    }
  }, [sessionId, tab.id]);

  async function recoverStaleSession(tabId: string) {
    creatingRef.current = true;
    try {
      await useTerminalStore.getState().createSession();
      const newSessionId = useTerminalStore.getState().activeSessionId;
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
      await useTerminalStore.getState().createSession();
      const newSessionId = useTerminalStore.getState().activeSessionId;
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
