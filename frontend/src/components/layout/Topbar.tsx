import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { MenuButton, type MenuItem as MenuButtonItem, type MenuButtonRef } from './MenuButton';
import { Menu, type MenuItem } from '../menu';
import { useTheme, THEMES, type ThemeId } from '../../hooks/useTheme';
import { LANGUAGES, type LanguageId } from '../../lib/i18n';
import { useSettingsStore } from '../../store/settingsStore';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import './Topbar.css';

const PAGE_TITLE_KEYS: Record<string, string> = {
  '/history': 'menu.history',
  '/tasklists': 'menu.tasklists',
  '/profiles': 'menu.profiles',
  '/allowlists': 'menu.allowlists',
  '/skills': 'menu.skills',
  '/mcp': 'menu.mcp',
  '/channels': 'menu.channels',
  '/credentials': 'menu.credentials',
  '/providers': 'menu.providers',
  '/settings': 'menu.restoreDefaults',
  '/help': 'menu.help',
  '/about': 'menu.about',
  '/update': 'menu.about',
};

const ROUTE_IDS: Record<string, string> = {
  '/history': 'history',
  '/tasklists': 'tasklists',
  '/profiles': 'profiles',
  '/allowlists': 'allowlists',
  '/skills': 'skills',
  '/mcp': 'mcp',
  '/channels': 'channels',
  '/credentials': 'credentials',
  '/providers': 'providers',
  '/settings': 'settings',
  '/help': 'help',
  '/about': 'about',
  '/update': 'update',
};

