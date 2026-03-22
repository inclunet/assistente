import type { NavigateFunction } from 'react-router-dom';
import { useChatStore } from '../store/chatStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useNavigationStore, type EditableResource } from '../store/navigationStore';
import { announce } from '../hooks/useAnnouncer';
import i18n from './i18n';

// ─── Protocol ────────────────────────────────────────────────────────
export const DEEP_LINK_PROTOCOL = 'assistente';
export const DEEP_LINK_PREFIX = `${DEEP_LINK_PROTOCOL}://`;

// ─── Action types ────────────────────────────────────────────────────
export type TabType = 'tasklist' | 'editor' | 'terminal';

export type DeepLinkAction =
  | { type: 'conversation:open'; conversationId: number }
  | { type: 'conversation:new'; message?: string; title?: string }
  | { type: 'conversation:send'; conversationId: number; message: string }
  | { type: 'navigate'; route: string }
  | { type: 'resource:edit'; resource: EditableResource; resourceId: string }
  | { type: 'resource:new'; resource: EditableResource }
  | { type: 'tab:open'; tabType: TabType; contentId: string };

// Routes the app actually supports (validated in navigate action)
const VALID_ROUTES = new Set([
  '', 'terminal', 'editor', 'allowlists', 'skills', 'mcp',
  'channels', 'credentials', 'providers', 'settings', 'profiles',
  'history', 'help', 'about', 'update',
]);

const EDITABLE_RESOURCES = new Set<EditableResource>([
  'profiles', 'providers', 'credentials', 'allowlists',
  'skills', 'mcp', 'channels',
]);

const TAB_RESOURCES = new Set<TabType>(['tasklist', 'editor', 'terminal']);

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

    // assistente://{resource}/edit/{id} or assistente://{resource}/new
    if (EDITABLE_RESOURCES.has(resource as EditableResource)) {
      const action = segments[1];
      if (action === 'new') {
        return { type: 'resource:new', resource: resource as EditableResource };
      }
      if (action === 'edit' && segments[2]) {
        const resourceId = decodeURIComponent(segments.slice(2).join('/'));
        return { type: 'resource:edit', resource: resource as EditableResource, resourceId };
      }
    }

    // assistente://tasklist/{id}, assistente://editor/{id}, assistente://terminal/{id}
    if (TAB_RESOURCES.has(resource as TabType) && segments[1]) {
      return { type: 'tab:open', tabType: resource as TabType, contentId: decodeURIComponent(segments[1]) };
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

    case 'resource:edit':
      return `${DEEP_LINK_PREFIX}${action.resource}/edit/${encodeURIComponent(action.resourceId)}`;

    case 'resource:new':
      return `${DEEP_LINK_PREFIX}${action.resource}/new`;

    case 'tab:open':
      return `${DEEP_LINK_PREFIX}${action.tabType}/${encodeURIComponent(action.contentId)}`;
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
    case 'resource:edit': {
      const rKey = ROUTE_I18N_KEYS[action.resource];
      const rLabel = rKey ? t(rKey) : action.resource;
      return t('deepLink.editResource', { page: rLabel, id: action.resourceId });
    }
    case 'resource:new': {
      const rKey = ROUTE_I18N_KEYS[action.resource];
      const rLabel = rKey ? t(rKey) : action.resource;
      return t('deepLink.newResource', { page: rLabel });
    }
    case 'tab:open':
      return t('deepLink.openTab', { type: action.tabType, id: action.contentId });
  }
}

// CSS class suffix per action type (used by the markdown plugin)
export function getDeepLinkTypeClass(action: DeepLinkAction): string {
  switch (action.type) {
    case 'conversation:open': return 'deep-link--conversation';
    case 'conversation:new': return 'deep-link--new-conversation';
    case 'conversation:send': return 'deep-link--send';
    case 'navigate': return 'deep-link--navigate';
    case 'resource:edit': return 'deep-link--resource-edit';
    case 'resource:new': return 'deep-link--resource-new';
    case 'tab:open': return `deep-link--${action.tabType}`;
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
  const t = i18n.t.bind(i18n);

  const wsStore = useWorkspaceStore.getState();

  const openOrCreateChatTab = (conversationId: number) => {
    const existing = (wsStore.workspace?.tabs || []).find(
      (tab) => tab.type === 'chat' && tab.contentId === String(conversationId),
    );
    if (existing) {
      void wsStore.setActiveTab(existing.id);
    } else {
      void wsStore.addTab('chat', String(conversationId), 'Conversa');
    }
  };

  switch (action.type) {
    case 'conversation:open': {
      openOrCreateChatTab(action.conversationId);
      deps.navigate('/');
      announce(t('deepLink.announcedOpen', { id: action.conversationId }));
      break;
    }

    case 'conversation:new': {
      void wsStore.addTab('chat', '', action.title || 'Nova Conversa');
      deps.navigate('/');
      if (action.message) {
        await new Promise((r) => setTimeout(r, 200));
        await useChatStore.getState().sendMessage(action.message);
      }
      announce(action.title || t('deepLink.announcedNewConversation'));
      break;
    }

    case 'conversation:send': {
      openOrCreateChatTab(action.conversationId);
      deps.navigate('/');
      await new Promise((r) => setTimeout(r, 200));
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

    case 'resource:edit': {
      const navStore = useNavigationStore.getState();
      navStore.requestResourceEdit(action.resource, action.resourceId, 'edit');
      deps.navigate(`/${action.resource}`);
      announce(t('deepLink.announcedEditResource', { id: action.resourceId }));
      break;
    }

    case 'resource:new': {
      const navStore = useNavigationStore.getState();
      navStore.requestResourceEdit(action.resource, '', 'new');
      deps.navigate(`/${action.resource}`);
      announce(t('deepLink.announcedNewResource'));
      break;
    }

    case 'tab:open': {
      const tabs = wsStore.workspace?.tabs || [];
      const existing = tabs.find(
        (tab) => tab.type === action.tabType && tab.contentId === action.contentId,
      );
      if (existing) {
        void wsStore.setActiveTab(existing.id);
      } else {
        void wsStore.addTab(action.tabType, action.contentId, `${action.tabType} ${action.contentId}`);
      }
      deps.navigate('/');
      announce(t('deepLink.announcedOpenTab', { type: action.tabType, id: action.contentId }));
      break;
    }
  }
}
