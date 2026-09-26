import { render, waitFor } from '@testing-library/react';
import * as React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData, type WorkspaceTab } from '../store/workspaceStore';
import { acquireCommandFocusTracking } from './commandFocusContext';
import {
  capturePagePresentationTarget,
  captureCommandSettingsKeyboardContext,
  COMMAND_SETTINGS_CREATE_COMMAND_ID,
  type PagePresentationCommandID,
  usePagePresentationCommands,
} from './commandPagePresentation';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

const command: PagePresentationCommandID = 'tasklists.edit.open';
const tab: WorkspaceTab = { id: 'tab-a', type: 'tasklist', title: 'Lista', position: 0 };

function workspace(id = 'workspace-a', activeTabId: string | null = tab.id): WorkspaceData {
  return { id, name: id, tabs: [tab], activeTabId };
}

function setOwner(userId = 'user-a', sessionId = 'session-a', workspaceId = 'workspace-a', activeTabId: string | null = tab.id) {
  useAuthStore.setState({ isAuthenticated: true, user: { userId, sessionId, role: 'user' } });
  useWorkspaceStore.setState({ workspace: workspace(workspaceId, activeTabId) });
}

interface HarnessState {
  target: object;
  current: boolean;
  canOpen: boolean;
  notify: (changed: () => void) => () => void;
  report: (state: HarnessReport) => void;
}

interface HarnessReport {
  root: HTMLDivElement;
  request: (id: PagePresentationCommandID) => boolean;
  open: ReturnType<typeof vi.fn>;
  listeners: Set<() => void>;
}

function Harness({ state, pathname = '/tasklists', tabId = tab.id, commands = [command], report, settingsManager = false }: {
  state: HarnessState;
  pathname?: string;
  tabId?: string;
  commands?: readonly PagePresentationCommandID[];
  settingsManager?: boolean;
  report: (report: HarnessReport) => void;
}) {
  const root = React.useRef<HTMLDivElement>(null);
  const listeners = React.useRef(new Set<() => void>());
  const open = React.useMemo(() => vi.fn(() => true), []);
  const { request } = usePagePresentationCommands({
    root,
    pathname,
    tabId,
    settingsManager,
    allowedCommands: commands,
    readTarget: () => state.target,
    isCurrent: () => state.current,
    canOpen: () => state.canOpen,
    open,
    subscribe: (changed) => {
      listeners.current.add(changed);
      return () => listeners.current.delete(changed);
    },
  });

  React.useLayoutEffect(() => {
    if (root.current) report({ root: root.current, request, open, listeners: listeners.current });
  }, [open, report, request]);

  return <div ref={root} tabIndex={-1} data-testid="page-source"
    className={settingsManager ? 'modal-overlay' : undefined}
    data-modal-id={settingsManager ? 'page-presentation-test-modal' : undefined} />;
}

function capture(readPathname: () => string = () => '/tasklists', id: string = command, instanceId?: string) {
  return capturePagePresentationTarget(readPathname, id, instanceId);
}

function instanceIdFromRequest(report: HarnessReport, id: PagePresentationCommandID): string {
  let instanceId = '';
  const listener = (event: Event) => {
    instanceId = (event as CustomEvent<{ instanceId?: string }>).detail.instanceId ?? '';
  };
  window.addEventListener('commands:page-presentation', listener);
  report.request(id);
  window.removeEventListener('commands:page-presentation', listener);
  return instanceId;
}

function notify(report: HarnessReport) {
  [...report.listeners].forEach((changed) => changed());
}

let releaseFocusTracking: (() => void) | undefined;

beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  releaseFocusTracking = acquireCommandFocusTracking(document);
  setOwner();
});

afterEach(() => {
  unregisterOpenModal('page-presentation-test-modal');
  document.body.replaceChildren();
  useAuthStore.setState({ isAuthenticated: false, user: null });
  useWorkspaceStore.setState({ workspace: null });
  releaseFocusTracking?.();
  releaseFocusTracking = undefined;
  vi.restoreAllMocks();
});

