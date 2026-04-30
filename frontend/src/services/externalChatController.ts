import {
  startChatEventController,
  type ChatEventControllerAdapter,
} from './chatEventController';

export interface ExternalChatIncomingData {
  channel: string;
  from: string;
  fromId?: string;
  text: string;
  conversationId: string;
  newConversation?: boolean;
}

export interface ExternalChatControllerAdapter {
  hasConversationSession: (conversationId: string) => boolean;
  loadConversationSession: (conversationId: string) => Promise<void>;
  chatEventAdapter: ChatEventControllerAdapter;
}

export function handleExternalChatIncoming(
  data: ExternalChatIncomingData,
  adapter: ExternalChatControllerAdapter,
) {
  const { channel, from, text, conversationId } = data;
  if (!conversationId) return;

  if (!adapter.hasConversationSession(conversationId)) {
    void adapter.loadConversationSession(conversationId);
  }

  startChatEventController({
    conversationId,
    external: { channel, from, text },
    adapter: adapter.chatEventAdapter,
  });
}
