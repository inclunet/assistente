import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useRef, useSyncExternalStore } from 'react';
import { Topbar } from './Topbar';
import { CommandContextProvider, useCommandContextScope } from '../../lib/commandContextReact';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';
import { useWorkspaceCommandSurface } from '../workspace/useWorkspaceCommandSurface';
import { useWorkspaceStore } from '../../store/workspaceStore';
import type { WorkspaceTab } from '../../store/workspaceStore';

const state = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  const workspace = {
    id: 'workspace-a', name: 'Workspace', profile: 'focused', activeTabId: 'tab-tasklist',
    tabs: [
      { id: 'tab-tasklist', type: 'tasklist', title: 'Tasks', position: 0 },
      { id: 'tab-chat', type: 'chat', title: 'Chat', position: 1 },
      { id: 'tab-editor', type: 'editor', title: 'Editor', position: 2 },
    ],
  };
  const store = {
    workspace,
    workspaces: [] as unknown[],
    switchWorkspace: vi.fn(),
    renameWorkspace: vi.fn(),
    setActiveTab: vi.fn((tabId: string) => {
      store.workspace.activeTabId = tabId;
      listeners.forEach(listener => listener());
    }),
  };
  return {
    events: new Map<string, (payload?: unknown) => void>(),
    listeners,
    auth: { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } },
    workspaceStore: store,
    navigate: vi.fn(),
    announce: vi.fn(),
    map: vi.fn(async () => ({
      generation: 'map-a', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [], localPaletteCommands: [],
    })),
  };
});

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'pt-BR' } }) }));
vi.mock('react-router-dom', () => ({ useNavigate: () => state.navigate, useLocation: () => ({ pathname: '/', search: '', hash: '', key: '/' }) }));
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({ acquireGlobalCommandOwnership: () => ({ isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve() }) }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => []) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({
  loadMap: state.map, beginLocalCommandUIKey: vi.fn(), resetLocalCommandKeyboard: vi.fn(), dispatchLocalCommandKey: vi.fn(),
}) }));
vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign((selector?: (value: typeof state.auth) => unknown) => selector ? selector(state.auth) : state.auth, {
    getState: () => state.auth, subscribe: () => () => undefined,
  }),
}));
vi.mock('../../store/workspaceStore', () => {
  const subscribe = (listener: () => void) => { state.listeners.add(listener); return () => state.listeners.delete(listener); };
  const useStore = (selector?: (value: typeof state.workspaceStore) => unknown) =>
    useSyncExternalStore(subscribe, () => selector ? selector(state.workspaceStore) : state.workspaceStore);
  return {
    flushWorkspaceNavigation: vi.fn(async () => true),
    useWorkspaceStore: Object.assign(useStore, { getState: () => state.workspaceStore, subscribe }),
  };
});
vi.mock('../../store/shortcutsHelpStore', () => ({ useShortcutsHelpStore: (selector: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) => selector({ isOpen: false, open: vi.fn(), close: vi.fn() }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: vi.fn() }) }));
vi.mock('../../store/terminalStore', () => ({
  useTerminalStore: Object.assign((selector?: (value: unknown) => unknown) => selector ? selector({ sessions: [], historyBySession: {}, activeEntryBySession: {} }) : {}, {
    getState: () => ({ sessions: [], historyBySession: {}, activeEntryBySession: {} }), subscribe: () => () => undefined,
  }),
}));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: vi.fn() }));
vi.mock('../../store/workspaceChatModalStore', () => ({ canPrepareWorkspaceChatOpen: () => false, registerWorkspaceChatCommandDispatcher: () => () => {}, prepareWorkspaceChatOpen: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, fn: (payload?: unknown) => void) => {
  state.events.set(name, fn); return () => state.events.delete(name);
} }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(async () => null) }));
vi.mock('../pickers/ProfilePicker', () => ({ ProfilePicker: () => null }));

const appAPI = {
  GetLocalCommandKeyboardMap: state.map,
  ResetLocalCommandKeyboard: vi.fn(async () => undefined),
};
const contexts = [
  { id: 'tab-tasklist', type: 'tasklist' },
  { id: 'tab-chat', type: 'chat' },
  { id: 'tab-editor', type: 'editor' },
] as const;
let registeredScope: ReturnType<typeof useCommandContextScope> = null;
let simulatedWindowFocus = true;
const updateWindowFocus = (event: FocusEvent) => { simulatedWindowFocus = event.type === 'focus'; };

function ScopeRecorder() {
  registeredScope = useCommandContextScope();
  return null;
}

function PanelSurface({ id, type }: { id: string; type: string }) {
  useCommandCommandSurfaceForTest(type, id);
  return <textarea aria-label={`${type} input`} data-source-id={id} />;
}

