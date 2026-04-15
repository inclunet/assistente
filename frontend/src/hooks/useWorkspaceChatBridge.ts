import { useEffect, useRef } from 'react';
import { useWorkspaceStore, registerTabRenameHandler } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';
import { RenameConversation } from '@wailsjs/go/main/App';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import type { TabType } from '../store/workspaceStore';

/** Abas onde a conversa só é criada ao abrir o mini-chat (não ao focar o painel). */
const MINI_CHAT_LAZY_CONVERSATION: ReadonlySet<TabType> = new Set(['editor', 'terminal', 'tasklist']);

/**
 * Sincroniza a aba ativa do workspace com o chatStore.
 * Cada aba pode ter um conversationId; abas editor/terminal/tasklist só criam conversa ao abrir o mini-chat.
 *
 * Fluxo:
 * 1. Workspace ativa qualquer aba
 * 2. Se conversationId > 0 → chatStore.loadConversation(id)
 * 3. Se conversationId vazio e aba é chat → ensureWorkspaceTabHasConversation
 * 4. Se conversationId vazio e aba é editor/terminal/tasklist → clearActiveConversation (conversa criada no requestOpen do mini-chat)
 * 5. Profile cascade: tab.profileOverride.slug → workspace.profile → null (global)
 */
export function useWorkspaceChatBridge() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab) return;

    const conversationId = activeTab.conversationId || 0;
    const syncKey = `${activeTab.id}:${conversationId}`;
    if (lastSyncedRef.current === syncKey) return;

    if (conversationId > 0) {
      void syncExistingConversation(conversationId).then(() => {
        lastSyncedRef.current = syncKey;
      });
      return;
    }

    if (MINI_CHAT_LAZY_CONVERSATION.has(activeTab.type)) {
      useChatStore.getState().clearActiveConversation();
      lastSyncedRef.current = syncKey;
      return;
    }

    void ensureWorkspaceTabHasConversation(activeTab)
      .then((id) => {
        lastSyncedRef.current = `${activeTab.id}:${id}`;
      })
      .catch((error) => {
        console.error('[WorkspaceChatBridge] Erro ao garantir conversa:', error);
      });
  }, [activeTab?.id, activeTab?.type, activeTab?.conversationId, isWsInitialized]);

  async function syncExistingConversation(conversationId: number) {
    const chatState = useChatStore.getState();
    if (chatState.activeConversationId === conversationId) return;
    await useChatStore.getState().loadConversation(conversationId);
  }

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
      const convId = parseInt(id, 10);
      if (convId) void RenameConversation(convId, newTitle);
    });
  }, []);
}
