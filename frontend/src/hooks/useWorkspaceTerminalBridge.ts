import { useEffect, useRef } from 'react';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useTerminalStore } from '../store/terminalStore';

/**
 * Sincroniza abas de terminal do workspace com o terminalStore.
 *
 * - state.sessionId vazio → cria sessão via terminalStore.createSession()
 * - state.sessionId existente → ativa sessão via terminalStore.setActiveSession()
 * - Remoção de aba → fecha sessão via terminalStore.closeSession()
 */
export function useWorkspaceTerminalBridge() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  const sessionId = (activeTab?.state?.sessionId as string) || '';

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab || activeTab.type !== 'terminal') return;

    const syncKey = `${activeTab.id}:${sessionId}`;
    if (lastSyncedRef.current === syncKey) return;

    if (sessionId) {
      const store = useTerminalStore.getState();
      const exists = store.sessions.some((s) => s.id === sessionId);
      if (exists) {
        if (store.activeSessionId !== sessionId) {
          store.setActiveSession(sessionId);
        }
        lastSyncedRef.current = syncKey;
      } else {
        store.loadSessions().then(() => {
          const reloaded = useTerminalStore.getState();
          if (reloaded.sessions.some((s) => s.id === sessionId)) {
            reloaded.setActiveSession(sessionId);
            lastSyncedRef.current = syncKey;
          } else {
            recoverStaleSession(activeTab!.id);
          }
        });
      }
    } else if (!creatingRef.current) {
      createSessionForTab(activeTab.id);
    }
  }, [activeTab?.id, activeTab?.type, sessionId, isWsInitialized]);

  async function recoverStaleSession(wsTabId: string) {
    creatingRef.current = true;
    try {
      await useTerminalStore.getState().createSession();
      const newSessionId = useTerminalStore.getState().activeSessionId;
      if (newSessionId) {
        await updateWsTab(wsTabId, { state: { sessionId: newSessionId } });
        lastSyncedRef.current = `${wsTabId}:${newSessionId}`;
      }
    } catch (error) {
      console.error('[WorkspaceTerminalBridge] Erro ao recuperar sessão obsoleta:', error);
    } finally {
      creatingRef.current = false;
    }
  }

  async function createSessionForTab(wsTabId: string) {
    creatingRef.current = true;
    try {
      await useTerminalStore.getState().createSession();
      const newSessionId = useTerminalStore.getState().activeSessionId;
      if (newSessionId) {
        await updateWsTab(wsTabId, { state: { sessionId: newSessionId } });
        lastSyncedRef.current = `${wsTabId}:${newSessionId}`;
      }
    } catch (error) {
      console.error('[WorkspaceTerminalBridge] Erro ao criar sessão:', error);
    } finally {
      creatingRef.current = false;
    }
  }

  // Cleanup: fechar sessão quando aba de terminal é removida
  const prevTermTabsRef = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    const unsub = useWorkspaceStore.subscribe((state) => {
      const wsTabs = state.workspace?.tabs || [];
      const currentTermTabs = new Map<string, string>();
      for (const t of wsTabs) {
        const sid = t.state?.sessionId as string | undefined;
        if (t.type === 'terminal' && sid) {
          currentTermTabs.set(t.id, sid);
        }
      }

      for (const [wsTabId, sid] of prevTermTabsRef.current) {
        if (!currentTermTabs.has(wsTabId) && sid) {
          void useTerminalStore.getState().closeSession(sid);
        }
      }

      prevTermTabsRef.current = currentTermTabs;
    });

    return unsub;
  }, []);
}
