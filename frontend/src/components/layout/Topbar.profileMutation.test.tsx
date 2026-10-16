import { afterEach, beforeEach, expect, it, vi } from 'vitest';
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
import { usePageMutationCommands, type PageMutationID, type PageMutationRequest } from '../../lib/commandPageMutation';

const state = vi.hoisted(() => ({
  pathname: '/tasklists',
  commandId: 'tasklists.update',
  ready: false,
  pageAvailable: true,
  flush: vi.fn(),
  begin: vi.fn(),
  beginKey: vi.fn(),
  prepare: vi.fn(),
  take: vi.fn(),
  commit: vi.fn(),
  result: vi.fn(),
  pageResult: vi.fn(),
  cancel: vi.fn(),
  complete: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  announce: vi.fn(),
  succeeded: vi.fn(),
  noop: () => undefined,
  t: (key: string) => key,
}));

const auth = {
  isAuthenticated: true,
  user: { userId: 'user-a', sessionId: 'session-a', role: 'user' },
};
const workspace = {
  id: 'workspace-a',
  name: 'Workspace',
  activeTabId: 'tasklists-a',
  tabs: [{ id: 'tasklists-a', type: 'chat' }],
};
const reservation = {
  ticket: 'ticket-a',
  invocationId: 'invocation-a',
  commandId: 'tasklists.update',
};
const handoff = { ...reservation, handoffId: 'handoff-a' };
const request: PageMutationRequest = {
  targetId: 'task-a',
  expectedFingerprint: 'fingerprint-a',
  title: 'Lista atualizada',
  description: 'Descrição atualizada',
};
const pageResult = { id: 'task-a', title: 'Lista atualizada' };
const profileRequest: PageMutationRequest = {
  targetId: 'profile-a', expectedFingerprint: 'fingerprint-a',
  title: '', description: '',
  profile: { name: 'Perfil atualizado' } as NonNullable<PageMutationRequest['profile']>,
};
const currentRequest = () => state.commandId.startsWith('profiles.') ? profileRequest : request;
const currentReservation = () => ({ ...reservation, commandId: state.commandId });

vi.mock('react-router-dom', async original => ({
  ...await original<typeof import('react-router-dom')>(),
  useNavigate: () => state.noop,
  useLocation: () => ({ pathname: state.pathname, search: '', hash: '', key: state.pathname }),
}));

vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign(
    (select?: (value: typeof auth) => unknown) => select ? select(auth) : auth,
    { getState: () => auth, subscribe: () => () => undefined },
  ),
}));

vi.mock('../../store/workspaceStore', () => ({
  flushWorkspaceNavigation: state.flush,
  useWorkspaceStore: Object.assign(
    (select?: (value: { workspace: typeof workspace; workspaces: never[]; setActiveTab: () => void }) => unknown) =>
      select ? select({ workspace, workspaces: [], setActiveTab: state.noop }) : { workspace, workspaces: [] },
    { getState: () => ({ workspace, workspaces: [] }), subscribe: () => () => undefined },
  ),
}));

vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (select: (value: { updateConfig: () => void }) => unknown) => select({ updateConfig: state.noop }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: (select: (value: { addToast: () => void }) => unknown) => select({ addToast: state.noop }),
}));

vi.mock('../../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: Object.assign(
    (select?: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) =>
      select ? select({ isOpen: false, open: state.noop, close: state.noop }) : { isOpen: false },
    { getState: () => ({ isOpen: false, open: state.noop, close: state.noop }) },
  ),
}));

vi.mock('../../store/workspaceChatModalStore', () => ({
  canPrepareWorkspaceChatOpen: () => false,
  prepareWorkspaceChatOpen: vi.fn(),
  registerWorkspaceChatCommandDispatcher: () => () => undefined,
}));

vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn() }));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, callback: (payload: unknown) => void) => {
    state.events.set(name, callback);
    return () => {
      if (state.events.get(name) === callback) state.events.delete(name);
    };
  },
}));

vi.mock('react-i18next', async original => ({
  ...await original<typeof import('react-i18next')>(),
  useTranslation: () => ({ t: state.t, i18n: { language: 'en' } }),
}));

vi.mock('../ui/KeyboardShortcutsHelp', () => ({ KeyboardShortcutsHelp: () => null }));
vi.mock('./ConnectionStatusIndicator', () => ({ ConnectionStatusIndicator: () => null }));
vi.mock('../menu', () => ({ Menu: () => null }));
vi.mock('./MenuButton', () => ({ MenuButton: () => null }));
vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
    openForTrigger: state.noop,
    openAtPoint: state.noop,
    closeMenu: state.noop,
    onSelectItem: state.noop,
  }),
}));
vi.mock('../../hooks/useToolbarKeyboardNav', () => ({ useToolbarKeyboardNav: state.noop }));
vi.mock('../../hooks/useAnnouncer', () => ({
  announce: state.announce,
  useAnnouncer: () => ({ announce: state.announce, announceRequest: () => true }),
}));
vi.mock('../../lib/commandContextReact', () => ({ useCommandContextScope: () => null }));

vi.mock('../../services/commandCatalog', () => ({
  listCommandCatalog: vi.fn(async () => [{
    id: state.commandId,
    name: state.commandId.startsWith('profiles.') ? 'Atualizar perfil' : 'Atualizar lista',
    description: state.commandId.startsWith('profiles.') ? 'Atualiza o perfil selecionado' : 'Atualiza a lista selecionada',
    category: state.commandId.startsWith('profiles.') ? 'profiles' : 'tasklists',
    aliases: ['atualizar'],
    risk: 'write',
    available: true,
    availabilityStatus: 'available',
    availabilityReason: '',
    readinessReason: '',
  }]),
}));

