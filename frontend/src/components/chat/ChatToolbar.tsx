import React, { useEffect, useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useChatStore } from '../../store/chatStore';
import { useNavigationStore } from '../../store/navigationStore';
import { ClearConversation, GetActiveProfileSlug } from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { HistoryPicker, HistoryPickerRef } from '../pickers';
import { ProfilePicker, ProfilePickerRef } from '../pickers/ProfilePicker';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { TokenStatsButton } from './TokenStatsButton';
import { TokenStatsModal } from './TokenStatsModal';
import { ResourcesMenu } from './ResourcesMenu';
import './ChatToolbar.css';

export interface ChatToolbarProps {
  onNewConversation?: () => void;
  inputRef?: React.RefObject<HTMLTextAreaElement>;
}

export const ChatToolbar: React.FC<ChatToolbarProps> = ({
  onNewConversation,
  inputRef,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { getActiveTab, clearActiveTab, isLoading, startNewConversationInActiveTab, loadConversationInActiveTab } = useChatStore();
  const { announce } = useAnnouncer();
  const activeTab = getActiveTab();
  const conversationTitle = activeTab?.title || t('chat.newConversation');

  const historyPickerRef = useRef<HistoryPickerRef>(null);
  const profilePickerRef = useRef<ProfilePickerRef>(null);

  const [isTokenModalOpen, setIsTokenModalOpen] = useState(false);
  const [activeProfileSlug, setActiveProfileSlug] = useState<string>('padrao');

  useEffect(() => {
    GetActiveProfileSlug().then((slug) => setActiveProfileSlug(slug || 'padrao'));
    const unsub = EventsOn('profile:changed', (data: { slug: string }) => {
      setActiveProfileSlug(data.slug || 'padrao');
    });
    return unsub;
  }, []);

  const {
    menu: contextMenu,
    openAtPoint: openContextMenu,
    closeMenu: closeContextMenu,
    onSelectItem: onSelectContextMenuItem,
  } = useAnchoredContextMenu();

  const getProfileMenuItems = useCallback((): MenuItem[] => [
    {
      id: 'edit-active-profile',
      label: t('chat.editActiveProfile'),
      icon: '✏️',
      action: () => {
        useNavigationStore.getState().requestResourceEdit('profiles', activeProfileSlug, 'edit');
        navigate('/profiles');
      },
    },
    {
      id: 'manage-profiles',
      label: t('chat.manageProfiles'),
      icon: '⚙️',
      action: () => {
        navigate('/profiles');
      },
    },
  ], [navigate, t, activeProfileSlug]);

  const handleProfileContextMenu = useCallback((e: React.MouseEvent<HTMLElement>) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, t('chat.profileMenuLabel'), getProfileMenuItems(), e.currentTarget);
  }, [openContextMenu, getProfileMenuItems]);

  const handleProfileKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, t('chat.profileMenuLabel'), getProfileMenuItems(), e.currentTarget);
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
      void startNewConversationInActiveTab(t('chat.newConversation'));
    }
    focusInput();
  }, [focusInput, startNewConversationInActiveTab, onNewConversation, t]);

  const handleClearConversation = useCallback(async () => {
    try {
      const tab = getActiveTab();

      if (tab?.conversationId) {
        await ClearConversation(tab.conversationId);
        await loadConversationInActiveTab(tab.conversationId, tab.title || t('chat.conversation'));
      } else {
        clearActiveTab();
      }

      announce(t('chat.conversationCleared'));
    } catch (error) {
      console.error('[ChatToolbar] Erro ao limpar conversa:', error);
      announce(t('chat.clearError'));
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
        const historyPicker = document.querySelector(`[aria-label*="${t('chat.historyLabel')}"]`) as HTMLElement;
        historyPicker?.click();
      }
      else if (e.ctrlKey && e.key === 't') {
        e.preventDefault();
        if (activeTab?.conversationId) {
          setIsTokenModalOpen(true);
          announce(t('chat.tokenStatsOpened'));
        }
      }
      else if (e.ctrlKey && e.key === 'p') {
        e.preventDefault();
        const profilePicker = document.querySelector(`[aria-label*="${t('chat.profileLabel')}"]`) as HTMLElement;
        profilePicker?.click();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeTab?.conversationId, announce, handleClearConversation, handleNewConversation]);

  const handleProfileChange = useCallback((_slug: string) => {
    focusInput();
  }, [focusInput]);

  const handleHistoryChange = async (conversationId: number, conversation: { title?: string }) => {
    try {
      await loadConversationInActiveTab(conversationId, conversation.title || t('chat.conversationLoaded'));
      announce(`${t('chat.conversationLoaded')}: ${conversation.title || t('chat.conversationLoaded')}`);
    } catch (error) {
      console.error('[ChatToolbar] Erro ao carregar conversa:', error);
      announce(t('chat.loadError'));
    }
    focusInput();
  };

  return (
    <>
      <Toolbar
        ariaLabel={t('chat.toolbarLabel')}
        isLoading={isLoading}
        left={
          <h2 className="chat-toolbar__title" id="chat-heading">
            {conversationTitle}
          </h2>
        }
        right={
          <>
            <ToolbarButton
              label={t('chat.newBtn')}
              icon="➕"
              shortcut="Ctrl+N"
              onClick={handleNewConversation}
              disabled={isLoading}
            />

            <ToolbarButton
              label={t('chat.clearBtn')}
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
                label={t('chat.historyBtn')}
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

            <ResourcesMenu
              conversationId={activeTab?.conversationId}
              disabled={isLoading}
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
                label={t('chat.profileBtn')}
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
