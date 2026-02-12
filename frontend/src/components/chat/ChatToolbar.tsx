import React, { useEffect, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useChatStore } from '../../store/chatStore';
import { HistoryPicker, HistoryPickerRef } from '../pickers';
import { ProfilePicker, ProfilePickerRef } from '../pickers/ProfilePicker';
import { Toolbar } from '../ui/Toolbar';
import { ContextMenu, MenuItem } from '../ui/ContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './ChatToolbar.css';

export interface ChatToolbarProps {
  onNewConversation?: () => void;
  inputRef?: React.RefObject<HTMLTextAreaElement>;
}

export const ChatToolbar: React.FC<ChatToolbarProps> = ({
  onNewConversation,
  inputRef,
}) => {
  const navigate = useNavigate();
  const { getActiveTab, clearActiveTab, isLoading, loadConversationInActiveTab } = useChatStore();
  const { announce } = useAnnouncer();
  const activeTab = getActiveTab();
  const conversationTitle = activeTab?.title || 'Nova conversa';

  // Refs para os pickers
  const historyPickerRef = useRef<HistoryPickerRef>(null);
  const profilePickerRef = useRef<ProfilePickerRef>(null);

  // Estado do menu de contexto
  const [contextMenu, setContextMenu] = React.useState<{
    visible: boolean;
    x: number;
    y: number;
    items: MenuItem[];
    ariaLabel: string;
  }>({
    visible: false,
    x: 0,
    y: 0,
    items: [],
    ariaLabel: '',
  });

  const closeContextMenu = useCallback(() => {
    setContextMenu(prev => ({ ...prev, visible: false }));
  }, []);

  // Abre menu de contexto numa posição
  const openContextMenu = useCallback((
    x: number,
    y: number,
    items: MenuItem[],
    ariaLabel: string
  ) => {
    setContextMenu({
      visible: true,
      x,
      y,
      items,
      ariaLabel,
    });
  }, []);

  // Itens do menu de contexto do perfil
  const getProfileMenuItems = useCallback((): MenuItem[] => [
    {
      id: 'manage-profiles',
      label: 'Gerenciar perfis',
      icon: '⚙️',
      action: () => {
        navigate('/profiles');
      },
    },
  ], [navigate]);

  // Handler de menu de contexto para ProfilePicker (mouse)
  const handleProfileContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, getProfileMenuItems(), 'Menu de opções do perfil');
  }, [openContextMenu, getProfileMenuItems]);

  // Handler de teclado para ProfilePicker (Applications key ou Shift+F10)
  const handleProfileKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, getProfileMenuItems(), 'Menu de opções do perfil');
    }
  }, [openContextMenu, getProfileMenuItems]);

  // Atalhos de teclado globais
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ctrl+N: Nova conversa
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        handleNewConversation();
      }
      // Ctrl+H: Focar no picker de histórico
      else if (e.ctrlKey && e.key === 'h') {
        e.preventDefault();
        const historyPicker = document.querySelector('[aria-label*="Histórico"]') as HTMLElement;
        historyPicker?.click();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const focusInput = useCallback(() => {
    setTimeout(() => {
      inputRef?.current?.focus();
    }, 100);
  }, [inputRef]);

  const handleNewConversation = () => {
    if (onNewConversation) {
      onNewConversation();
    } else {
      clearActiveTab();
    }
    focusInput();
  };

  const handleProfileChange = useCallback((_slug: string) => {
    focusInput();
  }, [focusInput]);

  const handleHistoryChange = async (conversationId: number, conversation: any) => {
    try {
      await loadConversationInActiveTab(conversationId, conversation.title || 'Conversa carregada');
      announce(`Conversa carregada: ${conversation.title || 'Conversa carregada'}`);
    } catch (error) {
      console.error('[ChatToolbar] Erro ao carregar conversa:', error);
      announce('Erro ao carregar conversa');
    }
    focusInput();
  };

  return (
    <>
      <Toolbar
        ariaLabel="Ferramentas do chat. Use setas para navegar entre os botões"
        left={
          <h2 className="chat-toolbar__title" id="chat-heading">
            {conversationTitle}
          </h2>
        }
        right={
          <>
            <button
              className="toolbar__button"
              onClick={handleNewConversation}
              aria-label="Nova conversa, Ctrl+N"
              title="Nova conversa (Ctrl+N)"
              disabled={isLoading}
              tabIndex={0}
            >
              <span aria-hidden="true">➕</span>
              <span>Nova</span>
            </button>

            <div>
              <HistoryPicker
                ref={historyPickerRef}
                value={activeTab?.conversationId}
                onChange={handleHistoryChange}
                label="Histórico (Ctrl+H)"
                maxWidth="200px"
                onAnnounce={announce}
                disabled={isLoading}
              />
            </div>

            <div className="toolbar__separator" aria-hidden="true"></div>

            <div
              onContextMenu={handleProfileContextMenu}
              onKeyDown={handleProfileKeyDown}
            >
              <ProfilePicker
                ref={profilePickerRef}
                onChange={handleProfileChange}
                variant="toolbar"
                label="Perfil"
                icon="💬"
                maxWidth="180px"
                onAnnounce={announce}
              />
            </div>
          </>
        }
      />

      {/* Menu de contexto para pickers */}
      <ContextMenu
        visible={contextMenu.visible}
        x={contextMenu.x}
        y={contextMenu.y}
        items={contextMenu.items}
        ariaLabel={contextMenu.ariaLabel}
        onClose={closeContextMenu}
      />
    </>
  );
};