vi.mock('../../lib/commandLocalKeyboardWails', () => ({
  createCommandLocalKeyboardWailsPort: () => ({
    loadMap: async () => {
      state.ready = true;
      return {
        generation: 'generation-a',
        ownerId: 'user-a',
        sessionId: 'session-a',
        workspaceId: 'workspace-a',
        localPaletteCommands: [],
        bindings: [{
          shortcut: { version: 1, code: 'KeyU', modifiers: ['Control'] },
          commandId: state.commandId,
          handler: 'contextual',
        }],
      };
    },
    beginLocalCommandUIKey: state.beginKey,
    dispatchLocalCommandKey: vi.fn(),
    resetLocalCommandKeyboard: vi.fn(),
  }),
}));

function port() {
  return {
    beginUICommand: state.begin,
    takeUICommand: state.take,
    completeUICommand: state.complete,
    cancelUICommand: state.cancel,
    getUICommandResult: state.result,
    commitBackendCommand: state.commit,
    preparePageMutationCommand: state.prepare,
    getPageMutationCommandResult: state.pageResult,
  };
}

vi.mock('../../lib/commandPageMutationWails', () => ({
  createPageMutationWailsPort: () => port(),
}));

function PageMutationSurface() {
  const root = useRef<HTMLDivElement>(null);
  const { request: requestMutation } = usePageMutationCommands({
    root,
    pathname: state.pathname,
    allowedCommands: [state.commandId as PageMutationID],
    canStart: () => state.pageAvailable,
    prepare: () => ({
      readRequest: async () => currentRequest(),
      isCurrent: () => true,
      succeeded: async result => state.succeeded(result),
    }),
  });

  return (
    <div ref={root}>
      <input aria-label="Origem da mutação" />
      <button type="button" onClick={() => { void requestMutation(state.commandId as PageMutationID); }}>
        Solicitar mutação nativa
      </button>
    </div>
  );
}

beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.ready = false;
  state.commandId = 'tasklists.update';
  state.pageAvailable = true;
  state.pathname = '/tasklists';
  state.events.clear();
  state.flush.mockResolvedValue(true);
  state.begin.mockImplementation(async (commandId: string) => ({ ...reservation, commandId }));
  state.beginKey.mockImplementation(async () => currentReservation());
  state.prepare.mockResolvedValue(undefined);
  state.take.mockImplementation(async () => ({ ...handoff, commandId: state.commandId }));
  state.commit.mockResolvedValue(undefined);
  state.result.mockResolvedValue({ invocationId: reservation.invocationId, status: 'succeeded' });
  state.pageResult.mockResolvedValue(pageResult);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

async function mount(commandId = 'tasklists.update', pathname = '/tasklists') {
  state.commandId = commandId;
  state.pathname = pathname;
  render(<><Topbar /><PageMutationSurface /></>);
  await waitFor(() => expect(state.ready).toBe(true));
  await act(async () => { await Promise.resolve(); });
}

async function trigger(source: 'native' | 'palette' | 'keyboard' | 'deck') {
  if (source === 'native') {
    await userEvent.setup().click(screen.getByRole('button', { name: 'Solicitar mutação nativa' }));
    return;
  }
  if (source === 'palette') {
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await user.click(await screen.findByRole('option', { name: /Atualizar (lista|perfil)/ }));
    return;
  }
  if (source === 'keyboard') {
    const input = screen.getByRole('textbox', { name: 'Origem da mutação' });
    input.focus();
    fireEvent.keyDown(input, { key: 'u', code: 'KeyU', ctrlKey: true });
    return;
  }
  act(() => { state.events.get('command:deck-ui-reservation')?.(currentReservation()); });
}

async function expectCommitted() {
  await waitFor(() => expect(state.pageResult).toHaveBeenCalledOnce());
  expect(state.prepare).toHaveBeenCalledExactlyOnceWith('ticket-a', currentRequest());
  expect(state.take).toHaveBeenCalledExactlyOnceWith('ticket-a');
  expect(state.commit).toHaveBeenCalledExactlyOnceWith('ticket-a', 'handoff-a');
  expect(state.cancel).not.toHaveBeenCalled();
  expect(state.complete).not.toHaveBeenCalled();
  expect(state.prepare.mock.invocationCallOrder[0]).toBeLessThan(state.take.mock.invocationCallOrder[0]);
  expect(state.take.mock.invocationCallOrder[0]).toBeLessThan(state.commit.mock.invocationCallOrder[0]);
  expect(state.succeeded).toHaveBeenCalledExactlyOnceWith(pageResult);
}

it.each(['profiles.create', 'profiles.update', 'profiles.duplicate', 'profiles.delete', 'profiles.activate'] as const)('encaminha %s por todos os ingressos Topbar', async commandId => {
  for (const source of ['native', 'palette', 'keyboard', 'deck'] as const) {
    await mount(commandId, '/profiles');
    await trigger(source);
    await expectCommitted();
    cleanup();
    vi.clearAllMocks();
    state.events.clear();
    state.pageResult.mockResolvedValue(pageResult);
    state.result.mockResolvedValue({ invocationId: reservation.invocationId, status: 'succeeded' });
  }
});
