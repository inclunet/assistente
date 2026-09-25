import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useLayoutEffect, useRef } from 'react';
import { Topbar } from './Topbar';
import { CommandContextProvider, useCommandContextScope, type CommandContextScope } from '../../lib/commandContextReact';
import { usePageMutationCommands, type PageMutationID } from '../../lib/commandPageMutation';
import { CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS, isContextualPagePaletteCommand } from '../../lib/commandContextualPalette';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import TaskListsPage from '../../pages/TaskListsPage';
import { DecisionDialog } from '../ui/DecisionDialog';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import type { TaskListWithWorkflow } from '../../types/tasklist';

const state = vi.hoisted(() => ({
  commandId: 'tasklists.duplicate' as PageMutationID, pathname: '/tasklists', targetRevision: 0,
  loadMap: vi.fn(), catalog: vi.fn(), navigate: vi.fn(), announce: vi.fn(), readRequest: vi.fn(), succeeded: vi.fn(),
  listeners: new Set<() => void>(), events: new Map<string, (payload?: unknown) => void>(),
  auth: { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a' } },
  workspace: { workspace: { id: 'workspace-a', profile: 'focused', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'tasklist' }] }, workspaces: [] },
  taskLists: new Map<string, TaskListWithWorkflow>(), fetchLists: vi.fn(async () => []), loadList: vi.fn(), addToast: vi.fn(),
}));
vi.mock('react-i18next', () => ({ initReactI18next: { type: '3rdParty', init: () => {} }, useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'pt-BR' } }) }));
vi.mock('react-router-dom', () => ({ useNavigate: () => state.navigate, useLocation: () => ({ pathname: state.pathname, search: '', hash: '', key: state.pathname }) }));
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({ acquireGlobalCommandOwnership: () => ({ isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve() }) }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: () => state.catalog() }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({ loadMap: state.loadMap, beginLocalCommandUIKey: vi.fn(), resetLocalCommandKeyboard: vi.fn(), dispatchLocalCommandKey: vi.fn() }) }));
vi.mock('../../store/authStore', () => ({ useAuthStore: Object.assign((selector?: (value: typeof state.auth) => unknown) => selector ? selector(state.auth) : state.auth, { getState: () => state.auth, subscribe: (listener: () => void) => { state.listeners.add(listener); return () => { state.listeners.delete(listener); }; } }) }));
vi.mock('../../store/workspaceStore', () => ({ flushWorkspaceNavigation: vi.fn(async () => true), useWorkspaceStore: Object.assign((selector?: (value: typeof state.workspace) => unknown) => selector ? selector(state.workspace) : state.workspace, { getState: () => state.workspace, subscribe: (listener: () => void) => { state.listeners.add(listener); return () => { state.listeners.delete(listener); }; } }) }));
vi.mock('../../store/shortcutsHelpStore', () => ({ useShortcutsHelpStore: (selector: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) => selector({ isOpen: false, open: vi.fn(), close: vi.fn() }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: state.addToast }) }));
vi.mock('../../store/taskListStore', () => ({ useTaskListStore: Object.assign((selector?: (value: unknown) => unknown) => {
  const value = { taskLists: state.taskLists, fetchAllTaskLists: state.fetchLists, loadTaskList: state.loadList, getCachedTaskList: (id: string) => state.taskLists.get(id) };
  return selector ? selector(value) : value;
}, { getState: () => ({ taskLists: state.taskLists }), subscribe: () => () => undefined }) }));
// The page, selection callbacks, mutation hook/provider and executor are real;
// this lightweight grid only replaces rendering, never command eligibility.
vi.mock('../ui/DataGrid', () => ({ DataGrid: ({ items, onFocusChange }: { items: TaskListWithWorkflow[]; onFocusChange: (item: TaskListWithWorkflow) => void }) =>
  <div>{items.map(item => <button key={item.id} data-testid={`real-list-${item.id}`} onFocus={() => onFocusChange(item)}>{item.title}</button>)}</div> }));