export function Topbar() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { announce } = useAnnouncer();
  const { workspace, workspaces, switchWorkspace, createWorkspace, renameWorkspace } = useWorkspaceStore();
  const { theme: currentTheme, setTheme } = useTheme();
  const updateConfig = useSettingsStore((s) => s.updateConfig);
  const currentLang = i18n.language as LanguageId;

  const isWorkspaceRoute = pathname === '/' || pathname === '';
  const menuButtonRef = useRef<MenuButtonRef>(null);
  const pickerButtonRef = useRef<HTMLButtonElement>(null);
  const renameInputRef = useRef<HTMLInputElement>(null);

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState('');

  // --- Page title ---
  const pageTitle = isWorkspaceRoute
    ? (workspace?.name || t('menu.appTitle'))
    : t(PAGE_TITLE_KEYS[pathname] || 'menu.appTitle');

  // --- Workspace picker (left, workspace route) — only workspace list ---
  const {
    menu: pickerMenu,
    openForTrigger: openPicker,
    closeMenu: closePicker,
    onSelectItem: onPickerSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => requestAnimationFrame(() => restoreDefaultFocus()),
    onAfterDismiss: () => pickerButtonRef.current?.focus(),
  });

  // --- Context menu (right-click) on picker button ---
  const {
    menu: ctxMenu,
    openAtPoint: openCtx,
    closeMenu: closeCtx,
    onSelectItem: onCtxSelect,
  } = useAnchoredContextMenu({
    onAfterSelect: () => requestAnimationFrame(() => restoreDefaultFocus()),
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
      console.error('[Topbar] Export error:', error);
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
      console.error('[Topbar] Import error:', error);
    }
  }, []);

  const startRename = useCallback(() => {
    if (!workspace) return;
    setIsRenaming(true);
    setRenameValue(workspace.name);
  }, [workspace]);

  // Picker items: workspace list only
  const pickerItems = useMemo((): MenuItem[] => {
    return workspaces.map((ws) => ({
      id: `ws-${ws.id}`,
      label: ws.name,
      icon: ws.is_active ? '●' : ' ',
      shortcut: `${ws.tab_count} ${ws.tab_count === 1 ? t('workspace.tabSingular', 'aba') : t('workspace.tabPlural', 'abas')}`,
      checked: ws.is_active,
      action: () => { if (!ws.is_active) void switchWorkspace(ws.id); },
    }));
  }, [workspaces, switchWorkspace, t]);

  // Context menu items: workspace management
  const ctxMenuItems = useMemo((): MenuItem[] => [
    {
      id: 'new-workspace',
      label: t('workspace.newWorkspace'),
      icon: '➕',
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
      icon: '✏️',
      shortcut: 'F2',
      action: startRename,
    },
    { id: 'sep-1', separator: true },
    {
      id: 'export-workspace',
      label: t('workspace.export', 'Exportar workspace'),
      icon: '📤',
      action: handleExportWorkspace,
    },
    {
      id: 'import-workspace',
      label: t('workspace.import', 'Importar workspace'),
      icon: '📥',
      action: handleImportWorkspace,
    },
  ], [workspaces.length, createWorkspace, announce, t, startRename, handleExportWorkspace, handleImportWorkspace]);

  const handleOpenPicker = useCallback(() => {
    if (pickerMenu.visible) { closePicker(); return; }
    if (pickerButtonRef.current) {
      openPicker(pickerButtonRef.current, t('workspace.workspaceList'), pickerItems);
    }
  }, [pickerMenu.visible, closePicker, openPicker, pickerItems, t]);

  const handlePickerContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    openCtx(e.clientX, e.clientY, t('workspace.workspaceOptions', 'Opções do workspace'), ctxMenuItems);
  }, [openCtx, t, ctxMenuItems]);

  const handlePickerKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'F2') {
      e.preventDefault();
      startRename();
    }
  }, [startRename]);

  // --- Rename ---
  useEffect(() => {
    if (isRenaming) {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    }
  }, [isRenaming]);

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

  // --- Main menu items (right, always) ---
  const setLanguage = useCallback((id: LanguageId) => {
    i18n.changeLanguage(id);
    updateConfig({ language: id });
  }, [i18n, updateConfig]);

  const currentPage = ROUTE_IDS[pathname] || 'workspace';

  const mainMenuItems: MenuButtonItem[] = useMemo(() => [
    { id: 'history', label: t('menu.history'), icon: '📜', shortcut: 'Alt+H', onClick: () => navigate('/history') },
    { id: 'tasklists', label: t('menu.tasklists'), icon: '✓', onClick: () => navigate('/tasklists') },
    { id: 'profiles', label: t('menu.profiles'), icon: '🎭', shortcut: 'Alt+P', onClick: () => navigate('/profiles') },
    { id: 'allowlists', label: t('menu.allowlists'), icon: '🛡️', onClick: () => navigate('/allowlists') },
    { id: 'skills', label: t('menu.skills'), icon: '🧠', onClick: () => navigate('/skills') },
    { id: 'mcp', label: t('menu.mcp'), icon: '🔌', onClick: () => navigate('/mcp') },
    { id: 'channels', label: t('menu.channels'), icon: '📡', onClick: () => navigate('/channels') },
    { id: 'credentials', label: t('menu.credentials'), icon: '🔐', onClick: () => navigate('/credentials') },
    { id: 'providers', label: t('menu.providers'), icon: '🤖', onClick: () => navigate('/providers') },
    {
      id: 'theme', label: t('menu.theme'), icon: '🎨',
      submenu: THEMES.map((th) => ({
        id: `theme-${th.id}`,
        label: th.label,
        icon: currentTheme === th.id ? '✓' : ' ',
        onClick: () => setTheme(th.id as ThemeId),
      })),
    },
    {
      id: 'language', label: t('menu.language'), icon: '🌐',
      submenu: LANGUAGES.map((lang) => ({
        id: `lang-${lang.id}`,
        label: lang.nativeLabel,
        icon: currentLang === lang.id ? '✓' : ' ',
        onClick: () => setLanguage(lang.id),
      })),
    },
    { id: 'settings', label: t('menu.restoreDefaults'), icon: '↩️', onClick: () => navigate('/settings') },
    { id: 'help', label: t('menu.help'), icon: '📚', shortcut: 'F1', onClick: () => navigate('/help') },
    { id: 'about', label: t('menu.about'), icon: 'ℹ️', onClick: () => navigate('/about') },
  ], [navigate, t, currentTheme, setTheme, currentLang, setLanguage]);

  // --- Keyboard shortcuts ---
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.altKey && !event.ctrlKey && !event.shiftKey && !event.metaKey) {
        const key = event.key.toLowerCase();
        if (key === 'm') { event.preventDefault(); menuButtonRef.current?.toggleMenu(); return; }
        const altRoutes: Record<string, string> = { h: '/history', p: '/profiles' };
        const target = altRoutes[key];
        if (target) { event.preventDefault(); navigate(target); return; }
      }
      if (event.key === 'F1') { event.preventDefault(); navigate('/help'); }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [navigate]);

  return (
    <>
      <header className="topbar" role="banner">
        <div className="topbar__left">
          {isWorkspaceRoute ? (
            isRenaming ? (
              <input
                ref={renameInputRef}
                className="topbar__rename-input"
                value={renameValue}
                onChange={(e) => setRenameValue(e.target.value)}
                onKeyDown={handleRenameKeyDown}
                onBlur={() => void handleConfirmRename()}
                aria-label={t('workspace.renamePlaceholder')}
              />
            ) : (
              <button
                ref={pickerButtonRef}
                className="topbar__picker"
                onClick={handleOpenPicker}
                onContextMenu={handlePickerContextMenu}
                onKeyDown={handlePickerKeyDown}
                onDoubleClick={startRename}
                aria-expanded={pickerMenu.visible}
                aria-label={t('workspace.workspaceList')}
              >
                <span className="topbar__picker-icon" aria-hidden="true">📂</span>
                <span className="topbar__picker-name">{workspace?.name || 'Workspace'}</span>
                <span className="topbar__picker-arrow" aria-hidden="true">▾</span>
              </button>
            )
          ) : (
            <button
              className="topbar__back"
              onClick={() => navigate('/')}
              aria-label={t('menu.backToWorkspace')}
              title={t('menu.backToWorkspace')}
            >
              <span aria-hidden="true">←</span>
              <span>{t('menu.backToWorkspace')}</span>
            </button>
          )}
        </div>

        <h1 className="topbar__title">{pageTitle}</h1>

        <div className="topbar__right">
          <MenuButton
            ref={menuButtonRef}
            items={mainMenuItems}
            currentItemId={currentPage}
            buttonLabel={t('menu.navLabel')}
          />
        </div>
      </header>

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
        items={ctxMenu.items}
        x={ctxMenu.x}
        y={ctxMenu.y}
        visible={ctxMenu.visible}
        ariaLabel={ctxMenu.ariaLabel || t('workspace.workspaceOptions', 'Opções do workspace')}
        onClose={closeCtx}
        onSelect={onCtxSelect}
      />
    </>
  );
}
