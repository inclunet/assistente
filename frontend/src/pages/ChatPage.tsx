import { useCallback, useEffect } from 'react';
import { useChatStore } from '../store/chatStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatSessionView } from '../components/chat/ChatSessionView';
import './ChatPage.css';

export default function ChatPage() {
  const sendMessage = useChatStore((s) => s.sendMessage);
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());

  useEffect(() => {
    if (!activeTab || activeTab.type !== 'chat') return;
    void ensureWorkspaceTabHasConversation(activeTab).catch((e) => {
      console.error('[ChatPage] falha ao garantir conversa:', e);
    });
  }, [activeTab?.id, activeTab?.type, activeTab?.conversationId]);

  const onSend = useCallback(
    async (content: string, mediaFiles?: Parameters<typeof sendMessage>[1]) => {
      await sendMessage(content, mediaFiles);
    },
    [sendMessage],
  );

  return <ChatSessionView variant="page" onSend={onSend} />;
}
