import React, { useEffect, useCallback, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useChatStore } from '../../store/chatStore';
import { ClearConversation } from '@wailsjs/go/main/App';
import { HistoryPicker, HistoryPickerRef } from '../pickers';
import { ProfilePicker, ProfilePickerRef } from '../pickers/ProfilePicker';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
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

  const historyPickerRef = useRef<HistoryPickerRef>(null);
  const profilePickerRef = useRef<ProfilePickerRef>(null);

  // Estado do modal de tokens
  const [isTokenModalOpen, setIsTokenModalOpen] = useState(false);

  const {
    menu: contextMenu,
    openAtPoint: openContextMenu,
    closeMenu: closeContextMenu,
    onSelectItem: onSelectContextMenuItem,
  } = useAnchoredContextMenu();

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

  const handleProfileContextMenu = useCallback((e: React.MouseEvent<HTMLElement>) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, 'Menu de opções do perfil', getProfileMenuItems(), e.currentTarget);
  }, [openContextMenu, getProfileMenuItems]);

  const handleProfileKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, 'Menu de opções do perfil', getProfileMenuItems(), e.currentTarget);
    }
  }, [openContextMenu, getProfileMenuItems]);

  const focusInput = useCallback(() => {
    setTimeout(() => {
      inputRef?.current?.focus();
    }, 100);
  }, [inputRef]);

  const handleNewConversation = useCallback(() => {
    if (onNewConversation) {
      onNewConversation();
    } else {
      // conversa nova "de verdade": limpa conversationId e mensagens na aba ativa
      void loadConversationInActiveTab(0, 'Nova Conversa');
    }
    focusInput();
  }, [focusInput, loadConversationInActiveTab, onNewConversation]);

  const handleClearConversation = useCallback(async () => {
    try {
      const tab = getActiveTab();

      if (tab?.conversationId) {
        await ClearConversation(tab.conversationId);
        await loadConversationInActiveTab(tab.conversationId, tab.title || 'Conversa');
      } else {
        clearActiveTab();
      }

      announce('Conversa limpa');
    } catch (error) {
      console.error('[ChatToolbar] Erro ao limpar conversa:', error);
      announce('Erro ao limpar conversa');
    }
    focusInput();
  }, [announce, clearActiveTab, focusInput, getActiveTab, loadConversationInActiveTab]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        handleNewConversation();
      }
      else if (e.ctrlKey && e.key === 'l') {
        e.preventDefault();
        void handleClearConversation();
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
  }, [activeTab?.conversationId, announce, handleClearConversation, handleNewConversation]);

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
        isLoading={isLoading}
        left={
          <h2 className="chat-toolbar__title" id="chat-heading">
            {conversationTitle}
          </h2>
        }
        right={
          <>
            <ToolbarButton
              label="Nova"
              icon="➕"
              shortcut="Ctrl+N"
              onClick={handleNewConversation}
              disabled={isLoading}
            />

            <ToolbarButton
              label="Limpar"
              icon="🧹"
              shortcut="Ctrl+L"
              variant="danger"
              onClick={() => void handleClearConversation()}
              disabled={isLoading}
            />

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

            <ToolbarSeparator />

            <TokenStatsButton
              conversationId={activeTab?.conversationId}
              onOpenModal={() => setIsTokenModalOpen(true)}
            />

            <ToolbarSeparator />

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

      <Menu
        visible={contextMenu.visible}
        x={contextMenu.x}
        y={contextMenu.y}
        items={contextMenu.items}
        ariaLabel={contextMenu.ariaLabel}
        onClose={closeContextMenu}
        onSelect={onSelectContextMenuItem}
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
