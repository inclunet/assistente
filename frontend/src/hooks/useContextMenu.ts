import { useState, useCallback, useEffect, useRef } from 'react';
import { Message, useChatStore } from '../store/chatStore';
import { MenuItem } from '../components/menu';
import { getMessageMenuItems, MenuItemsOptions } from '../lib/messageMenuItems';
import { ttsService } from '../services/tts';
import { VoiceRole } from '../services/tts/index';
import { messageAudioService } from '../services/messageAudio';
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
  // Guarda referência ao elemento que abriu o menu para restaurar foco
  const triggerElementRef = useRef<HTMLElement | null>(null);

  const showMenu = useCallback(
    (event: React.MouseEvent, message: Message, isUser: boolean) => {
      event.preventDefault();
      
      // Guarda o elemento que abriu o menu (ou o target do evento)
      triggerElementRef.current = (event.currentTarget as HTMLElement) || (event.target as HTMLElement);
      
      // Verifica estado de expansão do reasoning no momento de mostrar o menu
      const reasoningExpanded = options.isReasoningExpanded 
        ? (typeof options.isReasoningExpanded === 'function' 
            ? options.isReasoningExpanded(message.id) 
            : options.isReasoningExpanded)
        : false;
      
      const items = getMessageMenuItems(message, {
        ...options,
        isUser,
        onAnnounce: options.onAnnounce,
        isReasoningExpanded: reasoningExpanded,
      });

      setMenuItems(items);
      setMenuPosition({ x: event.clientX, y: event.clientY });
      setMenuVisible(true);
    },
    [options]
  );

  const hideMenu = useCallback(() => {
    setMenuVisible(false);
    
    // Restaura foco ao elemento que abriu o menu (exceto se edição foi iniciada)
    setTimeout(() => {
      // Verifica se deve pular a restauração de foco (ex: edição iniciada)
      const shouldSkip = useChatStore.getState().consumeSkipFocusRestore();
      if (shouldSkip) {
        triggerElementRef.current = null;
        return;
      }
      
      if (triggerElementRef.current) {
        triggerElementRef.current.focus();
        triggerElementRef.current = null;
      }
    }, 10);
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
    async (message: Message) => {
      if (!message.content || !message.id) return;

      const role: VoiceRole = message.role === 'user' ? 'user' : 'assistant';

      if (!ttsService.hasVoiceConfig(role)) return;

      messageAudioService.stopCurrentAudio();
      ttsService.stop();

      const numericId = typeof message.id === 'string' ? parseInt(message.id, 10) : message.id;
      if (numericId && !isNaN(numericId)) {
        const volume = ttsService.getVolume();
        const played = await messageAudioService.speakMessage(numericId, volume);
        if (played) return;
      }

      // Fallback: WebSpeech/SAPI5
      const text = stripMarkdown(message.content);
      await ttsService.speakAsRole(text, role);
    },
    []
  );

  return {
    copyMessage,
    speakMessage,
  };
}
