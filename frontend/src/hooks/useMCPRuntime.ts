import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useAuthStore } from '../store/authStore';
import { useMCPStore } from '../store/mcpStore';
import { announceWithOrigin } from '../services/voiceAccessibility/announcerBroker';

/** One session-scoped owner for MCP subscriptions and consent announcements. */
export function useMCPRuntime() {
  const { t } = useTranslation();
  const translation = useRef(t);
  useEffect(() => {
    translation.current = t;
  }, [t]);
  const authenticated = useAuthStore((s) => s.isAuthenticated);
  const userId = useAuthStore((s) => s.user?.userId);
  const sessionId = useAuthStore((s) => s.user?.sessionId);
  useEffect(() => {
    const store = useMCPStore.getState();
    store.reset();
    if (!authenticated || !userId || !sessionId) return;
    const unsubscribeEvents = store.setupEventListeners();
    // The backend owns opening the browser. This listener only announces it;
    // it never opens another window or asks for a duplicate confirmation.
    const announceConsent = (event: { slug: string }) => {
      const server = useMCPStore.getState().servers.find((s) => s.slug === event.slug);
      announceWithOrigin({ message: translation.current('mcp.announce.reauthorizing', { name: server?.name || event.slug }), eventType: 'user-action', announcePriority: 'assertive' });
    };
    const unsubscribeConsent = EventsOn('mcp:oauth_authorize', announceConsent);
    const unsubscribeDevice = EventsOn('mcp:oauth_device_verify', announceConsent);
    void store.loadServers();
    return () => {
      unsubscribeEvents();
      unsubscribeConsent();
      unsubscribeDevice();
      useMCPStore.getState().reset();
    };
  }, [authenticated, userId, sessionId]);
}
