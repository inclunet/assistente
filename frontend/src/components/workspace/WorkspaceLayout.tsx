import { useEffect, useMemo, useRef } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Spin } from 'antd';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useDocumentTitle } from '../../hooks/useDocumentTitle';
import { useWorkspaceKeyboardShortcuts } from '../../hooks/useWorkspaceKeyboardShortcuts';
import { useWorkspaceChatBridge } from '../../hooks/useWorkspaceChatBridge';
import { useWorkspaceTerminalBridge } from '../../hooks/useWorkspaceTerminalBridge';
import { useWorkspaceEditorBridge } from '../../hooks/useWorkspaceEditorBridge';
import { useWorkspaceTasklistBridge } from '../../hooks/useWorkspaceTasklistBridge';
import { useLandmarkNavigation, type Landmark } from '../../hooks/useLandmarkNavigation';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { ensureModalCleanup } from '../ui/Modal';
import { Topbar } from '../layout/Topbar';
import { WorkspaceToolbar } from './WorkspaceToolbar';
import { WorkspaceTabList } from './WorkspaceTabList';
import { WorkspaceContent } from './WorkspaceContent';
import { WorkspaceMiniChat } from './WorkspaceMiniChat';
import './WorkspaceLayout.css';

export function WorkspaceLayout() {
  useDocumentTitle();
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { workspace, isInitialized, initialize, setupEventListeners } = useWorkspaceStore();
  useEffect(() => {
    if (!isInitialized) {
      initialize();
    }
  }, [isInitialized, initialize]);

  useEffect(() => {
    const cleanup = setupEventListeners();
    return cleanup;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useWorkspaceKeyboardShortcuts();
  useWorkspaceChatBridge();
  useWorkspaceTerminalBridge();
  useWorkspaceEditorBridge();
  useWorkspaceTasklistBridge();

  const isWorkspaceRoute = pathname === '/' || pathname === '';

  const landmarks = useMemo((): Landmark[] => {
    const focusTopbar = () => {
      const toolbar = document.querySelector('.topbar[role="toolbar"]') as Element | null;
      if (!toolbar) return false;
      const active = toolbar.querySelector('[tabindex="0"]') as HTMLElement | null;
      const fallback = toolbar.querySelector('button:not([disabled])') as HTMLElement | null;
      const target = active || fallback;
      if (!target) return false;
      target.focus();
      return true;
    };

    const isSettingsRoute = pathname.startsWith('/settings');

    if (isSettingsRoute) {
      const activePanel = () =>
        document.querySelector('.settings-tabs__panel:not([hidden])') as HTMLElement | null;

      return [
        {
          id: 'topbar',
          label: t('landmarks.topbar', 'Barra de navegação'),
          focus: focusTopbar,
          contains: () => !!document.activeElement?.closest?.('.topbar'),
        },
        {
          id: 'settingsTabs',
          label: t('landmarks.settingsTabs', 'Abas de configurações'),
          focus: () => {
            const active = document.querySelector('.settings-tabs__list [role="tab"][aria-selected="true"]') as HTMLElement | null;
            const anyTab = document.querySelector('.settings-tabs__list [role="tab"]') as HTMLElement | null;
            (active || anyTab)?.focus();
            return !!(active || anyTab);
          },
          contains: () => !!document.activeElement?.closest?.('.settings-tabs__list'),
        },
        {
          id: 'settingsToolbar',
          label: t('landmarks.settingsToolbar', 'Barra de ferramentas'),
          isAvailable: () => !!activePanel()?.querySelector('[role="toolbar"]'),
          focus: () => {
            const panel = activePanel();
            if (!panel) return false;
            const toolbar = panel.querySelector('[role="toolbar"]') as Element | null;
            if (!toolbar) return false;
            const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
            if (!btn) return false;
            btn.focus();
            return true;
          },
          contains: () => {
            const panel = activePanel();
            if (!panel) return false;
            return !!document.activeElement?.closest?.('[role="toolbar"]') &&
                   panel.contains(document.activeElement);
          },
        },
        {
          id: 'settingsContent',
          label: t('landmarks.settingsContent', 'Conteúdo da configuração'),
          focus: () => {
            const panel = activePanel();
            if (!panel) return false;
            const grid = panel.querySelector('[role="grid"]') as HTMLElement | null;
            if (grid) {
              const cell = panel.querySelector('.datagrid-container [role="gridcell"][tabindex="0"], .datagrid-container [role="gridcell"]') as HTMLElement | null;
              if (cell) { cell.focus(); return true; }
              grid.focus();
              return true;
            }
            const toolbar = panel.querySelector('[role="toolbar"]');
            const focusable = Array.from(
              panel.querySelectorAll('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')
            ).find((el) => !toolbar?.contains(el)) as HTMLElement | undefined;
            if (focusable) { focusable.focus(); return true; }
            panel.setAttribute('tabindex', '-1');
            panel.focus();
            return true;
          },
          contains: () => {
            const panel = activePanel();
            if (!panel) return false;
            if (!panel.contains(document.activeElement)) return false;
            return !document.activeElement?.closest?.('[role="toolbar"]') ||
                   !panel.querySelector('[role="toolbar"]')?.contains(document.activeElement);
          },
        },
      ];
    }

    if (!isWorkspaceRoute) {
      return [
        {
          id: 'topbar',
          label: t('landmarks.topbar', 'Barra de navegação'),
          focus: focusTopbar,
          contains: () => !!document.activeElement?.closest?.('.topbar'),
        },
        {
          id: 'pageContent',
          label: t('landmarks.pageContent', 'Conteúdo da página'),
          focus: () => {
            const content = document.querySelector('.workspace-layout__config-content') as HTMLElement | null;
            if (!content) return false;
            const focusable = content.querySelector('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])') as HTMLElement | null;
            if (focusable) { focusable.focus(); return true; }
            content.setAttribute('tabindex', '-1');
            content.focus();
            return true;
          },
          contains: () => !!document.activeElement?.closest?.('.workspace-layout__config-content'),
        },
      ];
    }

    return [
      // 1. Topbar (workspace picker + menu principal)
      {
        id: 'topbar',
        label: t('landmarks.topbar', 'Barra de navegação'),
        focus: focusTopbar,
        contains: () => !!document.activeElement?.closest?.('.topbar'),
      },
      // 2. Workspace Toolbar (nova aba, opções, perfil)
      {
        id: 'workspaceToolbar',
        label: t('landmarks.workspaceToolbar', 'Workspace'),
        focus: () => {
          const toolbar = document.querySelector('.workspace-toolbar[role="toolbar"]') as Element | null;
          if (!toolbar) return false;
          const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
          if (!btn) return false;
          btn.focus();
          return true;
        },
        contains: () => !!document.activeElement?.closest?.('.workspace-toolbar'),
      },
      // 3. Tablist
      {
        id: 'workspaceTabs',
        label: t('landmarks.workspaceTabs', 'Painéis'),
        focus: () => {
          const active = document.querySelector('.ws-tabs [role="tab"][aria-selected="true"]') as HTMLElement | null;
          const anyTab = document.querySelector('.ws-tabs [role="tab"]') as HTMLElement | null;
          (active || anyTab)?.focus();
          return !!(active || anyTab);
        },
        contains: () => !!document.activeElement?.closest?.('.ws-tabs'),
      },
      // 3. Content Panel Toolbar (genérico, muda conforme tipo de aba)
      {
        id: 'contentToolbar',
        label: t('landmarks.contentToolbar', 'Barra de ferramentas do conteúdo'),
        focus: () => {
          const toolbar = document.querySelector('.ws-content .ws-content-toolbar') as Element | null;
          if (!toolbar) return false;
          const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
          if (!btn) return false;
          btn.focus();
          return true;
        },
        contains: () => !!document.activeElement?.closest?.('.ws-content-toolbar'),
      },
      // 4. Content Area (genérico, foco inteligente por tipo de aba)
      {
        id: 'contentArea',
        label: t('landmarks.contentArea', 'Área de conteúdo'),
        focus: () => {
          const area = document.querySelector('.ws-content .ws-content-area') as HTMLElement | null;
          if (!area) return false;

          // Chat/Terminal: foca no textarea do input
          const textarea = area.querySelector('.chat-input textarea') as HTMLElement | null;
          if (textarea) { textarea.focus(); return true; }

          // Editor: foca na superfície de edição
          const monaco = area.querySelector('.monaco-editor textarea') as HTMLElement | null;
          if (monaco) { monaco.focus(); return true; }
          const rich = area.querySelector('.rich-text-editor__content [contenteditable]') as HTMLElement | null;
          if (rich) { rich.focus(); return true; }

          // Fallback genérico (tasklist, etc.)
          const focusable = area.querySelector('button, input, select, textarea, [tabindex]:not([tabindex="-1"])') as HTMLElement | null;
          if (focusable) { focusable.focus(); return true; }

          area.setAttribute('tabindex', '-1');
          area.focus();
          return true;
        },
        contains: () => !!document.activeElement?.closest?.('.ws-content-area'),
      },
    ];
  }, [t, isWorkspaceRoute, pathname]);

  const isSettingsRoute = pathname.startsWith('/settings');
  const defaultLandmark = isWorkspaceRoute ? 'contentArea' : isSettingsRoute ? 'settingsContent' : 'pageContent';

  useLandmarkNavigation({
    landmarks,
    enabled: true,
    defaultLandmarkId: defaultLandmark,
  });

  const activeTabId = workspace?.activeTabId;
  const prevActiveTabIdRef = useRef(activeTabId);

  useEffect(() => {
    if (!isWorkspaceRoute || !activeTabId) return;
    if (activeTabId === prevActiveTabIdRef.current) return;
    prevActiveTabIdRef.current = activeTabId;

    requestAnimationFrame(() => restoreDefaultFocus());
  }, [activeTabId, isWorkspaceRoute]);

  useEffect(() => {
    ensureModalCleanup();
  }, [pathname]);

  if (!isInitialized && isWorkspaceRoute) {
    return (
      <div className="workspace-layout">
        <div className="workspace-layout__loading" aria-busy="true">
          <Spin size="large" />
        </div>
      </div>
    );
  }

  if (isWorkspaceRoute) {
    return (
      <div className="workspace-layout">
        <Topbar />
        <WorkspaceToolbar />
        {workspace && <WorkspaceTabList />}
        <WorkspaceContent />
        {/* Rota index (WorkspaceIndexRoute em router.tsx); conteúdo real vem de WorkspaceContent */}
        <Outlet />
        <WorkspaceMiniChat />
      </div>
    );
  }

  // Sub-rotas: Topbar + conteúdo
  return (
    <div className="workspace-layout">
      <Topbar />
      <main className="workspace-layout__config-content">
        <Outlet />
      </main>
    </div>
  );
}
