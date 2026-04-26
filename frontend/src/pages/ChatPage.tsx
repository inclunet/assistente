import { useCallback } from 'react';
import { useChatStore } from '../store/chatStore';
import { useActiveTab } from '../store/workspaceStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatSessionView } from '../components/chat/ChatSessionView';

export default function ChatPage() {
  const sendMessageToConversation = useChatStore((s) => s.sendMessageToConversation);
  const activeTab = useActiveTab();

  // NOTE: loadConversation já é feita pelo useWorkspaceChatBridge (WorkspaceLayout).
  // Não duplicar aqui — evita 2x GetConversationInfo + GetMessages a cada troca de aba.

  const onSend = useCallback(
    async (content: string, mediaFiles?: Parameters<typeof sendMessageToConversation>[2]) => {
      if (!activeTab || activeTab.type !== 'chat') {
        throw new Error('A aba ativa não suporta envio de mensagens.');
      }
      const conversationId = await ensureWorkspaceTabHasConversation(activeTab);
      if (!conversationId) {
        throw new Error('Conversa da aba de chat ainda não está pronta.');
      }
      await sendMessageToConversation(conversationId, content, mediaFiles);
    },
    [activeTab, sendMessageToConversation],
  );

  return <ChatSessionView variant="page" onSend={onSend} />;
}
