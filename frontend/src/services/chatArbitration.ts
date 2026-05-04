import i18next from 'i18next';
import { useWorkspaceStore } from '../store/workspaceStore';
import { announceWithOrigin } from './voiceAccessibility/announcerBroker';
import type { VoiceAccessibilityOrigin } from './voiceAccessibility/types';
import { playReceiveSound } from './audioFeedback';
import type { ChatSurfaceOrigin } from './chatSessionRegistry';

function findOriginTab(origin?: ChatSurfaceOrigin | null) {
  if (!origin?.tabId) return null;
  const workspace = useWorkspaceStore.getState().workspace;
  return workspace?.tabs.find((tab) => tab.id === origin.tabId) ?? null;
}

export function isChatConversationActive(conversationId: string, origin?: ChatSurfaceOrigin | null): boolean {
  const workspace = useWorkspaceStore.getState().workspace;
  if (origin?.tabId) {
    return workspace?.activeTabId === origin.tabId;
  }
  const activeTab = workspace?.tabs.find((tab) => tab.id === workspace.activeTabId);
  return activeTab?.conversationId === conversationId;
}

export function getChatConversationLabel(
  conversationId: string,
  fallbackTitle?: string | null,
  origin?: ChatSurfaceOrigin | null,
): string {
  const workspace = useWorkspaceStore.getState().workspace;
  const tab = findOriginTab(origin)
    ?? workspace?.tabs.find((candidate) => candidate.conversationId === conversationId);
  return String(
    tab?.title
      || fallbackTitle
      || i18next.t('chat.conversation', { defaultValue: 'Conversa' }),
  ).trim();
}

export function getChatConversationVoiceOrigin(
  conversationId: string,
  fallbackTitle?: string | null,
  origin?: ChatSurfaceOrigin | null,
): VoiceAccessibilityOrigin {
  const workspace = useWorkspaceStore.getState().workspace;
  const tab = findOriginTab(origin)
    ?? workspace?.tabs.find((candidate) => candidate.conversationId === conversationId);

  return {
    tabId: origin?.tabId ?? tab?.id,
    surfaceId: origin?.surfaceId ?? tab?.id ?? `conversation:${conversationId}`,
    sessionKey: origin?.sessionKey ?? (tab?.id ? `${tab.id}:${conversationId}` : `conversation:${conversationId}`),
    conversationId,
    surfaceType: origin?.surfaceType ?? tab?.type ?? 'page',
    profileSlug: (tab?.profileOverride?.slug as string | undefined) ?? workspace?.profile ?? null,
    title: getChatConversationLabel(conversationId, fallbackTitle, origin),
  };
}

export function announceForActiveChatConversation(
  conversationId: string,
  message: string,
  priority: 'polite' | 'assertive' = 'polite',
  origin?: ChatSurfaceOrigin | null,
) {
  announceWithOrigin({
    message,
    announcePriority: priority,
    origin: getChatConversationVoiceOrigin(conversationId, undefined, origin),
    eventType: 'progress',
  });
}

export function announceChatBackgroundResponseDone(
  conversationId: string,
  fallbackTitle?: string | null,
  origin?: ChatSurfaceOrigin | null,
) {
  if (isChatConversationActive(conversationId, origin)) return;

  announceWithOrigin({
    message: i18next.t('chat.announce.backgroundResponseDone', {
      title: getChatConversationLabel(conversationId, fallbackTitle, origin),
    }),
    announcePriority: 'polite',
    origin: getChatConversationVoiceOrigin(conversationId, fallbackTitle, origin),
    eventType: 'completion',
  });
}

export function playChatReceiveSoundIfActive(conversationId: string, origin?: ChatSurfaceOrigin | null) {
  if (isChatConversationActive(conversationId, origin)) {
    playReceiveSound();
  }
}
