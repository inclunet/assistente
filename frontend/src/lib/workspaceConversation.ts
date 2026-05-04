import { CreateConversation } from '@wailsjs/go/app/App';
import i18next from 'i18next';
import type { WorkspaceTab } from '../store/workspaceStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';
import { isBackendId } from './idUtils';

/** Evita duas criações em paralelo para a mesma aba (ex.: bridge + chat modal). */
const inflight = new Map<string, Promise<string>>();

/**
 * Garante que a aba do workspace tem `conversationId` persistido, sem assumir que deve
 * também trocar a conversa ativa global do `chatStore`.
 */
export async function ensureWorkspaceTabConversationId(wsTab: WorkspaceTab): Promise<string> {
  const pending = inflight.get(wsTab.id);
  if (pending) return pending;

  const run = async (): Promise<string> => {
    const fresh = useWorkspaceStore.getState().workspace?.tabs.find((t) => t.id === wsTab.id);
    if (!fresh) {
      throw new Error(`[workspaceConversation] Aba não encontrada: ${wsTab.id}`);
    }

    let cid = fresh.conversationId ?? '';
    if (cid && isBackendId(cid)) {
      return cid;
    }
    // Legacy numeric ID or invalid format — treat as missing
    cid = '';

    const title = i18next.t('chat.newConversation');
    const conv = await CreateConversation(title, '');
    cid = conv.id;
    await useWorkspaceStore.getState().updateTab(wsTab.id, { conversation_id: cid });
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
 * apenas quando o chamador pede explicitamente.
 */
export async function ensureWorkspaceTabHasConversation(
  wsTab: WorkspaceTab,
  options: { activate?: boolean } = { activate: true },
): Promise<string> {
  const cid = await ensureWorkspaceTabConversationId(wsTab);
  await useChatStore.getState().loadConversationSession(cid, { activate: options.activate ?? true });
  return cid;
}
