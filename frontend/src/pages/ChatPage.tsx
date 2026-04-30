import { useCallback } from 'react';
import { useChatStore } from '../store/chatStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatSessionView } from '../components/chat/ChatSessionView';
import { useWorkspacePanel } from '../components/workspace/WorkspacePanelContext';

export default function ChatPage() {
  const sendMessageToConversation = useChatStore((s) => s.sendMessageToConversation);
  const { tab } = useWorkspacePanel();
  const conversationId = tab?.type === 'chat' ? tab.conversationId : undefined;

  // NOTE: loadConversation já é feita pelo useWorkspaceChatBridge (WorkspaceLayout).
  // Não duplicar aqui — evita 2x GetConversationInfo + GetMessages a cada troca de aba.

  const onSend = useCallback(
    async (content: string, mediaFiles?: Parameters<typeof sendMessageToConversation>[2]) => {
      if (!tab || tab.type !== 'chat') {
        throw new Error('A aba ativa não suporta envio de mensagens.');
      }
      const conversationId = await ensureWorkspaceTabHasConversation(tab);
      if (!conversationId) {
        throw new Error('Conversa da aba de chat ainda não está pronta.');
      }
      await sendMessageToConversation(conversationId, content, mediaFiles);
    },
    [tab, sendMessageToConversation],
  );

  return <ChatSessionView variant="page" conversationId={conversationId} onSend={onSend} />;
}
