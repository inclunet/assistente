import { GetConversationInfo } from '@wailsjs/go/wailsapi/Conversations';
import i18next from 'i18next';

/** Metadados operacionais; janelas e mensagens pertencem à timeline canônica. */
export async function loadConversationSnapshot(conversationId: string) {
  const info = await GetConversationInfo(conversationId);
  return { title: info?.title || i18next.t('chat.conversation'),
    channel: info?.channel || undefined, contactId: info?.contact_id || undefined };
}
