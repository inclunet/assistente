import { useChatTimelineRecoveryStore } from '../../store/chatTimelineRecoveryStore';
import { useEffect, useMemo, useRef, useSyncExternalStore } from 'react';
import { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import { getChatTimelineClient } from '../../services/chatTimelineWails';
import { useAuthStore } from '../../store/authStore';
import { logger } from '../../utils/logger';

export function useChatTimelinePresentation(conversationId: string | null, surfaceId: string) {
  const owner = useAuthStore(state => state.user);
  const presentation = useMemo(() => conversationId && owner
    ? new ChatTimelinePresentation(getChatTimelineClient(), conversationId, `${surfaceId}:${crypto.randomUUID()}`)
    : null, [conversationId, surfaceId, owner?.userId, owner?.sessionId]);
  const lifetime = useRef(0);
  const opened = useRef<ChatTimelinePresentation | null>(null);
  useEffect(() => {
    const generation = ++lifetime.current;
    if (presentation && opened.current !== presentation) {
      opened.current = presentation;
      void presentation.open().catch(error => logger.error('[ChatTimeline] open failed', error));
    }
    return () => {
      queueMicrotask(() => {
        if (lifetime.current !== generation && opened.current === presentation) return;
        void presentation?.close().catch(error => {
          const current = useAuthStore.getState();
          if (current.isAuthenticated && current.user?.userId === owner?.userId && current.user?.sessionId === owner?.sessionId) {
            useChatTimelineRecoveryStore.getState().hold(presentation!);
          }
          logger.error('[ChatTimeline] close requires recovery', error);
        });
      });
    };
  }, [presentation, owner?.userId, owner?.sessionId]);
  const revision = useSyncExternalStore(presentation?.subscribe ?? emptySubscribe, presentation?.getRevision ?? emptyRevision);
  const nodes = useMemo(() => presentation?.nodes() ?? [], [presentation, revision]);
  return { presentation, nodes, revision };
}

const emptySubscribe = () => () => {};
const emptyRevision = () => 0;
