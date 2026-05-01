import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useChatStore } from '../store/chatStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatSessionView } from '../components/chat/ChatSessionView';
import { useWorkspacePanel } from '../components/workspace/WorkspacePanelContext';
import {
  normalizeChatSurfaceOrigin,
  type ChatSurfaceOrigin,
} from '../services/chatSessionRegistry';

export default function ChatPage() {
  const { t } = useTranslation();
  const sendMessageToConversation = useChatStore((s) => s.sendMessageToConversation);
  const { tab } = useWorkspacePanel();
  const conversationId = tab?.type === 'chat' ? tab.conversationId : undefined;

  // NOTE: loadConversation já é feita pelo useWorkspaceChatBridge (WorkspaceLayout).
  // Não duplicar aqui — evita 2x GetConversationInfo + GetMessages a cada troca de aba.

  const onSend = useCallback(
    async (content: string, mediaFiles?: Parameters<typeof sendMessageToConversation>[2], origin?: ChatSurfaceOrigin) => {
      if (!tab || tab.type !== 'chat') {
        throw new Error(t('chat.errors.tabCannotSend'));
      }
      const conversationId = await ensureWorkspaceTabHasConversation(tab);
      if (!conversationId) {
        throw new Error(t('chat.errors.chatTabNotReady'));
      }
      const sendOrigin = normalizeChatSurfaceOrigin(origin, conversationId);
      await sendMessageToConversation(conversationId, content, mediaFiles, undefined, { origin: sendOrigin });
    },
    [tab, sendMessageToConversation, t],
  );

  return <ChatSessionView variant="page" conversationId={conversationId} onSend={onSend} />;
}
