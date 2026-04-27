import type { NavigateFunction } from 'react-router-dom';
import { useChatStore } from '../store/chatStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useEditorStore } from '../store/editorStore';
import { useNavigationStore, type EditableResource } from '../store/navigationStore';
import { announce } from '../hooks/useAnnouncer';
import { EditorReadFile } from '@wailsjs/go/app/App';
import { RunTerminalCommand } from '@wailsjs/go/app/App';
import { BrowserOpenURL } from '@wailsjs/runtime/runtime';
import { isBackendId } from './idUtils';
import i18n from './i18n';

// ─── Protocol ────────────────────────────────────────────────────────
export const DEEP_LINK_PROTOCOL = 'assistente';
export const DEEP_LINK_PREFIX = `${DEEP_LINK_PROTOCOL}://`;

// ─── Action types ────────────────────────────────────────────────────
export type TabType = 'tasklist' | 'editor' | 'terminal';

export type DeepLinkAction =
  | { type: 'conversation:open'; conversationId: string; title?: string }
  | { type: 'conversation:new'; message?: string; title?: string }
  | { type: 'conversation:send'; conversationId: string; message: string }
  | { type: 'navigate'; route: string }
  | { type: 'resource:edit'; resource: EditableResource; resourceId: string }
  | { type: 'resource:new'; resource: EditableResource }
  | { type: 'tab:open'; tabType: TabType; contentId: string; title?: string }
  | { type: 'tab:new'; tabType: TabType; title?: string; file?: string; cmd?: string };

// Routes the app actually supports (validated in navigate action)
const VALID_ROUTES = new Set([
  '', 'settings', 'settings/providers', 'settings/mcp', 'settings/skills',
  'settings/channels', 'settings/contacts', 'settings/credentials',
  'settings/allowlists', 'settings/appearance', 'settings/restore-defaults',
  'profiles', 'history', 'tasklists', 'help', 'about', 'update',
]);

const EDITABLE_RESOURCES = new Set<EditableResource>([
  'profiles', 'providers', 'credentials', 'allowlists',
  'skills', 'mcp', 'channels', 'tasklists',
]);

const TAB_RESOURCES = new Set<TabType>(['tasklist', 'editor', 'terminal']);

function defaultTitleForNewTab(tabType: TabType): string {
  const typeLabel = i18n.t(`workspace.tabType.${tabType}`);
  return i18n.t('deepLink.newTab', { type: typeLabel });
}