vi.mock('../../hooks/useGridFocus', () => ({ useGridFocus: () => ({ handleGridReady: () => undefined }) }));
vi.mock('../../hooks/useGridPageLandmarks', () => ({ useGridPageLandmarks: () => undefined }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce, announceRequest: vi.fn() }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: vi.fn() }));
vi.mock('../../store/workspaceChatModalStore', () => ({ canPrepareWorkspaceChatOpen: () => false, registerWorkspaceChatCommandDispatcher: () => () => {}, prepareWorkspaceChatOpen: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (payload?: unknown) => void) => { state.events.set(name, callback); return () => { state.events.delete(name); }; } }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(async () => null) }));
vi.mock('../pickers/ProfilePicker', () => ({ ProfilePicker: () => null }));

const api = { BeginUICommand: vi.fn(), BeginContextualPaletteUICommand: vi.fn(), BeginContextualPagePaletteUICommand: vi.fn(),
  BeginContextualDeckPageUICommand: vi.fn(), BeginContextualDeckUICommand: vi.fn(),
  TakeUICommand: vi.fn(), CompleteUICommand: vi.fn(), CancelUICommand: vi.fn(), GetUICommandResult: vi.fn(),
  PreparePageMutationCommand: vi.fn(), CommitWorkspaceTabCommand: vi.fn(), GetPageMutationCommandResult: vi.fn(), ReadTaskListCommandTarget: vi.fn() };
const request = { targetId: 'selected-a', expectedFingerprint: 'fingerprint-a', title: 'Selected', description: '' };
const reservation = () => ({ ticket: 'ticket', invocationId: 'invocation', commandId: state.commandId });
const handoff = () => ({ ...reservation(), handoffId: 'handoff' });
let observedScope: CommandContextScope | null = null;
function Page({ rootVersion = 0 }: { rootVersion?: number }) {
  const root = useRef<HTMLDivElement>(null);
  usePageMutationCommands({ root, pathname: state.pathname, tabId: state.pathname === '/' ? 'tab-a' : undefined,
    allowedCommands: [state.commandId], canStart: () => true,
    prepare: () => { const version = state.targetRevision; return {
      readRequest: state.readRequest, isCurrent: () => version === state.targetRevision, succeeded: state.succeeded,
    }; },
  });
  return <div key={rootVersion} ref={root}><input data-testid="page-source" /><button data-testid="page-deck-source">Page source</button></div>;
}
function WorkspacePage({ rootVersion = 0 }: { rootVersion?: number }) {
  const scope = useCommandContextScope(); observedScope = scope;
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => state.pathname === '/' ? scope?.registerSurface('tab-a', root, () => ({ surfaceId: 'tab-a', surfaceType: 'tasklist', snapshotVersion: 'workspace-provider' })) : undefined, [scope]);
  return <div ref={root}><Page rootVersion={rootVersion} /></div>;
}
function view(rootVersion = 0) { return <CommandContextProvider><Topbar /><WorkspacePage rootVersion={rootVersion} /></CommandContextProvider>; }
function changeProfile() {
  state.workspace.workspace.profile = 'other'; state.listeners.forEach(fn => fn());
  state.workspace.workspace.profile = 'focused'; state.listeners.forEach(fn => fn());
}
function setupProjection(surfaceType = state.commandId.startsWith('profiles.') ? 'profiles' : 'tasklists') {
  state.loadMap.mockResolvedValue({ generation: 'page-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    bindings: [{ shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' }],
    contextualPaletteConditions: [{ commandId: state.commandId, bySurface: {}, fallback: false, byProfile: {
      focused: { commandId: state.commandId, bySurface: { [surfaceType]: true }, fallback: false },
    } }],
  });
}
async function mount() {
  const rendered = render(view());
  await act(async () => { await Promise.resolve(); });
  const source = screen.getByTestId('page-source'); source.focus();
  const surfaceId = observedScope?.surfaceForElement(source);
  return { rendered, source, surfaceId, user: userEvent.setup() };
}
async function open() {
  const mounted = await mount();
  await mounted.user.keyboard('{Control>}k{/Control}');
  const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
  await waitFor(() => expect(search).toHaveFocus());
  const option = await screen.findByRole('option', { name: /Page action/ });
  return { ...mounted, option };
}
function emitDeckPage(conditions?: unknown) {
  const type = state.pathname === '/' ? 'tasklist' : state.commandId.startsWith('profiles.') ? 'profiles' : 'tasklists';
  state.events.get('command:deck-contextual-ui')?.({ offerId: 'page-offer', generation: 'page-map', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    conditions: conditions ?? [{ commandId: state.commandId, bySurface: {}, fallback: false, byProfile: {
      focused: { commandId: state.commandId, bySurface: { [type]: true }, fallback: false },
    } }],
  });
}