function useCommandCommandSurfaceForTest(type: string, id: string) {
  return useWorkspaceCommandSurface(type, () => ({ surfaceType: type, surfaceId: id, snapshotVersion: `${id}-v1` }));
}

function Panel({ id, type }: { id: string; type: string }) {
  const root = useRef<HTMLDivElement>(null);
  const activeTabId = useActiveTabIdForTest();
  const tab = state.workspaceStore.workspace.tabs.find(candidate => candidate.id === id)! as WorkspaceTab;
  return (
    <WorkspacePanelProvider value={{ tab, isActive: activeTabId === id, rootRef: root }}>
      <div ref={root} className="ws-content__panel" data-tab-id={id} data-active={activeTabId === id ? 'true' : 'false'}
        hidden={activeTabId !== id} aria-hidden={activeTabId !== id ? 'true' : undefined}>
        <PanelSurface id={id} type={type} />
      </div>
    </WorkspacePanelProvider>
  );
}

function useActiveTabIdForTest(): string | null {
  // Hook lookup comes from the real imported store hook so panels rerender on active-tab changes.
  return useWorkspaceStore(value => value.workspace?.activeTabId ?? null);
}

function MixedWorkspace() {
  return (
    <CommandContextProvider>
      <div className="workspace-layout">
        <Topbar />
        <ScopeRecorder />
        {contexts.map(panel => <Panel key={panel.id} {...panel} />)}
      </div>
    </CommandContextProvider>
  );
}

function payload(commandId = 'workspace.tab.next') {
  return {
    commandId: '', generation: 'map-a', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    conditions: [{ commandId, bySurface: {}, fallback: true }],
  };
}

