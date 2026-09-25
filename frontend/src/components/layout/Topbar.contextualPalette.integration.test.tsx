import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useLayoutEffect, useRef } from 'react';
import { Topbar } from './Topbar';
import { CommandContextProvider, useCommandContextScope } from '../../lib/commandContextReact';
import { registerChatClearSurface } from '../../lib/commandChatClear';
import { registerChatMessagingSurface } from '../../lib/commandChatMessaging';
import { registerEditorFileSurface } from '../../lib/commandEditorFile';
import { registerEditorFormatAdapter } from '../../lib/commandEditorFormatting';
import { registerTerminalOperationSurface } from '../../lib/commandTerminalOperation';
import { isModalOpen, registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { isContextualPaletteCommand } from '../../lib/commandContextualPalette';
import * as fileExecution from '../../lib/commandEditorFileExecution';

const state = vi.hoisted(() => ({
  loadMap: vi.fn(), listCatalog: vi.fn(), navigate: vi.fn(), announce: vi.fn(), listeners: new Set<() => void>(),
  events: new Map<string, (payload?: unknown) => void>(),
  auth: { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a' } },
  workspace: { workspace: { id: 'workspace-a', profile: 'focused', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] }, workspaces: [], reconcileActiveSelection: vi.fn(async () => true) },
  terminal: { sessions: [{ id: 'pty-a', state: 'running' }], historyBySession: { 'pty-a': [{ id: 'command-a', output: '', endedAt: '' }] }, activeEntryBySession: { 'pty-a': 'command-a' }, loadSessions: vi.fn(async () => undefined) },
}));
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({ acquireGlobalCommandOwnership: () => ({ isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve() }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'pt-BR' } }) }));
vi.mock('react-router-dom', () => ({ useNavigate: () => state.navigate, useLocation: () => ({ pathname: '/', search: '', hash: '', key: '/' }) }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: (query: unknown) => state.listCatalog(query) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({ loadMap: state.loadMap, beginLocalCommandUIKey: vi.fn(), resetLocalCommandKeyboard: vi.fn(), dispatchLocalCommandKey: vi.fn() }) }));
vi.mock('../../store/authStore', () => ({ useAuthStore: Object.assign((selector?: (value: typeof state.auth) => unknown) => selector ? selector(state.auth) : state.auth, { getState: () => state.auth, subscribe: () => () => undefined }) }));
vi.mock('../../store/workspaceStore', () => ({
  flushWorkspaceNavigation: vi.fn(async () => true),
  useWorkspaceStore: Object.assign((selector?: (value: typeof state.workspace) => unknown) => selector ? selector(state.workspace) : state.workspace, { getState: () => state.workspace, subscribe: (listener: () => void) => { state.listeners.add(listener); return () => { state.listeners.delete(listener); }; } }),
}));
vi.mock('../../store/terminalStore', () => ({ useTerminalStore: Object.assign((selector?: (value: typeof state.terminal) => unknown) => selector ? selector(state.terminal) : state.terminal, { getState: () => state.terminal, subscribe: () => () => undefined }) }));
vi.mock('../../store/shortcutsHelpStore', () => ({ useShortcutsHelpStore: (selector: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) => selector({ isOpen: false, open: vi.fn(), close: vi.fn() }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: vi.fn() }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: vi.fn() }));
vi.mock('../../store/workspaceChatModalStore', () => ({ canPrepareWorkspaceChatOpen: () => false, registerWorkspaceChatCommandDispatcher: () => () => {}, prepareWorkspaceChatOpen: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (payload?: unknown) => void) => { state.events.set(name, callback); return () => { state.events.delete(name); }; } }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(async () => null) }));
vi.mock('../pickers/ProfilePicker', () => ({ ProfilePicker: () => null }));