describe('Page palette — production hook/provider, executor and Wails port', () => {
  beforeEach(() => {
    state.commandId = 'tasklists.duplicate'; state.pathname = '/tasklists'; state.targetRevision = 0;
    state.auth.isAuthenticated = true; state.auth.user = { userId: 'user-a', sessionId: 'session-a' };
    state.workspace.workspace.profile = 'focused'; state.workspace.workspace.activeTabId = 'tab-a';
    state.listeners.clear(); state.events.clear(); state.succeeded.mockReset();
    state.fetchLists.mockClear(); state.addToast.mockClear(); state.announce.mockClear();
    Object.values(api).forEach(spy => spy.mockReset());
    api.BeginUICommand.mockImplementation(async () => reservation());
    api.BeginContextualPagePaletteUICommand.mockImplementation(async () => reservation());
    api.BeginContextualDeckPageUICommand.mockImplementation(async () => reservation());
    api.TakeUICommand.mockImplementation(async () => handoff());
    api.PreparePageMutationCommand.mockResolvedValue(undefined); api.CommitWorkspaceTabCommand.mockResolvedValue(undefined);
    api.GetUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'succeeded' });
    api.GetPageMutationCommandResult.mockResolvedValue({ id: 'selected-a', title: 'Selected' });
    state.readRequest.mockReset(); state.readRequest.mockResolvedValue(request);
    state.catalog.mockReset(); state.catalog.mockImplementation(async () => [{ id: state.commandId, name: 'Page action', available: true }]);
    Object.assign(window, { go: { app: { App: api } } });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true); setupProjection();
  });
  afterEach(() => { unregisterOpenModal('page-decision'); Reflect.deleteProperty(window, 'go'); vi.restoreAllMocks(); });

  it.each(['restored', 'other-control', 'route', 'selection', 'ime'])('Deck decision uses real dialog and deferred Take: %s', async mode => {
    state.commandId = 'tasklists.clear'; setupProjection();
    const { rendered, user } = await mount();
    const source = screen.getByTestId('page-deck-source'); source.focus();
    let resolveTake!: (value: ReturnType<typeof handoff>) => void;
    api.TakeUICommand.mockImplementation(() => new Promise(resolve => { resolveTake = resolve; }));
    let dialog: ReturnType<typeof render> | undefined;
    const content = (isOpen: boolean) => <DecisionDialog isOpen={isOpen} title="Clear selected tasklist" description="Confirm destructive action"
      actions={[{ id: 'confirm', label: 'Confirm clear', primary: true, polarity: 'affirmative' }]}
      onAction={() => dialog?.rerender(content(false))} onCancel={() => dialog?.rerender(content(false))}
      dialogCommandScope={{ dialogId: 'page-decision', kind: 'decision', generation: '1', allowedCommandIds: ['decision.respond'], allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'] }} />;
    vi.mocked(restoreDefaultFocus).mockImplementation(() => { source.focus(); return true; });
    api.PreparePageMutationCommand.mockImplementation(async () => { dialog = render(content(true)); });
    try {
      await act(async () => emitDeckPage());
      const modal = await screen.findByRole('alertdialog');
      await waitFor(() => expect(modal.contains(document.activeElement)).toBe(true));
      expect(api.TakeUICommand).toHaveBeenCalledOnce();
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
      expect(api.CancelUICommand).not.toHaveBeenCalled();
      await user.click(screen.getByRole('button', { name: 'Confirm clear' }));
      await waitFor(() => expect(document.activeElement).toBe(source));
      expect(screen.queryByRole('alertdialog')).toBeNull();
      if (mode === 'other-control') screen.getByTestId('page-source').focus();
      if (mode === 'route') { state.pathname = '/profiles'; rendered.rerender(view()); }
      if (mode === 'selection') state.targetRevision++;
      if (mode === 'ime') { const input = screen.getByTestId('page-source'); input.focus(); fireEvent.compositionStart(input); }
      await act(async () => resolveTake(handoff()));
      if (mode === 'restored') {
        await waitFor(() => expect(state.succeeded).toHaveBeenCalledOnce());
        expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
      } else {
        await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalled());
        expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
        expect(state.succeeded).not.toHaveBeenCalled();
      }
      expect(api.BeginContextualDeckPageUICommand).toHaveBeenCalledOnce();
      expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled();
    } finally { fireEvent.compositionEnd(screen.getByTestId('page-source')); dialog?.unmount(); rendered.unmount(); vi.mocked(restoreDefaultFocus).mockReset(); }
  });

  it.each(CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS)('Deck %s uses actual page hook/provider, not the background tab', async id => {
    state.commandId = id; state.pathname = id.startsWith('profiles.') ? '/profiles' : '/tasklists'; setupProjection();
    const { rendered } = await mount();
    try {
      screen.getByTestId('page-deck-source').focus();
      const surfaceId = observedScope?.surfaceForElement(document.activeElement);
      expect(surfaceId).toBeTruthy(); expect(surfaceId).not.toBe('tab-a');
      await act(async () => emitDeckPage());
      await waitFor(() => expect(state.succeeded).toHaveBeenCalledOnce());
      expect(api.BeginContextualDeckPageUICommand).toHaveBeenCalledExactlyOnceWith('page-offer', 'page-map', state.pathname.slice(1), 'focused');
      expect(api.PreparePageMutationCommand).toHaveBeenCalledExactlyOnceWith('ticket', request);
      expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
      expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled(); expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it.each(['tasklists.duplicate', 'tasklists.clear', 'tasklists.delete'] as const)('Deck workspace tasklist %s keeps canonical tab provider', async id => {
    state.commandId = id; state.pathname = '/'; setupProjection('tasklist');
    const { rendered } = await mount();
    try {
      screen.getByTestId('page-deck-source').focus();
      expect(observedScope?.surfaceForElement(document.activeElement)).toBe('tab-a');
      await act(async () => emitDeckPage());
      if (id === 'tasklists.delete') expect(api.BeginContextualDeckPageUICommand).not.toHaveBeenCalled();
      else {
        await waitFor(() => expect(state.succeeded).toHaveBeenCalledOnce());
        expect(api.BeginContextualDeckPageUICommand).toHaveBeenCalledExactlyOnceWith('page-offer', 'page-map', 'tasklist', 'focused');
      }
      expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it.each(['selection-read', 'profile-read', 'selection-prepare', 'profile-take', 'root-take', 'session-take', 'ime-take', 'modal-take', 'expiry-take', 'missing-api', 'mismatch'])('Deck page rejects %s without fallback/Commit', async mode => {
    const deadline = Date.now() + 60000; (await state.loadMap()).validUntil = deadline;
    const { rendered } = await mount(); const overlay = document.createElement('div'); overlay.className = 'modal-overlay';
    const begin = api.BeginContextualDeckPageUICommand;
    try {
      screen.getByTestId('page-deck-source').focus();
      state.readRequest.mockImplementation(async () => {
        if (mode === 'selection-read') state.targetRevision++;
        if (mode === 'profile-read') changeProfile();
        return request;
      });
      api.PreparePageMutationCommand.mockImplementation(async () => { if (mode === 'selection-prepare') state.targetRevision++; });
      api.TakeUICommand.mockImplementation(async () => {
        if (mode === 'profile-take') changeProfile();
        if (mode === 'session-take') state.auth.user = { userId: 'user-a', sessionId: 'other' };
        if (mode === 'ime-take') { screen.getByTestId('page-source').focus(); fireEvent.compositionStart(screen.getByTestId('page-source')); }
        if (mode === 'modal-take') { document.body.append(overlay); registerOpenModal('page-decision'); }
        if (mode === 'expiry-take') vi.spyOn(Date, 'now').mockReturnValue(deadline);
        if (mode === 'root-take') rendered.rerender(view(1));
        return handoff();
      });
      if (mode === 'missing-api') Reflect.deleteProperty(api, 'BeginContextualDeckPageUICommand');
      if (mode === 'mismatch') api.BeginContextualDeckPageUICommand.mockResolvedValue({ ...reservation(), commandId: 'profiles.delete' });
      await act(async () => emitDeckPage());
      if (mode.endsWith('-read') || mode === 'missing-api') await waitFor(() => expect(state.readRequest).toHaveBeenCalledOnce());
      else await waitFor(() => expect(api.CancelUICommand.mock.calls.length + api.CompleteUICommand.mock.calls.length).toBeGreaterThan(0));
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(state.succeeded).not.toHaveBeenCalled();
      expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled(); expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { api.BeginContextualDeckPageUICommand = begin; unregisterOpenModal('page-decision'); overlay.remove(); rendered.unmount(); }
  });

  it('Deck profile activation self publication preserves succeeded without fallback or failure', async () => {
    state.commandId = 'profiles.activate'; state.pathname = '/profiles'; setupProjection();
    api.CommitWorkspaceTabCommand.mockImplementation(async () => { changeProfile(); state.events.get('command:keyboard-map-changed')?.(); });
    const { rendered } = await mount();
    try {
      screen.getByTestId('page-deck-source').focus();
      await act(async () => emitDeckPage());
      await waitFor(() => expect(api.GetPageMutationCommandResult).toHaveBeenCalledOnce());
      expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce(); expect(api.BeginContextualDeckPageUICommand).toHaveBeenCalledOnce();
      expect(api.CompleteUICommand).not.toHaveBeenCalled(); expect(api.CancelUICommand).not.toHaveBeenCalled();
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionFailed'); expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionUnknown');
      expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it.each(CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS)('%s uses the real standalone provider and page-specific Begin', async id => {
    state.commandId = id; state.pathname = id.startsWith('profiles.') ? '/profiles' : '/tasklists'; setupProjection();
    const { rendered, user, surfaceId, option } = await open();
    try {
      expect(surfaceId).toBeTruthy(); expect(surfaceId).not.toBe('command-toolbar');
      expect(option).not.toHaveAttribute('aria-disabled', 'true');
      await user.keyboard('{Enter}');
      await waitFor(() => expect(state.succeeded).toHaveBeenCalledOnce());
      expect(api.BeginContextualPagePaletteUICommand).toHaveBeenCalledExactlyOnceWith('page-map', id, id.startsWith('profiles.') ? 'profiles' : 'tasklists', 'focused');
      expect(api.PreparePageMutationCommand).toHaveBeenCalledExactlyOnceWith('ticket', request);
      expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
      expect(api.BeginUICommand).not.toHaveBeenCalled(); expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it.each(['accepted', 'empty', 'cancelled', 'selection-drift', 'fingerprint-read-drift', 'deck-accepted', 'deck-empty', 'deck-cancelled', 'deck-selection-drift', 'deck-fingerprint-read-drift'])('real TaskListsPage clear: %s', async scenario => {
    const deck = scenario.startsWith('deck-'); const mode = scenario.replace(/^deck-/, '');
    state.commandId = 'tasklists.clear'; setupProjection();
    const list: TaskListWithWorkflow = { id: 'selected-a', title: 'Lista selecionada', description: 'Description', taskCount: mode === 'empty' ? 0 : 3,
      tasks: [], preferredViewMode: 'list', createdAt: '', updatedAt: '',
      workflow: { id: 'workflow', taskListId: 'selected-a', statuses: [], allowedTransitions: {}, initialStatusId: 1, createdAt: '', updatedAt: '' } };
    state.taskLists = new Map([[list.id, list], ['other', { ...list, id: 'other', title: 'Outra lista' }]]);
    api.ReadTaskListCommandTarget.mockImplementation(async () => {
      if (mode === 'fingerprint-read-drift') state.taskLists.set(list.id, { ...list });
      return { taskList: list, fingerprint: 'real-list-fingerprint' };
    });
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay';
    let resolveTake!: (value: ReturnType<typeof handoff>) => void;
    let rejectTake!: (error: Error) => void;
    api.TakeUICommand.mockReturnValue(new Promise<ReturnType<typeof handoff>>((resolve, reject) => { resolveTake = resolve; rejectTake = reject; }));
    api.PreparePageMutationCommand.mockImplementation(async () => {
      document.body.append(overlay);
      registerOpenModal('page-decision', { dialogId: 'page-decision', kind: 'decision', generation: '1', allowedCommandIds: ['decision.respond'], allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'] });
    });
    const rendered = render(<CommandContextProvider><Topbar /><TaskListsPage /></CommandContextProvider>);
    const user = userEvent.setup();
    try {
      await act(async () => { await Promise.resolve(); });
      act(() => screen.getByTestId('real-list-selected-a').focus());
      const fetchCount = state.fetchLists.mock.calls.length;
      if (deck) await act(async () => emitDeckPage());
      else {
        await user.keyboard('{Control>}k{/Control}');
        const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
        await waitFor(() => expect(search).toHaveFocus());
        const option = await screen.findByRole('option', { name: /Page action/ });
        if (mode === 'empty') expect(option).toHaveAttribute('aria-disabled', 'true');
        else expect(option).not.toHaveAttribute('aria-disabled', 'true');
        await user.keyboard('{Enter}');
      }
      if (mode === 'empty') {
        expect(api.ReadTaskListCommandTarget).not.toHaveBeenCalled(); expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled();
        expect(api.BeginContextualDeckPageUICommand).not.toHaveBeenCalled(); return;
      }
      if (mode === 'fingerprint-read-drift') {
        await waitFor(() => expect(api.ReadTaskListCommandTarget).toHaveBeenCalledExactlyOnceWith(list.id));
        expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); return;
      }
      await waitFor(() => expect(api.TakeUICommand).toHaveBeenCalledOnce());
      expect(api.PreparePageMutationCommand).toHaveBeenCalledExactlyOnceWith('ticket', { targetId: list.id, expectedFingerprint: 'real-list-fingerprint', title: '', description: 'Description' });
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
      if (mode === 'accepted') {
        state.fetchLists.mockImplementationOnce(async () => {
          state.taskLists = new Map(state.taskLists).set(list.id, { ...list, taskCount: 0, tasks: [] });
          return [];
        });
      }
      await act(async () => {
        unregisterOpenModal('page-decision'); overlay.remove();
        if (mode === 'selection-drift') screen.getByTestId('real-list-other').focus();
        if (mode === 'cancelled') { api.GetUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'cancelled' }); rejectTake(new Error('decision-cancelled')); }
        else resolveTake(handoff());
      });
      if (mode === 'accepted') {
        await waitFor(() => expect(state.announce).toHaveBeenCalledWith('tasklist.clearedSuccess'));
        expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
        expect(state.fetchLists).toHaveBeenCalledTimes(fetchCount + 1);
        expect(state.taskLists.get(list.id)).not.toBe(list);
        expect(state.taskLists.get(list.id)?.taskCount).toBe(0);
        expect(screen.getByTestId('real-list-selected-a')).toBeInTheDocument();
        expect(state.announce).not.toHaveBeenCalledWith('tasklist.deletedSuccess');
      } else {
        await waitFor(() => expect(api.CompleteUICommand.mock.calls.length + api.CancelUICommand.mock.calls.length).toBeGreaterThan(0));
        expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(state.fetchLists).toHaveBeenCalledTimes(fetchCount);
        expect(state.announce).not.toHaveBeenCalledWith('tasklist.clearedSuccess');
      }
      expect(api.BeginUICommand).not.toHaveBeenCalled(); expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
      if (deck) {
        expect(api.BeginContextualDeckPageUICommand).toHaveBeenCalledExactlyOnceWith('page-offer', 'page-map', 'tasklists', 'focused');
        expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
      }
    } finally { unregisterOpenModal('page-decision'); overlay.remove(); rendered.unmount(); }
  });

  it.each(['tasklists.duplicate', 'tasklists.clear', 'tasklists.delete'] as const)('workspace tasklist keeps the original provider for %s', async id => {
    state.commandId = id; state.pathname = '/'; setupProjection('tasklist');
    const { rendered, user, surfaceId, option } = await open();
    try {
      expect(surfaceId).toBe('tab-a');
      expect(observedScope?.session.readSurfaceContext('tab-a')?.snapshotVersion).toBe('workspace-provider');
      await user.keyboard('{Enter}');
      if (id === 'tasklists.delete') {
        expect(option).toHaveAttribute('aria-disabled', 'true'); expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled();
      } else await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce());
    } finally { rendered.unmount(); }
  });

  it.each(['readRequest', 'prepare', 'take'])('profile ABA during %s never submits Commit', async phase => {
    if (phase === 'readRequest') state.readRequest.mockImplementation(async () => { changeProfile(); return request; });
    if (phase === 'prepare') api.PreparePageMutationCommand.mockImplementation(async () => { changeProfile(); });
    if (phase === 'take') api.TakeUICommand.mockImplementation(async () => { changeProfile(); return handoff(); });
    const { rendered, user } = await open();
    try {
      await user.keyboard('{Enter}');
      if (phase === 'readRequest') { await waitFor(() => expect(state.readRequest).toHaveBeenCalled()); expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); }
      else await waitFor(() => expect(api.CompleteUICommand.mock.calls.length + api.CancelUICommand.mock.calls.length).toBeGreaterThan(0));
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(state.succeeded).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it.each(['owner', 'selection', 'root', 'reload'])('invalidates %s while picker retains the old source', async drift => {
    const { rendered, user } = await open();
    try {
      await act(async () => {
        if (drift === 'owner') { state.auth.user = { userId: 'other', sessionId: 'session-a' }; state.listeners.forEach(fn => fn()); }
        if (drift === 'selection') state.targetRevision++;
        if (drift === 'root') rendered.rerender(view(1));
        if (drift === 'reload') state.events.get('command:keyboard-map-changed')?.();
      });
      await user.keyboard('{Enter}');
      expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it('does not re-present a succeeded Commit into a changed selection', async () => {
    api.CommitWorkspaceTabCommand.mockImplementation(async () => { state.targetRevision++; });
    const { rendered, user } = await open();
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.GetPageMutationCommandResult).toHaveBeenCalledOnce());
      expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce(); expect(state.succeeded).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it.each(['accepted', 'cancelled', 'profile-aba', 'selection'])('destructive decision %s preserves the prepared target and final guards', async mode => {
    state.commandId = 'tasklists.delete'; setupProjection();
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay';
    let resolveTake!: (value: ReturnType<typeof handoff>) => void;
    let rejectTake!: (error: Error) => void;
    const pendingTake = new Promise<ReturnType<typeof handoff>>((resolve, reject) => { resolveTake = resolve; rejectTake = reject; });
    api.PreparePageMutationCommand.mockImplementation(async () => {
      document.body.append(overlay);
      registerOpenModal('page-decision', { dialogId: 'page-decision', kind: 'decision', generation: '1', allowedCommandIds: ['decision.respond'], allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'] });
    });
    api.TakeUICommand.mockReturnValue(pendingTake);
    const { rendered, user } = await open();
    try {
      await user.keyboard('{Enter}'); await waitFor(() => expect(api.TakeUICommand).toHaveBeenCalledOnce());
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
      await act(async () => {
        if (mode === 'profile-aba') changeProfile();
        if (mode === 'selection') state.targetRevision++;
        unregisterOpenModal('page-decision'); overlay.remove();
        if (mode === 'cancelled') {
          api.GetUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'cancelled' });
          rejectTake(new Error('decision-cancelled'));
        } else resolveTake(handoff());
      });
      if (mode === 'accepted') await waitFor(() => expect(state.succeeded).toHaveBeenCalledOnce());
      else {
        await waitFor(() => expect(api.CompleteUICommand.mock.calls.length + api.CancelUICommand.mock.calls.length).toBeGreaterThan(0));
        expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(state.succeeded).not.toHaveBeenCalled();
      }
      expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { unregisterOpenModal('page-decision'); overlay.remove(); rendered.unmount(); }
  });

  it.each(['surface-id', 'nested-surface-id', 'wrong-domain', 'profile'])('fails closed on forbidden/mismatched projection %s', async kind => {
    const map = await state.loadMap();
    if (kind === 'surface-id') map.contextualPaletteConditions[0].bySurfaceId = {};
    if (kind === 'nested-surface-id') map.contextualPaletteConditions[0].byProfile.focused.bySurfaceId = {};
    if (kind === 'wrong-domain') map.contextualPaletteConditions[0].byProfile.focused.bySurface = { profiles: true };
    if (kind === 'profile') state.workspace.workspace.profile = 'other';
    state.loadMap.mockResolvedValue(map);
    const { rendered, user, option } = await open();
    try {
      expect(option).toHaveAttribute('aria-disabled', 'true'); await user.keyboard('{Enter}');
      expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled(); expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it('rerender preserves provider snapshot and source lease until its root remounts', async () => {
    const { rendered, source, surfaceId } = await mount();
    try {
      const before = observedScope!.session.readOwnedCommandContextFrame(surfaceId!);
      rendered.rerender(view());
      expect(observedScope!.session.readOwnedCommandContextFrame(surfaceId!)?.surfaceLease).toBe(before?.surfaceLease);
      expect(observedScope!.session.readSurfaceContext(surfaceId!)?.snapshotVersion).toBe(before?.frame.surface?.snapshotVersion);
      rendered.rerender(view(1));
      expect(source.isConnected).toBe(false);
      expect(observedScope!.session.readOwnedCommandContextFrame(surfaceId!)?.surfaceLease).not.toBe(before?.surfaceLease);
    } finally { rendered.unmount(); }
  });

  it('preserves unconditional Begin when the command has no conditional entry', async () => {
    const map = await state.loadMap(); delete map.contextualPaletteConditions; state.loadMap.mockResolvedValue(map);
    const { rendered, user } = await open();
    try { await user.keyboard('{Enter}'); await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce());
      expect(api.BeginUICommand).toHaveBeenCalledExactlyOnceWith(state.commandId); expect(api.BeginContextualPagePaletteUICommand).not.toHaveBeenCalled();
    } finally { rendered.unmount(); }
  });

  it.each(['tasklists.create', 'tasklists.update', 'profiles.create', 'profiles.update'])('keeps %s excluded and never opens the palette in its modal', async id => {
    expect(isContextualPagePaletteCommand(id)).toBe(false);
    const { rendered, source } = await mount();
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay);
    try {
      registerOpenModal('page-decision');
      fireEvent.keyDown(source, { key: 'k', code: 'KeyK', ctrlKey: true });
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument(); expect(state.catalog).not.toHaveBeenCalled();
    } finally { unregisterOpenModal('page-decision'); overlay.remove(); rendered.unmount(); }
  });
});
