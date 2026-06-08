import { logger } from '../utils/logger';
import { useEffect, useRef } from 'react';
import { useWorkspaceStore, registerTabRenameHandler } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';
import { RenameConversation } from '@wailsjs/go/app/App';
import { ensureWorkspaceTabConversationId } from '../lib/workspaceConversation';
import type { TabType } from '../store/workspaceStore';

/** Abas onde a conversa só é criada ao abrir o chat modal (não ao focar o painel). */
const MINI_CHAT_LAZY_CONVERSATION: ReadonlySet<TabType> = new Set(['editor', 'terminal', 'tasklist']);

/**
 * Sincroniza a aba ativa do workspace com o chatStore.
 * Cada aba pode ter um conversationId; abas editor/terminal/tasklist só criam conversa ao abrir o chat modal.
 *
 * Fluxo:
 * 1. Workspace ativa qualquer aba
 * 2. Se aba de chat tem conversationId válido → carrega o timeline dessa conversa
 * 3. Se conversationId vazio e aba é chat → garante `conversationId` e só sincroniza o chatStore
 *    se a aba continuar ativa ao concluir a criação
 * 4. Se aba é editor/terminal/tasklist → carrega a sessão em background apenas quando já houver conversa
 */
export function useWorkspaceChatBridge() {
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);
  const activeTab = useWorkspaceStore((s) => {
    const activeTabId = s.workspace?.activeTabId;
    return s.workspace?.tabs.find((tab) => tab.id === activeTabId);
  });

  const lastSyncedRef = useRef<string | null>(null);
  /** Invalida `loadConversation` / `ensure` assíncronos quando a aba ativa muda antes de concluírem. */
  const syncGenerationRef = useRef(0);

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab) return;

    const conversationId = activeTab.conversationId || '';
    const syncKey = `${activeTab.id}:${conversationId}`;
    if (lastSyncedRef.current === syncKey) return;

    const snapshotTabId = activeTab.id;

    if (conversationId && MINI_CHAT_LAZY_CONVERSATION.has(activeTab.type)) {
      void useChatStore.getState().loadConversationSession(conversationId);
      lastSyncedRef.current = syncKey;
      return;
    }

    if (conversationId) {
      const gen = ++syncGenerationRef.current;
      void (async () => {
        try {
          await useChatStore.getState().loadConversationSession(conversationId);
        } catch (error) {
          logger.error('[WorkspaceChatBridge] Erro ao carregar conversa:', error);
          return;
        }
        if (syncGenerationRef.current !== gen) return;
        const workspace = useWorkspaceStore.getState().workspace;
        const nowTab = workspace?.tabs.find((tab) => tab.id === workspace.activeTabId);
        if (!nowTab || nowTab.id !== snapshotTabId) return;
        if ((nowTab.conversationId || '') !== conversationId) return;
        lastSyncedRef.current = syncKey;
      })();
      return;
    }

    if (MINI_CHAT_LAZY_CONVERSATION.has(activeTab.type)) {
      lastSyncedRef.current = syncKey;
      return;
    }

    const gen = ++syncGenerationRef.current;
    void (async () => {
      try {
        const id = await ensureWorkspaceTabConversationId(activeTab);
        if (syncGenerationRef.current !== gen) return;
        const workspace = useWorkspaceStore.getState().workspace;
        const nowTab = workspace?.tabs.find((tab) => tab.id === workspace.activeTabId);
        if (!nowTab || nowTab.id !== snapshotTabId) return;
        if ((nowTab.conversationId || '') !== id) return;

        await useChatStore.getState().loadConversationSession(id);

        if (syncGenerationRef.current !== gen) return;
        const latestWorkspace = useWorkspaceStore.getState().workspace;
        const latestTab = latestWorkspace?.tabs.find((tab) => tab.id === latestWorkspace.activeTabId);
        if (!latestTab || latestTab.id !== snapshotTabId) return;
        if ((latestTab.conversationId || '') !== id) return;
        lastSyncedRef.current = `${snapshotTabId}:${id}`;
      } catch (error) {
        logger.error('[WorkspaceChatBridge] Erro ao garantir conversa:', error);
      }
    })();
  }, [activeTab?.id, activeTab?.type, activeTab?.conversationId, isWsInitialized]);

  // F2 tab rename → rename conversation in backend
  useEffect(() => {
    return registerTabRenameHandler('chat', (id, newTitle) => {
      if (id) void RenameConversation(id, newTitle);
    });
  }, []);
}
