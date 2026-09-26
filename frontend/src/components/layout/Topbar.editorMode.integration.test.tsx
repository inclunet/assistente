import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useLayoutEffect, useRef, useState } from 'react';
import { Topbar } from './Topbar';
import { createCommandContextScope } from '../../lib/commandContextReact';

// This fixture has no native hotkeys. The shared ownership bridge is exercised
// independently with real reservation frames in commandGlobalOwnershipWails tests.
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({
  acquireGlobalCommandOwnership: () => ({
    isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve(),
  }),
}));
import { Modal } from '../ui/Modal';
import { getTopmostModalID } from '../../lib/modalRegistry';
import {
  EDITOR_MODE_COMMAND_EVENT,
  registerEditorModeSurface,
  requestEditorModeCommand,
} from '../../lib/commandEditorMode';
import {
  EDITOR_PRESENTATION_COMMAND_IDS,
  registerEditorPresentationSurface,
} from '../../lib/commandEditorPresentation';
import type { EditorMode } from '../../store/editorStore';
import { registerEditorFileSurface } from '../../lib/commandEditorFile';
import { Editor } from '@tiptap/core';
import { selectionCell } from '@tiptap/pm/tables';
import { buildRichTextExtensions } from '../editor/buildRichTextExtensions';
import { registerEditorFormatting, registerEditorFormatAdapter, requestEditorFormatCommand } from '../../lib/commandEditorFormatting';
import { captureEditorMarkdown } from '../../lib/commandEditorMarkdown';
import { appendEditorSlideMarkdown, buildEditorSlideTemplate, isEditorSlideCommand } from '../../lib/commandEditorSlideTemplates';
import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';
import { CHAT_CLEAR_COMMAND, registerChatClearSurface, requestChatClear } from '../../lib/commandChatClear';
import { registerChatMessagingSurface, requestChatMessagingCommand, captureChatMessagingTarget, type ChatMessagingCommandID } from '../../lib/commandChatMessaging';
import {
  EDITOR_CELL_NAVIGATION_COMMAND_IDS,
  canNavigateCell,
  navigateEditorCell,
  requestEditorCellNavigationCommand,
} from '../../lib/commandEditorCellNavigation';

const state = vi.hoisted(() => ({
  pathname: '/',
  mapReady: false,
  modeViewBinding: true,
  modeViewShortcutCode: 'Digit3',
  contextualViewBinding: false,
  contextualViewByPage: false,
  contextualMarkdownBinding: false,
  simulateWorkspaceLandmarkBlock: false,
  commandScope: null as unknown,
  clearShortcut: false,
  messagingShortcut: null as string | null,
  modalOpen: false,
  result: 'succeeded' as 'succeeded' | 'failed' | 'denied' | 'outcome_unknown',
  targetCurrent: true,
  commit: vi.fn(),
  filePrepare: vi.fn(),
  fileCommit: vi.fn(),
  fileApply: vi.fn(),
  begin: vi.fn(),
  take: vi.fn(),
  getResult: vi.fn(),
  complete: vi.fn(),
  cancel: vi.fn(),
  beginLocalCommandUIKey: vi.fn(),
  cellCanOpen: undefined as ((commandID: string) => boolean) | undefined,
  cellOpen: undefined as ((commandID: string) => boolean) | undefined,
  topbarUnmount: undefined as (() => void) | undefined,
  applyMode: undefined as ((mode: EditorMode) => void) | undefined,
  focusView: vi.fn(),
  commitDeferred: false,
  resolveCommit: undefined as (() => void) | undefined,
  takeDeferred: false,
  resolveTake: undefined as (() => void) | undefined,
  events: new Map<string, (payload: unknown) => void>(),
  navigate: vi.fn(),
  announce: vi.fn(),
  t: (key: string) => key,
  menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
  noop: () => undefined,
}));

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return { ...actual, useNavigate: () => state.navigate, useLocation: () => ({ pathname: state.pathname, search: '', hash: '', key: 'editor' }) };
});

const auth = { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } };
const workspace = {
  id: 'workspace-a', name: 'Workspace', profile: '', activeTabId: 'editor-tab',
  tabs: [{ id: 'editor-tab', type: 'editor' as const }],
};

vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign((selector?: (value: typeof auth) => unknown) => selector ? selector(auth) : auth, {
    getState: () => auth,
    subscribe: () => () => undefined,
  }),
}));
vi.mock('../../store/workspaceStore', () => ({
  flushWorkspaceNavigation: vi.fn(async () => true),
  useWorkspaceStore: Object.assign((selector?: (value: { workspace: typeof workspace; workspaces: never[]; setActiveTab: () => void }) => unknown) =>
    selector ? selector({ workspace, workspaces: [], setActiveTab: () => undefined }) : { workspace, workspaces: [] }, {
      getState: () => ({ workspace, workspaces: [] }),
      subscribe: () => () => undefined,
    }),
}));
vi.mock('../../store/settingsStore', () => ({ useSettingsStore: (selector: (value: { updateConfig: () => void }) => unknown) => selector({ updateConfig: () => undefined }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: () => undefined }) }));
const shortcutsHelp = vi.hoisted(() => ({ isOpen: false, open: vi.fn(), close: vi.fn() }));
vi.mock('../../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: Object.assign((selector?: (value: typeof shortcutsHelp) => unknown) => selector ? selector(shortcutsHelp) : shortcutsHelp, {
    getState: () => shortcutsHelp,
  }),
}));
vi.mock('../../store/workspaceChatModalStore', () => ({
  canPrepareWorkspaceChatOpen: () => false,
  prepareWorkspaceChatOpen: vi.fn(),
  registerWorkspaceChatCommandDispatcher: () => () => undefined,
}));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (payload: unknown) => void) => {
  state.events.set(name, callback);
  return () => { if (state.events.get(name) === callback) state.events.delete(name); };
} }));
vi.mock('react-i18next', async (importOriginal) => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: state.t, i18n: { language: 'en' } }) }));
vi.mock('../ui/Modal', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../ui/Modal')>();
  return { ...actual, isModalOpen: () => state.modalOpen || actual.isModalOpen() };
});
vi.mock('../ui/KeyboardShortcutsHelp', () => ({ KeyboardShortcutsHelp: () => null }));
vi.mock('./ConnectionStatusIndicator', () => ({ ConnectionStatusIndicator: () => null }));
vi.mock('../menu', () => ({ Menu: () => null }));
vi.mock('./MenuButton', () => ({
  MenuButton: () => null,
}));
vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({ menu: state.menu, openForTrigger: state.noop, openAtPoint: state.noop, closeMenu: state.noop, onSelectItem: state.noop }),
}));
vi.mock('../../hooks/useToolbarKeyboardNav', () => ({ useToolbarKeyboardNav: () => undefined }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce, announceRequest: () => true }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: () => undefined }));
vi.mock('../../lib/commandContextReact', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../lib/commandContextReact')>(),
  useCommandContextScope: () => state.commandScope,
}));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => [
  { id: 'editor.mode.rich', name: 'Modo rico', available: true },
  { id: 'editor.file.save', name: 'Salvar arquivo', available: true },
  { id: 'editor.format.bold', name: 'Negrito', available: true },
  { id: 'editor.format.code_block', name: 'Bloco de código', available: true },
  { id: 'editor.format.table.merge', name: 'Mesclar células', available: true },
  { id: 'editor.slide.insert.basic', name: 'Inserir slide básico', available: true },
  { id: 'editor.format.list.bullet', name: 'Lista com marcadores', available: true },
  { id: 'chat.conversation.clear', name: 'Limpar conversa', available: true },
  ...['chat.message.send', 'chat.response.cancel', 'chat.message.retry'].map(id => ({ id, name: id, available: true })),
]) }));
vi.mock('../../lib/commandEditorFileWails', () => ({ createEditorFileCommandWailsPort: () => ({
  prepare: state.filePrepare, commit: state.fileCommit,
}) }));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({
  createCommandLocalKeyboardWailsPort: () => ({
    loadMap: async () => ({
      ...(() => { state.mapReady = true; return {}; })(),
      generation: 'generation-1', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      localPaletteCommands: [...EDITOR_CELL_NAVIGATION_COMMAND_IDS],
      bindings: [
        ...(state.messagingShortcut ? [{ shortcut: { version: 1 as const, code: 'KeyJ', modifiers: ['Control' as const] }, commandId: state.messagingShortcut, handler: 'contextual' as const }] : []),
        ...Array.from({ length: 7 }, (_, level) => ({ shortcut: { version: 1 as const, code: `Digit${level}`, modifiers: ['Control' as const, 'Alt' as const] }, commandId: level === 0 ? 'editor.format.paragraph' : `editor.format.heading.h${level}`, handler: 'ui' as const })),
        { shortcut: { version: 1 as const, code: 'Digit8', modifiers: ['Control' as const, 'Shift' as const] }, commandId: 'editor.format.list.bullet', handler: 'ui' as const },
        { shortcut: { version: 1 as const, code: 'Digit7', modifiers: ['Control' as const, 'Shift' as const] }, commandId: 'editor.format.list.ordered', handler: 'ui' as const },
        { shortcut: { version: 1 as const, code: 'KeyB', modifiers: ['Control' as const, 'Shift' as const] }, commandId: 'editor.format.blockquote', handler: 'ui' as const },
        { shortcut: { version: 1 as const, code: 'KeyC', modifiers: ['Control' as const, 'Alt' as const] }, commandId: 'editor.format.code_block', handler: 'ui' as const },
        { shortcut: { version: 1 as const, code: 'KeyB', modifiers: ['Control' as const] }, commandId: 'editor.format.bold', handler: 'ui' as const },
        { shortcut: { version: 1 as const, code: 'KeyI', modifiers: ['Control' as const] }, commandId: 'editor.format.italic', handler: 'ui' as const },
        { shortcut: { version: 1 as const, code: 'KeyX', modifiers: ['Control' as const, 'Shift' as const] }, commandId: 'editor.format.strike', handler: 'ui' as const },
        { shortcut: { version: 1 as const, code: 'ArrowRight', modifiers: ['Control' as const, 'Alt' as const] }, commandId: 'editor.table.cell.next', handler: 'local_ui' as const },
        { shortcut: { version: 1 as const, code: 'KeyL', modifiers: ['Control' as const] }, commandId: state.clearShortcut ? 'chat.conversation.clear' : 'editor.format.link.set', handler: state.clearShortcut ? 'contextual' as const : 'ui' as const },
        ...(!state.contextualMarkdownBinding ? [{ shortcut: { version: 1 as const, code: 'Digit1', modifiers: ['Alt' as const] }, commandId: 'editor.mode.markdown', handler: 'contextual' as const }] : []),
        { shortcut: { version: 1 as const, code: 'Digit2', modifiers: ['Alt' as const] }, commandId: 'editor.mode.rich', handler: 'contextual' as const },
        ...(state.modeViewBinding && !state.contextualViewBinding ? [{ shortcut: { version: 1 as const, code: state.modeViewShortcutCode, modifiers: ['Alt' as const] }, commandId: 'editor.mode.view', handler: 'contextual' as const }] : []),
        { shortcut: { version: 1 as const, code: 'KeyS', modifiers: ['Control' as const] }, commandId: 'editor.file.save', handler: 'contextual' as const },
        { shortcut: { version: 1 as const, code: 'KeyO', modifiers: ['Control' as const] }, commandId: 'editor.file.open', handler: 'contextual' as const },
        { shortcut: { version: 1 as const, code: 'KeyS', modifiers: ['Control' as const, 'Shift' as const] }, commandId: 'editor.file.save_copy', handler: 'contextual' as const },
      ],
      contextualBindings: [
        ...(state.contextualViewBinding && state.modeViewBinding ? [{
          shortcut: { version: 1 as const, code: state.modeViewShortcutCode, modifiers: ['Alt' as const] },
          bySurface: state.contextualViewByPage ? {} : { editor: {
            shortcut: { version: 1 as const, code: state.modeViewShortcutCode, modifiers: ['Alt' as const] },
            commandId: 'editor.mode.view', handler: 'contextual' as const,
          } },
          ...(state.contextualViewByPage ? { byPage: { workspace: {
            shortcut: { version: 1 as const, code: state.modeViewShortcutCode, modifiers: ['Alt' as const] },
            bySurface: { editor: {
              shortcut: { version: 1 as const, code: state.modeViewShortcutCode, modifiers: ['Alt' as const] },
              commandId: 'editor.mode.view', handler: 'contextual' as const,
            } }, fallback: null,
          } } } : {}),
          fallback: null,
        }] : []),
        ...(state.contextualMarkdownBinding ? [{
          shortcut: { version: 1 as const, code: 'Digit1', modifiers: ['Alt' as const] },
          bySurface: { editor: {
            shortcut: { version: 1 as const, code: 'Digit1', modifiers: ['Alt' as const] },
            commandId: 'editor.mode.markdown', handler: 'contextual' as const,
          } },
          fallback: null,
        }] : []),
      ],
    }),
    dispatchLocalCommandKey: vi.fn(async () => null),
    beginLocalCommandUIKey: state.beginLocalCommandUIKey,
    resetLocalCommandKeyboard: vi.fn(async () => undefined),
  }),
}));
vi.mock('../../lib/commandUIExecutionWails', () => ({ createCommandUIExecutionWailsPort: () => statePort() }));
vi.mock('../../lib/commandWorkspaceTabWails', () => ({ createCommandWorkspaceTabWailsPort: () => statePort() }));
function statePort() {
  state.cancel.mockResolvedValue(undefined);
  state.complete.mockResolvedValue(undefined);
  state.begin.mockImplementation(async (commandID: string) => ({ ticket: `ticket-${commandID}`, invocationId: 'invocation-1', commandId: commandID }));
  state.beginLocalCommandUIKey.mockImplementation(async (_generation: string, shortcut: { version: 1; code: string; modifiers: string[] }) => {
    if (state.messagingShortcut && shortcut.code === 'KeyJ') return { ticket: `ticket-${state.messagingShortcut}`, invocationId: 'invocation-1', commandId: state.messagingShortcut };
    if (state.clearShortcut && shortcut.code === 'KeyL') return { ticket: `ticket-${CHAT_CLEAR_COMMAND}`, invocationId: 'invocation-1', commandId: CHAT_CLEAR_COMMAND };
    const format = shortcut.modifiers.includes('Control') && shortcut.modifiers.includes('Alt')
      ? (shortcut.code === 'KeyC' ? 'editor.format.code_block' : shortcut.code === 'Digit0' ? 'editor.format.paragraph' : `editor.format.heading.h${shortcut.code.slice(-1)}`)
      : shortcut.modifiers.includes('Control') && shortcut.modifiers.includes('Shift') && ['Digit7', 'Digit8', 'KeyB'].includes(shortcut.code)
        ? ({ Digit7: 'editor.format.list.ordered', Digit8: 'editor.format.list.bullet', KeyB: 'editor.format.blockquote' } as Record<string, string>)[shortcut.code]
        : ({ KeyB: 'editor.format.bold', KeyI: 'editor.format.italic', KeyX: 'editor.format.strike' } as Record<string, string>)[shortcut.code];
    const commandID = format ?? (shortcut.code === 'KeyL' ? 'editor.format.link.set' : shortcut.code === 'KeyS' ? (shortcut.modifiers.includes('Shift') ? 'editor.file.save_copy' : 'editor.file.save') : shortcut.code === 'KeyO' ? 'editor.file.open' : shortcut.code === 'Digit1' ? 'editor.mode.markdown' :
      shortcut.code === 'Digit2' ? 'editor.mode.rich' : 'editor.mode.view');
    return { ticket: `ticket-${commandID}`, invocationId: 'invocation-1', commandId: commandID };
  });
  state.take.mockImplementation(async (ticket: string) => {
    if (state.takeDeferred) await new Promise<void>((resolve) => { state.resolveTake = resolve; });
    return { ticket, handoffId: 'handoff-1', invocationId: 'invocation-1', commandId: ticket.slice('ticket-'.length) };
  });
  state.commit.mockImplementation(async () => {
    if (state.commitDeferred) await new Promise<void>((resolve) => { state.resolveCommit = resolve; });
  });
  state.getResult.mockImplementation(async (ticket: string) => ({ invocationId: 'invocation-1', status: state.result, commandId: ticket.slice('ticket-'.length) }));
  return {
    beginUICommand: state.begin,
    takeUICommand: state.take,
    commitBackendCommand: state.commit,
    getUICommandResult: state.getResult,
    completeUICommand: state.complete,
    cancelUICommand: state.cancel,
  };
}

