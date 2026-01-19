import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface Message {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: number;
  isStreaming?: boolean;
}

export interface ChatTab {
  id: string;
  title: string;
  messages: Message[];
  createdAt: number;
  updatedAt: number;
}

interface ChatStore {
  tabs: ChatTab[];
  activeTabId: string | null;
  isLoading: boolean;
  streamingMessageId: string | null;

  // Tab management
  createTab: () => string;
  deleteTab: (tabId: string) => void;
  setActiveTab: (tabId: string) => void;
  updateTabTitle: (tabId: string, title: string) => void;

  // Message management
  addMessage: (tabId: string, message: Omit<Message, 'id' | 'timestamp'>) => string;
  updateMessage: (tabId: string, messageId: string, content: string) => void;
  clearMessages: (tabId: string) => void;
  clearActiveTab: () => void;

  // Chat actions
  sendMessage: (content: string) => Promise<void>;
  stopStreaming: () => void;

  // Utility
  getActiveTab: () => ChatTab | undefined;
  getTabMessages: (tabId: string) => Message[];
}

const generateId = () => `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

const createNewTab = (): ChatTab => ({
  id: generateId(),
  title: 'Nova Conversa',
  messages: [],
  createdAt: Date.now(),
  updatedAt: Date.now(),
});

export const useChatStore = create<ChatStore>()(
  persist(
    (set, get) => ({
      tabs: [],
      activeTabId: null,
      isLoading: false,
      streamingMessageId: null,

      createTab: () => {
        const newTab = createNewTab();
        set((state) => ({
          tabs: [...state.tabs, newTab],
          activeTabId: newTab.id,
        }));
        return newTab.id;
      },

      deleteTab: (tabId) => {
        set((state) => {
          const newTabs = state.tabs.filter((t) => t.id !== tabId);
          const newActiveTabId =
            state.activeTabId === tabId
              ? newTabs[0]?.id || null
              : state.activeTabId;

          return {
            tabs: newTabs,
            activeTabId: newActiveTabId,
          };
        });
      },

      setActiveTab: (tabId) => {
        set({ activeTabId: tabId });
      },

      updateTabTitle: (tabId, title) => {
        set((state) => ({
          tabs: state.tabs.map((tab) =>
            tab.id === tabId
              ? { ...tab, title, updatedAt: Date.now() }
              : tab
          ),
        }));
      },

      addMessage: (tabId, message) => {
        const messageId = generateId();
        const newMessage: Message = {
          ...message,
          id: messageId,
          timestamp: Date.now(),
        };

        set((state) => ({
          tabs: state.tabs.map((tab) =>
            tab.id === tabId
              ? {
                  ...tab,
                  messages: [...tab.messages, newMessage],
                  updatedAt: Date.now(),
                }
              : tab
          ),
        }));

        return messageId;
      },

      updateMessage: (tabId, messageId, content) => {
        set((state) => ({
          tabs: state.tabs.map((tab) =>
            tab.id === tabId
              ? {
                  ...tab,
                  messages: tab.messages.map((msg) =>
                    msg.id === messageId
                      ? { ...msg, content, isStreaming: false }
                      : msg
                  ),
                  updatedAt: Date.now(),
                }
              : tab
          ),
        }));
      },

      clearMessages: (tabId) => {
        set((state) => ({
          tabs: state.tabs.map((tab) =>
            tab.id === tabId
              ? { ...tab, messages: [], updatedAt: Date.now() }
              : tab
          ),
        }));
      },

      clearActiveTab: () => {
        const { activeTabId, clearMessages } = get();
        if (activeTabId) {
          clearMessages(activeTabId);
        }
      },

      sendMessage: async (content) => {
        const { activeTabId, addMessage, createTab } = get();
        
        // Ensure we have an active tab
        let currentTabId = activeTabId;
        if (!currentTabId) {
          currentTabId = createTab();
        }

        // Add user message
        addMessage(currentTabId, {
          role: 'user',
          content,
        });

        // Add empty assistant message for streaming
        const assistantMessageId = addMessage(currentTabId, {
          role: 'assistant',
          content: '',
          isStreaming: true,
        });

        set({ isLoading: true, streamingMessageId: assistantMessageId });

        // TODO: Integrate with backend API
        // For now, simulate a response
        try {
          // Simulate streaming delay
          await new Promise((resolve) => setTimeout(resolve, 500));
          
          const simulatedResponse = `Esta é uma resposta simulada para: "${content}"\n\nA integração com o backend será implementada em breve.`;
          
          get().updateMessage(currentTabId, assistantMessageId, simulatedResponse);
          
          // Auto-generate title from first message
          const tab = get().tabs.find((t) => t.id === currentTabId);
          if (tab && tab.messages.length === 2 && tab.title === 'Nova Conversa') {
            const title = content.slice(0, 50) + (content.length > 50 ? '...' : '');
            get().updateTabTitle(currentTabId, title);
          }
        } catch (error) {
          console.error('Error sending message:', error);
          get().updateMessage(
            currentTabId,
            assistantMessageId,
            'Erro ao enviar mensagem. Por favor, tente novamente.'
          );
        } finally {
          set({ isLoading: false, streamingMessageId: null });
        }
      },

      stopStreaming: () => {
        set({ isLoading: false, streamingMessageId: null });
        // TODO: Cancel backend request
      },

      getActiveTab: () => {
        const { tabs, activeTabId } = get();
        return tabs.find((tab) => tab.id === activeTabId);
      },

      getTabMessages: (tabId) => {
        const tab = get().tabs.find((t) => t.id === tabId);
        return tab?.messages || [];
      },
    }),
    {
      name: 'chat-storage',
      partialize: (state) => ({
        tabs: state.tabs,
        activeTabId: state.activeTabId,
      }),
    }
  )
);