// Human-readable labels per route (keys into i18n menu namespace)
const ROUTE_I18N_KEYS: Record<string, string> = {
  '': 'menu.chat',
  settings: 'menu.settings',
  'settings/providers': 'menu.providers',
  'settings/mcp': 'menu.mcp',
  'settings/skills': 'menu.skills',
  'settings/channels': 'menu.channels',
  'settings/contacts': 'settingsPage.tabs.contacts',
  'settings/credentials': 'menu.credentials',
  'settings/allowlists': 'menu.allowlists',
  'settings/appearance': 'appearance.pageTitle',
  'settings/restore-defaults': 'menu.restoreDefaults',
  profiles: 'menu.profiles',
  history: 'menu.history',
  tasklists: 'menu.tasklists',
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

      const id = segments[1] || '';
      if (!isBackendId(id)) return null;

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

    // assistente://tasklist/new, assistente://editor/new, assistente://editor/open, assistente://terminal/new
    if (TAB_RESOURCES.has(resource as TabType)) {
      if (segments[1] === 'new') {
        return {
          type: 'tab:new',
          tabType: resource as TabType,
          title: params.get('title') || undefined,
          cmd: resource === 'terminal' ? (params.get('cmd') || undefined) : undefined,
        };
      }
      if (resource === 'editor' && segments[1] === 'open') {
        const file = params.get('file');
        if (!file) return null;
        return { type: 'tab:new', tabType: 'editor', file, title: params.get('title') || undefined };
      }
      // assistente://tasklist/{id}, assistente://editor/{id}, assistente://terminal/{id}
      if (segments[1]) {
        return { type: 'tab:open', tabType: resource as TabType, contentId: decodeURIComponent(segments[1]) };
      }
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

    case 'tab:new': {
      if (action.file) {
        const params = new URLSearchParams();
        params.set('file', action.file);
        if (action.title) params.set('title', action.title);
        return `${DEEP_LINK_PREFIX}editor/open?${params}`;
      }
      const params = new URLSearchParams();
      if (action.title) params.set('title', action.title);
      if (action.cmd) params.set('cmd', action.cmd);
      const qs = params.toString();
      return `${DEEP_LINK_PREFIX}${action.tabType}/new${qs ? `?${qs}` : ''}`;
    }
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
    case 'tab:new':
      return t('deepLink.newTab', { type: action.tabType });
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
    case 'tab:new': return `deep-link--${action.tabType}-new`;
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

  const openOrCreateChatTab = async (conversationId: string, title?: string) => {
    const existing = (wsStore.workspace?.tabs || []).find(
      (tab) => tab.type === 'chat' && tab.conversationId === conversationId,
    );
    if (existing) {
      await wsStore.setActiveTab(existing.id);
    } else {
      await wsStore.addTab('chat', title || t('chat.newConversation'));
    }
  };

  switch (action.type) {
    case 'conversation:open': {
      await openOrCreateChatTab(action.conversationId, action.title);
      deps.navigate('/');
      announce(t('deepLink.announcedOpen', { id: action.conversationId }));
      break;
    }

    case 'conversation:new': {
      const title = action.title || t('chat.newConversation');
      await useChatStore.getState().createConversation(title);
      await wsStore.addTab('chat', title);
      deps.navigate('/');
      if (action.message) {
        await useChatStore.getState().sendMessage(action.message);
      }
      announce(title);
      break;
    }

    case 'conversation:send': {
      await openOrCreateChatTab(action.conversationId);
      deps.navigate('/');
      await useChatStore.getState().loadConversation(action.conversationId);
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
      const settingsResources = new Set(['providers', 'mcp', 'skills', 'channels', 'credentials', 'allowlists']);
      const path = settingsResources.has(action.resource) ? `/settings/${action.resource}` : `/${action.resource}`;
      deps.navigate(path);
      announce(t('deepLink.announcedEditResource', { id: action.resourceId }));
      break;
    }

    case 'resource:new': {
      const navStore = useNavigationStore.getState();
      navStore.requestResourceEdit(action.resource, '', 'new');
      const settingsResources = new Set(['providers', 'mcp', 'skills', 'channels', 'credentials', 'allowlists']);
      const path = settingsResources.has(action.resource) ? `/settings/${action.resource}` : `/${action.resource}`;
      deps.navigate(path);
      announce(t('deepLink.announcedNewResource'));
      break;
    }

    case 'tab:open': {
      const tabs = wsStore.workspace?.tabs || [];
      // Find existing tab by type-specific content identifier
      const existing = tabs.find((tab) => {
        if (tab.type !== action.tabType) return false;
        if (action.tabType === 'tasklist') return tab.state?.tasklistId === action.contentId;
        if (action.tabType === 'terminal') return tab.state?.sessionId === action.contentId;
        return false;
      });
      if (existing) {
        await wsStore.setActiveTab(existing.id);
      } else {
        const title = action.title || `${action.tabType} ${action.contentId}`;
        if (action.tabType === 'tasklist') {
          await wsStore.addTab(action.tabType, title, { tasklistId: action.contentId });
        } else if (action.tabType === 'terminal') {
          await wsStore.addTab(action.tabType, title, { sessionId: action.contentId });
        } else {
          await wsStore.addTab(action.tabType, title);
        }
      }
      deps.navigate('/');
      announce(t('deepLink.announcedOpenTab', { type: action.tabType, id: action.contentId }));
      break;
    }

    case 'tab:new': {
      if (action.file && action.tabType === 'editor') {
        const content = String(await EditorReadFile(action.file) || '');
        const fileName = action.file.split(/[/\\]/).pop() || i18n.t('editor.prompts.file');
        const title = action.title || fileName;
        const tabId = await wsStore.addTab('editor', title, { filePath: action.file });
        useEditorStore.getState().createDocument({ id: tabId, title, markdown: content, filePath: action.file });
      } else if (action.tabType === 'terminal') {
        const tabId = await wsStore.addTab('terminal', action.title || i18n.t('terminal.pageTitle'));
        if (action.cmd) {
          const waitForSessionId = (): Promise<string> => new Promise((resolve) => {
            const check = () => {
              const tab = (useWorkspaceStore.getState().workspace?.tabs || []).find(
                (t) => t.id === tabId,
              );
              const sid = tab?.state?.sessionId as string | undefined;
              if (sid) { resolve(sid); return; }
              setTimeout(check, 50);
            };
            check();
          });
          const sessionId = await waitForSessionId();
          await RunTerminalCommand(sessionId, action.cmd);
        }
      } else {
        await wsStore.addTab(action.tabType, action.title || defaultTitleForNewTab(action.tabType));
      }
      deps.navigate('/');
      announce(t('deepLink.announcedNewTab', { type: action.tabType }));
      break;
    }
  }
}

/**
 * Abre um link associado a uma task.
 * - Deep links internos (assistente://) são executados via parseDeepLink + executeDeepLink.
 * - URLs externas (http/https) são abertas no navegador do sistema.
 */
export function openTaskLink(url: string, deps: DeepLinkDeps): void {
  if (url.startsWith('assistente://')) {
    const action = parseDeepLink(url);
    if (action) {
      void executeDeepLink(action, deps);
    }
  } else if (url.startsWith('http://') || url.startsWith('https://')) {
    BrowserOpenURL(url);
  }
}
