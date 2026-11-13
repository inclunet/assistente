import { useAuthStore } from '../store/authStore';
import { useUIStore } from '../store/uiStore';
import { announceWithOrigin, estimateAnnouncementReadingMs } from './voiceAccessibility/announcerBroker';

type PendingNotice = { cancel: () => void; take: () => { message: string; type: 'success' | 'error' } };
const pending = new Map<string, PendingNotice>();
const activeSessions = new Map<string, object>();
const readingUntil = new Map<string, number>();
const sessionKey = (session: { userId: string; sessionId: string }) => JSON.stringify([session.userId, session.sessionId]);

/** The global MCP runtime owns notice delivery for the authenticated session. */
export function activateMCPAuthorizationNotices(session: { userId: string; sessionId: string }): () => void {
  const key = sessionKey(session);
  const owner = {};
  activeSessions.set(key, owner);
  return () => {
    if (activeSessions.get(key) !== owner) return;
    activeSessions.delete(key);
    readingUntil.delete(key);
    cancelMCPAuthorizationNotices(session);
  };
}

export function getMCPAuthorizationNoticeOwner(session: { userId: string; sessionId: string }): object | undefined {
  return activeSessions.get(sessionKey(session));
}

/** Present the newest result for a server when its originating session is focused. */
export function presentMCPAuthorizationNotice(
  session: { userId: string; sessionId: string },
  slug: string,
  owner: object | undefined,
  message: () => string,
  type: 'success' | 'error',
): () => void {
  const key = JSON.stringify([session.userId, session.sessionId, slug]);
  const sameSession = () => {
    const auth = useAuthStore.getState();
    return auth.isAuthenticated && auth.user?.userId === session.userId && auth.user?.sessionId === session.sessionId;
  };
  if (!sameSession() || !owner || activeSessions.get(sessionKey(session)) !== owner) return () => {};
  pending.get(key)?.cancel();
  let unsubscribe = () => {};
  let frame: number | null = null;
  let timer: ReturnType<typeof setTimeout> | null = null;
  const cleanup = () => {
    window.removeEventListener('focus', tryPresent);
    document.removeEventListener('visibilitychange', tryPresent);
    if (frame !== null) { cancelAnimationFrame(frame); frame = null; }
    if (timer !== null) { clearTimeout(timer); timer = null; }
    unsubscribe();
    if (pending.get(key)?.cancel === cleanup) pending.delete(key);
  };
  const flush = () => {
    frame = null;
    if (!sameSession() || activeSessions.get(sessionKey(session)) !== owner) { cleanup(); return; }
    if (document.visibilityState !== 'visible' || !document.hasFocus()) return;
    const prefix = `${sessionKey(session).slice(0, -1)},`;
    const notices = [...pending.entries()].filter(([pendingKey]) => pendingKey.startsWith(prefix)).map(([, item]) => item.take());
    if (notices.length === 0) return;
    notices.forEach((notice) => useUIStore.getState().addToast(notice.message, notice.type, undefined, undefined, { suppressAnnounce: true }));
    const combined = notices.map((notice) => notice.message).join('. ');
    readingUntil.set(sessionKey(session), Date.now() + estimateAnnouncementReadingMs(combined));
    announceWithOrigin({ message: combined, announcePriority: notices.some((notice) => notice.type === 'error') ? 'assertive' : 'polite', eventType: 'user-action' });
  };
  const tryPresent = () => {
    if (!sameSession() || activeSessions.get(sessionKey(session)) !== owner) { cleanup(); return; }
    if (document.visibilityState !== 'visible' || !document.hasFocus() || frame !== null || timer !== null) return;
    const delay = (readingUntil.get(sessionKey(session)) ?? 0) - Date.now();
    if (delay > 0) {
      timer = setTimeout(() => { timer = null; tryPresent(); }, delay);
      return;
    }
    frame = requestAnimationFrame(flush);
  };
  pending.set(key, { cancel: cleanup, take: () => { cleanup(); return { message: message(), type }; } });
  unsubscribe = useAuthStore.subscribe(() => { if (!sameSession()) cleanup(); });
  window.addEventListener('focus', tryPresent);
  document.addEventListener('visibilitychange', tryPresent);
  tryPresent();
  return cleanup;
}

/** Remove all delayed results when their session-scoped runtime is removed. */
export function cancelMCPAuthorizationNotices(session: { userId: string; sessionId: string }): void {
  const prefix = `${JSON.stringify([session.userId, session.sessionId]).slice(0, -1)},`;
  for (const [key, item] of pending) {
    if (key.startsWith(prefix)) item.cancel();
  }
}
