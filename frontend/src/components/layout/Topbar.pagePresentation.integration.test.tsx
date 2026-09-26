import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useRef } from 'react';
import { Topbar } from './Topbar';

// This fixture has no native hotkeys. The shared ownership bridge is exercised
// independently with real reservation frames in commandGlobalOwnershipWails tests.
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({
  acquireGlobalCommandOwnership: () => ({
    isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve(),
  }),
}));
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { requestCommandNavigation } from '../../lib/commandNavigation';
import {
  usePagePresentationCommands,
  COMMAND_SETTINGS_CREATE_COMMAND_ID,
  type PagePresentationCommandID,
} from '../../lib/commandPagePresentation';

const ids = [
  'tasklist.task.create.open',
  'tasklists.create.open', 'tasklists.edit.open', 'tasklists.search.focus',
  'profiles.create.open', 'profiles.edit.open', 'profiles.search.focus',
  'terminal.sessions.open', 'terminal.focus.input', 'terminal.focus.history',
] as const satisfies readonly PagePresentationCommandID[];
const palettePreferencesKey = 'assistente.command-palette.v1.user-a.workspace-a';
const shortcuts: Record<PagePresentationCommandID, string> = {
  'command_settings.create.open': 'Ctrl+N',
  'tasklist.task.create.open': 'Ctrl+Shift+N',
  'tasklists.create.open': 'Ctrl+Shift+L, Ctrl+N',
  'tasklists.edit.open': 'Ctrl+Shift+I',
  'tasklists.search.focus': 'Ctrl+Shift+K',
  'profiles.create.open': 'Ctrl+Shift+P, Ctrl+N',
  'profiles.edit.open': 'Ctrl+Shift+E',
  'profiles.search.focus': 'Ctrl+Shift+F',
  'terminal.sessions.open': 'Ctrl+Shift+S',
  'terminal.focus.input': 'Ctrl+Shift+Enter',
  'terminal.focus.history': 'Ctrl+Shift+H',
};
const paletteOptionName = (id: PagePresentationCommandID, unavailable = false) =>
  [id, shortcuts[id], ...(unavailable ? ['commandPalette.unavailable'] : [])].join('. ');

const pathFor = (id: PagePresentationCommandID) => id.startsWith('terminal.') || id === 'tasklist.task.create.open' ? '/' : `/${id.split('.')[0]}`;

const state = vi.hoisted(() => ({
  pathname: '/profiles',
  mapReady: false,
  historySuppressed: false,
  settingsSuppressed: false,
  settingsRemapped: false,
  presentationUnavailable: false,
  actions: [] as string[],
  events: new Map<string, (payload: unknown) => void>(),
  begin: vi.fn(), take: vi.fn(), complete: vi.fn(), cancel: vi.fn(), getResult: vi.fn(), commit: vi.fn(),
  navigate: vi.fn(), announce: vi.fn(), noop: () => undefined,
  t: (key: string) => key,
}));

const auth = { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } };
const workspace = {
  id: 'workspace-a', name: 'Workspace', profile: '', activeTabId: 'tab-a',
  tabs: [{ id: 'tab-a', type: 'chat' as const, conversationId: 'conversation-a' }],
};

