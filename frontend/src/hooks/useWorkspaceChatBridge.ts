import { useEffect, useRef } from 'react';
import { useWorkspaceStore, WorkspaceTab, registerTabRenameHandler } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';
import { CreateConversation, RenameConversation } from '@wailsjs/go/main/App';

/**
 * Sincroniza a aba ativa do workspace com o chatStore.
 * Funciona para TODAS as abas — cada aba tem seu próprio conversationId dedicado.
 *
 * Fluxo:
 * 1. Workspace ativa qualquer aba
 * 2. Se conversationId vazio → cria conversa NOVA (sempre fresh, sem reciclar)
 *    e salva o conversationId na aba do workspace
 * 3. Se conversationId existente → chatStore.loadConversation(id)
 * 4. Profile cascade: tab.profileOverride.slug → workspace.profile → null (global)
 */
export function useWorkspaceChatBridge() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab) return;

    const syncKey = `${activeTab.id}:${activeTab.conversationId || 0}`;
    if (lastSyncedRef.current === syncKey) return;

    const conversationId = activeTab.conversationId || 0;

    if (conversationId > 0) {
      syncExistingConversation(conversationId);
      lastSyncedRef.current = syncKey;
    } else if (!creatingRef.current) {
      createConversationForTab(activeTab);
    }
  }, [activeTab?.id, activeTab?.type, activeTab?.conversationId, isWsInitialized]);

  async function syncExistingConversation(conversationId: number) {
    const chatState = useChatStore.getState();
    if (chatState.activeConversationId === conversationId) return;
    await useChatStore.getState().loadConversation(conversationId);
  }

  async function createConversationForTab(wsTab: WorkspaceTab) {
    creatingRef.current = true;
    try {
      // Always create a fresh conversation — never recycle — so each tab gets its own unique conversation.
      const conv = await CreateConversation('Nova Conversa', '');
      const conversationId = conv.id;
      await updateWsTab(wsTab.id, { conversation_id: conversationId });
      await useChatStore.getState().loadConversation(conversationId);
      lastSyncedRef.current = `${wsTab.id}:${conversationId}`;
    } catch (error) {
      console.error('[WorkspaceChatBridge] Erro ao criar conversa:', error);
    } finally {
      creatingRef.current = false;
    }
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
