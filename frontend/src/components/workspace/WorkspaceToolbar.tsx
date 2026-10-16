import { logger } from '../../utils/logger';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  PlusOutlined,
  SettingOutlined,
  EditOutlined,
  ExportOutlined,
  ImportOutlined,
  FolderOutlined,
} from '@ant-design/icons';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useShallow } from 'zustand/shallow';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { ProfilePicker } from '../pickers/ProfilePicker';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { isModalOpen } from '../ui/Modal';
import { useUIStore } from '../../store/uiStore';
import { useWorkspaceTabCreationMenu, type WorkspaceTabCreationMenuRequest } from '../../lib/workspaceTabCreationMenu';
import { useCommandSequencePrefixHint, useCommandShortcutHints } from '../../lib/commandShortcutHints';
import { WORKSPACE_TAB_CREATE_COMMAND_IDS } from '../../lib/commandContextualBackendExecution';
import './WorkspaceToolbar.css';

export function WorkspaceToolbar() {
  const { t } = useTranslation();
  const shortcutHint = useCommandShortcutHints();
  const { announce } = useAnnouncer();
  const addToast = useUIStore((s) => s.addToast);
  const { workspace, setProfile, renameWorkspace } = useWorkspaceStore(
    useShallow((s) => ({ workspace: s.workspace, setProfile: s.setProfile, renameWorkspace: s.renameWorkspace }))
  );
  const newTabShortcut = useCommandSequencePrefixHint(WORKSPACE_TAB_CREATE_COMMAND_IDS, workspace?.tabs.find(tab => tab.id === workspace.activeTabId)?.type);

  const newTabButtonRef = useRef<HTMLButtonElement>(null);
  const newTabSelectionInProgressRef = useRef(false);
  const wsMenuButtonRef = useRef<HTMLButtonElement>(null);
  const renameInputRef = useRef<HTMLInputElement>(null);
  const profileContainerRef = useRef<HTMLDivElement>(null);
  const tabCreationMenu = useWorkspaceTabCreationMenu();

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState('');

  // --- New tab menu ---
  const {
    menu: newTabMenu,
    openForTrigger: openNewTab,
    closeMenu: closeNewTab,
    onSelectItem: onNewTabSelect,
  } = useAnchoredContextMenu({
    restoreTriggerFocusOnSelect: false,
    restoreTriggerFocusOnDismiss: false,
  });

  // --- Workspace management menu ---
  const {
    menu: wsMenu,
    openForTrigger: openWsMenu,
    closeMenu: closeWsMenu,
    onSelectItem: onWsMenuSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => {
      if (tabCreationMenu) {
        const handled = tabCreationMenu.completeWorkspaceCreate(() => wsMenuButtonRef.current?.focus());
        if (handled) return;
      }
      requestAnimationFrame(() => restoreDefaultFocus());
    },
    onAfterDismiss: () => wsMenuButtonRef.current?.focus(),
  });

  const handleExportWorkspace = useCallback(async () => {
    try {
      const yaml = await useWorkspaceStore.getState().exportWorkspace();
      const blob = new Blob([yaml], { type: 'application/x-yaml' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `workspace-${workspace?.name?.replace(/\s+/g, '-').toLowerCase() || 'export'}.yaml`;
      a.click();
      URL.revokeObjectURL(url);
      announce(t('workspace.exported'));
    } catch (error) {
      logger.error('[WorkspaceToolbar] Export error:', error);
    }
  }, [workspace?.name, announce, t]);

  const handleImportWorkspace = useCallback(async () => {
    try {
      const input = document.createElement('input');
      input.type = 'file';
      input.accept = '.yaml,.yml';
      input.onchange = async () => {
        const file = input.files?.[0];
        if (!file) return;
        const text = await file.text();
        await useWorkspaceStore.getState().importWorkspace(text);
      };
      input.click();
    } catch (error) {
      logger.error('[WorkspaceToolbar] Import error:', error);
    }
  }, []);

  const startRename = useCallback(() => {
    if (!workspace) return;
    setIsRenaming(true);
    setRenameValue(workspace.name);
  }, [workspace]);

  const handleConfirmRename = useCallback(async () => {
    const trimmed = renameValue.trim();
    if (trimmed && trimmed !== workspace?.name) {
      await renameWorkspace(trimmed);
      announce(`${t('workspace.renamed')}: ${trimmed}`);
    }
    setIsRenaming(false);
  }, [renameValue, workspace?.name, renameWorkspace, announce, t]);

  const handleRenameKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleConfirmRename();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setIsRenaming(false);
      wsMenuButtonRef.current?.focus();
    }
  }, [handleConfirmRename]);

  useEffect(() => {
    if (isRenaming) {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    }
  }, [isRenaming]);

  const wsMenuItems = useMemo((): MenuItem[] => [
    {
      id: 'new-workspace',
      label: t('workspace.newWorkspace'),
      icon: <PlusOutlined />,
      shortcut: shortcutHint('workspace.create'),
      action: () => { tabCreationMenu?.requestWorkspaceCreate(); },
    },
    {
      id: 'rename-workspace',
      label: t('workspace.rename'),
      icon: <EditOutlined />,
      shortcut: 'F2',
      action: startRename,
    },
    { id: 'sep-1', separator: true },
    {
      id: 'export-workspace',
      label: t('workspace.export'),
      icon: <ExportOutlined />,
      action: handleExportWorkspace,
    },
    {
      id: 'import-workspace',
      label: t('workspace.import'),
      icon: <ImportOutlined />,
      action: handleImportWorkspace,
    },
    { id: 'sep-2', separator: true },
    {
      id: 'set-workdir',
      label: t('workspace.setWorkDir'),
      icon: <FolderOutlined />,
      disabled: true,
    },
  ], [tabCreationMenu, announce, t, startRename, handleExportWorkspace, handleImportWorkspace, shortcutHint]);

  const handleOpenWsMenu = useCallback(() => {
    if (wsMenu.visible) { closeWsMenu(); return; }
    if (wsMenuButtonRef.current) {
      openWsMenu(wsMenuButtonRef.current, t('workspace.workspaceOptions'), wsMenuItems);
    }
  }, [wsMenu.visible, closeWsMenu, openWsMenu, wsMenuItems, t]);

  const tabCreationMenuHost = useMemo(() => ({
    show: (request: WorkspaceTabCreationMenuRequest, onSelect: (commandID: string, intent: WorkspaceTabCreationMenuRequest['intent']) => void) => {
      if (!newTabButtonRef.current) return false;
      const items: MenuItem[] = request.items.map((item) => ({
        id: item.commandID,
        label: item.label,
        icon: item.icon,
        shortcut: item.shortcut,
        disabled: item.disabled,
        action: item.disabled ? undefined : () => {
          newTabSelectionInProgressRef.current = true;
          onSelect(item.commandID, request.intent);
        },
      }));
      openNewTab(newTabButtonRef.current, t('workspace.newTabMenu'), items);
      return true;
    },
    close: (options?: { restoreFocus: boolean }) => {
      closeNewTab();
      if (options?.restoreFocus) newTabButtonRef.current?.focus();
    },
    focusTrigger: () => newTabButtonRef.current?.focus(),
    isOpen: () => newTabMenu.visible,
  }), [closeNewTab, newTabMenu.visible, openNewTab, t]);

  useEffect(() => {
    if (!tabCreationMenu) return undefined;
    return tabCreationMenu.registerHost(tabCreationMenuHost);
  }, [tabCreationMenu, tabCreationMenuHost]);

  const handleNewTabMenuClose = useCallback(() => {
    if (newTabSelectionInProgressRef.current) {
      newTabSelectionInProgressRef.current = false;
      closeNewTab();
      return;
    }
    tabCreationMenu?.cancel('outside');
    closeNewTab();
  }, [closeNewTab, tabCreationMenu]);

  // --- Profile ---
  const handleProfileChange = useCallback(async (slug: string) => {
    try {
      await setProfile(slug);
      announce(`${t('workspace.profileChanged')}: ${slug}`);
    } catch (error) {
      logger.error('[WorkspaceToolbar] Erro ao trocar perfil do workspace:', error);
      addToast(
        t('chat.profileChangeError'),
        'error'
      );
    }
  }, [setProfile, announce, t, addToast]);

  // Ctrl+Shift+P opens workspace profile picker
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && e.key === 'P') {
        e.preventDefault();
        // Não age na UI de fundo enquanto um modal está aberto (ex.: painel de atalhos).
        if (isModalOpen()) return;
        const btn = profileContainerRef.current?.querySelector('button.picker-button') as HTMLElement;
        btn?.click();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  return (
    <>
      <Toolbar
        ariaLabel={t('workspace.toolbarLabel')}
        className="workspace-toolbar"
        left={
          <>
            <ToolbarButton
              ref={newTabButtonRef}
              label={t('workspace.newTab')}
              icon={<PlusOutlined />}
              shortcut={newTabShortcut}
              onClick={() => {
                tabCreationMenu?.requestOpen();
              }}
              aria-expanded={newTabMenu.visible}
            />

            <ToolbarSeparator />

            {isRenaming ? (
              <input
                ref={renameInputRef}
                className="workspace-toolbar__rename-input"
                value={renameValue}
                onChange={(e) => setRenameValue(e.target.value)}
                onKeyDown={handleRenameKeyDown}
                onBlur={() => void handleConfirmRename()}
                aria-label={t('workspace.renamePlaceholder')}
              />
            ) : (
              <ToolbarButton
                ref={wsMenuButtonRef}
                label={t('workspace.workspaceOptions')}
                icon={<SettingOutlined />}
                onClick={handleOpenWsMenu}
                aria-expanded={wsMenu.visible}
              />
            )}
          </>
        }
        right={
          <div ref={profileContainerRef}>
            <ProfilePicker
              value={workspace?.profile || ''}
              variant="toolbar"
              label={t('workspace.profileLabel')}
              description={t('workspace.profileDescription')}
              icon=""
              onChange={handleProfileChange}
              onAnnounce={announce}
              onAfterSelect={() => restoreDefaultFocus()}
            />
          </div>
        }
      />

      <div data-workspace-tab-creation-menu onKeyDownCapture={(event) => {
        if (event.key !== 'Tab') return;
        event.preventDefault();
        event.stopPropagation();
        tabCreationMenu?.cancel('escape');
      }}>
        <Menu
          items={newTabMenu.items}
          x={newTabMenu.x}
          y={newTabMenu.y}
          visible={newTabMenu.visible}
          ariaLabel={newTabMenu.ariaLabel || t('workspace.newTabMenu')}
          restoreFocusOnClose={false}
          onClose={handleNewTabMenuClose}
          onSelect={onNewTabSelect}
          onItemKeyDown={(event) => {
            if (event.key !== 'Escape') return false;
            event.preventDefault();
            tabCreationMenu?.cancel('escape');
            return true;
          }}
        />
      </div>

      <Menu
        items={wsMenu.items}
        x={wsMenu.x}
        y={wsMenu.y}
        visible={wsMenu.visible}
        ariaLabel={wsMenu.ariaLabel || t('workspace.workspaceOptions')}
        onClose={closeWsMenu}
        onSelect={onWsMenuSelect}
      />
    </>
  );
}
