import i18n from '../lib/i18n';
import { useAuthStore } from '../store/authStore';
import { useMCPStore } from '../store/mcpStore';
import { getMCPAuthorizationNoticeOwner, presentMCPAuthorizationNotice } from './mcpAuthorizationNotice';
import { mcpOAuthErrorMessage } from '../lib/mcpOAuthErrors';

type PendingAuthorization = { promise: Promise<void>; observers: Set<object> };
const pending = new Map<string, PendingAuthorization>();

/** Shared user action for the MCP page, with one operation per server and session. */
export function reauthorizeMCPServer(server: { slug: string; name: string }): Promise<void> {
  const user = useAuthStore.getState().user;
  if (!user || !useAuthStore.getState().isAuthenticated) return Promise.resolve();
  const noticeOwner = getMCPAuthorizationNoticeOwner(user);
  const key = `${user.userId}:${user.sessionId}:${server.slug}`;
  const current = pending.get(key);
  if (current) {
    if (noticeOwner) current.observers.add(noticeOwner);
    return current.promise;
  }
  const observers = new Set<object>();
  if (noticeOwner) observers.add(noticeOwner);
  const sameSession = () => {
    const auth = useAuthStore.getState();
    return auth.isAuthenticated && auth.user?.userId === user.userId && auth.user?.sessionId === user.sessionId;
  };
  const operation = (async () => {
    try {
      await useMCPStore.getState().reauthorize(server.slug);
      if (sameSession()) observers.forEach((owner) => presentMCPAuthorizationNotice(user, server.slug, owner, () => i18n.t('mcp.toast.serverReauthorized', { name: server.name }), 'success'));
    } catch (error) {
      if (sameSession()) observers.forEach((owner) => presentMCPAuthorizationNotice(user, server.slug, owner, () => mcpOAuthErrorMessage(error, i18n.t.bind(i18n), i18n.t('mcp.error.reauthorizeFailed')), 'error'));
    }
  })().finally(() => { pending.delete(key); });
  pending.set(key, { promise: operation, observers });
  return operation;
}
