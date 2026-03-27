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
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import {
  ArrowLeftOutlined,
  FolderOutlined,
  HistoryOutlined,
  CheckSquareOutlined,
  UserSwitchOutlined,
  ThunderboltOutlined,
  BgColorsOutlined,
  GlobalOutlined,
  SettingOutlined,
  QuestionCircleOutlined,
  InfoCircleOutlined,
  PlusOutlined,
  EditOutlined,
  ExportOutlined,
  ImportOutlined,
  CheckOutlined,
  DownOutlined,
} from '@ant-design/icons';
import './Topbar.css';

const PAGE_TITLE_KEYS: Record<string, string> = {
  '/history': 'menu.history',
  '/tasklists': 'menu.tasklists',
  '/jobs': 'menu.jobs',
  '/profiles': 'menu.profiles',
  '/settings': 'menu.settings',
  '/help': 'menu.help',
  '/about': 'menu.about',
  '/update': 'menu.about',
};

const ROUTE_IDS: Record<string, string> = {
  '/history': 'history',
  '/tasklists': 'tasklists',
  '/jobs': 'jobs',
  '/profiles': 'profiles',
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
  const toolbarRef = useToolbarKeyboardNav();
  const menuButtonRef = useRef<MenuButtonRef>(null);
  const pickerButtonRef = useRef<HTMLButtonElement>(null);
  const renameInputRef = useRef<HTMLInputElement>(null);

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState('');

  // --- Page title ---
  const resolvedPath = pathname.startsWith('/settings') ? '/settings' : pathname;
  const pageTitle = isWorkspaceRoute
    ? (workspace?.name || t('menu.appTitle'))
    : t(PAGE_TITLE_KEYS[resolvedPath] || 'menu.appTitle');

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

  const pickerItems = useMemo((): MenuItem[] => {
    return workspaces.map((ws) => ({
      id: `ws-${ws.id}`,
      label: ws.name,
      icon: ws.is_active ? <CheckOutlined /> : undefined,
      shortcut: `${ws.tab_count} ${ws.tab_count === 1 ? t('workspace.tabSingular', 'aba') : t('workspace.tabPlural', 'abas')}`,
      checked: ws.is_active,
      action: () => { if (!ws.is_active) void switchWorkspace(ws.id); },
    }));
  }, [workspaces, switchWorkspace, t]);

  const ctxMenuItems = useMemo((): MenuItem[] => [
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

  const currentPage = ROUTE_IDS[resolvedPath] || 'workspace';

  const mainMenuItems: MenuButtonItem[] = useMemo(() => [
    ...(!isWorkspaceRoute ? [
      { id: 'back-to-workspace', label: t('menu.backToWorkspace'), icon: <ArrowLeftOutlined />, onClick: () => navigate('/') },
      { id: 'sep-back', separator: true as const },
    ] : []),
    { id: 'history', label: t('menu.history'), icon: <HistoryOutlined />, shortcut: 'Alt+H', onClick: () => navigate('/history') },
    { id: 'tasklists', label: t('menu.tasklists'), icon: <CheckSquareOutlined />, onClick: () => navigate('/tasklists') },
    { id: 'jobs', label: t('menu.jobs'), icon: <ThunderboltOutlined />, onClick: () => navigate('/jobs') },
    { id: 'profiles', label: t('menu.profiles'), icon: <UserSwitchOutlined />, shortcut: 'Alt+P', onClick: () => navigate('/profiles') },
    { id: 'settings', label: t('menu.settings'), icon: <SettingOutlined />, onClick: () => navigate('/settings') },
    {
      id: 'theme', label: t('menu.theme'), icon: <BgColorsOutlined />,
      submenu: THEMES.map((th) => ({
        id: `theme-${th.id}`,
        label: th.label,
        icon: currentTheme === th.id ? <CheckOutlined /> : undefined,
        onClick: () => setTheme(th.id as ThemeId),
      })),
    },
    {
      id: 'language', label: t('menu.language'), icon: <GlobalOutlined />,
      submenu: LANGUAGES.map((lang) => ({
        id: `lang-${lang.id}`,
        label: lang.nativeLabel,
        icon: currentLang === lang.id ? <CheckOutlined /> : undefined,
        onClick: () => setLanguage(lang.id),
      })),
    },
    { id: 'help', label: t('menu.help'), icon: <QuestionCircleOutlined />, shortcut: 'F1', onClick: () => navigate('/help') },
    { id: 'about', label: t('menu.about'), icon: <InfoCircleOutlined />, onClick: () => navigate('/about') },
  ], [navigate, t, currentTheme, setTheme, currentLang, setLanguage, isWorkspaceRoute]);

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
      <header
        className="topbar"
        role="toolbar"
        aria-label={t('landmarks.topbar', 'Barra de navegação')}
        ref={toolbarRef as React.RefObject<HTMLElement>}
      >
        <div className="topbar__left">
          <MenuButton
            ref={menuButtonRef}
            items={mainMenuItems}
            currentItemId={currentPage}
            buttonLabel={t('menu.navLabel')}
            tabIndex={-1}
          />
        </div>

        <h1 className="topbar__title">{pageTitle}</h1>

        <div className="topbar__right">
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
                tabIndex={-1}
              >
                <span className="topbar__picker-icon" aria-hidden="true"><FolderOutlined /></span>
                <span className="topbar__picker-name">{workspace?.name || 'Workspace'}</span>
                <span className="topbar__picker-arrow" aria-hidden="true"><DownOutlined /></span>
              </button>
            )
          ) : (
            <button
              className="topbar__back"
              onClick={() => navigate('/')}
              aria-label={t('menu.backToWorkspace')}
              title={t('menu.backToWorkspace')}
              tabIndex={-1}
            >
              <ArrowLeftOutlined aria-hidden="true" />
              <span>{t('menu.backToWorkspace')}</span>
            </button>
          )}
        </div>
      </header>

      <Menu
        items={pickerMenu.items}
        x={pickerMenu.x}
        y={pickerMenu.y}
        visible={pickerMenu.visible}
        ariaLabel={pickerMenu.ariaLabel || t('workspace.workspaceList')}
        searchable
        searchPlaceholder={t('workspace.searchWorkspaces', 'Buscar workspace...')}
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
