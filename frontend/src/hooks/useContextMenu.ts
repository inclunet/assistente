import { useState, useCallback, useEffect } from 'react';
import { Message } from '../store/chatStore';
import { MenuItem } from '../components/ui/ContextMenu';
import { getMessageMenuItems, MenuItemsOptions } from '../lib/messageMenuItems';
import { ttsService } from '../services/tts';
import { stripMarkdown } from '../lib/stripMarkdown';

export interface UseContextMenuResult {
  menuVisible: boolean;
  menuPosition: { x: number; y: number };
  menuItems: MenuItem[];
  showMenu: (event: React.MouseEvent, message: Message, isUser: boolean) => void;
  hideMenu: () => void;
}

export function useContextMenu(options: MenuItemsOptions): UseContextMenuResult {
  const [menuVisible, setMenuVisible] = useState(false);
  const [menuPosition, setMenuPosition] = useState({ x: 0, y: 0 });
  const [menuItems, setMenuItems] = useState<MenuItem[]>([]);

  const showMenu = useCallback(
    (event: React.MouseEvent, message: Message, isUser: boolean) => {
      event.preventDefault();
      
      const items = getMessageMenuItems(message, {
        ...options,
        isUser,
        onAnnounce: options.onAnnounce,
      });

      setMenuItems(items);
      setMenuPosition({ x: event.clientX, y: event.clientY });
      setMenuVisible(true);
    },
    [options]
  );

  const hideMenu = useCallback(() => {
    setMenuVisible(false);
  }, []);

  // Fecha o menu quando clicar fora (já tratado no ContextMenu, mas como fallback)
  useEffect(() => {
    if (menuVisible) {
      const handleClick = () => hideMenu();
      document.addEventListener('click', handleClick);
      return () => document.removeEventListener('click', handleClick);
    }
  }, [menuVisible, hideMenu]);

  return {
    menuVisible,
    menuPosition,
    menuItems,
    showMenu,
    hideMenu,
  };
}

export interface UseMessageActionsOptions {
  onAnnounce?: (message: string) => void;
}

export function useMessageActions(options: UseMessageActionsOptions = {}) {
  const { onAnnounce } = options;

  const copyMessage = useCallback(
    async (message: Message, asMarkdown: boolean) => {
      try {
        const text = asMarkdown ? message.content : stripMarkdown(message.content || '');
        await navigator.clipboard.writeText(text);
        onAnnounce?.('Mensagem copiada.');
      } catch (err) {
        console.error('Erro ao copiar:', err);
        onAnnounce?.('Erro ao copiar mensagem.');
      }
    },
    [onAnnounce]
  );

  const speakMessage = useCallback(
    (message: Message) => {
      if (!message.content) return;
      const text = stripMarkdown(message.content);
      ttsService.speak(text);
    },
    []
  );

  return {
    copyMessage,
    speakMessage,
  };
}
