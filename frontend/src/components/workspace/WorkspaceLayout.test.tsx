import { afterEach, describe, expect, it, beforeEach, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { WorkspaceLayout } from './WorkspaceLayout';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { useLandmarkNavigation, type Landmark } from '../../hooks/useLandmarkNavigation';
import { registerWorkspacePanelFocus } from './workspacePanelFocusRegistry';
import { useGridPageLandmarks } from '../../hooks/useGridPageLandmarks';
import { useContentPageLandmarks } from '../../hooks/useContentPageLandmarks';
import { captureLandmarkNavigationTarget } from '../../lib/commandLandmarkNavigation';

type MockWorkspaceState = {
  workspace: {
    activeTabId: string;
    tabs: Array<{
      id: string;
      type: 'chat' | 'editor' | 'tasklist';
      title: string;
      position: number;
      conversationId?: string;
      contentId?: string;
    }>;
  } & Record<string, unknown>;
  setActiveTab: ReturnType<typeof vi.fn>;
} & Record<string, unknown>;

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
  const useWorkspaceKeyboardShortcuts = vi.fn();
  return {
    useWorkspaceKeyboardShortcuts,
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
  useWorkspaceStore: Object.assign(
    (selector: (state: unknown) => unknown) => selector(storeMock.state),
    { getState: () => storeMock.state, subscribe: () => () => undefined },
  ),
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

vi.mock('../../hooks/useLandmarkNavigation', async importOriginal => {
  const actual = await importOriginal<typeof import('../../hooks/useLandmarkNavigation')>();
  return { ...actual, useLandmarkNavigation: vi.fn(actual.useLandmarkNavigation) };
});

vi.mock('../../store/authStore', () => ({ useAuthStore: {
  getState: () => ({ isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a' } }),
  subscribe: () => () => undefined,
} }));

vi.mock('../../hooks/useDefaultFocus', async importOriginal => ({
  ...await importOriginal<typeof import('../../hooks/useDefaultFocus')>(),
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
    <main className="ws-content__panel" data-active="true">
      <div className="ws-content-area">
        <button type="button" data-editor-rendered-document="true">
          Documento renderizado
        </button>
        <button type="button" data-testid="default-focus">
          Area default
        </button>
      </div>
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

function GridPage() {
  useGridPageLandmarks({ pageClass: 'profiles-page' });
  return <div className="profiles-page"><div role="toolbar"><button>Profile toolbar</button></div><div className="datagrid-container"><div role="grid"><div role="row"><div role="gridcell" tabIndex={0}>Profile cell</div></div></div></div></div>;
}
function ContentPage() {
  useContentPageLandmarks({ pageClass: 'help-page' });
  return <div className="help-page"><button>Help content</button></div>;
}

describe('WorkspaceLayout - foco ao navegar workspace tabs', () => {
  let requestAnimationFrameSpy: { mockRestore: () => void };

  beforeEach(() => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    storeMock.state.workspace.activeTabId = 'tab-1';
    storeMock.state.workspace.tabs[1].type = 'editor';
    storeMock.setActiveTab.mockClear();
    shortcutMock.useWorkspaceKeyboardShortcuts.mockClear();
    vi.mocked(restoreDefaultFocus).mockClear();
    vi.mocked(useLandmarkNavigation).mockClear();
    requestAnimationFrameSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0);
      return 1;
    });
  });

  afterEach(() => {
    requestAnimationFrameSpy.mockRestore();
    vi.restoreAllMocks();
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

  it('a área de conteúdo delega o foco ao handler do painel ativo', () => {
    const focusPanel = vi.fn(() => true);
    const unregister = registerWorkspacePanelFocus('tab-1', focusPanel);
    renderWorkspaceLayout();
    const calls = vi.mocked(useLandmarkNavigation).mock.calls;
    const options = calls[calls.length - 1]?.[0] as {
      landmarks: Landmark[];
      defaultLandmarkId?: string;
    };
    const contentArea = options.landmarks.find((landmark) => landmark.id === 'contentArea');

    expect(options.defaultLandmarkId).toBe('contentArea');
    expect(contentArea?.focus()).toBe(true);
    // Sem duplicar o conhecimento de cada tipo: o landmark só roteia ao painel.
    expect(focusPanel).toHaveBeenCalled();
    unregister();
  });

  it('a área de conteúdo cai em foco genérico quando o painel ainda não registrou handler', () => {
    renderWorkspaceLayout();
    const calls = vi.mocked(useLandmarkNavigation).mock.calls;
    const options = calls[calls.length - 1]?.[0] as {
      landmarks: Landmark[];
      defaultLandmarkId?: string;
    };
    const contentArea = options.landmarks.find((landmark) => landmark.id === 'contentArea');

    // Sem handler para a aba ativa, o fallback foca o primeiro elemento focável
    // do painel — sem travar a navegação por F6.
    expect(contentArea?.focus()).toBe(true);
    expect(screen.getByRole('button', { name: 'Documento renderizado' })).toHaveFocus();
  });

  it('não restaura foco fora da rota workspace', () => {
    render(
      <MemoryRouter initialEntries={['/settings']}>
        <WorkspaceLayout />
      </MemoryRouter>,
    );
    vi.mocked(restoreDefaultFocus).mockClear();
    expect(restoreDefaultFocus).not.toHaveBeenCalled();
  });

  it.each(['/history', '/jobs', '/profiles', '/memories', '/tasklists', '/help', '/about', '/update'])('não registra owner genérico para %s', path => {
    render(<MemoryRouter initialEntries={[path]}><WorkspaceLayout /></MemoryRouter>);
    const options = vi.mocked(useLandmarkNavigation).mock.calls[0][0];
    expect(options.enabled).toBe(false); expect(options.defaultLandmarkId).toBeUndefined(); expect(options.landmarks).toEqual([]);
    expect(captureLandmarkNavigationTarget(() => path)).toBeUndefined();
  });

  it.each(['/profiles', '/help'])('layout e hook real de página %s mantêm um único owner', path => {
    render(<MemoryRouter initialEntries={[path]}><Routes><Route element={<WorkspaceLayout />}><Route path="/profiles" element={<GridPage />} /><Route path="/help" element={<ContentPage />} /></Route></Routes></MemoryRouter>);
    const topbar = screen.getByRole('button', { name: 'Topbar' }); topbar.focus();
    const target = captureLandmarkNavigationTarget(() => path);
    expect(target).toBeDefined();
    expect(target?.open('navigation.landmark.next')).toBe(true); target?.dispose();
    expect(screen.getByRole('button', { name: path === '/profiles' ? 'Profile toolbar' : 'Help content' })).toHaveFocus();
    if (path === '/profiles') {
      const grid = captureLandmarkNavigationTarget(() => path);
      expect(grid?.open('navigation.landmark.next')).toBe(true); grid?.dispose();
      expect(screen.getByRole('gridcell')).toHaveFocus();
    }
  });
});
