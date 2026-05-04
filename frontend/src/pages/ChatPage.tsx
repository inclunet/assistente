import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useChatStore } from '../store/chatStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatPanel, type ChatPanelSendContext } from '../components/chat/ChatPanel';
import { useWorkspacePanel } from '../components/workspace/WorkspacePanelContext';
import { createChatSurfaceIdentity, normalizeChatSurfaceOrigin } from '../services/chatSessionRegistry';

export default function ChatPage() {
  const { t } = useTranslation();
  const sendMessageToConversation = useChatStore((s) => s.sendMessageToConversation);
  const { tab } = useWorkspacePanel();
  const conversationId = tab?.type === 'chat' ? tab.conversationId : undefined;
  const surface = useMemo(() => createChatSurfaceIdentity({
    conversationId: conversationId ?? null,
    surfaceType: 'page',
    tabId: tab.id,
  }), [conversationId, tab.id]);

  // NOTE: loadConversation já é feita pelo useWorkspaceChatBridge (WorkspaceLayout).
  // Não duplicar aqui — evita 2x GetConversationInfo + GetMessages a cada troca de aba.

  const onSend = useCallback(
    async (content: string, mediaFiles: Parameters<typeof sendMessageToConversation>[2], context: ChatPanelSendContext) => {
      if (!tab || tab.type !== 'chat') {
        throw new Error(t('chat.errors.tabCannotSend'));
      }
      const conversationId = await ensureWorkspaceTabHasConversation(tab, { activate: true });
      if (!conversationId) {
        throw new Error(t('chat.errors.chatTabNotReady'));
      }
      const sendOrigin = normalizeChatSurfaceOrigin(context.origin, conversationId);
      await sendMessageToConversation(conversationId, content, mediaFiles, undefined, { origin: sendOrigin });
    },
    [tab, sendMessageToConversation, t],
  );

  return <ChatPanel surface={surface} onSend={onSend} />;
}
