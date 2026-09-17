import { useCallback } from 'react';
import i18next from 'i18next';
import { useChatStore } from '../../store/chatStore';
import type { MediaFile } from '../../services/mediaService';
import type { llm } from '../../../wailsjs/go/models';
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
) => Promise<boolean | void>;

export async function sendChatSurfaceMessage(
  conversationId: string,
  content: string,
  mediaFiles: MediaFile[] | undefined,
  paramsOverride: Partial<llm.ChatParams> | undefined,
  origin: ChatSurfaceOrigin | undefined,
): Promise<boolean> {
  return useChatStore.getState().sendMessageToConversation(
    conversationId,
    content,
    mediaFiles,
    paramsOverride,
    { origin },
  );
}

export interface ChatSurfaceController extends ChatSessionContextValue {
  sendMessage: (content: string, mediaFiles?: MediaFile[]) => Promise<boolean>;
}

export interface ChatSurfaceControllerOptions {
  onSend?: ChatSurfaceSendHandler;
}

export function useChatSurfaceController({
  onSend,
}: ChatSurfaceControllerOptions = {}): ChatSurfaceController {
  const chatSession = useChatSession();
  const sendMessageToConversation = useChatStore((state) => state.sendMessageToConversation);

  const sendMessage = useCallback(async (content: string, mediaFiles?: MediaFile[]): Promise<boolean> => {
    const targetConversationId = chatSession.conversationId ?? chatSession.origin.conversationId;
    if (onSend) {
      return (await onSend(content, mediaFiles, {
        conversationId: targetConversationId,
        origin: chatSession.origin,
      })) !== false;
    }

    if (!targetConversationId) {
      throw new Error(i18next.t('chat.errors.chatTabNotReady'));
    }

    const origin = normalizeChatSurfaceOrigin(chatSession.origin, targetConversationId);
    return sendMessageToConversation(targetConversationId, content, mediaFiles, undefined, { origin });
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
