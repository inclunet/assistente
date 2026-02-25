import React, { useEffect, useCallback, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useChatStore } from '../../store/chatStore';
import { HistoryPicker, HistoryPickerRef } from '../pickers';
import { ProfilePicker, ProfilePickerRef } from '../pickers/ProfilePicker';
import { Toolbar } from '../ui/Toolbar';
import { ContextMenu, MenuItem } from '../ui/ContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { TokenStatsButton } from './TokenStatsButton';
import { TokenStatsModal } from './TokenStatsModal';
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

  // Debug: log activeTab
  useEffect(() => {
    console.log('[ChatToolbar] activeTab mudou:', {
      id: activeTab?.id,
      conversationId: activeTab?.conversationId,
      title: activeTab?.title,
      hasConversationId: !!activeTab?.conversationId
    });
  }, [activeTab]);

  // Refs para os pickers
  const historyPickerRef = useRef<HistoryPickerRef>(null);
  const profilePickerRef = useRef<ProfilePickerRef>(null);

  // Estado do modal de tokens
  const [isTokenModalOpen, setIsTokenModalOpen] = useState(false);

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

  const handleProfileContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, getProfileMenuItems(), 'Menu de opções do perfil');
  }, [openContextMenu, getProfileMenuItems]);

  const handleProfileKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, getProfileMenuItems(), 'Menu de opções do perfil');
    }
  }, [openContextMenu, getProfileMenuItems]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        handleNewConversation();
      }
      else if (e.ctrlKey && e.key === 'h') {
        e.preventDefault();
        const historyPicker = document.querySelector('[aria-label*="Histórico"]') as HTMLElement;
        historyPicker?.click();
      }
      else if (e.ctrlKey && e.key === 't') {
        e.preventDefault();
        if (activeTab?.conversationId) {
          setIsTokenModalOpen(true);
          announce('Modal de estatísticas de tokens aberto');
        }
      }
      else if (e.ctrlKey && e.key === 'p') {
        e.preventDefault();
        const profilePicker = document.querySelector('[aria-label*="Perfil"]') as HTMLElement;
        profilePicker?.click();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeTab?.conversationId, announce]);

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
                label="Histórico, Ctrl+H"
                maxWidth="200px"
                onAnnounce={announce}
                disabled={isLoading}
              />
            </div>

            <div className="toolbar__separator" aria-hidden="true"></div>

            <TokenStatsButton
              conversationId={activeTab?.conversationId}
              onOpenModal={() => setIsTokenModalOpen(true)}
            />

            <div className="toolbar__separator" aria-hidden="true"></div>

            <div
              onContextMenu={handleProfileContextMenu}
              onKeyDown={handleProfileKeyDown}
            >
              <ProfilePicker
                ref={profilePickerRef}
                onChange={handleProfileChange}
                variant="toolbar"
                label="Perfil, Ctrl+P"
                icon="💬"
                maxWidth="180px"
                onAnnounce={announce}
              />
            </div>
          </>
        }
      />

      <ContextMenu
        visible={contextMenu.visible}
        x={contextMenu.x}
        y={contextMenu.y}
        items={contextMenu.items}
        ariaLabel={contextMenu.ariaLabel}
        onClose={closeContextMenu}
      />

      {activeTab?.conversationId && (
        <TokenStatsModal
          conversationId={activeTab.conversationId}
          isOpen={isTokenModalOpen}
          onClose={() => setIsTokenModalOpen(false)}
        />
      )}
    </>
  );
};
