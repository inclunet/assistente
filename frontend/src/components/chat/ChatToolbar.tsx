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
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { TokenStatsButton } from './TokenStatsButton';
import { TokenStatsModal } from './TokenStatsModal';
import './ChatToolbar.css';

export interface ChatToolbarProps {
  inputRef?: React.RefObject<HTMLTextAreaElement>;
}

export const ChatToolbar: React.FC<ChatToolbarProps> = ({
  inputRef,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { getActiveConversation, clearMessages, isLoading, loadConversation } = useChatStore();
  const { announce } = useAnnouncer();
  const activeConversation = getActiveConversation();
  const conversationTitle = activeConversation?.title || t('chat.newConversation');

  const wsActiveTab = useWorkspaceStore((s) => s.getActiveTab());
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);

  const tabProfileSlug = wsActiveTab?.profileOverride?.slug as string | undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';

  const historyPickerRef = useRef<HistoryPickerRef>(null);
  const profilePickerRef = useRef<ProfilePickerRef>(null);
  const historyContainerRef = useRef<HTMLDivElement>(null);
  const profileContainerRef = useRef<HTMLDivElement>(null);

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

  const handleClearConversation = useCallback(async () => {
    try {
      const conv = getActiveConversation();

      if (conv?.id) {
        await ClearConversation(conv.id);
        await loadConversation(conv.id);
      } else {
        clearMessages();
      }

      announce(t('chat.conversationCleared'));
    } catch (error) {
      console.error('[ChatToolbar] Erro ao limpar conversa:', error);
      announce(t('chat.clearError'));
    }
    focusInput();
  }, [announce, clearMessages, focusInput, getActiveConversation, loadConversation]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'l') {
        e.preventDefault();
        void handleClearConversation();
      }
      else if (e.ctrlKey && e.key === 'h') {
        e.preventDefault();
        const btn = historyContainerRef.current?.querySelector('button.picker-button') as HTMLElement;
        btn?.click();
      }
      else if (e.ctrlKey && e.key === 'p') {
        e.preventDefault();
        const btn = profileContainerRef.current?.querySelector('button.picker-button') as HTMLElement;
        btn?.click();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleClearConversation]);

  const handleProfileChange = useCallback((slug: string) => {
    if (wsActiveTab) {
      void updateWsTab(wsActiveTab.id, {
        profile_override: { slug },
      });
    }
    focusInput();
  }, [focusInput, wsActiveTab, updateWsTab]);

  const handleHistoryChange = async (conversationId: number, conversation: { title?: string }) => {
    try {
      await loadConversation(conversationId);
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
              label={t('chat.clearBtn')}
              icon="🧹"
              shortcut="Ctrl+L"
              title={t('chat.clearDescription')}
              aria-label={t('chat.clearBtn')}
              variant="danger"
              onClick={() => void handleClearConversation()}
              disabled={isLoading}
            />

            <div ref={historyContainerRef}>
              <HistoryPicker
                ref={historyPickerRef}
                value={activeConversation?.id}
                onChange={handleHistoryChange}
                label={t('chat.historyBtn')}
                description={t('chat.historyDescription')}
                maxWidth="200px"
                onAnnounce={announce}
                disabled={isLoading}
              />
            </div>

            <ToolbarSeparator />

            <TokenStatsButton
              conversationId={activeConversation?.id}
              onOpenModal={() => setIsTokenModalOpen(true)}
            />

            <ToolbarSeparator />

            <div
              ref={profileContainerRef}
              onContextMenu={handleProfileContextMenu}
              onKeyDown={handleProfileKeyDown}
            >
              <ProfilePicker
                ref={profilePickerRef}
                onChange={handleProfileChange}
                variant="toolbar"
                label={t('workspace.tabProfileLabel', 'Perfil')}
                description={t('workspace.tabProfileDescription')}
                icon="💬"
                maxWidth="180px"
                onAnnounce={announce}
                value={effectiveProfileSlug}
                onAfterSelect={() => restoreDefaultFocus()}
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

      {activeConversation?.id && (
        <TokenStatsModal
          conversationId={activeConversation.id}
          isOpen={isTokenModalOpen}
          onClose={() => setIsTokenModalOpen(false)}
        />
      )}
    </>
  );
};
