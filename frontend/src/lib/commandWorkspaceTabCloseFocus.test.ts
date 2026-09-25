import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { registerWorkspacePanelFocus } from '../components/workspace/workspacePanelFocusRegistry';
import { captureWorkspaceTabCloseFocus } from './commandWorkspaceTabCloseFocus';

const { announce } = vi.hoisted(() => ({ announce: vi.fn() }));
vi.mock('../hooks/useAnnouncer', () => ({ announce }));

type Fixture = ReturnType<typeof fixture>;
const cleanups: Array<() => void> = [];

function fixture() {
  const user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
  const source = { id: 'source', type: 'chat' as const, title: 'Source', position: 0 };
  const successor = { id: 'successor', type: 'editor' as const, title: 'Successor', position: 1 };
  const workspace = { id: 'workspace-a', name: 'Workspace', tabs: [source, successor], activeTabId: source.id };
  useAuthStore.setState({ isAuthenticated: true, user });
  useWorkspaceStore.setState({ workspace });

  const sourceRoot = document.createElement('section');
  sourceRoot.className = 'ws-content__panel';
  sourceRoot.dataset.tabId = source.id;
  sourceRoot.dataset.tabType = source.type;
  const input = document.createElement('input');
  sourceRoot.append(input);
  const successorRoot = document.createElement('section');
  successorRoot.className = 'ws-content__panel';
  successorRoot.dataset.tabId = successor.id;
  successorRoot.dataset.tabType = successor.type;
  successorRoot.tabIndex = -1;
  const successorButton = document.createElement('button');
  successorButton.setAttribute('role', 'tab');
  successorButton.dataset.tabValue = successor.id;
  const workspaceRoot = document.createElement('div');
  workspaceRoot.className = 'workspace-layout';
  workspaceRoot.append(sourceRoot, successorRoot, successorButton);
  document.body.append(workspaceRoot);

  let pathname = '/';
  const immediate = vi.fn(() => { successorRoot.focus(); return true; });
  function capture() {
    const lease = captureWorkspaceTabCloseFocus(() => pathname);
    if (lease) cleanups.push(lease.dispose);
    return lease;
  }
  function closeSource() {
    useWorkspaceStore.setState({ workspace: { ...workspace, tabs: [successor], activeTabId: successor.id } });
    sourceRoot.remove();
    document.body.focus();
  }

  return {
    user, workspace, sourceRoot, successorRoot, successorButton, input, immediate, workspaceRoot,
    capture, closeSource, setPath: (value: string) => { pathname = value; },
  };
}

function registerImmediate(f: Fixture) {
  const unregister = registerWorkspacePanelFocus('successor', f.immediate, f.immediate, () => true);
  cleanups.push(unregister);
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  unregisterOpenModal('close-focus-test');
  document.body.replaceChildren();
  announce.mockReset();
  vi.restoreAllMocks();
  useAuthStore.setState({ isAuthenticated: false, user: null });
  useWorkspaceStore.setState({ workspace: null });
});

