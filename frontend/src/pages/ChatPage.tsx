import { useCallback } from 'react';
import { useChatStore } from '../store/chatStore';
import { ChatSessionView } from '../components/chat/ChatSessionView';
import './ChatPage.css';

export default function ChatPage() {
  const sendMessage = useChatStore((s) => s.sendMessage);

  const onSend = useCallback(
    async (content: string, mediaFiles?: Parameters<typeof sendMessage>[1]) => {
      await sendMessage(content, mediaFiles);
    },
    [sendMessage],
  );

  return <ChatSessionView variant="page" onSend={onSend} />;
}
