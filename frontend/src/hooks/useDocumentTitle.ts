/**
 * useDocumentTitle - Hook para gerenciar o título do documento e da janela
 * 
 * Atualiza o document.title e o título da janela do Wails
 * baseado na página atual e nome da conversa.
 */

import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { WindowSetTitle } from '../../wailsjs/runtime/runtime';
import { useChatStore } from '../store/chatStore';

const PAGE_TITLES: Record<string, string> = {
  '/': 'Chat',
  '/settings': 'Configurações',
  '/history': 'Histórico',
  '/voice-profiles': 'Perfis de Voz',
  '/interaction-profiles': 'Perfis de Interação',
};

const APP_NAME = 'Assistente IA';

export function useDocumentTitle(): void {
  const location = useLocation();
  const activeTab = useChatStore((state) => {
    const tab = state.tabs.find(t => t.id === state.activeTabId);
    return tab;
  });

  useEffect(() => {
    const pathname = location.pathname;
    let title = APP_NAME;

    // Página de chat - usa o título da conversa ativa
    if (pathname === '/' || pathname === '') {
      if (activeTab?.title && activeTab.title !== 'Nova Conversa') {
        title = `${activeTab.title} - ${APP_NAME}`;
      } else {
        title = `Chat - ${APP_NAME}`;
      }
    } else {
      // Outras páginas - usa o nome da página
      const pageTitle = PAGE_TITLES[pathname];
      if (pageTitle) {
        title = `${pageTitle} - ${APP_NAME}`;
      }
    }

    // Atualiza título do documento (browser tab)
    document.title = title;
    
    // Atualiza título da janela do Wails (window title bar)
    try {
      WindowSetTitle(title);
    } catch (e) {
      // Ignora erro se não estiver no contexto Wails (ex: dev mode no browser)
      console.debug('[useDocumentTitle] WindowSetTitle not available:', e);
    }
  }, [location.pathname, activeTab?.title]);
}

export default useDocumentTitle;