const commandId = 'workspace.tab.chat.create';
const reservation = { ticket: 'ticket', invocationId: 'invocation', commandId };
const handoff = { ...reservation, handoffId: 'handoff' };
const api = {
  ExecuteContextualPaletteLayerCommand: vi.fn(), ExecutePaletteCommand: vi.fn(),
  BeginUICommand: vi.fn(), BeginContextualPaletteUICommand: vi.fn(), TakeUICommand: vi.fn(),
  CompleteUICommand: vi.fn(), GetUICommandResult: vi.fn(), CancelUICommand: vi.fn(), CommitWorkspaceTabCommand: vi.fn(),
  PrepareTerminalInterruptCommand: vi.fn(), PrepareTerminalSessionCommand: vi.fn(),
};
const fileAPI = { EditorPrepareCommand: vi.fn(), EditorCommitCommand: vi.fn() };
const releases: Array<() => void> = [];
function Source({ registered = true, surfaceType = 'chat' }: { registered?: boolean; surfaceType?: string }) {
  const scope = useCommandContextScope();
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => registered ? scope?.registerSurface('tab-a', root, () => ({ surfaceType, surfaceId: 'tab-a', snapshotVersion: 'surface-1' })) : undefined, [scope, registered, surfaceType]);
  return <div ref={root}><textarea data-testid="source" /></div>;
}
function changeProfile(profile: string) {
  state.workspace.workspace.profile = profile;
  state.listeners.forEach(listener => listener());
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

describe('Topbar — paleta contextual durável pela bridge real', () => {
  beforeEach(() => {
    state.listeners.clear(); state.events.clear();
    state.auth.isAuthenticated = true;
    state.auth.user = { userId: 'user-a', sessionId: 'session-a' };
    state.workspace.workspace = { id: 'workspace-a', profile: 'focused', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] };
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    Object.values(api).forEach(spy => spy.mockReset());
    api.BeginUICommand.mockResolvedValue(reservation);
    api.BeginContextualPaletteUICommand.mockResolvedValue(reservation);
    api.TakeUICommand.mockResolvedValue(handoff);
    api.CommitWorkspaceTabCommand.mockResolvedValue(undefined);
    api.CompleteUICommand.mockResolvedValue(undefined);
    api.GetUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'succeeded' });
    Object.values(fileAPI).forEach(spy => spy.mockReset());
    fileAPI.EditorPrepareCommand.mockResolvedValue({ token: 'file-token', path: '/document.md', requiresOverwrite: false, cancelled: false });
    fileAPI.EditorCommitCommand.mockResolvedValue({ tabId: 'tab-a', path: '/document.md', written: true });
    Object.assign(window, { go: { app: { App: api }, wailsapi: { Editor: fileAPI } } });
    state.loadMap.mockResolvedValue({ generation: 'palette-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' }],
      localPaletteCommands: ['navigation.palette.open'],
      contextualPaletteConditions: [{ commandId, bySurface: {}, fallback: false, byProfile: { focused: { commandId, bySurface: { chat: false }, bySurfaceId: { chat: { 'tab-a': true } }, fallback: false } } }],
    });
    state.listCatalog.mockResolvedValue([{ id: commandId, name: 'Criar chat', available: true }]);
  });
  afterEach(() => { releases.splice(0).forEach(release => release()); unregisterOpenModal('contextual-decision'); Reflect.deleteProperty(window, 'go'); vi.restoreAllMocks(); });

  async function open(registered = true, surfaceType = 'chat', setup?: (root: HTMLElement) => void) {
    const user = userEvent.setup();
    const view = render(<CommandContextProvider><div className="workspace-layout"><Topbar /><Source registered={registered} surfaceType={surfaceType} /></div></CommandContextProvider>);
    await act(async () => { await Promise.resolve(); });
    setup?.(screen.getByTestId('source').parentElement!);
    screen.getByTestId('source').focus();
    await user.keyboard('{Control>}k{/Control}');
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Criar chat');
    const option = await screen.findByRole('option', { name: /Criar chat/ });
    return { user, view, option };
  }

  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('contextual %s submits once, survives its own map publication, never uses UI handoff', async id => {
    await project(id);
    state.announce.mockClear();
    api.ExecuteContextualPaletteLayerCommand.mockImplementation(async () => {
      state.events.get('command:keyboard-map-changed')?.();
      return { invocationId: 'layer-invocation', status: 'succeeded' };
    });
    const { user, view } = await open();
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(state.announce).toHaveBeenCalledWith('commandSettings.layerActionCompleted'));
      expect(api.ExecuteContextualPaletteLayerCommand).toHaveBeenCalledExactlyOnceWith('palette-map', id, { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused' });
      expect(api.ExecutePaletteCommand).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
      expect(api.TakeUICommand).not.toHaveBeenCalled();
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionUnknown');
    } finally { view.unmount(); }
  });

  it.each(['chat', 'editor', 'terminal', 'tasklist'])('layer uses registered %s tab origin, never the picker surface', async surfaceType => {
    state.workspace.workspace.tabs[0].type = surfaceType;
    await project('layer.back', surfaceType);
    api.ExecuteContextualPaletteLayerCommand.mockResolvedValue({ invocationId: 'layer', status: 'succeeded' });
    const { user, view } = await open(true, surfaceType);
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.ExecuteContextualPaletteLayerCommand).toHaveBeenCalledExactlyOnceWith('palette-map', 'layer.back', { surfaceType, surfaceId: 'tab-a', profile: 'focused' }));
      expect(api.ExecutePaletteCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['blur', 'owner'])('layer completion after %s does not retry or report false failure', async drift => {
    await project('layer.activate'); state.announce.mockClear();
    const result = deferred<{ invocationId: string; status: string }>();
    api.ExecuteContextualPaletteLayerCommand.mockReturnValue(result.promise);
    const { user, view } = await open();
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.ExecuteContextualPaletteLayerCommand).toHaveBeenCalledOnce());
      await act(async () => {
        if (drift === 'blur') { vi.mocked(document.hasFocus).mockReturnValue(false); fireEvent.blur(window); }
        else state.auth.user = { userId: 'other', sessionId: 'other' };
        result.resolve({ invocationId: 'layer', status: 'succeeded' });
      });
      expect(api.ExecuteContextualPaletteLayerCommand).toHaveBeenCalledOnce();
      expect(api.ExecutePaletteCommand).not.toHaveBeenCalled();
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionUnknown');
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionFailed');
      if (drift === 'owner') expect(state.announce).not.toHaveBeenCalledWith('commandSettings.layerActionCompleted');
    } finally { view.unmount(); }
  });

  it.each(['profile-aba', 'tab', 'owner', 'deadline', 'modal', 'ime', 'missing-api', 'missing-provider', 'page'])('contextual layer rejects %s without legacy execution', async drift => {
    await project('layer.toggle', drift === 'page' ? 'profiles' : 'chat');
    const expires = Date.now() + 60000;
    if (drift === 'deadline') (await state.loadMap()).validUntil = expires;
    const { user, view, option } = await open(drift !== 'missing-provider', drift === 'page' ? 'profiles' : 'chat');
    try {
      await act(async () => {
        if (drift === 'profile-aba') { changeProfile('other'); changeProfile('focused'); }
        if (drift === 'tab') state.workspace.workspace.activeTabId = 'tab-b';
        if (drift === 'owner') state.auth.user = { userId: 'other', sessionId: 'other' };
        if (drift === 'deadline') vi.spyOn(Date, 'now').mockReturnValue(expires);
      });
      if (drift === 'missing-api') Reflect.deleteProperty(api, 'ExecuteContextualPaletteLayerCommand');
      if (drift === 'modal') {
        const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay);
        releases.push(() => overlay.remove()); registerOpenModal('contextual-decision');
      }
      if (drift === 'ime') fireEvent.compositionStart(screen.getByRole('combobox'));
      await user.keyboard('{Enter}');
      if (drift === 'missing-provider' || drift === 'page') expect(option).toHaveAttribute('aria-disabled', 'true');
      if (drift !== 'missing-api') expect(api.ExecuteContextualPaletteLayerCommand).not.toHaveBeenCalled();
      expect(api.ExecutePaletteCommand).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
    } finally {
      if (drift === 'missing-api') api.ExecuteContextualPaletteLayerCommand = vi.fn();
      view.unmount();
    }
  });

  it('unconditional layer retains the existing backend executor', async () => {
    await project('layer.back');
    const map = await state.loadMap(); map.contextualPaletteConditions = [];
    api.ExecutePaletteCommand.mockResolvedValue({ invocationId: 'old-path', status: 'succeeded' });
    const { user, view } = await open();
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.ExecutePaletteCommand).toHaveBeenCalledExactlyOnceWith('layer.back', {}));
      expect(api.ExecuteContextualPaletteLayerCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  async function project(id: string, surfaceType = 'chat') {
    const map = await state.loadMap();
    map.contextualPaletteConditions = [{ commandId: id, bySurface: {}, fallback: false, byProfile: {
      focused: { commandId: id, bySurface: { [surfaceType]: true }, fallback: false },
    } }];
    state.loadMap.mockResolvedValue(map);
    state.listCatalog.mockResolvedValue([{ id, name: 'Criar chat', available: true }]);
    api.BeginContextualPaletteUICommand.mockResolvedValue({ ...reservation, commandId: id });
    api.TakeUICommand.mockResolvedValue({ ...handoff, commandId: id });
  }

  it.each(['tasklists.create', 'profiles.update', 'layer.unknown', 'chat.message.edit.open', 'editor.file.unknown'])('predicate não amplia lista fechada: %s', id => {
    expect(isContextualPaletteCommand(id)).toBe(false);
  });

  it.each(['success', 'cancel', 'profile-aba'])('limpeza destrutiva preserva decisão e condição: %s', async mode => {
    const id = 'chat.conversation.clear'; await project(id);
    const take = deferred<typeof handoff>(); api.TakeUICommand.mockReturnValue(take.promise);
    const succeeded = vi.fn();
    const { user, view, option } = await open(true, 'chat', root => {
      releases.push(registerChatClearSurface({ root, instanceId: 'clear-source', isCurrent: () => true,
        canStart: () => !isModalOpen(), subscribe: () => () => {}, succeeded }));
    });
    try {
      expect(option).not.toHaveAttribute('aria-disabled', 'true');
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.TakeUICommand).toHaveBeenCalledOnce());
      act(() => registerOpenModal('contextual-decision'));
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
      await act(async () => {
        if (mode === 'profile-aba') { changeProfile('other'); changeProfile('focused'); }
        unregisterOpenModal('contextual-decision');
        if (mode === 'cancel') take.reject(new Error('decision-cancelled'));
        else take.resolve({ ...handoff, commandId: id });
      });
      if (mode === 'success') await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce());
      else {
        if (mode === 'cancel') await waitFor(() => expect(api.CancelUICommand).toHaveBeenCalledWith('ticket'));
        else await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled'));
        expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(succeeded).not.toHaveBeenCalled();
      }
      expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['success', 'prepare-drift', 'take-drift'])('messaging usa executor próprio e lease: %s', async mode => {
    const id = 'chat.message.copy'; await project(id);
    const execute = vi.fn(async () => undefined);
    const prepare = vi.fn(async () => { if (mode === 'prepare-drift') { changeProfile('other'); changeProfile('focused'); } });
    api.TakeUICommand.mockImplementation(async () => {
      if (mode === 'take-drift') { changeProfile('other'); changeProfile('focused'); }
      return { ...handoff, commandId: id };
    });
    const { user, view } = await open(true, 'chat', root => {
      releases.push(registerChatMessagingSurface({ root, instanceId: 'message-source', isCurrent: () => true,
        canStart: command => command === id, subscribe: () => () => {}, prepare: () => ({
          isCurrent: () => true, canCommit: () => true, prepareAdmission: prepare, execute, dispose: () => {},
        }) }));
    });
    try {
      await user.keyboard('{Enter}');
      if (mode === 'success') await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'succeeded'));
      else await waitFor(() => expect(api.CompleteUICommand.mock.calls.length + api.CancelUICommand.mock.calls.length).toBeGreaterThan(0));
      expect(execute).toHaveBeenCalledTimes(mode === 'success' ? 1 : 0);
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['success', 'before-prepare', 'during-prepare', 'take-drift'])('formatar mantém preparação e efeito próprios: %s', async mode => {
    const id = 'editor.format.bold'; await project(id, 'editor');
    state.workspace.workspace.tabs[0].type = 'editor';
    const execute = vi.fn(() => true);
    const prepare = vi.fn(async () => { if (mode === 'during-prepare') { changeProfile('other'); changeProfile('focused'); } return true; });
    api.TakeUICommand.mockImplementation(async () => { if (mode === 'take-drift') changeProfile('other'); return { ...handoff, commandId: id }; });
    releases.push(registerEditorFormatAdapter({ capture: () => ({ isCurrent: () => true, canExecute: () => true, prepare, execute, dispose: () => {} }) }));
    const { user, view } = await open(true, 'editor');
    try {
      if (mode === 'before-prepare') act(() => changeProfile('other'));
      await user.keyboard('{Enter}');
      if (mode === 'success') await waitFor(() => expect(execute).toHaveBeenCalledOnce());
      else if (mode === 'take-drift') await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled'));
      else await act(async () => { await Promise.resolve(); });
      if (mode === 'before-prepare' || mode === 'during-prepare') expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
      expect(execute).toHaveBeenCalledTimes(mode === 'success' ? 1 : 0);
      expect(api.BeginUICommand).not.toHaveBeenCalled(); expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['terminal.command.interrupt', 'terminal.session.create', 'terminal.session.close'])('terminal %s preserva Prepare específico e Commit', async id => {
    await project(id, 'terminal');
    state.workspace.workspace.tabs[0].type = 'terminal';
    Object.assign(state.workspace.workspace.tabs[0], { state: { sessionId: 'pty-a' } });
    const { user, view, option } = await open(true, 'terminal', root => {
      releases.push(registerTerminalOperationSurface({ root, instanceId: 'terminal-source', tabId: 'tab-a', isCurrent: () => true, canStart: () => true, subscribe: () => () => {} }));
    });
    try {
      expect(option).not.toHaveAttribute('aria-disabled', 'true');
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce());
      expect(id === 'terminal.command.interrupt' ? api.PrepareTerminalInterruptCommand : api.PrepareTerminalSessionCommand).toHaveBeenCalledOnce();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['editor.file.save', 'editor.file.open', 'editor.file.save_copy', 'profile-aba', 'owner', 'tab', 'prepare-drift', 'prepare-ime', 'prepare-modal', 'prepare-twice'])('arquivo e continuação nativa: %s', async mode => {
    const id = mode.startsWith('editor.file.') ? mode : 'editor.file.save'; await project(id, 'editor');
    state.workspace.workspace.tabs[0].type = 'editor';
    if (mode === 'prepare-twice') {
      const execute = fileExecution.executeEditorFileCommand;
      vi.spyOn(fileExecution, 'executeEditorFileCommand').mockImplementation((port, command, options) => execute(port, command, {
        ...options, authorizePreparation: () => {
          expect(options.authorizePreparation?.()).toBe(true);
          const repeated = options.authorizePreparation?.();
          expect(repeated).toBe(false);
          return repeated === true;
        },
      }));
    }
    const prepare = vi.fn(async () => {
      if (mode === 'prepare-drift') { changeProfile('other'); changeProfile('focused'); }
      if (mode === 'prepare-ime') {
        screen.getByTestId('source').focus();
        fireEvent.compositionStart(screen.getByTestId('source'));
      }
      if (mode === 'prepare-modal') {
        const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay);
        releases.push(() => overlay.remove());
        registerOpenModal('contextual-decision');
      }
      return { content: 'original', labels: {}, suggestedFilename: 'document.md', confirmOverwrite: false };
    });
    fileAPI.EditorPrepareCommand.mockImplementation(async () => {
      fireEvent.blur(window);
      state.events.get('command:keyboard-map-changed')?.();
      if (mode === 'profile-aba') { changeProfile('other'); changeProfile('focused'); }
      if (mode === 'owner') state.auth.user = { userId: 'other', sessionId: 'session-a' };
      if (mode === 'tab') { state.workspace.workspace.activeTabId = 'tab-b'; state.listeners.forEach(listener => listener()); }
      fireEvent.focus(window);
      return { token: 'file-token', path: '/document.md', requiresOverwrite: false, cancelled: false };
    });
    const { user, view } = await open(true, 'editor', root => {
      releases.push(registerEditorFileSurface({ root, ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a', tabId: 'tab-a',
        documentId: 'doc-a', instanceId: 'file-source', generation: 'doc-1', isActive: () => true, isCurrent: () => true,
        canExecute: () => true, canCommit: () => true, prepare, confirmOverwrite: async () => false }));
    });
    try {
      await user.keyboard('{Enter}');
      if (mode.startsWith('editor.file.')) await waitFor(() => expect(fileAPI.EditorCommitCommand).toHaveBeenCalledOnce());
      else {
        await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled'));
        expect(fileAPI.EditorCommitCommand).not.toHaveBeenCalled();
      }
      if (mode.startsWith('prepare-')) expect(fileAPI.EditorPrepareCommand).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled(); expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it('preserva a origem anterior ao picker e submete Begin/Take/Commit sem Begin genérico', async () => {
    const { user, view, option } = await open();
    try {
      expect(option).not.toHaveAttribute('aria-disabled', 'true');
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff'));
      expect(api.BeginContextualPaletteUICommand).toHaveBeenCalledExactlyOnceWith('palette-map', commandId, { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused' });
      expect(api.TakeUICommand).toHaveBeenCalledExactlyOnceWith('ticket');
      expect(api.BeginUICommand).not.toHaveBeenCalled();
      expect(api.CompleteUICommand).not.toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
    } finally { view.unmount(); }
  });

  it.each(['profile', 'tab', 'profile-aba'])('nega %s antes de Begin, sem tocar ledger', async mode => {
    const { user, view } = await open();
    try {
      act(() => {
        if (mode === 'tab') { state.workspace.workspace.activeTabId = 'tab-b'; state.listeners.forEach(listener => listener()); }
        else { changeProfile('other'); if (mode === 'profile-aba') changeProfile('focused'); }
      });
      await user.keyboard('{Enter}');
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
      expect(api.TakeUICommand).not.toHaveBeenCalled();
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['profile', 'surface-id', 'malformed', 'missing-provider'])('indisponibilidade canônica: %s não admite comando', async mode => {
    const map = await state.loadMap();
    if (mode === 'profile') state.workspace.workspace.profile = 'other';
    if (mode === 'surface-id') map.contextualPaletteConditions[0].byProfile.focused.bySurfaceId.chat = { 'tab-b': true };
    if (mode === 'malformed') map.contextualPaletteConditions[0].byProfile.focused.bySurface.chat = 'true';
    if (mode === 'missing-provider') map.contextualPaletteConditions = [{ commandId, bySurface: {}, fallback: true }];
    state.loadMap.mockResolvedValue(map);
    const { user, view, option } = await open(mode !== 'missing-provider');
    try {
      expect(option).toHaveAttribute('aria-disabled', 'true');
      await user.keyboard('{Enter}');
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
      expect(api.TakeUICommand).not.toHaveBeenCalled();
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it('ID fora da projeção conserva o pipeline incondicional', async () => {
    const map = await state.loadMap();
    delete map.contextualPaletteConditions;
    state.loadMap.mockResolvedValue(map);
    const { user, view } = await open();
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce());
      expect(api.BeginUICommand).toHaveBeenCalledExactlyOnceWith(commandId);
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['begin', 'take'])('invalidação durante %s impede Commit e cancela reserva', async phase => {
    const begin = deferred<typeof reservation>(); const take = deferred<typeof handoff>();
    if (phase === 'begin') api.BeginContextualPaletteUICommand.mockReturnValue(begin.promise);
    else api.TakeUICommand.mockReturnValue(take.promise);
    const { user, view } = await open();
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(phase === 'begin' ? api.BeginContextualPaletteUICommand : api.TakeUICommand).toHaveBeenCalledOnce());
      await act(async () => { changeProfile('other'); changeProfile('focused'); begin.resolve(reservation); take.resolve(handoff); });
      await waitFor(() => expect(api.CompleteUICommand.mock.calls.length + api.CancelUICommand.mock.calls.length).toBeGreaterThan(0));
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['owner', 'expiry', 'reload'])('revalida %s depois de Take, antes do Commit', async mode => {
    const deadline = Date.now() + 60_000;
    const map = await state.loadMap(); map.validUntil = deadline;
    state.loadMap.mockResolvedValue(map);
    const take = deferred<typeof handoff>(); api.TakeUICommand.mockReturnValue(take.promise);
    const { user, view } = await open();
    try {
      await user.keyboard('{Enter}');
      await waitFor(() => expect(api.TakeUICommand).toHaveBeenCalledOnce());
      await act(async () => {
        if (mode === 'owner') state.auth.user = { userId: 'other', sessionId: 'session-a' };
        if (mode === 'expiry') vi.spyOn(Date, 'now').mockReturnValue(deadline + 1);
        if (mode === 'reload') state.events.get('command:keyboard-map-changed')?.();
        take.resolve(handoff);
      });
      await waitFor(() => expect(api.CompleteUICommand.mock.calls.length + api.CancelUICommand.mock.calls.length).toBeGreaterThan(0));
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
      expect(api.BeginUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });
});
