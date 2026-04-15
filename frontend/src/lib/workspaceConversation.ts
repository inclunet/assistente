import { CreateConversation } from '@wailsjs/go/main/App';
import i18next from 'i18next';
import type { WorkspaceTab } from '../store/workspaceStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';

/** Evita duas criações em paralelo para a mesma aba (ex.: bridge + mini-chat). */
const inflight = new Map<string, Promise<number>>();

/**
 * Garante que a aba do workspace tem `conversationId` persistido, sem assumir que deve
 * também trocar a conversa ativa global do `chatStore`.
 */
export async function ensureWorkspaceTabConversationId(wsTab: WorkspaceTab): Promise<number> {
  const pending = inflight.get(wsTab.id);
  if (pending) return pending;

  const run = async (): Promise<number> => {
    const fresh = useWorkspaceStore.getState().workspace?.tabs.find((t) => t.id === wsTab.id);
    if (!fresh) {
      throw new Error(`[workspaceConversation] Aba não encontrada: ${wsTab.id}`);
    }

    let cid = fresh.conversationId ?? 0;
    if (cid > 0) {
      return cid;
    }

    const title = i18next.t('chat.newConversation');
    const conv = await CreateConversation(title, '');
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

/**
 * Garante que a aba do workspace tem conversa no backend e sincroniza o `chatStore`
 * com essa conversa quando necessário.
 */
export async function ensureWorkspaceTabHasConversation(wsTab: WorkspaceTab): Promise<number> {
  const cid = await ensureWorkspaceTabConversationId(wsTab);
  const chat = useChatStore.getState();
  if (chat.activeConversationId !== cid) {
    await useChatStore.getState().loadConversation(cid);
  }
  return cid;
}
