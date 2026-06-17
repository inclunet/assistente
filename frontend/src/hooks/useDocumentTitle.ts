/**
 * useDocumentTitle - Hook para gerenciar o título do documento e da janela
 *
 * Usa o título da aba ativa do workspace como fonte única de verdade.
 * Cada bridge (chat, editor, tasklist) é responsável por manter
 * o título da respectiva aba atualizado.
 */

import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { WindowSetTitle } from '@wailsjs/runtime/runtime';
import { useWorkspaceStore } from '../store/workspaceStore';
import i18n from '../lib/i18n';

const ROUTE_I18N_KEYS: Record<string, string> = {
  '/settings': 'menu.settings',
  '/profiles': 'menu.profiles',
  '/history': 'menu.history',
  '/memories': 'menu.memories',
  '/help': 'menu.help',
  '/about': 'menu.about',
  '/update': 'update.pageTitle',
  '/jobs': 'menu.jobs',
};

export function useDocumentTitle(): void {
  const { t } = useTranslation();
  const location = useLocation();
  const activeTabTitle = useWorkspaceStore((s) => s.getActiveTab()?.title);

  useEffect(() => {
    const pathname = location.pathname;
    const appName = t('menu.appTitle');
    let title: string;

    if (pathname === '/' || pathname === '') {
      const defaultChatTitle = i18n.t('chat.newConversation').toLowerCase();
      const isDefault =
        !activeTabTitle ||
        activeTabTitle.toLowerCase() === defaultChatTitle;
      title = isDefault
        ? `${t('chat.newConversation')} — ${appName}`
        : `${activeTabTitle} — ${appName}`;
    } else {
      const resolvedPath = pathname.startsWith('/settings') ? '/settings' : pathname;
      const i18nKey = ROUTE_I18N_KEYS[resolvedPath];
      const pageTitle = i18nKey ? t(i18nKey) : pathname.slice(1);
      title = `${pageTitle} — ${appName}`;
    }

    document.title = title;

    try {
      WindowSetTitle(title);
    } catch {
      // Ignora erro se não estiver no contexto Wails (ex: dev mode no browser)
    }
  }, [location.pathname, activeTabTitle, t]);
}

export default useDocumentTitle;
