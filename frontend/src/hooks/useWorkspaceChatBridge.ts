import { useEffect, useRef } from 'react';
import { useWorkspaceStore, useActiveTab, registerTabRenameHandler } from '../store/workspaceStore';
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
 * 2. Se aba de chat tem conversationId válido → ativa a sessão dessa conversa
 * 3. Se conversationId vazio e aba é chat → garante `conversationId` e só sincroniza o chatStore
 *    se a aba continuar ativa ao concluir a criação
 * 4. Se aba é editor/terminal/tasklist → carrega a sessão em background apenas quando já houver conversa
 * 5. Profile cascade: tab.profileOverride.slug → workspace.profile → null (global)
 */
export function useWorkspaceChatBridge() {
  const activeTab = useActiveTab();
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

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
      void useChatStore.getState().loadConversationSession(conversationId, { activate: false });
      useChatStore.getState().setActiveConversationId(null);
      lastSyncedRef.current = syncKey;
      return;
    }

    if (conversationId) {
      const gen = ++syncGenerationRef.current;
      void (async () => {
        try {
          const chatState = useChatStore.getState();
          if (chatState.activeConversationId !== conversationId) {
            await chatState.loadConversationSession(conversationId, { activate: true });
          }
        } catch (error) {
          console.error('[WorkspaceChatBridge] Erro ao carregar conversa:', error);
          return;
        }
        if (syncGenerationRef.current !== gen) return;
        const nowTab = useWorkspaceStore.getState().getActiveTab();
        if (!nowTab || nowTab.id !== snapshotTabId) return;
        if ((nowTab.conversationId || '') !== conversationId) return;
        lastSyncedRef.current = syncKey;
      })();
      return;
    }

    if (MINI_CHAT_LAZY_CONVERSATION.has(activeTab.type)) {
      useChatStore.getState().setActiveConversationId(null);
      lastSyncedRef.current = syncKey;
      return;
    }

    const gen = ++syncGenerationRef.current;
    void (async () => {
      try {
        const id = await ensureWorkspaceTabConversationId(activeTab);
        if (syncGenerationRef.current !== gen) return;
        const nowTab = useWorkspaceStore.getState().getActiveTab();
        if (!nowTab || nowTab.id !== snapshotTabId) return;
        if ((nowTab.conversationId || '') !== id) return;

        const chatState = useChatStore.getState();
        if (chatState.activeConversationId !== id) {
          await chatState.loadConversationSession(id, { activate: true });
        }

        if (syncGenerationRef.current !== gen) return;
        const latestTab = useWorkspaceStore.getState().getActiveTab();
        if (!latestTab || latestTab.id !== snapshotTabId) return;
        if ((latestTab.conversationId || '') !== id) return;
        lastSyncedRef.current = `${snapshotTabId}:${id}`;
      } catch (error) {
        console.error('[WorkspaceChatBridge] Erro ao garantir conversa:', error);
      }
    })();
  }, [activeTab?.id, activeTab?.type, activeTab?.conversationId, isWsInitialized]);

  // Profile cascade: tab.profileOverride.slug → workspace.profile → null (global)
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const tabProfileSlug = activeTab?.profileOverride?.slug as string | undefined;

  useEffect(() => {
    const effectiveSlug = tabProfileSlug || wsProfile || null;
    useChatStore.getState().setContextProfileSlug(effectiveSlug);
  }, [tabProfileSlug, wsProfile]);

  useEffect(() => {
    return () => {
      useChatStore.getState().setContextProfileSlug(null);
    };
  }, []);

  // F2 tab rename → rename conversation in backend
  useEffect(() => {
    return registerTabRenameHandler('chat', (id, newTitle) => {
      if (id) void RenameConversation(id, newTitle);
    });
  }, []);
}
