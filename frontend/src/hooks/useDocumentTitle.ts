/**
 * useDocumentTitle - Hook para gerenciar o título do documento e da janela
 *
 * Atualiza o document.title e o título da janela do Wails
 * baseado na página atual e no título da aba ativa do workspace.
 */

import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { WindowSetTitle } from '@wailsjs/runtime/runtime';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useChatStore } from '../store/chatStore';

const DEFAULT_TITLE_RE = /^nova\s+conversa$/i;

const ROUTE_I18N_KEYS: Record<string, string> = {
  '/allowlists': 'menu.allowlists',
  '/skills': 'menu.skills',
  '/mcp': 'menu.mcp',
  '/channels': 'menu.channels',
  '/credentials': 'menu.credentials',
  '/providers': 'menu.providers',
  '/settings': 'menu.restoreDefaults',
  '/profiles': 'menu.profiles',
  '/history': 'menu.history',
  '/help': 'menu.help',
  '/about': 'menu.about',
  '/update': 'update.pageTitle',
};

export function useDocumentTitle(): void {
  const { t } = useTranslation();
  const location = useLocation();
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const conversationTitle = useChatStore((s) => s.activeConversation?.title);

  useEffect(() => {
    const pathname = location.pathname;
    const appName = t('menu.appTitle');
    let title: string;

    if (pathname === '/' || pathname === '') {
      // Workspace route: usa título da aba ativa + chatStore como fallback
      const tabTitle = activeTab?.title;
      const effectiveTitle = conversationTitle
        && activeTab?.type === 'chat'
        && !DEFAULT_TITLE_RE.test(conversationTitle)
        ? conversationTitle
        : tabTitle;

      const isDefault = !effectiveTitle || DEFAULT_TITLE_RE.test(effectiveTitle);
      title = isDefault
        ? `${t('chat.newConversation')} — ${appName}`
        : `${effectiveTitle} — ${appName}`;
    } else {
      const i18nKey = ROUTE_I18N_KEYS[pathname];
      const pageTitle = i18nKey ? t(i18nKey) : pathname.slice(1);
      title = `${pageTitle} — ${appName}`;
    }

    document.title = title;

    try {
      WindowSetTitle(title);
    } catch {
      // Ignora erro se não estiver no contexto Wails (ex: dev mode no browser)
    }
  }, [location.pathname, activeTab?.title, activeTab?.type, conversationTitle, t]);
}

export default useDocumentTitle;
