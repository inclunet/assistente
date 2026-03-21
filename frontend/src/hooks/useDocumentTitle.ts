/**
 * useDocumentTitle - Hook para gerenciar o título do documento e da janela
 * 
 * Atualiza o document.title e o título da janela do Wails
 * baseado na página atual e nome da conversa.
 */

import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { WindowSetTitle } from '@wailsjs/runtime/runtime';
import { useChatStore } from '../store/chatStore';

const ROUTE_I18N_KEYS: Record<string, string> = {
  '/': 'menu.chat',
  '/terminal': 'menu.terminal',
  '/editor': 'menu.editor',
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
  const activeConversation = useChatStore((state) => state.activeConversation);

  useEffect(() => {
    const pathname = location.pathname;
    const appName = t('menu.appTitle');
    let title: string;

    if (pathname === '/' || pathname === '') {
      const conversationTitle = activeConversation?.title;
      const isNewConversation = !conversationTitle
        || conversationTitle === t('chat.newConversation')
        || conversationTitle.toLowerCase() === 'nova conversa';
      if (isNewConversation) {
        title = `${t('chat.newConversation')} — ${appName}`;
      } else {
        title = `${conversationTitle} — ${appName}`;
      }
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
  }, [location.pathname, activeConversation?.title, t]);
}

export default useDocumentTitle;
