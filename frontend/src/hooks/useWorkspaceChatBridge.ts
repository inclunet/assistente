import { useEffect, useRef } from 'react';
import { useWorkspaceStore, WorkspaceTab } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';

/**
 * Sincroniza a aba de chat ativa do workspace com o chatStore.
 *
 * Fluxo:
 * 1. Workspace ativa uma aba do tipo chat
 * 2. Se contentId vazio → cria conversa via chatStore.createConversation()
 *    e salva o conversationId como contentId da aba do workspace
 * 3. Se contentId existente → chatStore.loadConversation(id)
 * 4. Título da conversa no chatStore é sincronizado de volta ao workspace
 */
export function useWorkspaceChatBridge() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab || activeTab.type !== 'chat') return;

    const syncKey = `${activeTab.id}:${activeTab.contentId}`;
    if (lastSyncedRef.current === syncKey) return;

    const conversationId = activeTab.contentId ? parseInt(activeTab.contentId, 10) : 0;

    if (conversationId > 0) {
      syncExistingConversation(conversationId);
      lastSyncedRef.current = syncKey;
    } else if (!creatingRef.current) {
      createConversationForTab(activeTab);
    }
  }, [activeTab?.id, activeTab?.type, activeTab?.contentId, isWsInitialized]);

  async function syncExistingConversation(conversationId: number) {
    const chatState = useChatStore.getState();
    if (chatState.activeConversationId === conversationId) return;
    await useChatStore.getState().loadConversation(conversationId);
  }

  async function createConversationForTab(wsTab: WorkspaceTab) {
    creatingRef.current = true;
    try {
      const conversationId = await useChatStore.getState().createConversation();
      await updateWsTab(wsTab.id, { content_id: String(conversationId) });
      lastSyncedRef.current = `${wsTab.id}:${conversationId}`;
    } catch (error) {
      console.error('[WorkspaceChatBridge] Erro ao criar conversa:', error);
    } finally {
      creatingRef.current = false;
    }
  }

  // Profile cascade: tab.profileOverride.slug → workspace.profile → null (global)
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const tabProfileSlug = activeTab?.type === 'chat'
    ? (activeTab.profileOverride?.slug as string | undefined)
    : undefined;

  useEffect(() => {
    const effectiveSlug = tabProfileSlug || wsProfile || null;
    useChatStore.getState().setContextProfileSlug(effectiveSlug);
  }, [tabProfileSlug, wsProfile]);

  useEffect(() => {
    return () => {
      useChatStore.getState().setContextProfileSlug(null);
    };
  }, []);

  // Sync titulo do chatStore → workspace quando o chatStore renomeia via AI
  useEffect(() => {
    if (!activeTab || activeTab.type !== 'chat' || !activeTab.contentId) return;
    const conversationId = parseInt(activeTab.contentId, 10);
    if (!conversationId) return;

    const unsub = useChatStore.subscribe((state, prevState) => {
      const conv = state.activeConversation;
      const prevConv = prevState.activeConversation;
      if (
        conv && prevConv &&
        conv.id === conversationId &&
        conv.title !== prevConv.title &&
        conv.title !== activeTab.title
      ) {
        updateWsTab(activeTab.id, { title: conv.title });
      }
    });

    return unsub;
  }, [activeTab?.id, activeTab?.contentId, activeTab?.type, updateWsTab]);
}
