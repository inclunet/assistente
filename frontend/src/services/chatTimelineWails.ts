import * as rpc from '@wailsjs/go/wailsapi/ChatTimeline';
import { apidto } from '@wailsjs/go/models';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useAuthStore } from '../store/authStore';
import { CHAT_TIMELINE_EVENT } from '../types/chatTimelineEvents';
import { logger } from '../utils/logger';
import { ChatTimelineClient, type TimelineTransport } from './chatTimelineClient';
import type { TimelineSnapshot } from './timelineConversationState';

export function normalizeTimelineSnapshot(snapshot: apidto.SubscribedTimelineSnapshot): TimelineSnapshot {
  return {
    ...snapshot,
    scopeParentId: snapshot.scopeParentId ?? null,
    orderedItemIds: snapshot.orderedItemIds ?? [],
    retainedItemIds: snapshot.retainedItemIds ?? [],
    items: (snapshot.items ?? []).map(item => ({
      ...item,
      scopeParentId: item.scopeParentId ?? null,
      turnId: item.turnId ?? null,
      representativeMessageId: item.representativeMessageId ?? null,
      messageIds: item.messageIds ?? [],
      segments: item.segments ?? [],
    })),
  };
}

const transport: TimelineTransport = {
  listen: receive => EventsOn(CHAT_TIMELINE_EVENT, receive),
  open: async (conversationId, surfaceId, scopeParentId, position) => normalizeTimelineSnapshot(await rpc.Open(
    new apidto.OpenTimelineRequest({ conversationId, surfaceId, scopeParentId, ...position }),
  )),
  refresh: async (windowId, windowRevision, position) => normalizeTimelineSnapshot(await rpc.Refresh(
    windowId, new apidto.RefreshTimelineRequest({ windowRevision, position }),
  )),
  navigate: async (windowId, epoch, windowRevision, mode, position) => normalizeTimelineSnapshot(await rpc.Navigate(
    windowId, new apidto.TimelineWindowNavigation({ epoch, windowRevision, mode, ...position }),
  )),
  retain: (windowId, epoch, windowRevision, itemIds) => rpc.SetRetention(
    windowId, new apidto.TimelineRetentionRequest({ epoch, windowRevision, itemIds }),
  ),
  close: rpc.Close,
};

let current: { userId: string; sessionId: string; client: ChatTimelineClient } | undefined;

/** Ativação lazy: importar o módulo não abre inscrições nem escuta eventos de chat. */
export function getChatTimelineClient(): ChatTimelineClient {
  const { user, isAuthenticated } = useAuthStore.getState();
  if (!isAuthenticated || !user) throw new Error('Authenticated timeline session required');
  if (!current) {
    const client = new ChatTimelineClient(transport);
    current = { userId: user.userId, sessionId: user.sessionId, client };
    const unsubscribe = useAuthStore.subscribe(state => {
      if (state.isAuthenticated && state.user?.userId === user.userId && state.user?.sessionId === user.sessionId) return;
      unsubscribe();
      if (current?.client === client) current = undefined;
      void client.dispose().catch(error => logger.error('[chatTimeline] subscription cleanup failed', error));
    });
  }
  return current.client;
}
