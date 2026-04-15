import { CreateConversation } from '@wailsjs/go/main/App';
import type { WorkspaceTab } from '../store/workspaceStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';

/** Evita duas criações em paralelo para a mesma aba (ex.: bridge + mini-chat). */
const inflight = new Map<string, Promise<number>>();

/**
 * Garante que a aba do workspace tem conversa no backend e que o chatStore está sincronizado.
 * Usa sempre CreateConversation (conversa nova), alinhado com useWorkspaceChatBridge.
 */
export async function ensureWorkspaceTabHasConversation(wsTab: WorkspaceTab): Promise<number> {
  const pending = inflight.get(wsTab.id);
  if (pending) return pending;

  const run = async (): Promise<number> => {
    const fresh = useWorkspaceStore.getState().workspace?.tabs.find((t) => t.id === wsTab.id);
    if (!fresh) {
      throw new Error(`[workspaceConversation] Aba não encontrada: ${wsTab.id}`);
    }

    let cid = fresh.conversationId ?? 0;
    if (cid > 0) {
      const chat = useChatStore.getState();
      if (chat.activeConversationId !== cid) {
        await useChatStore.getState().loadConversation(cid);
      }
      return cid;
    }

    const conv = await CreateConversation('Nova Conversa', '');
    cid = conv.id;
    await useWorkspaceStore.getState().updateTab(wsTab.id, { conversation_id: cid });
    await useChatStore.getState().loadConversation(cid);
    return cid;
  };

  const promise = run().finally(() => {
    inflight.delete(wsTab.id);
  });
  inflight.set(wsTab.id, promise);
  return promise;
}