function EditorSurface({ initialMode = 'markdown' as EditorMode }: { initialMode?: EditorMode }) {
  const [mode, setMode] = useState(initialMode);
  const rootRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    state.applyMode = setMode;
    const root = rootRef.current;
    if (!root) return () => { state.applyMode = undefined; };
    const contextScope = state.commandScope as ReturnType<typeof createCommandContextScope> | null;
    const unregisterContext = contextScope?.registerSurface('editor-tab', rootRef, () => ({
      surfaceId: 'editor-tab', surfaceType: 'editor', snapshotVersion: 'editor-snapshot-1',
    }));
    const unregisterFile = registerEditorFileSurface({
      root, ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      tabId: 'editor-tab', documentId: 'document-a', instanceId: 'file-instance', generation: 'file-generation',
      isActive: () => true, isCurrent: () => state.targetCurrent, canExecute: () => true,
      prepare: async () => ({ content: 'document bytes', labels: {}, suggestedFilename: 'file.md', confirmOverwrite: false }),
      confirmOverwrite: async () => true, applyCommitted: state.fileApply,
    });
    const unregisterMode = registerEditorModeSurface({
      root,
      ownerId: 'user-a',
      sessionId: 'session-a',
      workspaceId: 'workspace-a',
      tabId: 'editor-tab',
      documentId: 'document-a',
      instanceId: 'editor-instance',
      generation: 'editor-generation',
      mode,
      readOnly: false,
      isAsking: () => false,
      isActive: () => true,
      isCurrent: () => state.targetCurrent,
      isBlocked: () => state.modalOpen || (state.simulateWorkspaceLandmarkBlock && !root.contains(document.activeElement)),
      canFocusViewFromWorkspaceLandmark: () => state.simulateWorkspaceLandmarkBlock && !state.modalOpen && document.activeElement instanceof HTMLElement &&
        document.activeElement.matches('.topbar button, .workspace-toolbar button, .ws-tabs [role="tab"]'),
      focusCurrentView: () => {
        if (mode !== 'view' || !state.targetCurrent || state.modalOpen) return false;
        const reading = root.querySelector<HTMLElement>('[data-testid="rendered-reading-document"]');
        if (!reading) return false;
        reading.focus();
        state.focusView();
        return document.activeElement === reading;
      },
      applyCommitted: (nextMode) => {
        state.applyMode?.(nextMode);
        return true;
      },
    });
    const unregisterPresentation = registerEditorPresentationSurface({
      root,
      ownerId: 'user-a',
      sessionId: 'session-a',
      workspaceId: 'workspace-a',
      tabId: 'editor-tab',
      documentId: 'document-a',
      instanceId: 'editor-presentation-instance',
      generation: 'editor-presentation-generation',
      allowedCommandIds: EDITOR_PRESENTATION_COMMAND_IDS,
      isActive: () => true,
      isCurrent: () => state.targetCurrent,
      canOpen: (commandID) => !EDITOR_CELL_NAVIGATION_COMMAND_IDS.includes(commandID as typeof EDITOR_CELL_NAVIGATION_COMMAND_IDS[number]) || state.cellCanOpen?.(commandID) === true,
      open: (commandID) => state.cellOpen?.(commandID) === true,
    });
    return () => {
      unregisterMode();
      unregisterContext?.();
      unregisterFile();
      unregisterPresentation();
      state.cellCanOpen = undefined;
      state.cellOpen = undefined;
      state.applyMode = undefined;
    };
  }, [mode]);
  return <div ref={rootRef} data-testid="editor-surface" data-mode={mode}>
    <textarea aria-label="editor input" />
    <div data-testid="rendered-reading-document" tabIndex={-1} hidden={mode !== 'view'} />
  </div>;
}

async function mount(initialMode: EditorMode = 'markdown', contextualViewBinding = false) {
  state.targetCurrent = true;
  state.result = 'succeeded';
  state.contextualViewBinding = contextualViewBinding;
  state.commandScope = contextualViewBinding ? createCommandContextScope(undefined, state.pathname) : null;
  state.filePrepare.mockResolvedValue({ token: 'file-token', path: 'file.md', requiresOverwrite: false, cancelled: false });
  state.fileCommit.mockResolvedValue({ tabId: 'editor-tab', path: 'file.md', written: true });
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  const view = render(<><Topbar /><EditorSurface initialMode={initialMode} /></>);
  state.topbarUnmount = view.unmount;
  await waitFor(() => expect(state.mapReady).toBe(true));
  await act(async () => { await new Promise<void>((resolve) => setTimeout(resolve, 50)); });
  (document.querySelector('[aria-label="editor input"]') as HTMLTextAreaElement).focus();
  return view;
}