describe('commandPagePresentation', () => {
  it.each(['valid', 'target', 'owner', 'workspace', 'route', 'hidden', 'focus', 'modal-generation'] as const)(
    'licença do gerenciador contém apenas criação e invalida em %s', async change => {
      const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
      let latest!: HarnessReport;
      let path = '/settings/commands';
      render(<Harness state={state} pathname={path} settingsManager commands={[COMMAND_SETTINGS_CREATE_COMMAND_ID]}
        report={report => { latest = report; }} />);
      registerOpenModal('page-presentation-test-modal'); latest.root.focus();
      const lease = captureCommandSettingsKeyboardContext(() => path);
      expect(lease?.allowedCommandIds).toEqual([COMMAND_SETTINGS_CREATE_COMMAND_ID]);
      expect(lease?.isCurrent()).toBe(true);
      if (change === 'target') state.target = {};
      if (change === 'owner') setOwner('another-user');
      if (change === 'workspace') setOwner('user-a', 'session-a', 'workspace-b');
      if (change === 'route') path = '/profiles';
      if (change === 'hidden') latest.root.hidden = true;
      if (change === 'focus') { const button = document.createElement('button'); document.body.append(button); button.focus(); }
      if (change === 'modal-generation') {
        unregisterOpenModal('page-presentation-test-modal');
        registerOpenModal('page-presentation-test-modal');
      }
      expect(lease?.isCurrent()).toBe(change === 'valid');
    },
  );

  it('não estende a licença de modal a outros comandos de apresentação', () => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    render(<Harness state={state} pathname="/settings/commands" settingsManager
      commands={[COMMAND_SETTINGS_CREATE_COMMAND_ID, 'profiles.create.open']} report={report => { latest = report; }} />);
    registerOpenModal('page-presentation-test-modal'); latest.root.focus();
    expect(captureCommandSettingsKeyboardContext(() => '/settings/commands')).toBeUndefined();
    expect(capture(() => '/settings/commands', 'profiles.create.open')).toBeUndefined();
  });
  it('registra o hook real, executa uma vez e recusa canOpen falso', async () => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    const view = render(<Harness state={state} report={(report) => { latest = report; }} />);
    await waitFor(() => expect(latest).toBeDefined());
    latest.root.focus();

    const lease = capture()!;
    expect(lease.canOpen(command)).toBe(true);
    expect(lease.open(command)).toBe(true);
    expect(lease.open(command)).toBe(false);
    expect(latest.open).toHaveBeenCalledOnce();

    state.canOpen = false;
    expect(capture()).toBeUndefined();
    view.unmount();
  });

  it.each([
    ['owner', () => { const user = useAuthStore.getState().user!; useAuthStore.setState({ user: { ...user, userId: 'other' } }); }],
    ['session', () => { const user = useAuthStore.getState().user!; useAuthStore.setState({ user: { ...user, sessionId: 'other' } }); }],
    ['workspace', () => { useWorkspaceStore.setState({ workspace: workspace('other') }); }],
    ['activeTab', () => { useWorkspaceStore.setState({ workspace: workspace('workspace-a', 'other-tab') }); }],
  ] as const)('invalida a lease quando muda %s', async (_kind, change) => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    render(<Harness state={state} report={(report) => { latest = report; }} />);
    await waitFor(() => expect(latest).toBeDefined());
    latest.root.focus();
    const lease = capture()!;
    change();
    expect(lease.isCurrent()).toBe(false);
    expect(latest.listeners).toHaveLength(0);
  });

  it('invalida selected object ABA mesmo com o mesmo id lógico', async () => {
    const selected = { id: 'same-id', version: 1 };
    const state: HarnessState = { target: selected, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    render(<Harness state={state} report={(report) => { latest = report; }} />);
    await waitFor(() => expect(latest).toBeDefined());
    latest.root.focus();
    const lease = capture()!;
    state.target = { id: 'same-id', version: 2 };
    notify(latest);
    expect(lease.isCurrent()).toBe(false);
    expect(latest.listeners).toHaveLength(0);
  });

  it('recusa ambiguidade e aceita somente instanceId explícito', async () => {
    const first: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    const second: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let firstReport!: HarnessReport;
    let secondReport!: HarnessReport;
    render(<><Harness state={first} report={(report) => { firstReport = report; }} /><Harness state={second} report={(report) => { secondReport = report; }} /></>);
    await waitFor(() => expect(firstReport).toBeDefined());
    firstReport.root.focus();
    expect(capture()).toBeUndefined();
    const secondInstanceId = instanceIdFromRequest(secondReport, command);
    const explicit = capture(() => '/tasklists', command, secondInstanceId);
    expect(explicit).toBeDefined();
    explicit?.open(command);
    expect(secondReport.open).toHaveBeenCalledOnce();
  });

  it('StrictMode/unmount não deixa source nem subscription fantasma', async () => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    const view = render(<React.StrictMode><Harness state={state} report={(report) => { latest = report; }} /></React.StrictMode>);
    await waitFor(() => expect(latest).toBeDefined());
    latest.root.focus();
    const lease = capture()!;
    expect(latest.listeners).toHaveLength(1);
    view.unmount();
    expect(lease.isCurrent()).toBe(false);
    expect(latest.listeners).toHaveLength(0);
  });

  it('limpa subscription própria ao dispose', async () => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    render(<Harness state={state} report={(report) => { latest = report; }} />);
    await waitFor(() => expect(latest).toBeDefined());
    latest.root.focus();
    const lease = capture()!;
    expect(latest.listeners).toHaveLength(1);
    lease.dispose();
    lease.dispose();
    expect(latest.listeners).toHaveLength(0);
  });

  it.each(['compositionstart', 'blur'])('invalida por %s', async (eventName) => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    render(<Harness state={state} report={(report) => { latest = report; }} />);
    await waitFor(() => expect(latest).toBeDefined());
    latest.root.focus();
    const lease = capture()!;
    if (eventName === 'compositionstart') document.dispatchEvent(new CompositionEvent(eventName));
    else window.dispatchEvent(new Event(eventName));
    expect(lease.isCurrent()).toBe(false);
  });

  it('recusa modal, rota obsoleta e IME ativo antes da captura', async () => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    render(<Harness state={state} report={(report) => { latest = report; }} />);
    await waitFor(() => expect(latest).toBeDefined());
    latest.root.focus();
    const modalRoot = document.createElement('div');
    modalRoot.className = 'modal-overlay';
    modalRoot.dataset.modalId = 'page-presentation-test-modal';
    document.body.append(modalRoot);
    registerOpenModal('page-presentation-test-modal');
    expect(capture()).toBeUndefined();
    unregisterOpenModal('page-presentation-test-modal');
    const input = document.createElement('input');
    latest.root.append(input);
    input.focus();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(capture()).toBeUndefined();
    input.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    latest.root.focus();
    expect(capture(() => '/other')).toBeUndefined();
    const lease = capture()!;
    state.current = false;
    expect(lease.isCurrent()).toBe(false);
  });

  it('bloqueia menu automático, mas permite ingresso explícito', async () => {
    const state: HarnessState = { target: {}, current: true, canOpen: true, notify: () => () => {}, report: () => {} };
    let latest!: HarnessReport;
    render(<Harness state={state} report={(report) => { latest = report; }} />);
    await waitFor(() => expect(latest).toBeDefined());
    const menu = document.createElement('div');
    menu.setAttribute('role', 'menu');
    const item = document.createElement('button');
    menu.append(item);
    latest.root.append(menu);
    item.focus();
    expect(capture()).toBeUndefined();
    const instanceId = instanceIdFromRequest(latest, command);
    const explicit = capture(() => '/tasklists', command, instanceId);
    expect(explicit).toBeDefined();
    explicit?.open(command);
    expect(latest.open).toHaveBeenCalledOnce();
  });
});
