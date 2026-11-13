import { useAuthStore } from '../store/authStore';
import { useUIStore } from '../store/uiStore';

const pending = new Map<string, () => void>();
const activeSessions = new Map<string, object>();
const sessionKey = (session: { userId: string; sessionId: string }) => JSON.stringify([session.userId, session.sessionId]);

/** The global MCP runtime owns notice delivery for the authenticated session. */
export function activateMCPAuthorizationNotices(session: { userId: string; sessionId: string }): () => void {
  const key = sessionKey(session);
  const owner = {};
  activeSessions.set(key, owner);
  return () => {
    if (activeSessions.get(key) !== owner) return;
    activeSessions.delete(key);
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
  pending.get(key)?.();
  if (document.visibilityState === 'visible' && document.hasFocus()) {
    useUIStore.getState().addToast(message(), type);
    return () => {};
  }
  let unsubscribe = () => {};
  const cleanup = () => {
    window.removeEventListener('focus', tryPresent);
    document.removeEventListener('visibilitychange', tryPresent);
    unsubscribe();
    if (pending.get(key) === cleanup) pending.delete(key);
  };
  const tryPresent = () => {
    if (!sameSession() || activeSessions.get(sessionKey(session)) !== owner) { cleanup(); return; }
    if (document.visibilityState !== 'visible' || !document.hasFocus()) return;
    cleanup();
    useUIStore.getState().addToast(message(), type);
  };
  pending.set(key, cleanup);
  unsubscribe = useAuthStore.subscribe(() => { if (!sameSession()) cleanup(); });
  window.addEventListener('focus', tryPresent);
  document.addEventListener('visibilitychange', tryPresent);
  tryPresent();
  return cleanup;
}

/** Remove all delayed results when their session-scoped runtime is removed. */
export function cancelMCPAuthorizationNotices(session: { userId: string; sessionId: string }): void {
  const prefix = `${JSON.stringify([session.userId, session.sessionId]).slice(0, -1)},`;
  for (const [key, cancel] of pending) {
    if (key.startsWith(prefix)) cancel();
  }
}
