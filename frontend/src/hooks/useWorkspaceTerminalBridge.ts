import { useEffect, useRef } from 'react';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useTerminalStore } from '../store/terminalStore';

/**
 * Sincroniza abas de terminal do workspace com o terminalStore.
 *
 * - contentId vazio → cria sessão via terminalStore.createSession()
 * - contentId existente → ativa sessão via terminalStore.setActiveSession()
 * - Remoção de aba → fecha sessão via terminalStore.closeSession()
 */
export function useWorkspaceTerminalBridge() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab || activeTab.type !== 'terminal') return;

    const syncKey = `${activeTab.id}:${activeTab.contentId}`;
    if (lastSyncedRef.current === syncKey) return;

    const sessionId = activeTab.contentId || '';

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
  }, [activeTab?.id, activeTab?.type, activeTab?.contentId, isWsInitialized]);

  async function recoverStaleSession(wsTabId: string) {
    creatingRef.current = true;
    try {
      await useTerminalStore.getState().createSession();
      const newSessionId = useTerminalStore.getState().activeSessionId;
      if (newSessionId) {
        await updateWsTab(wsTabId, { content_id: newSessionId });
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
        await updateWsTab(wsTabId, { content_id: newSessionId });
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
        if (t.type === 'terminal' && t.contentId) {
          currentTermTabs.set(t.id, t.contentId);
        }
      }

      for (const [wsTabId, sessionId] of prevTermTabsRef.current) {
        if (!currentTermTabs.has(wsTabId) && sessionId) {
          void useTerminalStore.getState().closeSession(sessionId);
        }
      }

      prevTermTabsRef.current = currentTermTabs;
    });

    return unsub;
  }, []);
}
