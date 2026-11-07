import { useCallback } from 'react';
import i18next from 'i18next';
import { useChatStore } from '../../store/chatStore';
import type { MediaFile } from '../../services/mediaService';
import type { ChatMessagingExecution } from '../../lib/commandChatMessaging';
import type { llm } from '../../../wailsjs/go/models';
import {
  getConversationInfo,
  normalizeChatSurfaceOrigin,
  type ChatSurfaceOrigin,
  type ConversationInfo,
} from '../../services/chatSessionRegistry';
import {
  useChatSession,
  type ChatSessionContextValue,
} from './ChatSessionContext';

export interface ChatSurfaceSendContext {
  command?: ChatMessagingExecution;
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
  command?: ChatMessagingExecution,
  onExecutionStarted?: (executionId: string) => void,
): Promise<boolean> {
  return useChatStore.getState().sendMessageToConversation(
    conversationId,
    content,
    mediaFiles,
    paramsOverride,
    { origin, command, onExecutionStarted },
  );
}

export interface ChatSurfaceController extends ChatSessionContextValue {
  sendMessage: (content: string, mediaFiles?: MediaFile[], command?: ChatMessagingExecution) => Promise<boolean>;
}

export interface ChatSurfaceControllerOptions {
  onSend?: ChatSurfaceSendHandler;
}

export function useChatSurfaceController({
  onSend,
}: ChatSurfaceControllerOptions = {}): ChatSurfaceController {
  const chatSession = useChatSession();
  const sendMessageToConversation = useChatStore((state) => state.sendMessageToConversation);

  const sendMessage = useCallback(async (content: string, mediaFiles?: MediaFile[], command?: ChatMessagingExecution): Promise<boolean> => {
    const targetConversationId = chatSession.conversationId ?? chatSession.origin.conversationId;
    if (onSend) {
      return (await onSend(content, mediaFiles, {
        conversationId: targetConversationId,
        origin: chatSession.origin,
        command,
      })) !== false;
    }

    if (!targetConversationId) {
      throw new Error(i18next.t('chat.errors.chatTabNotReady'));
    }

    const origin = normalizeChatSurfaceOrigin(chatSession.origin, targetConversationId);
    return sendMessageToConversation(targetConversationId, content, mediaFiles, undefined, { origin, command });
  }, [chatSession.conversationId, chatSession.origin, onSend, sendMessageToConversation]);

  return {
    ...chatSession,
    sendMessage,
  };
}

export function useChatConversationInfo(conversationId?: string | null): ConversationInfo | null {
  return useChatStore((state) => (
    conversationId ? getConversationInfo(state, conversationId) : null
  ));
}
