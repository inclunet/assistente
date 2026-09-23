import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Topbar } from './Topbar';
import { CommandContextProvider, useCommandContextScope } from '../../lib/commandContextReact';
import { useLayoutEffect, useRef } from 'react';
import type { SurfaceContextGetter } from '../../lib/commandContextProviders';

const state = vi.hoisted(() => ({
  loadMap: vi.fn(),
  listCatalog: vi.fn(),
  deck: new Map<string, (payload?: unknown) => void>(),
  auth: { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a' } },
  workspace: { workspace: { id: 'workspace-a', profile: 'focused', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] }, workspaces: [] },
  announce: vi.fn(),
  navigate: vi.fn(),
  durable: { begin: vi.fn(), take: vi.fn(), complete: vi.fn(), commit: vi.fn() },
}));

vi.mock('../../lib/commandGlobalOwnershipWails', () => ({
  acquireGlobalCommandOwnership: () => ({ isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve() }),
}));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'pt-BR' } }) }));
vi.mock('react-router-dom', () => ({ useNavigate: () => state.navigate, useLocation: () => ({ pathname: '/', search: '', hash: '', key: '/' }) }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: (query: unknown) => state.listCatalog(query) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({
  createCommandLocalKeyboardWailsPort: () => ({
    loadMap: state.loadMap, beginLocalCommandUIKey: vi.fn(), resetLocalCommandKeyboard: vi.fn(), dispatchLocalCommandKey: vi.fn(),
  }),
}));
vi.mock('../../store/authStore', () => ({ useAuthStore: Object.assign((selector?: (value: typeof state.auth) => unknown) => selector ? selector(state.auth) : state.auth, { getState: () => state.auth, subscribe: () => () => undefined }) }));
vi.mock('../../store/workspaceStore', () => ({
  flushWorkspaceNavigation: vi.fn(async () => true),
  useWorkspaceStore: Object.assign((selector?: (value: typeof state.workspace) => unknown) => selector ? selector(state.workspace) : state.workspace, { getState: () => state.workspace, subscribe: () => () => undefined }),
}));
vi.mock('../../store/shortcutsHelpStore', () => ({ useShortcutsHelpStore: (selector: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) => selector({ isOpen: false, open: vi.fn(), close: vi.fn() }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: vi.fn() }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: vi.fn() }));
vi.mock('../../lib/commandContextReact', async (importOriginal) => await importOriginal<typeof import('../../lib/commandContextReact')>());
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (payload?: unknown) => void) => { state.deck.set(name, callback); return () => state.deck.delete(name); } }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(async () => null) }));
vi.mock('../../lib/commandUIExecutionWails', () => ({ createCommandUIExecutionWailsPort: () => ({ beginUICommand: state.durable.begin, takeUICommand: state.durable.take, completeUICommand: state.durable.complete, getUICommandResult: vi.fn(), cancelUICommand: vi.fn() }) }));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('../../lib/commandWorkspaceTabWails', () => ({ createCommandWorkspaceTabWailsPort: () => ({ beginUICommand: state.durable.begin, takeUICommand: state.durable.take, completeUICommand: state.durable.complete, getUICommandResult: vi.fn(), cancelUICommand: vi.fn(), commitBackendCommand: state.durable.commit }) }));
vi.mock('../pickers/ProfilePicker', () => ({ ProfilePicker: () => null }));

function Source({ getter }: { getter: SurfaceContextGetter }) {
  const scope = useCommandContextScope();
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => scope?.registerSurface('deck-source', root, getter), [scope, getter]);
  return <div ref={root}><button data-testid="deck-source-button">source</button><input data-testid="deck-source-input" /></div>;
}

