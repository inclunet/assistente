import { useCallback } from 'react';
import i18next from 'i18next';
import { useChatStore } from '../../store/chatStore';
import type { MediaFile } from '../../services/mediaService';
import {
  getConversationTimeline,
  normalizeChatSurfaceOrigin,
  type ChatSurfaceOrigin,
  type ConversationTimeline,
} from '../../services/chatSessionRegistry';
import {
  useChatSession,
  type ChatSessionContextValue,
} from './ChatSessionContext';

export interface ChatSurfaceSendContext {
  conversationId: string | null;
  origin: ChatSurfaceOrigin;
}

export type ChatSurfaceSendHandler = (
  content: string,
  mediaFiles: MediaFile[] | undefined,
  context: ChatSurfaceSendContext,
) => Promise<void>;

export interface ChatSurfaceController extends ChatSessionContextValue {
  sendMessage: (content: string, mediaFiles?: MediaFile[]) => Promise<void>;
}

export interface ChatSurfaceControllerOptions {
  onSend?: ChatSurfaceSendHandler;
}

export function useChatSurfaceController({
  onSend,
}: ChatSurfaceControllerOptions = {}): ChatSurfaceController {
  const chatSession = useChatSession();
  const sendMessageToConversation = useChatStore((state) => state.sendMessageToConversation);

  const sendMessage = useCallback(async (content: string, mediaFiles?: MediaFile[]) => {
    const targetConversationId = chatSession.conversationId ?? chatSession.origin.conversationId;
    if (onSend) {
      await onSend(content, mediaFiles, {
        conversationId: targetConversationId,
        origin: chatSession.origin,
      });
      return;
    }

    if (!targetConversationId) {
      throw new Error(i18next.t('chat.errors.chatTabNotReady'));
    }

    const origin = normalizeChatSurfaceOrigin(chatSession.origin, targetConversationId);
    await sendMessageToConversation(targetConversationId, content, mediaFiles, undefined, { origin });
  }, [chatSession.conversationId, chatSession.origin, onSend, sendMessageToConversation]);

  return {
    ...chatSession,
    sendMessage,
  };
}

export function useChatConversationTimeline(conversationId?: string | null): ConversationTimeline | null {
  return useChatStore((state) => (
    conversationId ? getConversationTimeline(state, conversationId) : null
  ));
}
