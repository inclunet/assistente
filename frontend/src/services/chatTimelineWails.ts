import * as rpc from '@wailsjs/go/wailsapi/ChatTimeline';
import { apidto } from '@wailsjs/go/models';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useAuthStore } from '../store/authStore';
import { CHAT_TIMELINE_EVENT } from '../types/chatTimelineEvents';
import { logger } from '../utils/logger';
import { ChatTimelineClient, type TimelineTransport } from './chatTimelineClient';
import type { TimelineSnapshot } from './timelineConversationState';

export function normalizeTimelineItem(item: apidto.CanonicalItem) {
  return { ...item, scopeParentId: item.scopeParentId ?? null, turnId: item.turnId ?? null,
    representativeMessageId: item.representativeMessageId ?? null,
    messageIds: item.messageIds ?? [], segments: item.segments ?? [] };
}

export function normalizeTimelineSnapshot(snapshot: apidto.SubscribedTimelineSnapshot): TimelineSnapshot {
  return {
    ...snapshot,
    scopeParentId: snapshot.scopeParentId ?? null,
    orderedItemIds: snapshot.orderedItemIds ?? [],
    retainedItemIds: snapshot.retainedItemIds ?? [],
    items: (snapshot.items ?? []).map(normalizeTimelineItem),
  };
}

const transport: TimelineTransport = {
  reserveItem: (windowId, identity) => rpc.ReserveRecoveryItem(windowId, new apidto.TimelineRecoveryItemRequest(identity)),
  readItem: async (windowId, identity) => normalizeTimelineItem(await rpc.ReadRecoveryItem(windowId, new apidto.TimelineRecoveryItemRequest(identity))),
  releaseItem: (windowId, identity) => rpc.ReleaseRecoveryItem(windowId, new apidto.TimelineRecoveryItemRequest(identity)),
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
  readRecovery: (windowId, reference, cursor) => rpc.ReadRecoveryPage(
    windowId, new apidto.TimelineRecoveryPageRequest({ reference, cursor }),
  ),
  reserveRecovery: (windowId, reference, contentVersion) => rpc.ReserveRecovery(
    windowId, new apidto.TimelineRecoveryReservationRequest({ reference, contentVersion }),
  ),
  releaseRecovery: (windowId, reference, contentVersion) => rpc.ReleaseRecovery(
    windowId, new apidto.TimelineRecoveryReservationRequest({ reference, contentVersion }),
  ),
  close: rpc.Close,
};

let current: { userId: string; sessionId: string; client: ChatTimelineClient; unsubscribe: () => void } | undefined;

function discardCurrent() {
  const previous = current;
  current = undefined;
  if (!previous) return;
  previous.unsubscribe();
  void previous.client.dispose().catch(error => logger.error('[chatTimeline] subscription cleanup failed', error));
}

/** Ativação lazy: importar o módulo não abre inscrições nem escuta eventos de chat. */
export function getChatTimelineClient(): ChatTimelineClient {
  const { user, isAuthenticated } = useAuthStore.getState();
  if (current && (!isAuthenticated || user?.userId !== current.userId || user?.sessionId !== current.sessionId)) discardCurrent();
  if (!isAuthenticated || !user) throw new Error('Authenticated timeline session required');
  if (!current) {
    const client = new ChatTimelineClient(transport);
    const unsubscribe = useAuthStore.subscribe(state => {
      if (state.isAuthenticated && state.user?.userId === user.userId && state.user?.sessionId === user.sessionId) return;
      if (current?.client === client) discardCurrent();
    });
    current = { userId: user.userId, sessionId: user.sessionId, client, unsubscribe };
  }
  return current.client;
}