// Lote90: the six message actions have their own protocol fixture in Topbar.messageActions.integration.test.tsx.
const CHAT_MESSAGING_COMMAND_IDS = ['chat.message.send', 'chat.response.cancel', 'chat.message.retry'] as const;
describe('Topbar audited chat messaging integration', () => {
  let unregister: (() => void) | undefined;
  let root: HTMLDivElement;
  const changeListeners = new Set<() => void>();
  const changed = () => changeListeners.forEach(callback => callback());
  let version = 0;
  const execute = vi.fn(async () => undefined);
  async function chat(commandID: ChatMessagingCommandID, modal = false) {
    state.messagingShortcut = commandID;
    await mount();
    root = document.createElement('div');
    const input = document.createElement('textarea');
    input.setAttribute('data-testid', 'chat-input');
    root.appendChild(input); document.body.appendChild(root);
    if (modal) {
      render(<Modal isOpen onClose={() => undefined} title="Chat owner"><div data-testid="modal-chat-host" /></Modal>);
      screen.getByTestId('modal-chat-host').appendChild(root);
    }
    const modalId = modal ? getTopmostModalID() ?? undefined : undefined;
    unregister = registerChatMessagingSurface({
      root, modalId, instanceId: 'messaging-chat', isCurrent: () => true,
      canStart: (_id, target) => !target || root.contains(target as Node),
      subscribe(callback) { changeListeners.add(callback); return () => { changeListeners.delete(callback); }; },
      prepare() {
        const captured = version;
        return { isCurrent: () => version === captured, canCommit: () => version === captured && !state.modalOpen && getTopmostModalID() === (modalId ?? null),
          execute, dispose: () => undefined };
      },
    });
    input.focus();
    return input;
  }
  afterEach(() => { unregister?.(); unregister = undefined; root?.remove(); changeListeners.clear(); version = 0; execute.mockClear(); });

  it.each(CHAT_MESSAGING_COMMAND_IDS.flatMap(commandID => ['button', 'keyboard', 'palette', 'deck'].map(source => ({ commandID, source }))))(
    '$commandID via $source waits for admission and executes once', async ({ commandID, source }) => {
      const input = await chat(commandID);
      state.takeDeferred = true;
      if (source === 'button') requestChatMessagingCommand(commandID, 'messaging-chat');
      if (source === 'keyboard') fireEvent.keyDown(input, { key: 'j', code: 'KeyJ', ctrlKey: true });
      if (source === 'deck') act(() => state.events.get('command:deck-ui-reservation')?.({ ticket: `ticket-${commandID}`, invocationId: 'invocation-1', commandId: commandID }));
      if (source === 'palette') {
        const user = userEvent.setup();
        await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
        await user.type(await screen.findByRole('combobox'), commandID);
        await user.keyboard('{ArrowDown}{Enter}');
      }
      await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
      expect(execute).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
      await act(async () => state.resolveTake?.());
      await waitFor(() => expect(execute).toHaveBeenCalledExactlyOnceWith({ ticket: `ticket-${commandID}`, handoffId: 'handoff-1' }));
      if (commandID === 'chat.response.cancel') expect(state.commit).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`, 'handoff-1');
      else expect(state.commit).not.toHaveBeenCalled();
      expect(state.complete).not.toHaveBeenCalledWith(expect.anything(), expect.anything(), 'succeeded');
      if (source === 'deck') { expect(state.begin).not.toHaveBeenCalled(); expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled(); }
    },
  );


  it.each(['button', 'keyboard'])('announces missing conversation once via %s', async source => {
    const input = await chat('chat.message.send');
    state.begin.mockRejectedValueOnce('chat_conversation_unavailable');
    state.beginLocalCommandUIKey.mockRejectedValueOnce('chat_conversation_unavailable');
    state.announce.mockClear();
    if (source === 'button') requestChatMessagingCommand('chat.message.send', 'messaging-chat');
    else fireEvent.keyDown(input, { key: 'j', code: 'KeyJ', ctrlKey: true });
    await waitFor(() => expect(state.announce).toHaveBeenCalledWith('chat.conversationUnavailable'));
    expect(state.announce.mock.calls.filter(call => call[0] === 'chat.conversationUnavailable')).toHaveLength(1);
    expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionFailed');
    expect(execute).not.toHaveBeenCalled();
  });
  it('rejects ABA while take is pending without executing a replacement target', async () => {
    await chat('chat.message.send'); state.takeDeferred = true;
    requestChatMessagingCommand('chat.message.send', 'messaging-chat');
    await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
    version++; changed?.(); version--; changed?.();
    await act(async () => state.resolveTake?.());
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-chat.message.send', 'handoff-1', 'cancelled'));
    expect(execute).not.toHaveBeenCalled();
  });

  it.each(['failed', 'denied', 'outcome_unknown'] as const)('announces %s once without repeating the send', async (status) => {
    await chat('chat.message.send'); state.result = status;
    state.announce.mockClear();
    requestChatMessagingCommand('chat.message.send', 'messaging-chat');
    const key = status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed';
    await waitFor(() => expect(state.announce).toHaveBeenCalledExactlyOnceWith(key));
    expect(execute).toHaveBeenCalledTimes(1);
    expect(state.begin).toHaveBeenCalledTimes(1);
  });

  it('does not announce an old result in a different route', async () => {
    await chat('chat.message.send'); state.result = 'outcome_unknown'; state.takeDeferred = true;
    requestChatMessagingCommand('chat.message.send', 'messaging-chat');
    await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
    state.pathname = '/settings';
    state.topbarUnmount?.();
    state.announce.mockClear();
    await act(async () => state.resolveTake?.());
    expect(state.announce).not.toHaveBeenCalled();
    expect(execute).not.toHaveBeenCalled();
  });

  it('preserves the supplied override target rather than recapturing', async () => {
    await chat('chat.message.retry');
    const target = captureChatMessagingTarget(() => '/', 'chat.message.retry', 'messaging-chat');
    expect(target).toBeDefined();
    const override = vi.fn(async () => undefined);
    requestChatMessagingCommand('chat.message.retry', 'messaging-chat', { ...target!, execute: override });
    await waitFor(() => expect(override).toHaveBeenCalledTimes(1));
    expect(execute).not.toHaveBeenCalled();
  });

  it('keeps the palette draft lease invalid after ABA instead of recapturing on selection', async () => {
    await chat('chat.message.send');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await user.type(await screen.findByRole('combobox'), 'chat.message.send');
    version++; changed?.(); version--; changed?.();
    await user.keyboard('{ArrowDown}{Enter}');
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).not.toHaveBeenCalled(); expect(execute).not.toHaveBeenCalled();
  });

  it('does not clean up a new turn after cancellation committed against the old target', async () => {
    await chat('chat.response.cancel'); state.commitDeferred = true;
    requestChatMessagingCommand('chat.response.cancel', 'messaging-chat');
    await waitFor(() => expect(state.resolveCommit).toBeTypeOf('function'));
    version++; changed?.();
    await act(async () => state.resolveCommit?.());
    await waitFor(() => expect(state.getResult).toHaveBeenCalled());
    expect(execute).not.toHaveBeenCalled();
    expect(state.commit).toHaveBeenCalledTimes(1);
  });

  it('cancels a taken lease on Topbar unmount without sending', async () => {
    await chat('chat.message.send'); state.takeDeferred = true;
    requestChatMessagingCommand('chat.message.send', 'messaging-chat');
    await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
    act(() => state.topbarUnmount?.());
    await act(async () => state.resolveTake?.());
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-chat.message.send', 'handoff-1', 'cancelled'));
    expect(execute).not.toHaveBeenCalled();
  });

  it('rejects repeat, IME and blocking modal without admission', async () => {
    const input = await chat('chat.message.send');
    fireEvent.keyDown(input, { key: 'j', code: 'KeyJ', ctrlKey: true, repeat: true });
    fireEvent.keyDown(input, { key: 'j', code: 'KeyJ', ctrlKey: true, isComposing: true });
    state.modalOpen = true;
    expect(requestChatMessagingCommand('chat.message.send', 'messaging-chat')).toBe(false);
    fireEvent.keyDown(input, { key: 'j', code: 'KeyJ', ctrlKey: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).not.toHaveBeenCalled(); expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(execute).not.toHaveBeenCalled();
  });

  it.each(CHAT_MESSAGING_COMMAND_IDS)('allows %s inside its real owning modal and rejects a topmost overlay', async (commandID) => {
    await chat(commandID, true);
    expect(requestChatMessagingCommand(commandID, 'messaging-chat')).toBe(true);
    await waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    const begins = state.begin.mock.calls.length;
    render(<Modal isOpen onClose={() => undefined} title="Blocking modal"><button>Other</button></Modal>);
    expect(requestChatMessagingCommand(commandID, 'messaging-chat')).toBe(false);
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).toHaveBeenCalledTimes(begins);
    expect(execute).toHaveBeenCalledTimes(1);
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  state.pathname = '/';
  state.mapReady = false;
  state.clearShortcut = false;
  state.messagingShortcut = null;
  state.modalOpen = false;
  state.result = 'succeeded';
  state.targetCurrent = true;
  state.commit.mockReset();
  state.filePrepare.mockReset(); state.fileCommit.mockReset(); state.fileApply.mockReset();
  state.begin.mockReset();
  state.take.mockReset();
  state.getResult.mockReset();
  state.complete.mockReset();
  state.cancel.mockReset();
  state.beginLocalCommandUIKey.mockReset();
  state.applyMode = undefined;
  state.modeViewBinding = true;
  state.modeViewShortcutCode = 'Digit3';
  state.contextualViewBinding = false;
  state.contextualViewByPage = false;
  state.contextualMarkdownBinding = false;
  state.simulateWorkspaceLandmarkBlock = false;
  (state.commandScope as ReturnType<typeof createCommandContextScope> | null)?.dispose();
  state.commandScope = null;
  state.focusView.mockReset();
  auth.user.userId = 'user-a';
  auth.user.sessionId = 'session-a';
  state.commitDeferred = false;
  state.resolveCommit = undefined;
  state.takeDeferred = false;
  state.resolveTake = undefined;
  state.cellCanOpen = undefined;
  state.cellOpen = undefined;
  state.topbarUnmount = undefined;
  useQuestionnaireUIStore.setState({ active: null, activeScope: null, queue: [], _activeResolve: null });
});

describe('Topbar editor.mode contextual integration', () => {
  it('após F6 no botão New tab, Alt+3 só refoca pelo binding efetivo de view', async () => {
    state.simulateWorkspaceLandmarkBlock = true;
    await mount('view', true);
    const toolbar = document.createElement('div');
    toolbar.className = 'workspace-toolbar';
    const landmark = document.createElement('button');
    landmark.type = 'button';
    landmark.textContent = 'New tab';
    toolbar.appendChild(landmark);
    document.body.appendChild(toolbar);
    try {
      landmark.focus();
      expect(landmark).toHaveFocus();
      fireEvent.keyDown(landmark, { key: '3', code: 'Digit3', altKey: true, bubbles: true });
      expect(screen.getByTestId('rendered-reading-document')).toHaveFocus();
      expect(state.focusView).toHaveBeenCalledTimes(1);
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.begin).not.toHaveBeenCalled();
      expect(state.take).not.toHaveBeenCalled();
      expect(state.commit).not.toHaveBeenCalled();
      expect(state.getResult).not.toHaveBeenCalled();
      expect(state.complete).not.toHaveBeenCalled();
    } finally {
      toolbar.remove();
    }
  });

  it('após F6, Alt+3 refoca pelo ramo byPage e não despacha comando', async () => {
    state.contextualViewBinding = true;
    state.contextualViewByPage = true;
    state.simulateWorkspaceLandmarkBlock = true;
    await mount('view', true);
    const toolbar = document.createElement('div');
    toolbar.className = 'workspace-toolbar';
    const landmark = document.createElement('button');
    landmark.type = 'button';
    landmark.textContent = 'New tab';
    toolbar.appendChild(landmark);
    document.body.appendChild(toolbar);
    try {
      landmark.focus();
      expect(landmark).toHaveFocus();
      fireEvent.keyDown(landmark, { key: '3', code: 'Digit3', altKey: true, bubbles: true });
      expect(screen.getByTestId('rendered-reading-document')).toHaveFocus();
      expect(state.focusView).toHaveBeenCalledTimes(1);
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.begin).not.toHaveBeenCalled();
      expect(state.take).not.toHaveBeenCalled();
      expect(state.commit).not.toHaveBeenCalled();
      expect(state.getResult).not.toHaveBeenCalled();
      expect(state.complete).not.toHaveBeenCalled();
    } finally {
      toolbar.remove();
    }
  });

  it('não amplia o lease pós-F6 para outro comando contextual do editor', async () => {
    state.contextualMarkdownBinding = true;
    state.simulateWorkspaceLandmarkBlock = true;
    await mount('view', true);
    const toolbar = document.createElement('div');
    toolbar.className = 'workspace-toolbar';
    const landmark = document.createElement('button');
    landmark.type = 'button';
    toolbar.appendChild(landmark);
    document.body.appendChild(toolbar);
    try {
      landmark.focus();
      fireEvent.keyDown(landmark, { key: '1', code: 'Digit1', altKey: true, bubbles: true });
      expect(landmark).toHaveFocus();
      expect(state.focusView).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.begin).not.toHaveBeenCalled();
      expect(state.take).not.toHaveBeenCalled();
      expect(state.commit).not.toHaveBeenCalled();
      expect(screen.getByTestId('editor-surface')).toHaveAttribute('data-mode', 'view');
    } finally {
      toolbar.remove();
    }
  });

  it('Alt+3 em view refoca apenas pela binding contextual, sem Begin/Take/Commit', async () => {
    await mount('view', true);
    const reading = screen.getByTestId('rendered-reading-document');
    expect(reading).not.toHaveFocus();
    fireEvent.keyDown(document.activeElement!, { key: '3', code: 'Digit3', altKey: true, bubbles: true });
    expect(reading).toHaveFocus();
    expect(state.focusView).toHaveBeenCalledTimes(1);
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled();
    expect(state.commit).not.toHaveBeenCalled();
    expect(state.getResult).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalled();
    expect(screen.getByTestId('editor-surface')).toHaveAttribute('data-mode', 'view');
  });

  it('não cria fallback quando Alt+3 não está no mapa efetivo', async () => {
    state.modeViewBinding = false;
    await mount('view', true);
    fireEvent.keyDown(document.activeElement!, { key: '3', code: 'Digit3', altKey: true, bubbles: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.focusView).not.toHaveBeenCalled();
    expect(screen.getByTestId('rendered-reading-document')).not.toHaveFocus();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.commit).not.toHaveBeenCalled();
  });

  it('segue o remapeamento efetivo e não mantém Alt+3 legado', async () => {
    state.modeViewShortcutCode = 'Digit4';
    await mount('view', true);
    fireEvent.keyDown(document.activeElement!, { key: '3', code: 'Digit3', altKey: true, bubbles: true });
    expect(state.focusView).not.toHaveBeenCalled();
    fireEvent.keyDown(document.activeElement!, { key: '4', code: 'Digit4', altKey: true, bubbles: true });
    expect(state.focusView).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('rendered-reading-document')).toHaveFocus();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.commit).not.toHaveBeenCalled();
  });

  it.each([
    ['owner', 'userId', 'other-user'],
    ['session', 'sessionId', 'other-session'],
  ] as const)('não refoca depois de trocar %s desde a captura do mapa', async (_label, key, replacement) => {
    await mount('view', true);
    const original = auth.user[key];
    auth.user[key] = replacement;
    try {
      fireEvent.keyDown(document.activeElement!, { key: '3', code: 'Digit3', altKey: true, bubbles: true });
      await act(async () => { await Promise.resolve(); });
      expect(state.focusView).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.begin).not.toHaveBeenCalled();
      expect(state.commit).not.toHaveBeenCalled();
    } finally {
      auth.user[key] = original;
    }
  });

  it('não refoca sob modal bloqueante', async () => {
    await mount('view');
    state.modalOpen = true;
    fireEvent.keyDown(document.activeElement!, { key: '3', code: 'Digit3', altKey: true, bubbles: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.focusView).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.commit).not.toHaveBeenCalled();
  });

  it.each([
    ['1', 'editor.mode.markdown', 'markdown', 'view'],
    ['2', 'editor.mode.rich', 'rich', 'view'],
    ['3', 'editor.mode.view', 'view', 'markdown'],
  ] as const)('Alt+%s commita %s antes de aplicar', async (key, commandID, mode, initialMode) => {
    await mount(initialMode);
    const surface = document.querySelector('[data-testid="editor-surface"]')!;
    fireEvent.keyDown(document.activeElement!, { key, code: `Digit${key}`, altKey: true, bubbles: true });
    await waitFor(() => expect(state.commit, `local=${state.beginLocalCommandUIKey.mock.calls.length}`).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(surface.getAttribute('data-mode')).toBe(mode));
    expect(state.take).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`);
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledWith(
      'generation-1',
      expect.objectContaining({ version: 1, code: `Digit${key}`, modifiers: ['Alt'] }),
      false,
    );
  });

  it('não aplica o modo quando o commit termina em falha', async () => {
    await mount('view');
    state.result = 'failed';
    const surface = document.querySelector('[data-testid="editor-surface"]')!;
    fireEvent.keyDown(document.activeElement!, { key: '1', code: 'Digit1', altKey: true, bubbles: true });
    await waitFor(() => expect(state.commit).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(state.getResult).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(surface.getAttribute('data-mode')).toBe('view'));
    expect(surface.getAttribute('data-mode')).toBe('view');
  });

  it('cancela sem commit quando a aba fica obsoleta entre take e commit', async () => {
    await mount('view');
    state.takeDeferred = true;
    fireEvent.keyDown(document.activeElement!, { key: '1', code: 'Digit1', altKey: true, bubbles: true });
    await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
    expect(state.commit).not.toHaveBeenCalled();
    state.targetCurrent = false;
    state.resolveTake?.();
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.mode.markdown', 'handoff-1', 'cancelled'));
    expect(state.commit).not.toHaveBeenCalled();
    expect(document.querySelector('[data-testid="editor-surface"]')?.getAttribute('data-mode')).toBe('view');
  });

  it('leva evento de menu ao mesmo pipeline e preserva commandID', async () => {
    await mount('view');
    const event = new CustomEvent<{ commandID: string }>(EDITOR_MODE_COMMAND_EVENT, {
      detail: { commandID: 'editor.mode.rich' }, cancelable: true,
    });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    await waitFor(() => expect(state.commit).toHaveBeenCalledTimes(1));
    expect(state.begin).toHaveBeenCalledWith('editor.mode.rich');
    expect(document.querySelector('[data-testid="editor-surface"]')?.getAttribute('data-mode')).toBe('rich');
  });

  it('requestEditorModeCommand usa o mesmo evento e pipeline', async () => {
    await mount('markdown');
    expect(requestEditorModeCommand('editor.mode.view')).toBe(true);
    await waitFor(() => expect(state.commit).toHaveBeenCalledTimes(1));
    expect(state.begin).toHaveBeenCalledWith('editor.mode.view');
  });

  it('não aplica antes da confirmação do commit', async () => {
    state.commitDeferred = true;
    await mount('view');
    const surface = document.querySelector('[data-testid="editor-surface"]')!;
    fireEvent.keyDown(document.activeElement!, { key: '1', code: 'Digit1', altKey: true, bubbles: true });
    await waitFor(() => expect(state.commit).toHaveBeenCalledTimes(1));
    expect(surface.getAttribute('data-mode')).toBe('view');
    state.resolveCommit?.();
    await waitFor(() => expect(surface.getAttribute('data-mode')).toBe('markdown'));
  });

  it('ignora repetição automática sem iniciar uma gravação', async () => {
    await mount('view');
    fireEvent.keyDown(document.activeElement!, { key: '1', code: 'Digit1', altKey: true, repeat: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.commit).not.toHaveBeenCalled();
  });

  it('aplica pelo Deck usando a reserva existente, sem novo Begin', async () => {
    await mount('view');
    await act(async () => {
      state.events.get('command:deck-ui-reservation')?.({
        ticket: 'ticket-editor.mode.rich', invocationId: 'invocation-1', commandId: 'editor.mode.rich',
      });
    });
    await waitFor(() => expect(screen.getByTestId('editor-surface')).toHaveAttribute('data-mode', 'rich'));
    expect(state.commit).toHaveBeenCalledTimes(1);
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
  });

  it('aplica pela paleta com Combobox, registry e coordenador reais', async () => {
    await mount('view');
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await screen.findByRole('combobox');
    await user.type(screen.getByRole('combobox'), 'Modo rico');
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(screen.getByTestId('editor-surface')).toHaveAttribute('data-mode', 'rich'));
    expect(state.begin).toHaveBeenCalledExactlyOnceWith('editor.mode.rich');
    expect(state.commit).toHaveBeenCalledTimes(1);
  });
});