vi.mock('react-router-dom', async importOriginal => ({
  ...await importOriginal<typeof import('react-router-dom')>(),
  useNavigate: () => state.navigate,
  useLocation: () => ({ pathname: state.pathname, search: '', hash: '', key: state.pathname }),
}));
vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign(
    (selector?: (value: typeof auth) => unknown) => selector ? selector(auth) : auth,
    { getState: () => auth, subscribe: () => () => undefined },
  ),
}));
vi.mock('../../store/workspaceStore', () => ({
  useActiveTab: () => workspace.tabs[0],
  useWorkspaceTabs: () => workspace.tabs,
  flushWorkspaceNavigation: vi.fn(async () => true),
  useWorkspaceStore: Object.assign(
    (selector?: (value: { workspace: typeof workspace; workspaces: never[]; setActiveTab: () => void }) => unknown) =>
      selector ? selector({ workspace, workspaces: [], setActiveTab: state.noop }) : { workspace, workspaces: [] },
    { getState: () => ({ workspace, workspaces: [] }), subscribe: () => () => undefined },
  ),
}));
vi.mock('../../store/settingsStore', () => ({ useSettingsStore: (selector: (value: { updateConfig: () => void }) => unknown) => selector({ updateConfig: state.noop }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: state.noop }) }));
vi.mock('../../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: Object.assign(
    (selector?: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) =>
      selector ? selector({ isOpen: false, open: state.noop, close: state.noop }) : { isOpen: false },
    { getState: () => ({ isOpen: false, open: state.noop, close: state.noop }) },
  ),
}));
vi.mock('../../store/workspaceChatModalStore', () => ({
  canPrepareWorkspaceChatOpen: () => false,
  prepareWorkspaceChatOpen: vi.fn(),
  registerWorkspaceChatCommandDispatcher: () => () => undefined,
  useWorkspaceChatModalStore: Object.assign(
    (selector?: (value: { isOpen: boolean }) => unknown) => selector ? selector({ isOpen: false }) : { isOpen: false },
    { getState: () => ({ isOpen: false }) },
  ),
}));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, callback: (payload: unknown) => void) => {
    state.events.set(name, callback);
    return () => { if (state.events.get(name) === callback) state.events.delete(name); };
  },
}));
vi.mock('react-i18next', async importOriginal => ({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation: () => ({ t: state.t, i18n: { language: 'en' } }),
}));
vi.mock('../ui/KeyboardShortcutsHelp', () => ({ KeyboardShortcutsHelp: () => null }));
vi.mock('./ConnectionStatusIndicator', () => ({ ConnectionStatusIndicator: () => null }));
vi.mock('../menu', () => ({ Menu: () => null }));
vi.mock('./MenuButton', () => ({ MenuButton: () => null }));
vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({ menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' }, openForTrigger: state.noop, openAtPoint: state.noop, closeMenu: state.noop, onSelectItem: state.noop }),
}));
vi.mock('../../hooks/useToolbarKeyboardNav', () => ({ useToolbarKeyboardNav: state.noop }));
vi.mock('../../hooks/useAnnouncer', () => ({ announce: state.announce, useAnnouncer: () => ({ announce: state.announce, announceRequest: () => true }) }));
vi.mock('../../lib/commandContextReact', () => ({ useCommandContextScope: () => null }));
vi.mock('../../services/commandCatalog', () => ({
  listCommandCatalog: vi.fn(async () => ids.map(id => ({ id, name: id, available: true }))),
}));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({
  createCommandLocalKeyboardWailsPort: () => ({
    loadMap: async () => {
      state.mapReady = true;
      return {
        generation: 'generation-1', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
        localPaletteCommands: [...ids, 'navigation.workspace.open'],
        bindings: [{ shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' }, ...[
          ['KeyP', 'profiles.create.open'], ['KeyE', 'profiles.edit.open'], ['KeyF', 'profiles.search.focus'],
          ['KeyL', 'tasklists.create.open'], ['KeyI', 'tasklists.edit.open'], ['KeyK', 'tasklists.search.focus'],
          ['KeyS', 'terminal.sessions.open'], ['Enter', 'terminal.focus.input'], ['KeyH', 'terminal.focus.history'],
          ['KeyN', state.settingsRemapped ? COMMAND_SETTINGS_CREATE_COMMAND_ID : 'tasklist.task.create.open'],
        ].map(([code, commandId]) => ({ shortcut: { version: 1, code, modifiers: ['Control', 'Shift'] }, commandId, handler: 'local_ui' }))],
        contextualBindings: [{
          shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] },
          bySurface: {
            profiles: { shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] }, commandId: 'profiles.create.open', handler: 'local_ui' },
            tasklists: { shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] }, commandId: 'tasklists.create.open', handler: 'local_ui' },
            history: state.historySuppressed ? null : { shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] }, commandId: 'navigation.workspace.open', handler: 'local_ui' },
          },
          fallback: null,
          fallbackToSequences: true,
          ...(state.pathname === '/settings/commands' ? { bySurface: {}, fallbackToSequences: false, byPage: { settings: {
            shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] },
            bySurface: { toolbar: state.settingsSuppressed ? null : {
              shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] },
              commandId: COMMAND_SETTINGS_CREATE_COMMAND_ID, handler: 'local_ui',
            } }, fallback: null,
          } } } : {}),
        }],
      };
    },
    dispatchLocalCommandKey: vi.fn(async () => null),
    beginLocalCommandUIKey: vi.fn(async () => null),
    resetLocalCommandKeyboard: vi.fn(async () => undefined),
  }),
}));
function transport() {
  return { beginUICommand: state.begin, takeUICommand: state.take, completeUICommand: state.complete, cancelUICommand: state.cancel, getUICommandResult: state.getResult, commitBackendCommand: state.commit };
}
vi.mock('../../lib/commandUIExecutionWails', () => ({ createCommandUIExecutionWailsPort: () => transport() }));
vi.mock('../../lib/commandWorkspaceTabWails', () => ({ createCommandWorkspaceTabWailsPort: () => transport() }));