describe('captureWorkspaceTabCloseFocus', () => {
  beforeEach(() => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  });

  it('foca a sucessora pelo handler imediato e anuncia o fechamento', () => {
    const f = fixture();
    f.input.focus();
    registerImmediate(f);
    const lease = f.capture();
    expect(lease).toBeDefined();

    f.closeSource();
    lease?.apply();

    expect(f.immediate).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(f.successorRoot);
    expect(announce).toHaveBeenCalledWith(expect.any(String));
  });

  it('usa o botão da aba sucessora quando o painel ainda está lazy', () => {
    const f = fixture();
    f.input.focus();
    const lease = f.capture();

    f.closeSource();
    lease?.apply();

    expect(document.activeElement).toBe(f.successorButton);
    expect(announce).toHaveBeenCalledOnce();
  });

  it('não falha nem anuncia quando a sucessora ainda não tem DOM visual', () => {
    const f = fixture();
    f.input.focus();
    f.successorButton.remove();
    const lease = f.capture();

    f.closeSource();
    lease?.apply();

    expect(announce).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(document.body);
  });

  it.each(['focus', 'modal', 'route', 'workspace', 'auth', 'source-present'] as const)(
    'recusa aplicação quando o contexto fica stale por %s', (reason) => {
      const f = fixture();
      f.input.focus();
      const lease = f.capture();
      const outside = document.createElement('button');
      document.body.append(outside);

      if (reason === 'focus') outside.focus();
      if (reason === 'modal') {
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        document.body.append(overlay);
        registerOpenModal('close-focus-test');
      }
      if (reason === 'route') f.setPath('/settings');
      if (reason === 'workspace') useWorkspaceStore.setState({ workspace: { ...f.workspace, id: 'workspace-other' } });
      if (reason === 'auth') useAuthStore.setState({ isAuthenticated: false, user: null });
      if (reason === 'source-present') useWorkspaceStore.setState({ workspace: { ...f.workspace, activeTabId: 'successor', tabs: [f.workspace.tabs[0], f.workspace.tabs[1]] } });
      if (reason !== 'source-present' && reason !== 'workspace' && reason !== 'auth' && reason !== 'route' && reason !== 'modal' && reason !== 'focus') {
        throw new Error(`unknown reason ${reason}`);
      }

      if (reason !== 'source-present') f.closeSource();
      lease?.apply();

      expect(announce).not.toHaveBeenCalled();
      if (reason === 'focus') expect(document.activeElement).toBe(outside);
      if (reason === 'source-present') expect(document.activeElement).toBe(f.input);
    },
  );

  it('invalida após blur mesmo que o foco retorne', () => {
    const f = fixture();
    f.input.focus();
    const lease = f.capture();
    window.dispatchEvent(new Event('blur'));
    f.closeSource();
    lease?.apply();

    expect(announce).not.toHaveBeenCalled();
  });

  it('não revalida foco depois de source removida e foco do usuário alterado', () => {
    const f = fixture();
    f.input.focus();
    const lease = f.capture();
    f.closeSource();
    const other = document.createElement('button');
    f.workspaceRoot.append(other);
    other.focus();
    other.remove();
    expect(document.activeElement).toBe(document.body);
    lease?.apply();

    expect(announce).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(f.successorButton);
  });

  it('não aplica duas vezes e dispose encerra as assinaturas', () => {
    const f = fixture();
    f.input.focus();
    const lease = f.capture();
    f.closeSource();
    lease?.apply();
    lease?.apply();
    lease?.dispose();
    lease?.dispose();

    expect(announce).toHaveBeenCalledOnce();
  });

  it('mantém o contrato da última aba substituída por uma sucessora', () => {
    const f = fixture();
    useWorkspaceStore.setState({ workspace: { ...f.workspace, tabs: [f.workspace.tabs[0]], activeTabId: 'source' } });
    f.input.focus();
    const lease = f.capture();
    useWorkspaceStore.setState({
      workspace: {
        ...f.workspace,
        tabs: [{ ...f.workspace.tabs[0], id: 'replacement', type: 'chat' }],
        activeTabId: 'replacement',
      },
    });
    f.sourceRoot.remove();
    document.body.focus();
    f.successorButton.dataset.tabValue = 'replacement';
    lease?.apply();

    expect(document.activeElement).toBe(f.successorButton);
    expect(announce).toHaveBeenCalledOnce();
  });

  it('não aceita ABA da substituta da última aba', () => {
    const f = fixture();
    useWorkspaceStore.setState({ workspace: { ...f.workspace, tabs: [f.workspace.tabs[0]], activeTabId: 'source' } });
    f.input.focus();
    const lease = f.capture();

    useWorkspaceStore.setState({
      workspace: { ...f.workspace, tabs: [{ ...f.workspace.tabs[0], id: 'replacement-a', type: 'chat' }], activeTabId: 'replacement-a' },
    });
    f.sourceRoot.remove();
    document.body.focus();
    useWorkspaceStore.setState({
      workspace: { ...f.workspace, tabs: [{ ...f.workspace.tabs[0], id: 'replacement-b', type: 'chat' }], activeTabId: 'replacement-b' },
    });

    useWorkspaceStore.setState({
      workspace: { ...f.workspace, tabs: [{ ...f.workspace.tabs[0], id: 'replacement-a', type: 'chat' }], activeTabId: 'replacement-a' },
    });

    lease?.apply();

    expect(announce).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(document.body);
  });

  it.each(['detached', 'hidden', 'replaced'])('não transfere foco para workspace %s após fechamento', (kind) => {
    const f = fixture();
    f.input.focus();
    const lease = f.capture();
    f.closeSource();
    if (kind === 'detached') f.workspaceRoot.remove();
    else if (kind === 'hidden') f.workspaceRoot.hidden = true;
    else f.workspaceRoot.replaceWith(f.workspaceRoot.cloneNode(true));
    lease?.apply();
    expect(announce).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(f.successorButton);
  });
});
