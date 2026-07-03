import { afterEach, describe, expect, it, beforeEach, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { WorkspaceLayout } from './WorkspaceLayout';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';

type MockWorkspaceState = {
  workspace: {
    activeTabId: string;
    tabs: Array<{
      id: string;
      type: 'chat' | 'editor';
      title: string;
      position: number;
      conversationId?: string;
      contentId?: string;
    }>;
  } & Record<string, unknown>;
  setActiveTab: ReturnType<typeof vi.fn>;
} & Record<string, unknown>;

type KeyboardShortcutOptions = {
  onTabShortcutNavigation?: (tabId: string) => void;
};

const storeMock = vi.hoisted(() => {
  let state: MockWorkspaceState;
  const setActiveTab = vi.fn((tabId: string) => {
    state.workspace.activeTabId = tabId;
    return Promise.resolve();
  });
  state = {
    workspace: {
      id: 'ws-1',
      name: 'Workspace teste',
      profile: '',
      tabs: [
        { id: 'tab-1', type: 'chat', title: 'Chat', position: 0, conversationId: 'conv-1' },
        { id: 'tab-2', type: 'editor', title: 'Editor', position: 1, contentId: 'file-1' },
      ],
      activeTabId: 'tab-1',
    },
    workspaces: [],
    isInitialized: true,
    initialize: vi.fn(),
    setupEventListeners: vi.fn(() => vi.fn()),
    setActiveTab,
    removeTab: vi.fn(() => Promise.resolve()),
    updateTab: vi.fn(() => Promise.resolve()),
    reorderTabs: vi.fn(() => Promise.resolve()),
    moveTabToWorkspace: vi.fn(() => Promise.resolve()),
    renameTabContent: vi.fn(),
  };
  return { state, setActiveTab };
});

const shortcutMock = vi.hoisted(() => {
  let latestOptions: KeyboardShortcutOptions | undefined;
  const useWorkspaceKeyboardShortcuts = vi.fn((options?: KeyboardShortcutOptions) => {
    latestOptions = options;
  });
  return {
    useWorkspaceKeyboardShortcuts,
    getLatestOptions: () => latestOptions,
  };
});

vi.mock('zustand/shallow', () => ({
  useShallow: <T,>(fn: T) => fn,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('antd', () => ({
  Spin: () => <div role="status">Carregando</div>,
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (state: unknown) => unknown) => selector(storeMock.state),
}));

vi.mock('../../hooks/useDocumentTitle', () => ({
  useDocumentTitle: vi.fn(),
}));

vi.mock('../../hooks/useWorkspaceKeyboardShortcuts', () => ({
  useWorkspaceKeyboardShortcuts: shortcutMock.useWorkspaceKeyboardShortcuts,
}));

vi.mock('../../hooks/useWorkspaceChatBridge', () => ({
  useWorkspaceChatBridge: vi.fn(),
}));

vi.mock('../../hooks/useLandmarkNavigation', () => ({
  useLandmarkNavigation: vi.fn(),
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: vi.fn(() => {
    document.querySelector<HTMLButtonElement>('[data-testid="default-focus"]')?.focus();
    return true;
  }),
}));

vi.mock('../../services/voiceAccessibility/workspaceResolver', () => ({
  useVoiceAccessibilityWorkspaceResolver: vi.fn(),
}));

vi.mock('../ui/Modal', () => ({
  ensureModalCleanup: vi.fn(),
}));

vi.mock('../layout/Topbar', () => ({
  Topbar: () => (
    <div className="topbar" role="toolbar">
      <button type="button">Topbar</button>
    </div>
  ),
}));

vi.mock('./WorkspaceToolbar', () => ({
  WorkspaceToolbar: () => (
    <div className="workspace-toolbar" role="toolbar">
      <button type="button">Nova aba</button>
    </div>
  ),
}));

vi.mock('./WorkspaceContent', () => ({
  WorkspaceContent: () => (
    <main>
      <button type="button" data-testid="default-focus">
        Area default
      </button>
    </main>
  ),
}));

vi.mock('./WorkspaceChatModal', () => ({
  WorkspaceChatModal: () => null,
}));

vi.mock('./useWorkspacePanelRenameHandlers', () => ({
  useWorkspacePanelRenameHandlers: vi.fn(),
}));

vi.mock('./useWorkspacePanelLifecycleCleanup', () => ({
  useWorkspacePanelLifecycleCleanup: vi.fn(),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: vi.fn(),
}));

function renderWorkspaceLayout() {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <WorkspaceLayout />
    </MemoryRouter>,
  );
}

describe('WorkspaceLayout - foco ao navegar workspace tabs', () => {
  let requestAnimationFrameSpy: { mockRestore: () => void };

  beforeEach(() => {
    storeMock.state.workspace.activeTabId = 'tab-1';
    storeMock.setActiveTab.mockClear();
    shortcutMock.useWorkspaceKeyboardShortcuts.mockClear();
    vi.mocked(restoreDefaultFocus).mockClear();
    requestAnimationFrameSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0);
      return 1;
    });
  });

  afterEach(() => {
    requestAnimationFrameSpy.mockRestore();
  });

  it('setas na tablist trocam aba mantendo foco na tablist', () => {
    const { rerender } = renderWorkspaceLayout();

    const tablist = screen.getByRole('tablist', { name: 'workspace.tabListLabel' });
    const chatTab = screen.getByRole('tab', { name: /Chat/ });
    chatTab.focus();

    fireEvent.keyDown(chatTab, { key: 'ArrowRight' });
    expect(storeMock.setActiveTab).toHaveBeenCalledWith('tab-2');

    rerender(
      <MemoryRouter initialEntries={['/']}>
        <WorkspaceLayout />
      </MemoryRouter>,
    );

    const editorTab = screen.getByRole('tab', { name: /Editor/ });
    expect(editorTab).toHaveFocus();
    expect(tablist.contains(document.activeElement)).toBe(true);
    expect(restoreDefaultFocus).not.toHaveBeenCalled();
  });

  it('atalho global de aba restaura foco na area default apos troca', () => {
    const { rerender } = renderWorkspaceLayout();

    shortcutMock.getLatestOptions()?.onTabShortcutNavigation?.('tab-2');
    storeMock.state.workspace.activeTabId = 'tab-2';

    rerender(
      <MemoryRouter initialEntries={['/']}>
        <WorkspaceLayout />
      </MemoryRouter>,
    );

    expect(restoreDefaultFocus).toHaveBeenCalled();
    expect(screen.getByTestId('default-focus')).toHaveFocus();
  });
});