type SurfaceProps = { api: { request?: (id: PagePresentationCommandID) => boolean } | null };
function PresentationSurface({ api }: SurfaceProps) {
  const root = useRef<HTMLDivElement>(null);
  const target = useRef<{ id: string; version: number }>({ id: 'row-a', version: 0 });
  const version = useRef(0);
  const registered = usePagePresentationCommands({
    root,
    pathname: state.pathname,
    tabId: state.pathname === '/' ? 'tab-a' : undefined,
    allowedCommands: ids,
    readTarget: () => target.current,
    isCurrent: () => state.pathname === '/profiles' || state.pathname === '/tasklists' || state.pathname === '/',
    canOpen: (id) => !state.presentationUnavailable && pathFor(id) === state.pathname,
    open: (id) => {
      state.actions.push(id);
      if (id === 'profiles.search.focus' || id === 'tasklists.search.focus') root.current?.querySelector<HTMLInputElement>('[data-search]')?.focus();
      if (id === 'terminal.focus.input') root.current?.querySelector<HTMLElement>('[data-terminal-input]')?.focus();
      if (id === 'terminal.focus.history') root.current?.querySelector<HTMLElement>('[data-terminal-history]')?.focus();
      return true;
    },
  });
  if (api) api.request = registered.request;
  return <div ref={root}>
    <input data-search aria-label="Busca" />
    <button data-terminal-input aria-label="Entrada" />
    <button data-terminal-history aria-label="Histórico" />
    <button aria-label="Origem" onClick={() => { version.current += 1; target.current = { id: 'row-a', version: version.current }; }}>origem</button>
  </div>;
}

