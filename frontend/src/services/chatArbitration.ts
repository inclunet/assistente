import i18next from 'i18next';
import { useWorkspaceStore } from '../store/workspaceStore';
import { announceWithOrigin } from './voiceAccessibility/announcerBroker';
import type { VoiceAccessibilityOrigin } from './voiceAccessibility/types';
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

export function getChatConversationVoiceOrigin(
  conversationId: string,
  fallbackTitle?: string | null,
): VoiceAccessibilityOrigin {
  const workspace = useWorkspaceStore.getState().workspace;
  const tab = workspace?.tabs.find((candidate) => candidate.conversationId === conversationId);

  return {
    tabId: tab?.id,
    surfaceId: tab?.id ?? `conversation:${conversationId}`,
    sessionKey: tab?.id ? `${tab.id}:${conversationId}` : `conversation:${conversationId}`,
    conversationId,
    surfaceType: tab?.type ?? 'page',
    profileSlug: (tab?.profileOverride?.slug as string | undefined) ?? workspace?.profile ?? null,
    title: getChatConversationLabel(conversationId, fallbackTitle),
  };
}

export function announceForActiveChatConversation(
  conversationId: string,
  message: string,
  priority: 'polite' | 'assertive' = 'polite',
) {
  announceWithOrigin({
    message,
    announcePriority: priority,
    origin: getChatConversationVoiceOrigin(conversationId),
    eventType: 'progress',
  });
}

export function announceChatBackgroundResponseDone(conversationId: string, fallbackTitle?: string | null) {
  if (isChatConversationActive(conversationId)) return;

  announceWithOrigin({
    message: i18next.t('chat.announce.backgroundResponseDone', {
      title: getChatConversationLabel(conversationId, fallbackTitle),
    }),
    announcePriority: 'polite',
    origin: getChatConversationVoiceOrigin(conversationId, fallbackTitle),
    eventType: 'completion',
  });
}

export function playChatReceiveSoundIfActive(conversationId: string) {
  if (isChatConversationActive(conversationId)) {
    playReceiveSound();
  }
}
