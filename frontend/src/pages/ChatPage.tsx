import { useCallback, useEffect } from 'react';
import { useChatStore } from '../store/chatStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatSessionView } from '../components/chat/ChatSessionView';

export default function ChatPage() {
  const sendMessageToConversation = useChatStore((s) => s.sendMessageToConversation);
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());

  useEffect(() => {
    if (!activeTab || activeTab.type !== 'chat') return;
    void ensureWorkspaceTabHasConversation(activeTab).catch((e) => {
      console.error('[ChatPage] falha ao garantir conversa:', e);
    });
  }, [activeTab?.id, activeTab?.type, activeTab?.conversationId]);

  const onSend = useCallback(
    async (content: string, mediaFiles?: Parameters<typeof sendMessageToConversation>[2]) => {
      const conversationId = activeTab?.type === 'chat' ? activeTab.conversationId : 0;
      if (!conversationId) {
        throw new Error('Conversa da aba de chat ainda não está pronta.');
      }
      await sendMessageToConversation(conversationId, content, mediaFiles);
    },
    [activeTab?.conversationId, activeTab?.type, sendMessageToConversation],
  );

  return <ChatSessionView variant="page" onSend={onSend} />;
}
