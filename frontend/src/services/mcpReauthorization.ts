import i18n from '../lib/i18n';
import { useAuthStore } from '../store/authStore';
import { useMCPStore } from '../store/mcpStore';
import { useUIStore } from '../store/uiStore';
import { mcpOAuthErrorMessage } from '../lib/mcpOAuthErrors';

const pending = new Map<string, Promise<void>>();

/** Shared user action for the MCP page, with one operation per server and session. */
export function reauthorizeMCPServer(server: { slug: string; name: string }): Promise<void> {
  const user = useAuthStore.getState().user;
  if (!user || !useAuthStore.getState().isAuthenticated) return Promise.resolve();
  const key = `${user.userId}:${user.sessionId}:${server.slug}`;
  const current = pending.get(key);
  if (current) return current;
  const sameSession = () => {
    const auth = useAuthStore.getState();
    return auth.isAuthenticated && auth.user?.userId === user.userId && auth.user?.sessionId === user.sessionId;
  };
  const operation = (async () => {
    try {
      await useMCPStore.getState().reauthorize(server.slug);
      if (sameSession()) useUIStore.getState().addToast(i18n.t('mcp.toast.serverReauthorized', { name: server.name }), 'success');
    } catch (error) {
      if (sameSession()) useUIStore.getState().addToast(mcpOAuthErrorMessage(error, i18n.t.bind(i18n), i18n.t('mcp.error.reauthorizeFailed')), 'error');
    }
  })().finally(() => { pending.delete(key); });
  pending.set(key, operation);
  return operation;
}
