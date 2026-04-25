import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  PlusOutlined,
  SettingOutlined,
  MessageOutlined,
  FileTextOutlined,
  CodeOutlined,
  CheckSquareOutlined,
  EditOutlined,
  ExportOutlined,
  ImportOutlined,
  FolderOutlined,
} from '@ant-design/icons';
import { useWorkspaceStore, type TabType } from '../../store/workspaceStore';
import { useShallow } from 'zustand/shallow';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { ProfilePicker } from '../pickers/ProfilePicker';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import './WorkspaceToolbar.css';

const TAB_TYPE_OPTIONS: { type: TabType; icon: ReactNode; labelKey: string; chordKey: string }[] = [
  { type: 'chat', icon: <MessageOutlined />, labelKey: 'workspace.newChat', chordKey: 'C' },
  { type: 'editor', icon: <FileTextOutlined />, labelKey: 'workspace.newEditor', chordKey: 'E' },
  { type: 'terminal', icon: <CodeOutlined />, labelKey: 'workspace.newTerminal', chordKey: 'R' },
  { type: 'tasklist', icon: <CheckSquareOutlined />, labelKey: 'workspace.newTasklist', chordKey: 'T' },
];

const TAB_TYPE_DEFAULTS: Record<TabType, string> = {
  chat: 'Nova conversa',
  editor: 'Novo documento',
  terminal: 'Terminal',
  tasklist: 'Tarefas',
};

export function WorkspaceToolbar() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const { workspace, workspaces, addTab, setProfile, createWorkspace, renameWorkspace } = useWorkspaceStore(
    useShallow((s) => ({ workspace: s.workspace, workspaces: s.workspaces, addTab: s.addTab, setProfile: s.setProfile, createWorkspace: s.createWorkspace, renameWorkspace: s.renameWorkspace }))
  );

  const newTabButtonRef = useRef<HTMLButtonElement>(null);
  const wsMenuButtonRef = useRef<HTMLButtonElement>(null);
  const renameInputRef = useRef<HTMLInputElement>(null);
  const profileContainerRef = useRef<HTMLDivElement>(null);

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState('');

  // --- New tab menu ---
  const {
    menu: newTabMenu,
    openForTrigger: openNewTab,
    closeMenu: closeNewTab,
    onSelectItem: onNewTabSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => { requestAnimationFrame(() => restoreDefaultFocus()); },
    onAfterDismiss: () => newTabButtonRef.current?.focus(),
  });

  // --- Workspace management menu ---
  const {
    menu: wsMenu,
    openForTrigger: openWsMenu,
    closeMenu: closeWsMenu,
    onSelectItem: onWsMenuSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => { requestAnimationFrame(() => restoreDefaultFocus()); },
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
      announce(t('workspace.exported', 'Workspace exportado'));
    } catch (error) {
      console.error('[WorkspaceToolbar] Export error:', error);
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
      console.error('[WorkspaceToolbar] Import error:', error);
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
      announce(`${t('workspace.renamed', 'Workspace renomeado')}: ${trimmed}`);
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
      shortcut: 'Ctrl+Shift+N',
      action: () => {
        const name = `Workspace ${workspaces.length + 1}`;
        void createWorkspace(name);
        announce(`${t('workspace.created')}: ${name}`);
      },
    },
    {
      id: 'rename-workspace',
      label: t('workspace.rename', 'Renomear workspace'),
      icon: <EditOutlined />,
      shortcut: 'F2',
      action: startRename,
    },
    { id: 'sep-1', separator: true },
    {
      id: 'export-workspace',
      label: t('workspace.export', 'Exportar workspace'),
      icon: <ExportOutlined />,
      action: handleExportWorkspace,
    },
    {
      id: 'import-workspace',
      label: t('workspace.import', 'Importar workspace'),
      icon: <ImportOutlined />,
      action: handleImportWorkspace,
    },
    { id: 'sep-2', separator: true },
    {
      id: 'set-workdir',
      label: t('workspace.setWorkDir', 'Diretório de trabalho...'),
      icon: <FolderOutlined />,
      disabled: true,
    },
  ], [workspaces.length, createWorkspace, announce, t, startRename, handleExportWorkspace, handleImportWorkspace]);

  const handleOpenWsMenu = useCallback(() => {
    if (wsMenu.visible) { closeWsMenu(); return; }
    if (wsMenuButtonRef.current) {
      openWsMenu(wsMenuButtonRef.current, t('workspace.workspaceOptions', 'Opções do workspace'), wsMenuItems);
    }
  }, [wsMenu.visible, closeWsMenu, openWsMenu, wsMenuItems, t]);

  const newTabItems = useMemo((): MenuItem[] =>
    TAB_TYPE_OPTIONS.map(({ type, icon, labelKey, chordKey }) => ({
      id: `tab-${type}`,
      label: t(labelKey),
      icon,
      shortcut: chordKey,
      action: () => {
        void addTab(type, TAB_TYPE_DEFAULTS[type]);
        announce(`${t('workspace.tabCreated', 'Aba criada')}: ${t(labelKey)}`);
      },
    })),
  [addTab, announce, t]);

  const handleOpenNewTab = useCallback(() => {
    if (newTabMenu.visible) {
      closeNewTab();
      return;
    }
    if (newTabButtonRef.current) {
      openNewTab(newTabButtonRef.current, t('workspace.newTabMenu'), newTabItems);
    }
  }, [newTabMenu.visible, closeNewTab, openNewTab, newTabItems, t]);

  // Ctrl+N dispatches this event to visually open the menu
  useEffect(() => {
    const handleEvent = () => {
      if (newTabButtonRef.current && !newTabMenu.visible) {
        openNewTab(newTabButtonRef.current, t('workspace.newTabMenu'), newTabItems);
      }
    };
    window.addEventListener('workspace:open-new-tab-menu', handleEvent);
    return () => window.removeEventListener('workspace:open-new-tab-menu', handleEvent);
  }, [openNewTab, newTabItems, newTabMenu.visible, t]);

  // --- Profile ---
  const handleProfileChange = useCallback(async (slug: string) => {
    await setProfile(slug);
    announce(`${t('workspace.profileChanged', 'Perfil alterado')}: ${slug}`);
  }, [setProfile, announce, t]);

  // Ctrl+Shift+P opens workspace profile picker
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && e.key === 'P') {
        e.preventDefault();
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
              shortcut="Ctrl+N"
              onClick={handleOpenNewTab}
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
                label={t('workspace.workspaceOptions', 'Opções do workspace')}
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
              label={t('workspace.profileLabel', 'Perfil')}
              description={t('workspace.profileDescription')}
              icon=""
              onChange={handleProfileChange}
              onAnnounce={announce}
              onAfterSelect={() => restoreDefaultFocus()}
            />
          </div>
        }
      />

      <Menu
        items={newTabMenu.items}
        x={newTabMenu.x}
        y={newTabMenu.y}
        visible={newTabMenu.visible}
        ariaLabel={newTabMenu.ariaLabel || t('workspace.newTabMenu')}
        onClose={closeNewTab}
        onSelect={onNewTabSelect}
      />

      <Menu
        items={wsMenu.items}
        x={wsMenu.x}
        y={wsMenu.y}
        visible={wsMenu.visible}
        ariaLabel={wsMenu.ariaLabel || t('workspace.workspaceOptions', 'Opções do workspace')}
        onClose={closeWsMenu}
        onSelect={onWsMenuSelect}
      />
    </>
  );
}
