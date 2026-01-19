import { useEffect, useRef } from 'react';
import { useTabsStore } from '../store/tabsStore';
import { useChatStore } from '../store/chatStore';
import { MessageList } from '../components/chat/MessageList';
import { ChatInput } from '../components/chat/ChatInput';
import { ChatToolbar } from '../components/chat/ChatToolbar';
import { ChatTabs } from '../components/tabs/ChatTabs';
import { useChatKeyboardNav } from '../hooks/useChatKeyboardNav';
import { useTabsKeyboardShortcuts } from '../hooks/useTabsKeyboardShortcuts';
import './ChatPage.css';

export default function ChatPage() {
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  
  const { tabs, activeTabId, loadTabs } = useTabsStore();
  const {
    isLoading,
    sendMessage,
    getActiveTab,
  } = useChatStore();

  // Enable keyboard navigation for chat messages
  useChatKeyboardNav({
    enabled: true,
    inputRef,
    messagesContainerRef,
  });

  // Enable global keyboard shortcuts for tabs (Ctrl+T, Ctrl+W, Ctrl+Tab, etc.)
  useTabsKeyboardShortcuts();

  // Load tabs on mount
  useEffect(() => {
    loadTabs();
  }, [loadTabs]);

  const activeTab = getActiveTab();
  const messages = activeTab?.messages || [];

  const handleSendMessage = async (content: string) => {
    await sendMessage(content);
  };

  return (
    <div className="chat-page">
      <ChatTabs />
      <ChatToolbar />
      <MessageList 
        messages={messages} 
        isLoading={isLoading}
        ref={messagesContainerRef}
      />
      <ChatInput 
        onSend={handleSendMessage} 
        disabled={isLoading}
        ref={inputRef}
      />
    </div>
  );
}
