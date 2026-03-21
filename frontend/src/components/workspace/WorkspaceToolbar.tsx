import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useWorkspaceStore, type TabType } from '../../store/workspaceStore';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { ProfilePicker } from '../pickers/ProfilePicker';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './WorkspaceToolbar.css';

const TAB_TYPE_OPTIONS: { type: TabType; icon: string; labelKey: string; chordKey: string }[] = [
  { type: 'chat', icon: '💬', labelKey: 'workspace.newChat', chordKey: 'C' },
  { type: 'editor', icon: '📝', labelKey: 'workspace.newEditor', chordKey: 'E' },
  { type: 'terminal', icon: '>_', labelKey: 'workspace.newTerminal', chordKey: 'R' },
  { type: 'tasklist', icon: '✅', labelKey: 'workspace.newTasklist', chordKey: 'T' },
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
  const { workspace, workspaces, addTab, switchWorkspace, createWorkspace, renameWorkspace, setProfile } = useWorkspaceStore();

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);

  const pickerButtonRef = useRef<HTMLButtonElement>(null);
  const newTabButtonRef = useRef<HTMLButtonElement>(null);

  // --- Workspace picker menu ---
  const {
    menu: pickerMenu,
    openForTrigger: openPicker,
    closeMenu: closePicker,
    onSelectItem: onPickerSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => pickerButtonRef.current?.focus(),
    onAfterDismiss: () => pickerButtonRef.current?.focus(),
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

  const pickerItems = useMemo((): MenuItem[] => {
    const items: MenuItem[] = workspaces.map((ws) => ({
      id: `ws-${ws.id}`,
      label: ws.name,
      icon: ws.is_active ? '●' : ' ',
      shortcut: `${ws.tab_count} ${ws.tab_count === 1 ? t('workspace.tabSingular', 'aba') : t('workspace.tabPlural', 'abas')}`,
      checked: ws.is_active,
      action: () => {
        if (!ws.is_active) void switchWorkspace(ws.id);
      },
    }));

    items.push({ id: 'sep', separator: true });
    items.push({
      id: 'new-workspace',
      label: t('workspace.newWorkspace'),
      icon: '➕',
      shortcut: 'Ctrl+Shift+N',
      action: () => {
        const name = `Workspace ${workspaces.length + 1}`;
        void createWorkspace(name);
        announce(`${t('workspace.created')}: ${name}`);
      },
    });

    items.push({ id: 'sep-export', separator: true });
    items.push({
      id: 'export-workspace',
      label: t('workspace.export', 'Exportar workspace'),
      icon: '📤',
      action: handleExportWorkspace,
    });
    items.push({
      id: 'import-workspace',
      label: t('workspace.import', 'Importar workspace'),
      icon: '📥',
      action: handleImportWorkspace,
    });

    return items;
  }, [workspaces, switchWorkspace, createWorkspace, announce, t, handleExportWorkspace, handleImportWorkspace]);

  const handleOpenPicker = useCallback(() => {
    if (pickerMenu.visible) {
      closePicker();
      return;
    }
    if (pickerButtonRef.current) {
      openPicker(pickerButtonRef.current, t('workspace.workspaceList'), pickerItems);
    }
  }, [pickerMenu.visible, closePicker, openPicker, pickerItems, t]);

  // --- New tab menu ---
  const {
    menu: newTabMenu,
    openForTrigger: openNewTab,
    closeMenu: closeNewTab,
    onSelectItem: onNewTabSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => newTabButtonRef.current?.focus(),
    onAfterDismiss: () => newTabButtonRef.current?.focus(),
  });

  const newTabItems = useMemo((): MenuItem[] =>
    TAB_TYPE_OPTIONS.map(({ type, icon, labelKey, chordKey }) => ({
      id: `tab-${type}`,
      label: t(labelKey),
      icon,
      shortcut: chordKey,
      action: () => {
        void addTab(type, '', TAB_TYPE_DEFAULTS[type]);
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

  // Ctrl+N dispara esse evento para abrir o menu visualmente
  useEffect(() => {
    const handleEvent = () => {
      if (newTabButtonRef.current && !newTabMenu.visible) {
        openNewTab(newTabButtonRef.current, t('workspace.newTabMenu'), newTabItems);
      }
    };
    window.addEventListener('workspace:open-new-tab-menu', handleEvent);
    return () => window.removeEventListener('workspace:open-new-tab-menu', handleEvent);
  }, [openNewTab, newTabItems, newTabMenu.visible, t]);

  // --- Rename ---
  useEffect(() => {
    if (isRenaming) {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    }
  }, [isRenaming]);

  const handleStartRename = useCallback(() => {
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
      pickerButtonRef.current?.focus();
    }
  }, [handleConfirmRename]);

  // --- Profile ---
  const handleProfileChange = useCallback(async (slug: string) => {
    await setProfile(slug);
    announce(`${t('workspace.profileChanged', 'Perfil do workspace alterado')}: ${slug}`);
  }, [setProfile, announce, t]);

  return (
    <>
      <Toolbar
        ariaLabel={t('workspace.toolbarLabel')}
        className="workspace-toolbar"
        left={
          isRenaming ? (
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
              ref={pickerButtonRef}
              label={workspace?.name || 'Workspace'}
              icon="📂"
              endIcon="▾"
              onClick={handleOpenPicker}
              onDoubleClick={handleStartRename}
              aria-expanded={pickerMenu.visible}
              aria-haspopup="menu"
            />
          )
        }
        right={
          <>
            <ToolbarButton
              ref={newTabButtonRef}
              label={t('workspace.newTab')}
              icon="➕"
              shortcut="Ctrl+N"
              onClick={handleOpenNewTab}
              aria-expanded={newTabMenu.visible}
              aria-haspopup="menu"
            />

            <ToolbarSeparator />

            <ProfilePicker
              value={workspace?.profile || ''}
              variant="toolbar"
              label={t('workspace.profileLabel', 'Perfil do workspace')}
              icon="🎭"
              onChange={handleProfileChange}
              onAnnounce={announce}
            />
          </>
        }
      />

      <Menu
        items={pickerMenu.items}
        x={pickerMenu.x}
        y={pickerMenu.y}
        visible={pickerMenu.visible}
        ariaLabel={pickerMenu.ariaLabel || t('workspace.workspaceList')}
        onClose={closePicker}
        onSelect={onPickerSelect}
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
    </>
  );
}