let surfaceApi: SurfaceProps['api'];
function SettingsManagerSurface({ manager = true }: { manager?: boolean }) {
  const root = useRef<HTMLDivElement>(null);
  usePagePresentationCommands({
    root, pathname: '/settings/commands', settingsManager: manager,
    allowedCommands: [COMMAND_SETTINGS_CREATE_COMMAND_ID],
    readTarget: () => 'layer-a:bindings', isCurrent: () => !state.presentationUnavailable,
    canOpen: () => !state.presentationUnavailable,
    open: (id) => { state.actions.push(id); return true; },
  });
  return <div className={manager ? 'modal-overlay' : undefined} data-modal-id={manager ? 'settings-manager-test' : undefined}>
    <button aria-label="Manager header close">close</button>
    <div ref={root}><button aria-label="Manager row">row</button></div>
  </div>;
}
function mount() {
  surfaceApi = {};
  const view = render(<><Topbar /><PresentationSurface api={surfaceApi} /></>);
  return view;
}
async function ready() {
  await waitFor(() => expect(state.mapReady).toBe(true));
  await act(async () => { await Promise.resolve(); });
  // A real presentation request is issued from a focused page source. Keeping
  // focus on the source also exercises ReadFocusContext's ownership guard.
  await userEvent.setup().click(screen.getByRole('button', { name: 'Origem' }));
}
function noTransport() {
  expect(state.begin).not.toHaveBeenCalled();
  expect(state.take).not.toHaveBeenCalled();
  expect(state.complete).not.toHaveBeenCalled();
  expect(state.commit).not.toHaveBeenCalled();
}
function deck(id: PagePresentationCommandID) {
  act(() => state.events.get('command:deck-local-ui')?.({ commandId: id, generation: 'generation-1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
}
async function palette(id: PagePresentationCommandID, keyboard = false) {
  const user = userEvent.setup();
  if (keyboard) await user.keyboard('{Control>}k{/Control}');
  else await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
  const search = await screen.findByRole('combobox');
  await user.type(search, id);
  await user.click(await screen.findByRole('option', { name: paletteOptionName(id) }));
}

beforeEach(() => {
  localStorage.removeItem(palettePreferencesKey);
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.pathname = '/profiles'; state.mapReady = false; state.actions = []; state.events.clear();
  state.historySuppressed = false; state.settingsSuppressed = false; state.settingsRemapped = false; state.presentationUnavailable = false;
  auth.isAuthenticated = true; auth.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
  workspace.id = 'workspace-a'; workspace.activeTabId = 'tab-a';
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.clearAllMocks(); });

describe('Topbar + registry real de apresentação contextual', () => {
  it.each(['allowed', 'page', 'header', 'outside', 'remapped', 'remapped-header', 'remapped-outside',
    'suppressed', 'child-modal', 'ime', 'repeat', 'unavailable', 'logout'] as const)(
    'Ctrl+N do gerenciador usa o mapa central e preserva guardas (%s)', async reason => {
      state.pathname = '/settings/commands';
      state.settingsSuppressed = reason === 'suppressed';
      state.settingsRemapped = reason.startsWith('remapped');
      render(<><Topbar /><SettingsManagerSurface manager={reason !== 'page'} /></>);
      await waitFor(() => expect(state.mapReady).toBe(true));
      await act(async () => { await Promise.resolve(); });
      if (reason !== 'page') registerOpenModal('settings-manager-test');
      const row = screen.getByRole('button', { name: 'Manager row' });
      row.focus();
      if (reason.includes('header')) screen.getByRole('button', { name: 'Manager header close' }).focus();
      const outside = document.createElement('button'); document.body.append(outside);
      if (reason.includes('outside')) outside.focus();
      const child = document.createElement('div'); child.className = 'modal-overlay';
      if (reason === 'child-modal') { document.body.append(child); registerOpenModal('settings-child-test'); }
      if (reason === 'unavailable') state.presentationUnavailable = true;
      if (reason === 'logout') auth.isAuthenticated = false;
      try {
        fireEvent.keyDown(document.activeElement!, { key: 'n', code: 'KeyN', ctrlKey: true, shiftKey: state.settingsRemapped,
          repeat: reason === 'repeat', isComposing: reason === 'ime' });
        expect(state.actions).toEqual(['allowed', 'page', 'header', 'remapped', 'remapped-header'].includes(reason) ? [COMMAND_SETTINGS_CREATE_COMMAND_ID] : []);
        noTransport();
      } finally {
        fireEvent.keyUp(row, { key: 'n', code: 'KeyN', ctrlKey: true });
        unregisterOpenModal('settings-child-test'); child.remove(); outside.remove();
        unregisterOpenModal('settings-manager-test');
      }
    },
  );
  it('pedido do botão de navegação usa dispatcher local e recusa modal', async () => {
    state.pathname = '/history'; mount(); await ready();
    expect(requestCommandNavigation('navigation.workspace.open')).toBe(true);
    expect(state.navigate).toHaveBeenCalledExactlyOnceWith('/');
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay);
    registerOpenModal('navigation-blocker');
    try {
      expect(requestCommandNavigation('navigation.workspace.open')).toBe(false);
      expect(state.navigate).toHaveBeenCalledOnce();
    } finally { unregisterOpenModal('navigation-blocker'); overlay.remove(); }
    noTransport();
  });
  it('Ctrl+N no Histórico retorna ao workspace sem criar conversa ou invocação', async () => {
    state.pathname = '/history'; mount(); await ready();
    fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true });
    expect(state.navigate).toHaveBeenCalledExactlyOnceWith('/');
    expect(state.actions).toEqual([]);
    noTransport();
  });

  it.each(['suppressed', 'repeat', 'ime', 'modal'] as const)('Histórico não tem fallback direto sob %s', async reason => {
    state.pathname = '/history'; state.historySuppressed = reason === 'suppressed';
    mount(); await ready();
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay';
    if (reason === 'modal') { document.body.append(overlay); registerOpenModal('history-blocker'); }
    try {
      fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true, repeat: reason === 'repeat', isComposing: reason === 'ime' });
      expect(state.navigate).not.toHaveBeenCalled();
      expect(state.actions).toEqual([]);
      noTransport();
    } finally { if (reason === 'modal') { unregisterOpenModal('history-blocker'); overlay.remove(); } }
  });

  it.each(ids)('native request executa %s pela mesma ação local', async id => {
    state.pathname = pathFor(id); mount(); await ready();
    expect(surfaceApi?.request?.(id)).toBe(true);
    expect(state.actions).toEqual([id]);
    noTransport();
  });

  it.each([
    ['/profiles', 'KeyP', 'profiles.create.open'], ['/profiles', 'KeyE', 'profiles.edit.open'], ['/profiles', 'KeyF', 'profiles.search.focus'],
    ['/tasklists', 'KeyL', 'tasklists.create.open'], ['/tasklists', 'KeyI', 'tasklists.edit.open'], ['/tasklists', 'KeyK', 'tasklists.search.focus'],
    ['/', 'KeyS', 'terminal.sessions.open'], ['/', 'Enter', 'terminal.focus.input'], ['/', 'KeyH', 'terminal.focus.history'],
    ['/', 'KeyN', 'tasklist.task.create.open'],
  ] as const)('atalho personalizado %s/%s usa a mesma ação', async (path, code, id) => {
    state.pathname = path; mount(); await ready();
    const user = userEvent.setup();
    const key = code === 'Enter' ? '{Enter}' : code.replace('Key', '').toLowerCase();
    await user.keyboard(`{Control>}{Shift>}${key}{/Shift}{/Control}`);
    await waitFor(() => expect(state.actions).toEqual([id]));
    noTransport();
  });

  it.each([
    ['/profiles', 'profiles.create.open'], ['/tasklists', 'tasklists.create.open'],
  ] as const)('Ctrl+N contextual em %s usa fallbackToSequences sem abrir sequência', async (path, id) => {
    state.pathname = path; mount(); await ready();
    fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true });
    expect(state.actions).toEqual([id]);
    noTransport();
  });

  it('Ctrl+N na página aceita body focado quando a captura da apresentação continua válida', async () => {
    state.pathname = '/profiles'; mount(); await ready();
    const body = document.body;
    const previousTabIndex = body.getAttribute('tabindex');
    body.setAttribute('tabindex', '-1'); body.focus();
    try {
      expect(document.activeElement).toBe(body);
      fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true });
      expect(state.actions).toEqual(['profiles.create.open']);
      noTransport();
    } finally {
      if (previousTabIndex === null) body.removeAttribute('tabindex');
      else body.setAttribute('tabindex', previousTabIndex);
    }
  });

  it.each(['modal', 'unavailable-source'] as const)('body sem controle não contorna captura inválida (%s)', async reason => {
    state.pathname = '/profiles'; mount(); await ready();
    if (reason === 'unavailable-source') state.presentationUnavailable = true;
    const body = document.body;
    const previousTabIndex = body.getAttribute('tabindex');
    body.setAttribute('tabindex', '-1'); body.focus();
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay';
    if (reason === 'modal') { document.body.append(overlay); registerOpenModal('body-page-blocker'); }
    try {
      fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true });
      expect(state.actions).toEqual([]);
      noTransport();
    } finally {
      if (reason === 'modal') { unregisterOpenModal('body-page-blocker'); overlay.remove(); }
      if (previousTabIndex === null) body.removeAttribute('tabindex');
      else body.setAttribute('tabindex', previousTabIndex);
    }
  });

  it('body focado não estende a exceção a atalhos locais que não sejam apresentação de página', async () => {
    state.pathname = '/profiles'; mount(); await ready();
    const body = document.body;
    const previousTabIndex = body.getAttribute('tabindex');
    body.setAttribute('tabindex', '-1'); body.focus();
    try {
      fireEvent.keyDown(window, { key: 'k', code: 'KeyK', ctrlKey: true });
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
      expect(state.actions).toEqual([]);
      noTransport();
    } finally {
      if (previousTabIndex === null) body.removeAttribute('tabindex');
      else body.setAttribute('tabindex', previousTabIndex);
    }
  });

  it.each(ids)('Deck %s usa a mesma ação local', async id => {
    state.pathname = pathFor(id);
    mount(); await ready(); deck(id);
    await waitFor(() => expect(state.actions).toEqual([id]));
    noTransport();
  });

  it.each(ids)('paleta captura a origem antes da busca para %s', async id => {
    state.pathname = pathFor(id); mount(); await ready();
    await palette(id);
    await waitFor(() => expect(state.actions).toEqual([id]));
    noTransport();
  });

  it.each(ids)('paleta via Ctrl+K conserva a origem para %s', async id => {
    state.pathname = pathFor(id); mount(); await ready();
    await palette(id, true);
    await waitFor(() => expect(state.actions).toEqual([id]));
    noTransport();
  });

  it.each(['selection-aba', 'route', 'modal', 'modal-aba', 'owner'] as const)('recusa apresentação após mudança de %s', async change => {
    const view = mount(); await ready();
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await user.type(await screen.findByRole('combobox'), 'profiles.create.open');
    if (change === 'selection-aba') fireEvent.click(screen.getByRole('button', { name: 'Origem' }));
    if (change === 'route') {
      state.pathname = '/tasklists';
      view.rerender(<><Topbar /><PresentationSurface api={surfaceApi} /></>);
      await waitFor(() => expect(screen.queryByRole('combobox')).not.toBeInTheDocument());
      expect(state.actions).toEqual([]);
      noTransport();
      return;
    }
    if (change === 'owner') auth.user = { userId: 'user-b', sessionId: 'session-b', role: 'user' };
    const overlay = document.createElement('div');
    if (change === 'modal' || change === 'modal-aba') {
      overlay.className = 'modal-overlay';
      document.body.append(overlay);
      registerOpenModal('test-modal');
      if (change === 'modal-aba') { unregisterOpenModal('test-modal'); overlay.remove(); }
    }
    const option = await screen.findByRole('option', { name: paletteOptionName('profiles.create.open') });
    await user.click(option);
    await act(async () => { await new Promise(resolve => requestAnimationFrame(resolve)); });
    expect(state.actions).toEqual([]);
    if (change === 'modal') { unregisterOpenModal('test-modal'); overlay.remove(); }
    noTransport();
  });
});
