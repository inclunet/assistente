import i18next from 'i18next';
import { announce } from '../hooks/useAnnouncer';
import { useWorkspaceStore } from '../store/workspaceStore';
import { playReceiveSound } from './audioFeedback';

export function isChatConversationActive(conversationId: string): boolean {
  return useWorkspaceStore.getState().getActiveTab?.()?.conversationId === conversationId;
}

export function getChatConversationLabel(conversationId: string, fallbackTitle?: string | null): string {
  const workspace = useWorkspaceStore.getState().workspace;
  const tab = workspace?.tabs.find((candidate) => candidate.conversationId === conversationId);
  return String(
    tab?.title
      || fallbackTitle
      || i18next.t('chat.conversation', { defaultValue: 'Conversa' }),
  ).trim();
}

export function announceForActiveChatConversation(
  conversationId: string,
  message: string,
  priority: 'polite' | 'assertive' = 'polite',
) {
  if (isChatConversationActive(conversationId)) {
    announce(message, priority);
  }
}

export function announceChatBackgroundResponseDone(conversationId: string, fallbackTitle?: string | null) {
  if (isChatConversationActive(conversationId)) return;
  announce(
    i18next.t('chat.announce.backgroundResponseDone', {
      title: getChatConversationLabel(conversationId, fallbackTitle),
    }),
    'polite',
  );
}

export function playChatReceiveSoundIfActive(conversationId: string) {
  if (isChatConversationActive(conversationId)) {
    playReceiveSound();
  }
}
