import { useEffect, useMemo } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useDocumentTitle } from '../../hooks/useDocumentTitle';
import { useWorkspaceKeyboardShortcuts } from '../../hooks/useWorkspaceKeyboardShortcuts';
import { useWorkspaceChatBridge } from '../../hooks/useWorkspaceChatBridge';
import { useWorkspaceTerminalBridge } from '../../hooks/useWorkspaceTerminalBridge';
import { useWorkspaceEditorBridge } from '../../hooks/useWorkspaceEditorBridge';
import { useLandmarkNavigation, type Landmark } from '../../hooks/useLandmarkNavigation';
import { ensureModalCleanup } from '../ui/Modal';
import { Topbar } from '../layout/Topbar';
import { WorkspaceToolbar } from './WorkspaceToolbar';
import { WorkspaceTabList } from './WorkspaceTabList';
import { WorkspaceContent } from './WorkspaceContent';
import './WorkspaceLayout.css';

export function WorkspaceLayout() {
  useDocumentTitle();
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { workspace, isInitialized, initialize, setupEventListeners } = useWorkspaceStore();
  const activeTabType = useWorkspaceStore((s) => s.getActiveTab()?.type);
  useEffect(() => {
    if (!isInitialized) {
      initialize();
    }
  }, [isInitialized, initialize]);

  useEffect(() => {
    const cleanup = setupEventListeners();
    return cleanup;
  }, [setupEventListeners]);

  useWorkspaceKeyboardShortcuts();
  useWorkspaceChatBridge();
  useWorkspaceTerminalBridge();
  useWorkspaceEditorBridge();

  const isWorkspaceRoute = pathname === '/' || pathname === '';
  const isChatActive = activeTabType === 'chat';
  const isTerminalActive = activeTabType === 'terminal';
  const isEditorActive = activeTabType === 'editor';

  const landmarks = useMemo((): Landmark[] => [
    {
      id: 'workspaceToolbar',
      label: t('landmarks.workspaceToolbar', 'Barra de ferramentas do workspace'),
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
    {
      id: 'workspaceTabs',
      label: t('landmarks.workspaceTabs', 'Abas do workspace'),
      focus: () => {
        const active = document.querySelector('.ws-tabs [role="tab"][aria-selected="true"]') as HTMLElement | null;
        const anyTab = document.querySelector('.ws-tabs [role="tab"]') as HTMLElement | null;
        (active || anyTab)?.focus();
        return !!(active || anyTab);
      },
      contains: () => !!document.activeElement?.closest?.('.ws-tabs'),
    },
    // Chat landmarks
    {
      id: 'chatToolbar',
      label: t('landmarks.toolbar'),
      isAvailable: () => isChatActive,
      focus: () => {
        const toolbar = document.querySelector('.chat-page [role="toolbar"]') as Element | null;
        if (!toolbar) return false;
        const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
        if (!btn) return false;
        btn.focus();
        return true;
      },
      contains: () => !!document.activeElement?.closest?.('.chat-page [role="toolbar"]'),
    },
    {
      id: 'chatHistory',
      label: t('landmarks.chatHistory'),
      isAvailable: () => isChatActive,
      focus: () => {
        const container = document.querySelector('.message-list') as HTMLElement | null;
        if (!container) return false;
        const lastMsg = container.querySelector('[data-message-node]:last-child') as HTMLElement | null;
        if (lastMsg) { lastMsg.focus(); return true; }
        container.setAttribute('tabindex', '-1');
        container.focus();
        return true;
      },
      contains: () => !!document.activeElement?.closest?.('.message-list'),
    },
    {
      id: 'chatInput',
      label: t('landmarks.chatInput'),
      isAvailable: () => isChatActive,
      focus: () => {
        const input = document.querySelector('.chat-input textarea') as HTMLElement | null;
        if (!input) return false;
        input.focus();
        return true;
      },
      contains: () => !!document.activeElement?.closest?.('.chat-input'),
    },
    // Terminal landmarks
    {
      id: 'terminalToolbar',
      label: t('landmarks.terminalToolbar', 'Barra de ferramentas do terminal'),
      isAvailable: () => isTerminalActive,
      focus: () => {
        const toolbar = document.querySelector('.terminal-page [role="toolbar"]') as Element | null;
        if (!toolbar) return false;
        const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
        if (!btn) return false;
        btn.focus();
        return true;
      },
      contains: () => !!document.activeElement?.closest?.('.terminal-page [role="toolbar"]'),
    },
    {
      id: 'terminalHistory',
      label: t('landmarks.terminalHistory', 'Histórico do terminal'),
      isAvailable: () => isTerminalActive,
      focus: () => {
        const container = document.querySelector('.terminal-history') as HTMLElement | null;
        if (!container) return false;
        const lastNode = container.querySelector('.terminal-node:last-child') as HTMLElement | null;
        if (lastNode) { lastNode.focus(); return true; }
        container.setAttribute('tabindex', '-1');
        container.focus();
        return true;
      },
      contains: () => !!document.activeElement?.closest?.('.terminal-history'),
    },
    {
      id: 'terminalInput',
      label: t('landmarks.terminalInput', 'Campo de comando'),
      isAvailable: () => isTerminalActive,
      focus: () => {
        const input = document.querySelector('.terminal-page__input-container textarea') as HTMLElement | null;
        if (!input) return false;
        input.focus();
        return true;
      },
      contains: () => !!document.activeElement?.closest?.('.terminal-page__input-container'),
    },
    // Editor landmarks
    {
      id: 'editorToolbar',
      label: t('landmarks.editorToolbar', 'Barra de ferramentas do editor'),
      isAvailable: () => isEditorActive,
      focus: () => {
        const toolbar = document.querySelector('.editor-page__toolbar') as Element | null;
        if (!toolbar) return false;
        const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
        if (!btn) return false;
        btn.focus();
        return true;
      },
      contains: () => !!document.activeElement?.closest?.('.editor-page__toolbar'),
    },
    {
      id: 'editorContent',
      label: t('landmarks.editorContent', 'Editor de texto'),
      isAvailable: () => isEditorActive,
      focus: () => {
        const monaco = document.querySelector('.editor-page .monaco-editor textarea') as HTMLElement | null;
        if (monaco) { monaco.focus(); return true; }
        const rich = document.querySelector('.editor-page .rich-text-editor__content [contenteditable]') as HTMLElement | null;
        if (rich) { rich.focus(); return true; }
        return false;
      },
      contains: () =>
        !!document.activeElement?.closest?.('.monaco-editor') ||
        !!document.activeElement?.closest?.('.rich-text-editor__content'),
    },
  ], [t, isChatActive, isTerminalActive, isEditorActive]);

  const defaultLandmark = isChatActive ? 'chatInput'
    : isTerminalActive ? 'terminalInput'
    : isEditorActive ? 'editorContent'
    : 'workspaceTabs';

  useLandmarkNavigation({
    landmarks,
    enabled: isWorkspaceRoute,
    defaultLandmarkId: defaultLandmark,
  });

  useEffect(() => {
    ensureModalCleanup();
  }, [pathname]);

  if (!isInitialized && isWorkspaceRoute) {
    return (
      <div className="workspace-layout">
        <div className="workspace-layout__loading" aria-busy="true" />
      </div>
    );
  }

  if (isWorkspaceRoute) {
    return (
      <div className="workspace-layout">
        <WorkspaceToolbar />
        {workspace && <WorkspaceTabList />}
        <WorkspaceContent />
      </div>
    );
  }

  // Sub-rotas de configuração: Topbar + conteúdo
  return (
    <div className="workspace-layout">
      <Topbar />
      <main className="workspace-layout__config-content">
        <Outlet />
      </main>
    </div>
  );
}