describe('Topbar destructive chat clear integration', () => {
  let unregister: (() => void) | undefined;
  const succeeded = vi.fn();
  afterEach(() => { unregister?.(); unregister = undefined; succeeded.mockReset(); });
  async function chat() {
    state.clearShortcut = true;
    await mount();
    const root = screen.getByTestId('editor-surface');
    const input = document.querySelector('[aria-label="editor input"]')!;
    input.setAttribute('data-testid', 'chat-input');
    unregister = registerChatClearSurface({
      root, instanceId: 'chat-clear-test', isCurrent: () => state.targetCurrent,
      canStart: () => !document.querySelector('[role="alertdialog"], [role="listbox"]'),
      subscribe: () => () => undefined, succeeded,
    });
    return input;
  }
  it.each(['button', 'keyboard', 'palette', 'deck'] as const)('%s shares confirmed backend commit', async source => {
    const input = await chat();
    state.takeDeferred = true;
    if (source === 'button') requestChatClear('chat-clear-test');
    if (source === 'keyboard') fireEvent.keyDown(input, { key: 'l', code: 'KeyL', ctrlKey: true });
    if (source === 'deck') act(() => state.events.get('command:deck-ui-reservation')?.({
      ticket: `ticket-${CHAT_CLEAR_COMMAND}`, invocationId: 'invocation-1', commandId: CHAT_CLEAR_COMMAND,
    }));
    if (source === 'palette') {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await user.type(await screen.findByRole('combobox'), 'Limpar conversa');
      await user.keyboard('{ArrowDown}{Enter}');
    }
    await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
    expect(state.commit).not.toHaveBeenCalled();
    expect(succeeded).not.toHaveBeenCalled();
    await act(async () => state.resolveTake?.());
    await waitFor(() => expect(succeeded).toHaveBeenCalledTimes(1));
    expect(state.commit).toHaveBeenCalledExactlyOnceWith(`ticket-${CHAT_CLEAR_COMMAND}`, 'handoff-1');
    expect(state.complete).not.toHaveBeenCalledWith(expect.anything(), expect.anything(), 'succeeded');
    if (source === 'deck') expect(state.begin).not.toHaveBeenCalled();
  });
  it('does not commit if the captured conversation changes during confirmation', async () => {
    await chat(); state.takeDeferred = true;
    requestChatClear('chat-clear-test');
    await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
    state.targetCurrent = false;
    await act(async () => state.resolveTake?.());
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-${CHAT_CLEAR_COMMAND}`, 'handoff-1', 'cancelled'));
    expect(state.commit).not.toHaveBeenCalled();
    expect(succeeded).not.toHaveBeenCalled();
  });
  it('ignores repeat and ordinary editable controls', async () => {
    const input = await chat();
    fireEvent.keyDown(input, { key: 'l', code: 'KeyL', ctrlKey: true, repeat: true });
    input.removeAttribute('data-testid');
    fireEvent.keyDown(input, { key: 'l', code: 'KeyL', ctrlKey: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.commit).not.toHaveBeenCalled();
  });
});

describe('Topbar rich formatting integration', () => {
  const cleanup: Array<() => void> = [];
  afterEach(() => { cleanup.splice(0).reverse().forEach(fn => fn()); });
  async function rich() {
    await mount('rich');
    // JSDOM has no layout; keep real editor selection/transactions, stub only scrolling.
    const root = document.createElement('div');
    root.className = 'rich-text-editor';
    document.body.append(root);
    const editor = new Editor({ element: root, editorProps: { handleScrollToSelection: () => true }, content: 'hello world', extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: 'image', imageLabelPrefix: 'image' }) });
    const unregister = registerEditorFormatting({ root, editor, isCurrent: () => state.targetCurrent, subscribe: () => () => undefined });
    editor.commands.setTextSelection({ from: 1, to: 6 });
    editor.view.dom.setAttribute('contenteditable', 'true');
    editor.view.dom.focus();
    state.cellCanOpen = (commandID) => canNavigateCell(editor, commandID);
    state.cellOpen = (commandID) => navigateEditorCell(editor, commandID);
    cleanup.push(() => { unregister(); editor.destroy(); root.remove(); });
    return editor;
  }
  async function tableRich() {
    const editor = await rich();
    editor.commands.setContent({ type: 'doc', content: [{ type: 'table', content: [{ type: 'tableRow', content: [
      { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'a' }] }] },
      { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'b' }] }] },
      { type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'c' }] }] },
    ] }] }] });
    const firstCell: number[] = [];
    editor.state.doc.descendants((node, pos) => {
      if (node.type.name === 'tableCell' && firstCell.length === 0) firstCell.push(pos);
    });
    editor.commands.setCellSelection({ anchorCell: firstCell[0], headCell: firstCell[0] });
    editor.view.focus();
    return editor;
  }
  it.each([['b', 'KeyB', false, 'bold'], ['i', 'KeyI', false, 'italic'], ['X', 'KeyX', true, 'strike']] as const)('keyboard applies %s once through admission', async (key, code, shiftKey, mark) => {
    const editor = await rich();
    fireEvent.keyDown(editor.view.dom, { key, code, ctrlKey: true, shiftKey });
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-editor.format.${mark}`, 'handoff-1', 'succeeded'));
    expect(editor.state.doc.rangeHasMark(1, 6, editor.schema.marks[mark])).toBe(true);
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledTimes(1);
    expect(state.begin).not.toHaveBeenCalled();
  });
  it('palette preserves original rich selection', async () => {
    const editor = await rich();
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await user.type(await screen.findByRole('combobox'), 'Negrito');
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.bold', 'handoff-1', 'succeeded'));
    expect(editor.state.doc.rangeHasMark(1, 6, editor.schema.marks.bold)).toBe(true);
    expect(state.begin).toHaveBeenCalledExactlyOnceWith('editor.format.bold');
  });
  it.each(['menu', 'palette', 'deck'] as const)('code block real via %s converges without a duplicate handler', async origin => {
    const editor = await rich();
    const commandID = 'editor.format.code_block';
    if (origin === 'menu') requestEditorFormatCommand(commandID);
    if (origin === 'deck') await act(async () => state.events.get('command:deck-ui-reservation')?.({
      ticket: `ticket-${commandID}`, invocationId: 'invocation-1', commandId: commandID,
    }));
    if (origin === 'palette') {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await user.type(await screen.findByRole('combobox'), 'Bloco de código');
      await user.keyboard('{ArrowDown}{Enter}');
    }
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-${commandID}`, 'handoff-1', 'succeeded'));
    expect(editor.state.doc.firstChild?.type.name).toBe('codeBlock');
    expect(editor.state.doc.textContent).toBe('hello world');
    expect(state.take).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`);
    expect(state.complete).toHaveBeenCalledTimes(1);
    if (origin === 'deck') {
      expect(state.begin).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    } else {
      expect(state.begin).toHaveBeenCalledExactlyOnceWith(commandID);
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    }
  });
  it('Ctrl+Alt+C uses the default local binding and transforms the real editor once', async () => {
    const editor = await rich();
    fireEvent.keyDown(editor.view.dom, { key: 'Control', code: 'ControlLeft', ctrlKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'Alt', code: 'AltLeft', ctrlKey: true, altKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'c', code: 'KeyC', ctrlKey: true, altKey: true });
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.code_block', 'handoff-1', 'succeeded'));
    expect(editor.state.doc.firstChild?.type.name).toBe('codeBlock');
    expect(editor.state.doc.textContent).toBe('hello world');
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledExactlyOnceWith(
      'generation-1', expect.objectContaining({ version: 1, code: 'KeyC', modifiers: ['Control', 'Alt'] }), false,
    );
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.take).toHaveBeenCalledExactlyOnceWith('ticket-editor.format.code_block');
    expect(state.complete).toHaveBeenCalledTimes(1);
  });
  it('não transforma quando o contexto capturado fica obsoleto durante Take', async () => {
    const editor = await rich();
    state.takeDeferred = true;
    fireEvent.keyDown(editor.view.dom, { key: 'Control', code: 'ControlLeft', ctrlKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'Alt', code: 'AltLeft', ctrlKey: true, altKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'c', code: 'KeyC', ctrlKey: true, altKey: true });
    await waitFor(() => expect(state.resolveTake).toBeTypeOf('function'));
    state.targetCurrent = false;
    await act(async () => state.resolveTake?.());
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.code_block', 'handoff-1', 'cancelled'));
    expect(editor.state.doc.firstChild?.type.name).toBe('paragraph');
    expect(state.complete).toHaveBeenCalledTimes(1);
  });
  it('rejeita Ctrl+Alt+C em editor readonly antes de qualquer admissão', async () => {
    const editor = await rich();
    editor.setEditable(false);
    fireEvent.keyDown(editor.view.dom, { key: 'c', code: 'KeyC', ctrlKey: true, altKey: true });
    await act(async () => { await Promise.resolve(); });
    expect(editor.state.doc.firstChild?.type.name).toBe('paragraph');
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalled();
  });
  it.each([
    ['0', 'Digit0', true, false, 'paragraph', 'paragraph'],
    ...([1, 2, 3, 4, 5, 6] as const).map(level => [`${level}`, `Digit${level}`, true, false, `heading.h${level}`, 'heading'] as const),
    ['B', 'KeyB', false, true, 'blockquote', 'blockquote'],
    ['c', 'KeyC', true, false, 'code_block', 'codeBlock'],
    ['*', 'Digit8', false, true, 'list.bullet', 'bulletList'],
    ['&', 'Digit7', false, true, 'list.ordered', 'orderedList'],
  ] as const)('routes expanded shortcut %s via the common executor', async (key, code, altKey, shiftKey, suffix, node) => {
    const editor = await rich();
    if (suffix === 'paragraph') editor.commands.setHeading({ level: 2 });
    if (altKey) {
      fireEvent.keyDown(editor.view.dom, { key: 'Control', code: 'ControlLeft', ctrlKey: true });
      fireEvent.keyDown(editor.view.dom, { key: 'Alt', code: 'AltLeft', ctrlKey: true, altKey: true });
    }
    fireEvent.keyDown(editor.view.dom, { key, code, ctrlKey: true, altKey, shiftKey });
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-editor.format.${suffix}`, 'handoff-1', 'succeeded'));
    expect(editor.isActive(node)).toBe(true);
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledTimes(1);
  });
  it('palette preserves a real table CellSelection for merge', async () => {
    const editor = await rich();
    editor.commands.setContent({ type: 'doc', content: [{ type: 'table', content: [{ type: 'tableRow', content: ['a', 'b'].map(text => ({ type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text }] }] })) }] }] });
    const cells: number[] = [];
    editor.state.doc.descendants((node, pos) => { if (node.type.name === 'tableCell') cells.push(pos); });
    editor.commands.setCellSelection({ anchorCell: cells[0], headCell: cells[1] });
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await user.type(await screen.findByRole('combobox'), 'Mesclar células');
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.table.merge', 'handoff-1', 'succeeded'));
    expect(editor.state.doc.firstChild?.firstChild?.childCount).toBe(1);
    expect(editor.getText()).toContain('a');
    expect(editor.getText()).toContain('b');
  });
  it('Deck table deletion uses the original reservation', async () => {
    const editor = await rich();
    editor.commands.setContent({ type: 'doc', content: [{ type: 'table', content: [{ type: 'tableRow', content: [{ type: 'tableCell', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'a' }] }] }] }] }] });
    const firstCell: number[] = [];
    editor.state.doc.descendants((node, pos) => {
      if (node.type.name === 'tableCell' && firstCell.length === 0) firstCell.push(pos);
    });
    editor.commands.setCellSelection({ anchorCell: firstCell[0], headCell: firstCell[0] });
    await act(async () => state.events.get('command:deck-ui-reservation')?.({ ticket: 'ticket-editor.format.table.delete', invocationId: 'invocation-1', commandId: 'editor.format.table.delete' }));
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.table.delete', 'handoff-1', 'succeeded'));
    expect(editor.getHTML()).not.toContain('<table');
    expect(state.begin).not.toHaveBeenCalled();
  });
  it('menu uses the same admitted execution', async () => {
    const editor = await rich();
    requestEditorFormatCommand('editor.format.bold');
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.bold', 'handoff-1', 'succeeded'));
    expect(editor.isActive('bold')).toBe(true);
  });
  it('cell-navigation event captures presentation lease and executes after menu close', async () => {
    const editor = await tableRich();
    const before = selectionCell(editor.state)?.pos;
    requestEditorCellNavigationCommand('editor.table.cell.next');
    await waitFor(() => expect(selectionCell(editor.state)?.pos).toBeGreaterThan(before ?? -1));
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalled();
  });
  it('custom local keyboard navigation uses real table capability without handoff', async () => {
    const editor = await tableRich();
    const cellPosition = () => selectionCell(editor.state)?.pos;
    const first = cellPosition();
    fireEvent.keyDown(editor.view.dom, { key: 'Control', code: 'ControlLeft', ctrlKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'Alt', code: 'AltLeft', ctrlKey: true, altKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'ArrowRight', code: 'ArrowRight', ctrlKey: true, altKey: true });
    await waitFor(() => expect(cellPosition()).toBeGreaterThan(first ?? -1));
    const second = cellPosition();
    fireEvent.keyDown(editor.view.dom, { key: 'ArrowRight', code: 'ArrowRight', ctrlKey: true, altKey: true, repeat: true });
    await waitFor(() => expect(cellPosition()).toBeGreaterThan(second ?? -1));
    const third = cellPosition();
    fireEvent.keyDown(editor.view.dom, { key: 'ArrowRight', code: 'ArrowRight', ctrlKey: true, altKey: true, repeat: true });
    await act(async () => { await Promise.resolve(); });
    expect(cellPosition()).toBe(third);
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalled();
  });
  it('Deck local navigation executes without Begin/Take/Complete', async () => {
    const editor = await tableRich();
    const before = selectionCell(editor.state)?.pos;
    await act(async () => state.events.get('command:deck-local-ui')?.({
      commandId: 'editor.table.cell.next', generation: 'generation-1',
      userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    }));
    await waitFor(() => expect(selectionCell(editor.state)?.pos).toBeGreaterThan(before ?? -1));
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalled();
  });
  it('keyboard link questionnaire cancela sem Begin nem mutação', async () => {
    const editor = await rich();
    fireEvent.keyDown(editor.view.dom, { key: 'Control', code: 'ControlLeft', ctrlKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'l', code: 'KeyL', ctrlKey: true });
    await waitFor(() => expect(useQuestionnaireUIStore.getState().active).not.toBeNull());
    useQuestionnaireUIStore.getState().cancel();
    await waitFor(() => expect(useQuestionnaireUIStore.getState().active).toBeNull());
    expect(state.begin).not.toHaveBeenCalled();
    expect(editor.getHTML()).not.toContain('href=');
  });
  it('desmontagem do Topbar aborta questionnaire de link pendente', async () => {
    const editor = await rich();
    fireEvent.keyDown(editor.view.dom, { key: 'Control', code: 'ControlLeft', ctrlKey: true });
    fireEvent.keyDown(editor.view.dom, { key: 'l', code: 'KeyL', ctrlKey: true });
    await waitFor(() => expect(useQuestionnaireUIStore.getState().active).not.toBeNull());
    state.topbarUnmount?.();
    await waitFor(() => expect(useQuestionnaireUIStore.getState().active).toBeNull());
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled();
    expect(editor.getHTML()).not.toContain('href=');
  });
  it('keyboard link waits for input before admission and preserves the selected text', async () => {
    const editor = await rich();
    fireEvent.keyDown(editor.view.dom, { key: 'l', code: 'KeyL', ctrlKey: true });
    await waitFor(() => expect(useQuestionnaireUIStore.getState().active).not.toBeNull());
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled();
    act(() => useQuestionnaireUIStore.getState().submit({ href: 'https://example.com' }));
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.link.set', 'handoff-1', 'succeeded'));
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledTimes(1);
    expect(state.begin).not.toHaveBeenCalled();
    expect(editor.state.doc.textContent).toBe('hello world');
    expect(editor.state.doc.rangeHasMark(1, 6, editor.schema.marks.link)).toBe(true);
    expect(editor.state.doc.rangeHasMark(6, 12, editor.schema.marks.link)).toBe(false);
  });
  it.each(['confirm', 'cancel'])('Deck table questionnaire %s reuses or cancels the original reservation', async outcome => {
    const editor = await rich();
    const before = editor.getJSON();
    await act(async () => state.events.get('command:deck-ui-reservation')?.({
      ticket: 'ticket-editor.format.table.insert', invocationId: 'invocation-1', commandId: 'editor.format.table.insert',
    }));
    await waitFor(() => expect(useQuestionnaireUIStore.getState().active).not.toBeNull());
    expect(state.take).not.toHaveBeenCalled();
    act(() => {
      if (outcome === 'confirm') useQuestionnaireUIStore.getState().submit({ rows: '2', cols: '3', withHeaderRow: true });
      else useQuestionnaireUIStore.getState().cancel();
    });
    if (outcome === 'confirm') {
      await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.table.insert', 'handoff-1', 'succeeded'));
      expect(editor.view.dom.querySelectorAll('table')).toHaveLength(1);
      expect(editor.view.dom.querySelectorAll('th')).toHaveLength(3);
    } else {
      await waitFor(() => expect(state.cancel).toHaveBeenCalledWith('ticket-editor.format.table.insert'));
      expect(editor.getJSON()).toEqual(before);
      expect(state.take).not.toHaveBeenCalled();
    }
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
  });
  it('event origin accepts a prepared table payload before Begin', async () => {
    const editor = await rich();
    requestEditorFormatCommand('editor.format.table.insert', {
      kind: 'table', rows: 2, cols: 3, withHeaderRow: true,
    });
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.table.insert', 'handoff-1', 'succeeded'));
    expect(editor.state.doc.firstChild?.type.name).toBe('table');
    expect(editor.state.doc.firstChild?.firstChild?.firstChild?.type.name).toBe('tableHeader');
  });
  it('event origin inserts a plain code block without opening input UI', async () => {
    const editor = await rich();
    requestEditorFormatCommand('editor.format.code_block.insert');
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.code_block.insert', 'handoff-1', 'succeeded'));
    expect(editor.state.doc.firstChild?.type.name).toBe('codeBlock');
    expect(editor.state.doc.firstChild?.attrs.language).toBe('');
  });
  it('Deck consumes the supplied reservation without new Begin', async () => {
    const editor = await rich();
    await act(async () => state.events.get('command:deck-ui-reservation')?.({ ticket: 'ticket-editor.format.bold', invocationId: 'invocation-1', commandId: 'editor.format.bold' }));
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.bold', 'handoff-1', 'succeeded'));
    expect(editor.isActive('bold')).toBe(true);
    expect(state.begin).not.toHaveBeenCalled();
  });
  it('selection change while Take waits cancels instead of retargeting', async () => {
    const editor = await rich();
    state.takeDeferred = true;
    fireEvent.keyDown(editor.view.dom, { key: 'b', code: 'KeyB', ctrlKey: true });
    await waitFor(() => expect(state.resolveTake).toBeDefined());
    editor.commands.setTextSelection(8);
    await act(async () => state.resolveTake?.());
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.format.bold', 'handoff-1', 'cancelled'));
    expect(editor.state.doc.rangeHasMark(1, 6, editor.schema.marks.bold)).toBe(false);
  });
  it('does not format the document from another text field', async () => {
    const editor = await rich();
    const other = screen.getByRole('textbox', { name: 'editor input' });
    other.focus();
    fireEvent.keyDown(other, { key: 'b', code: 'KeyB', ctrlKey: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(editor.isActive('bold')).toBe(false);
  });
});

describe('Topbar inserções capturadas em Markdown e slides', () => {
  const cleanup: Array<() => void> = [];
  afterEach(() => cleanup.splice(0).forEach(dispose => dispose()));
  async function insertionSurface(slide = false) {
    await mount('markdown');
    const root = document.createElement('div');
    root.className = 'monaco-editor';
    const input = document.createElement('textarea');
    root.append(input);
    screen.getByTestId('editor-surface').append(root);
    input.focus();
    let value = 'Original';
    let version = 1;
    const listeners = new Set<() => void>();
    const subscribe = (listener: () => void) => { listeners.add(listener); return { dispose: () => { listeners.delete(listener); } }; };
    const model = {
      getVersionId: () => version, getValue: () => value, getValueInRange: () => value,
      getOffsetAt: (position: { column: number }) => position.column - 1, getPositionAt: (offset: number) => ({ lineNumber: 1, column: offset + 1 }),
      getLineContent: () => value, isDisposed: () => false, onDidChangeContent: subscribe,
    };
    const selection = { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 9 };
    const edit = vi.fn((_source: string, edits: Array<{ text: string }>) => {
      value = edits[0].text; version++; listeners.forEach(listener => listener()); return true;
    });
    const editor = {
      getModel: () => model, getSelection: () => selection, getDomNode: () => root,
      getOption: () => false, executeEdits: edit, pushUndoStop: () => true, focus: () => input.focus(),
      setPosition: vi.fn(), revealPositionInCenter: vi.fn(),
      onDidChangeCursorSelection: () => ({ dispose() {} }), onDidChangeModel: subscribe,
      onDidDispose: subscribe, onDidCompositionStart: subscribe,
      onDidChangeConfiguration: () => ({ dispose() {} }),
    };
    const unregister = registerEditorFormatAdapter({ capture() {
      if (!slide) return captureEditorMarkdown(editor as never, { editor: { EditorOption: { readOnly: 1 } } } as never,
        root, () => state.targetCurrent);
      let disposed = false;
      let prepared: string | undefined;
      return {
        isCurrent: () => !disposed && state.targetCurrent,
        canExecute: id => !disposed && state.targetCurrent && isEditorSlideCommand(id),
        async prepare(id) { prepared = buildEditorSlideTemplate(id); return !!prepared; },
        execute() {
          if (disposed || !prepared || !state.targetCurrent) return false;
          edit('slide', [{ text: appendEditorSlideMarkdown(value, prepared) }]);
          return true;
        },
        dispose() { disposed = true; },
      };
    } });
    cleanup.push(() => { unregister(); root.remove(); });
    return { edit, input, value: () => value };
  }
  it.each(['menu', 'palette', 'deck'] as const)('slide por %s passa pelo executor e só o ID vai ao backend', async origin => {
    const surface = await insertionSurface(true);
    const id = 'editor.slide.insert.basic';
    if (origin === 'menu') requestEditorFormatCommand(id);
    if (origin === 'deck') await act(async () => state.events.get('command:deck-ui-reservation')?.({ ticket: `ticket-${id}`, invocationId: 'invocation-1', commandId: id }));
    if (origin === 'palette') {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await user.type(await screen.findByRole('combobox'), 'Inserir slide básico');
      await user.keyboard('{ArrowDown}{Enter}');
    }
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-${id}`, 'handoff-1', 'succeeded'));
    expect(surface.edit).toHaveBeenCalledTimes(1);
    expect(surface.value()).toContain('Original\n\n---\n\n<!-- .slide:');
    if (origin === 'deck') expect(state.begin).not.toHaveBeenCalled();
    else expect(state.begin).toHaveBeenCalledExactlyOnceWith(id);
  });
  it.each(['keyboard', 'palette', 'deck'] as const)('inserção Markdown real por %s usa a seleção original', async origin => {
    const surface = await insertionSurface();
    const id = 'editor.format.list.bullet';
    if (origin === 'keyboard') fireEvent.keyDown(surface.input, { key: '*', code: 'Digit8', ctrlKey: true, shiftKey: true });
    if (origin === 'deck') await act(async () => state.events.get('command:deck-ui-reservation')?.({ ticket: `ticket-${id}`, invocationId: 'invocation-1', commandId: id }));
    if (origin === 'palette') {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await user.type(await screen.findByRole('combobox'), 'Lista com marcadores');
      await user.keyboard('{ArrowDown}{Enter}');
    }
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-${id}`, 'handoff-1', 'succeeded'));
    expect(surface.edit).toHaveBeenCalledTimes(1);
    expect(surface.value()).toBe('- Original');
    if (origin === 'keyboard') expect(state.beginLocalCommandUIKey).toHaveBeenCalledTimes(1);
    if (origin === 'deck') expect(state.begin).not.toHaveBeenCalled();
  });
  it('documento obsoleto durante Take não recebe slide', async () => {
    const surface = await insertionSurface(true);
    state.takeDeferred = true;
    requestEditorFormatCommand('editor.slide.insert.basic');
    await waitFor(() => expect(state.resolveTake).toBeDefined());
    state.targetCurrent = false;
    await act(async () => state.resolveTake?.());
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.slide.insert.basic', 'handoff-1', 'cancelled'));
    expect(surface.edit).not.toHaveBeenCalled();
  });
});

describe('Topbar editor.file durable integration', () => {
  it.each([
    ['s', 'KeyS', false, 'editor.file.save'],
    ['o', 'KeyO', false, 'editor.file.open'],
    ['S', 'KeyS', true, 'editor.file.save_copy'],
  ] as const)('executa %s shift=%s no campo de texto', async (key, code, shiftKey, commandID) => {
    await mount();
    fireEvent.keyDown(document.activeElement!, { key, code, ctrlKey: true, shiftKey });
    await waitFor(() => expect(state.fileCommit).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(state.fileApply).toHaveBeenCalledTimes(1));
    expect(state.take).toHaveBeenCalledWith(`ticket-${commandID}`);
    expect(state.fileCommit).toHaveBeenCalledWith(expect.objectContaining({ commandId: commandID }), 'file-token', false);
    expect(state.complete).not.toHaveBeenCalled();
  });

  it('usa a reserva física sem novo Begin', async () => {
    await mount();
    await act(async () => state.events.get('command:deck-ui-reservation')?.({ ticket: 'ticket-editor.file.save', invocationId: 'invocation-1', commandId: 'editor.file.save' }));
    await waitFor(() => expect(state.fileApply).toHaveBeenCalledTimes(1));
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
  });

  it('encaminha o menu para a mesma preparação e commit', async () => {
    await mount();
    const event = new CustomEvent('commands:editor-file', { detail: { commandID: 'editor.file.save' }, cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    await waitFor(() => expect(state.fileApply).toHaveBeenCalledTimes(1));
    expect(state.begin).toHaveBeenCalledWith('editor.file.save');
  });

  it('executa a seleção pela paleta real', async () => {
    await mount();
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await screen.findByRole('combobox');
    await user.type(screen.getByRole('combobox'), 'Salvar arquivo');
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(state.fileApply).toHaveBeenCalledTimes(1));
    expect(state.begin).toHaveBeenCalledWith('editor.file.save');
  });

  it('não aplica antes da confirmação nem depois de falha durável', async () => {
    await mount();
    let release!: () => void;
    state.fileCommit.mockImplementation(() => new Promise(resolve => { release = () => resolve({ tabId: 'editor-tab', path: 'file.md', written: true }); }));
    fireEvent.keyDown(document.activeElement!, { key: 's', code: 'KeyS', ctrlKey: true });
    await waitFor(() => expect(state.fileCommit).toHaveBeenCalledTimes(1));
    expect(state.fileApply).not.toHaveBeenCalled();
    state.result = 'failed';
    await act(async () => release());
    await waitFor(() => expect(state.getResult).toHaveBeenCalledTimes(1));
    expect(state.fileApply).not.toHaveBeenCalled();
  });

  it('cancela após o diálogo se o alvo mudou, sem gravar', async () => {
    await mount();
    state.filePrepare.mockImplementation(async () => { state.targetCurrent = false; return { token: 'token', path: 'file.md', cancelled: false, requiresOverwrite: false }; });
    fireEvent.keyDown(document.activeElement!, { key: 's', code: 'KeyS', ctrlKey: true });
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith('ticket-editor.file.save', 'handoff-1', 'cancelled'));
    expect(state.fileCommit).not.toHaveBeenCalled();
  });
});
