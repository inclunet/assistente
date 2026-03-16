import type { NavigateFunction } from 'react-router-dom';
import { useChatStore } from '../store/chatStore';
import { announce } from '../hooks/useAnnouncer';
import i18n from './i18n';

// ─── Protocol ────────────────────────────────────────────────────────
export const DEEP_LINK_PROTOCOL = 'assistente';
export const DEEP_LINK_PREFIX = `${DEEP_LINK_PROTOCOL}://`;

// ─── Action types ────────────────────────────────────────────────────
export type DeepLinkAction =
  | { type: 'conversation:open'; conversationId: number }
  | { type: 'conversation:new'; message?: string; title?: string }
  | { type: 'conversation:send'; conversationId: number; message: string }
  | { type: 'navigate'; route: string };

// Routes the app actually supports (validated in navigate action)
const VALID_ROUTES = new Set([
  '', 'terminal', 'editor', 'allowlists', 'skills', 'mcp',
  'channels', 'credentials', 'providers', 'settings', 'profiles',
  'history', 'help', 'about', 'update',
]);

// Human-readable labels per route (keys into i18n menu namespace)
const ROUTE_I18N_KEYS: Record<string, string> = {
  '': 'menu.chat',
  terminal: 'menu.terminal',
  editor: 'menu.editor',
  allowlists: 'menu.allowlists',
  skills: 'menu.skills',
  mcp: 'menu.mcpServers',
  channels: 'menu.channels',
  credentials: 'menu.credentials',
  providers: 'menu.providers',
  settings: 'menu.settings',
  profiles: 'menu.profiles',
  history: 'menu.history',
  help: 'menu.help',
  about: 'menu.about',
  update: 'menu.update',
};

// ─── Detection ───────────────────────────────────────────────────────

export function isDeepLink(href: string): boolean {
  return href.startsWith(DEEP_LINK_PREFIX);
}

// ─── Parser ──────────────────────────────────────────────────────────

export function parseDeepLink(uri: string): DeepLinkAction | null {
  if (!isDeepLink(uri)) return null;

  try {
    const withoutProtocol = uri.slice(DEEP_LINK_PREFIX.length);
    const [pathAndQuery] = withoutProtocol.split('#');
    const [pathPart, queryPart] = pathAndQuery.split('?');
    const segments = pathPart.split('/').filter(Boolean);
    const params = new URLSearchParams(queryPart || '');

    if (segments.length === 0) return null;

    const resource = segments[0];

    if (resource === 'conversation') {
      // assistente://conversation/new?message=...&title=...
      if (segments[1] === 'new') {
        return {
          type: 'conversation:new',
          message: params.get('message') || undefined,
          title: params.get('title') || undefined,
        };
      }

      const id = Number(segments[1]);
      if (!Number.isInteger(id) || id <= 0) return null;

      // assistente://conversation/{id}/send?message=...
      if (segments[2] === 'send') {
        const message = params.get('message');
        if (!message) return null;
        return { type: 'conversation:send', conversationId: id, message };
      }

      // assistente://conversation/{id}
      return { type: 'conversation:open', conversationId: id };
    }

    if (resource === 'navigate') {
      const route = segments.slice(1).join('/');
      if (!VALID_ROUTES.has(route)) return null;
      return { type: 'navigate', route };
    }

    return null;
  } catch {
    return null;
  }
}

// ─── Builder ─────────────────────────────────────────────────────────

export function buildDeepLink(action: DeepLinkAction): string {
  switch (action.type) {
    case 'conversation:open':
      return `${DEEP_LINK_PREFIX}conversation/${action.conversationId}`;

    case 'conversation:new': {
      const params = new URLSearchParams();
      if (action.message) params.set('message', action.message);
      if (action.title) params.set('title', action.title);
      const qs = params.toString();
      return `${DEEP_LINK_PREFIX}conversation/new${qs ? `?${qs}` : ''}`;
    }

    case 'conversation:send': {
      const params = new URLSearchParams();
      params.set('message', action.message);
      return `${DEEP_LINK_PREFIX}conversation/${action.conversationId}/send?${params}`;
    }

    case 'navigate':
      return `${DEEP_LINK_PREFIX}navigate/${action.route}`;
  }
}

// ─── Label for display ───────────────────────────────────────────────

export function getDeepLinkLabel(action: DeepLinkAction): string {
  const t = i18n.t.bind(i18n);

  switch (action.type) {
    case 'conversation:open':
      return t('deepLink.openConversation', { id: action.conversationId });
    case 'conversation:new':
      return action.title || t('deepLink.newConversation');
    case 'conversation:send':
      return t('deepLink.sendMessage', { id: action.conversationId });
    case 'navigate': {
      const key = ROUTE_I18N_KEYS[action.route];
      const label = key ? t(key) : action.route;
      return t('deepLink.navigateTo', { page: label });
    }
  }
}

// CSS class suffix per action type (used by the markdown plugin)
export function getDeepLinkTypeClass(action: DeepLinkAction): string {
  switch (action.type) {
    case 'conversation:open': return 'deep-link--conversation';
    case 'conversation:new': return 'deep-link--new-conversation';
    case 'conversation:send': return 'deep-link--send';
    case 'navigate': return 'deep-link--navigate';
  }
}

// ─── Executor ────────────────────────────────────────────────────────

export interface DeepLinkDeps {
  navigate: NavigateFunction;
}

export async function executeDeepLink(
  action: DeepLinkAction,
  deps: DeepLinkDeps,
): Promise<void> {
  const store = useChatStore.getState();
  const t = i18n.t.bind(i18n);

  switch (action.type) {
    case 'conversation:open': {
      const existingTab = store.tabs.find(
        (tab) => tab.conversationId === action.conversationId,
      );
      if (existingTab) {
        await store.setActiveTab(existingTab.id);
      } else {
        await store.openConversationInNewTab(action.conversationId);
      }
      deps.navigate('/');
      announce(t('deepLink.announcedOpen', { id: action.conversationId }));
      break;
    }

    case 'conversation:new': {
      const tabId = await store.createTab(true);
      deps.navigate('/');
      if (action.message) {
        // Small delay to ensure the tab is mounted before sending
        await new Promise((r) => setTimeout(r, 100));
        await useChatStore.getState().sendMessage(action.message);
      }
      announce(action.title || t('deepLink.announcedNewConversation'));
      void tabId; // used for createTab
      break;
    }

    case 'conversation:send': {
      // Find tab with this conversation, or open it
      const existingTab = store.tabs.find(
        (t) => t.conversationId === action.conversationId,
      );
      if (existingTab) {
        await store.setActiveTab(existingTab.id);
      } else {
        await store.openConversationInNewTab(action.conversationId);
      }
      deps.navigate('/');
      await new Promise((r) => setTimeout(r, 100));
      await useChatStore.getState().sendMessage(action.message);
      announce(t('deepLink.announcedSent', { id: action.conversationId }));
      break;
    }

    case 'navigate': {
      const path = action.route ? `/${action.route}` : '/';
      deps.navigate(path);
      const key = ROUTE_I18N_KEYS[action.route];
      const label = key ? t(key) : action.route;
      announce(t('deepLink.announcedNavigate', { page: label }));
      break;
    }
  }
}