describe('Topbar — StreamDeck conditions', () => {
  beforeEach(() => {
    state.deck.clear(); state.auth.isAuthenticated = true;
    Object.values(state.durable).forEach(spy => spy.mockReset());
    state.navigate.mockReset();
    state.auth.user = { userId: 'user-a', sessionId: 'session-a' };
    state.workspace.workspace = { id: 'workspace-a', profile: 'focused', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] };
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({ generation: 'deck-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a', bindings: [], localPaletteCommands: [] });
    state.listCatalog.mockResolvedValue([{ id: 'navigation.palette.open', name: 'Palette', available: true }]);
  });

  async function mount(getter: SurfaceContextGetter = () => ({ surfaceType: 'chat', surfaceId: 'deck-source', snapshotVersion: 'surface-1' })) {
    const view = render(<CommandContextProvider><Topbar /><Source getter={getter} /></CommandContextProvider>);
    await waitFor(() => expect(state.deck.has('command:deck-local-ui')).toBe(true));
    const button = screen.getByTestId('deck-source-button');
    button.focus();
    return { view, button, event: state.deck.get('command:deck-local-ui')! };
  }

  const envelope = { commandId: '', generation: 'deck-map', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' };

  it('executa exatamente um comando condicionado no provider visual real, sem ledger', async () => {
    const { view, event } = await mount();
    try {
      act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, byProfile: { focused: { commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false } }, fallback: false }] }));
      await waitFor(() => expect(state.navigate).toHaveBeenCalledWith('/history'));
    } finally { view.unmount(); }
  });

  it('aceita fallback explícito em surface desconhecida e não cria ledger', async () => {
    const { view, event } = await mount(() => ({ surfaceType: 'new-surface', surfaceId: 'deck-source', snapshotVersion: 'surface-unknown' }));
    try {
      act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: {}, fallback: true }] }));
      await waitFor(() => expect(state.navigate).toHaveBeenCalledWith('/history'));
      expect(Object.values(state.durable).some(spy => spy.mock.calls.length > 0)).toBe(false);
    } finally { view.unmount(); }
  });

  it('rejeita provider reentrante que troca owner ou foco durante a captura', async () => {
    let changeOwner = true;
    const ownerCase = await mount(() => {
      if (changeOwner) { changeOwner = false; state.auth.user = { userId: 'other-user', sessionId: 'session-a' }; }
      return { surfaceType: 'chat', surfaceId: 'deck-source', snapshotVersion: 'owner-race' };
    });
    act(() => ownerCase.event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
    expect(state.navigate).not.toHaveBeenCalled();
    ownerCase.view.unmount();

    state.auth.user = { userId: 'user-a', sessionId: 'session-a' };
    let changeFocus = true;
    const focusCase = await mount(() => {
      if (changeFocus) { changeFocus = false; screen.getByTestId('deck-source-input').focus(); }
      return { surfaceType: 'chat', surfaceId: 'deck-source', snapshotVersion: 'focus-race' };
    });
    act(() => focusCase.event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
    expect(state.navigate).not.toHaveBeenCalled();
    focusCase.view.unmount();
  });

  it.each(['surface-mismatch', 'provider-missing', 'focus', 'ime', 'owner', 'generation', 'expiry', 'ambiguous', 'malformed'])('recusa condition Deck: %s', async (failure) => {
    const { view, event } = await mount();
    try {
      if (failure === 'surface-mismatch') {
        act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { editor: true }, fallback: false }] }));
      } else if (failure === 'provider-missing') {
        view.unmount();
        const remounted = await mount(() => null);
        act(() => remounted.event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
        remounted.view.unmount();
      } else if (failure === 'focus') {
        vi.mocked(document.hasFocus).mockReturnValue(false);
        act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
      } else if (failure === 'ime') {
        const input = screen.getByTestId('deck-source-input');
        input.focus();
        fireEvent.compositionStart(input);
        act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
      } else if (failure === 'owner') {
        state.auth.user = { userId: 'other-user', sessionId: 'session-a' };
        act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
      } else if (failure === 'generation') {
        act(() => event({ ...envelope, generation: 'old', conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
      } else if (failure === 'expiry') {
        state.loadMap.mockResolvedValue({ generation: 'deck-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a', validUntil: Date.now() - 1, bindings: [], localPaletteCommands: [] });
        view.unmount();
        const remounted = await mount();
        act(() => remounted.event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }] }));
        remounted.view.unmount();
      } else if (failure === 'ambiguous') {
        act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: true }, fallback: false }, { commandId: 'navigation.settings.open', bySurface: { chat: true }, fallback: false }] }));
      } else {
        act(() => event({ ...envelope, conditions: [{ commandId: 'navigation.history.open', bySurface: { chat: 'yes' }, fallback: false }] }));
      }
      expect(state.navigate).not.toHaveBeenCalled();
      expect(Object.values(state.durable).some(spy => spy.mock.calls.length > 0)).toBe(false);
    } finally { view.unmount(); }
  });
});