describe('Deck tab navigation with real panel surface registration', () => {
  beforeEach(() => {
    simulatedWindowFocus = true;
    vi.spyOn(document, 'hasFocus').mockImplementation(() => simulatedWindowFocus);
    window.addEventListener('focus', updateWindowFocus, true);
    window.addEventListener('blur', updateWindowFocus, true);
    state.events.clear(); state.listeners.clear(); state.navigate.mockClear(); state.announce.mockClear();
    state.workspaceStore.workspace.activeTabId = 'tab-tasklist';
    registeredScope = null;
    state.workspaceStore.setActiveTab.mockClear();
    state.map.mockClear();
    appAPI.ResetLocalCommandKeyboard.mockClear();
    Object.assign(window, { go: { app: { App: appAPI } } });
  });

  afterEach(() => {
    window.removeEventListener('focus', updateWindowFocus, true);
    window.removeEventListener('blur', updateWindowFocus, true);
    Reflect.deleteProperty(window, 'go');
    vi.restoreAllMocks();
  });
  it('navigates tasklist → chat → editor → tasklist with live registrations and fallback=true from real focused panels', async () => {
    const view = render(<MixedWorkspace />);
    try {
      // jsdom has no native window focus; the shim is event-driven and the blur test below verifies both edges.
      expect(document.hasFocus()).toBe(true);
      await waitFor(() => expect(state.map).toHaveBeenCalled());
      const initialMap = state.map.mock.results[0]!.value;
      await act(async () => {
        await initialMap;
        await Promise.resolve();
      });

      const transitions = [
        { from: contexts[0], command: 'workspace.tab.second', to: contexts[1] },
        { from: contexts[1], command: 'workspace.tab.third', to: contexts[2] },
        { from: contexts[2], command: 'workspace.tab.first', to: contexts[0] },
      ];
      for (const transition of transitions) {
        expect(state.workspaceStore.workspace.activeTabId).toBe(transition.from.id);
        const input = screen.getByRole('textbox', { name: `${transition.from.type} input` });
        act(() => input.focus());
        await waitFor(() => expect(document.activeElement).toBe(input));
        await waitFor(() => expect(registeredScope?.surfaceForElement(input)).toBe(transition.from.id));
        expect(document.querySelector(`[data-tab-id="${transition.from.id}"]`)).toHaveAttribute('data-active', 'true');

        await act(async () => { state.events.get('command:deck-local-ui')?.(payload(transition.command)); });

        await waitFor(() => expect(state.workspaceStore.workspace.activeTabId).toBe(transition.to.id));
        await waitFor(() => expect(document.querySelector(`[data-tab-id="${transition.to.id}"]`)).toHaveAttribute('data-active', 'true'));
      }
      expect(state.workspaceStore.setActiveTab.mock.calls).toEqual([['tab-chat'], ['tab-editor'], ['tab-tasklist']]);
    } finally { view.unmount(); }
  });

  it('executa workspace.tab.go_to com os argumentos do evento e recusa workspace divergente', async () => {
    const view = render(<MixedWorkspace />);
    try {
      const input = await screen.findByRole('textbox', { name: 'tasklist input' });
      act(() => input.focus());
      await waitFor(() => expect(registeredScope?.surfaceForElement(input)).toBe('tab-tasklist'));
      await waitFor(() => expect(state.events.has('command:deck-local-ui')).toBe(true));
      const send = (workspace_id: string) => state.events.get('command:deck-local-ui')?.({
        commandId: 'workspace.tab.go_to', generation: 'map-a', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
        arguments: { workspace_id, target_mode: 'position', position: 3 },
      });
      await act(async () => { send('workspace-a'); });
      await waitFor(() => expect(state.workspaceStore.setActiveTab).toHaveBeenCalledExactlyOnceWith('tab-editor'));
      await act(async () => { send('workspace-other'); });
      expect(state.workspaceStore.setActiveTab).toHaveBeenCalledOnce();
    } finally { view.unmount(); }
  });

  it('despacha os argumentos do ramo Deck selecionado para o mesmo go_to', async () => {
    const view = render(<MixedWorkspace />);
    try {
      const input = await screen.findByRole('textbox', { name: 'tasklist input' });
      act(() => input.focus());
      await waitFor(() => expect(registeredScope?.surfaceForElement(input)).toBe('tab-tasklist'));
      await waitFor(() => expect(state.events.has('command:deck-local-ui')).toBe(true));
      await act(async () => { state.events.get('command:deck-local-ui')?.({
        commandId: '', generation: 'map-a', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
        conditions: [{ commandId: 'workspace.tab.go_to', bySurface: { tasklist: true, tasklists: true, editor: true }, fallback: false,
          bySurfaceArguments: {
            tasklist: { workspace_id: 'workspace-a', target_mode: 'position', position: 3 },
            tasklists: { workspace_id: 'workspace-a', target_mode: 'position', position: 3 },
            editor: { workspace_id: 'workspace-a', target_mode: 'specific', tab_id: 'tab-chat' },
          } }],
      }); });
      await waitFor(() => expect(state.workspaceStore.setActiveTab).toHaveBeenCalledExactlyOnceWith('tab-editor'));
    } finally { view.unmount(); }
  });

  it('refuses while IME is active in the focused textarea; a real move to the toolbar changes only the composition eligibility', async () => {
    const view = render(<MixedWorkspace />);
    try {
      const input = await screen.findByRole('textbox', { name: 'tasklist input' });
      act(() => input.focus());
      fireEvent.compositionStart(input);
      expect(document.hasFocus()).toBe(true);

      await act(async () => { state.events.get('command:deck-local-ui')?.(payload('workspace.tab.third')); });
      expect(state.workspaceStore.setActiveTab).not.toHaveBeenCalled();

      const menuButton = document.querySelector<HTMLElement>('.topbar button');
      expect(menuButton).not.toBeNull();
      act(() => menuButton!.focus());
      await waitFor(() => expect(document.activeElement).toBe(menuButton));
      await act(async () => { state.events.get('command:deck-local-ui')?.(payload('workspace.tab.third')); });

      await waitFor(() => expect(state.workspaceStore.setActiveTab).toHaveBeenCalledOnce());
      expect(state.workspaceStore.setActiveTab).toHaveBeenCalledWith('tab-editor');
    } finally { view.unmount(); }
  });

  it('drops a Deck press during real window-blur lifecycle and accepts again only after focus reloads the map', async () => {
    const view = render(<MixedWorkspace />);
    try {
      const input = await screen.findByRole('textbox', { name: 'tasklist input' });
      act(() => input.focus());
      await waitFor(() => expect(registeredScope?.surfaceForElement(input)).toBe('tab-tasklist'));
      await waitFor(() => expect(state.map).toHaveBeenCalledOnce());
      await act(async () => { await state.map.mock.results[0]!.value; await Promise.resolve(); });

      act(() => { fireEvent.blur(window); });
      expect(document.hasFocus()).toBe(false);
      await act(async () => { state.events.get('command:deck-local-ui')?.(payload('workspace.tab.second')); });
      expect(state.workspaceStore.setActiveTab).not.toHaveBeenCalled();

      act(() => { fireEvent.focus(window); });
      expect(document.hasFocus()).toBe(true);
      await waitFor(() => expect(state.map).toHaveBeenCalledTimes(2));
      await act(async () => { await state.map.mock.results[1]!.value; await Promise.resolve(); });
      await act(async () => { state.events.get('command:deck-local-ui')?.(payload('workspace.tab.second')); });

      await waitFor(() => expect(state.workspaceStore.setActiveTab).toHaveBeenCalledExactlyOnceWith('tab-chat'));
    } finally { view.unmount(); }
  });
});
