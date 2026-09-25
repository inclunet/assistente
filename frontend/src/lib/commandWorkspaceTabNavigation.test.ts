import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { registerWorkspacePanelFocus } from '../components/workspace/workspacePanelFocusRegistry';
import {
  WORKSPACE_TAB_NAVIGATION_COMMAND_IDS,
  captureWorkspaceTabNavigationFocus,
  captureWorkspaceTabNavigationTarget,
  resolveWorkspaceTabNavigationTarget,
  isWorkspaceTabNavigationCommand,
} from './commandWorkspaceTabNavigation';

const { announce } = vi.hoisted(() => ({ announce: vi.fn() }));
vi.mock('../hooks/useAnnouncer', () => ({ announce }));

const cleanups: Array<() => void> = [];
function flushRaf(): Promise<void> {
  return new Promise((resolve) => window.requestAnimationFrame(() => resolve()));
}

function makeWorkspace(tabIds = ['a', 'b', 'c'], activeTabId = tabIds[0]): WorkspaceData {
  return {
    id: 'workspace-a',
    name: 'Workspace',
    tabs: tabIds.map((id, position) => ({ id, type: 'chat' as const, title: id, position })),
    activeTabId,
  };
}

function fixture() {
  const workspace = makeWorkspace();
  useAuthStore.setState({
    isAuthenticated: true,
    user: { userId: 'user-a', sessionId: 'session-a', role: 'user' },
  });
  useWorkspaceStore.setState({ workspace });

  const root = document.createElement('div');
  root.className = 'workspace-layout';
  const tabs = document.createElement('div');
  tabs.className = 'ws-tabs';
  for (const tab of workspace.tabs) {
    const button = document.createElement('button');
    button.type = 'button';
    button.setAttribute('role', 'tab');
    button.dataset.tabValue = tab.id;
    tabs.append(button);
  }
  root.append(tabs);
  document.body.append(root);
  const sourceFocus = document.createElement('input');
  root.append(sourceFocus);
  let pathname = '/';

  return {
    workspace,
    root,
    sourceFocus,
    setPathname: (value: string) => { pathname = value; },
    readPathname: () => pathname,
    button: (id: string) => root.querySelector<HTMLButtonElement>(`button[data-tab-value="${id}"]`),
  };
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  unregisterOpenModal('navigation-test');
  document.body.replaceChildren();
  vi.restoreAllMocks();
  announce.mockReset();
  useAuthStore.setState({ isAuthenticated: false, user: null });
  useWorkspaceStore.setState({ workspace: null });
});

describe('workspace tab navigation commands', () => {
  beforeEach(() => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  });

  it('expõe os IDs fechados na ordem consumida pelo contextual executor', () => {
    expect([...WORKSPACE_TAB_NAVIGATION_COMMAND_IDS]).toEqual([
      'workspace.tab.next', 'workspace.tab.previous', 'workspace.tab.first',
      'workspace.tab.second', 'workspace.tab.third', 'workspace.tab.fourth',
      'workspace.tab.fifth', 'workspace.tab.sixth', 'workspace.tab.seventh',
      'workspace.tab.eighth', 'workspace.tab.ninth',
    ]);
    expect(WORKSPACE_TAB_NAVIGATION_COMMAND_IDS.every(isWorkspaceTabNavigationCommand)).toBe(true);
    expect(isWorkspaceTabNavigationCommand('workspace.tab.close')).toBe(false);
  });

  it('resolve next/previous com wrap e todas as nove posições sem efeito colateral', () => {
    const workspace = makeWorkspace(['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i'], 'a');
    expect(resolveWorkspaceTabNavigationTarget(workspace, 'workspace.tab.next')?.id).toBe('b');
    expect(resolveWorkspaceTabNavigationTarget(workspace, 'workspace.tab.previous')?.id).toBe('i');
    expect(['first', 'second', 'third', 'fourth', 'fifth', 'sixth', 'seventh', 'eighth', 'ninth']
      .map((position) => resolveWorkspaceTabNavigationTarget(workspace, `workspace.tab.${position}`)?.id))
      .toEqual(['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i']);
    expect(workspace.activeTabId).toBe('a');
  });

  it('mantém noop em workspace unitário e retorna undefined para dados sem destino', () => {
    const single = makeWorkspace(['only'], 'only');
    expect(resolveWorkspaceTabNavigationTarget(single, 'workspace.tab.next')?.id).toBe('only');
    expect(resolveWorkspaceTabNavigationTarget(single, 'workspace.tab.previous')?.id).toBe('only');
    expect(resolveWorkspaceTabNavigationTarget(single, 'workspace.tab.first')?.id).toBe('only');
    expect(resolveWorkspaceTabNavigationTarget(makeWorkspace(['a', 'b'], 'missing'), 'workspace.tab.next')).toBeUndefined();
    expect(resolveWorkspaceTabNavigationTarget(single, 'workspace.tab.ninth')).toBeUndefined();
    expect(resolveWorkspaceTabNavigationTarget(null, 'workspace.tab.next')).toBeUndefined();
  });

  it('captura target e invalida rota, sessão, ordem, aba ativa e raiz', () => {
    const f = fixture();
    const lease = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    expect(lease?.isCurrent()).toBe(true);

    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'c', 'b'], 'a') });
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();

    const second = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    expect(second?.isCurrent()).toBe(false);
    second?.dispose();

    const third = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    useAuthStore.setState({ isAuthenticated: false, user: null });
    expect(third?.isCurrent()).toBe(false);
    third?.dispose();

    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } });
    useWorkspaceStore.setState({ workspace: makeWorkspace() });
    const fourth = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    f.root.replaceWith(f.root.cloneNode(true));
    expect(fourth?.isCurrent()).toBe(false);
    fourth?.dispose();
  });

  it('não revalida target após ABA fonte→destino→fonte nem logout→restore', () => {
    const f = fixture();
    const lease = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'a') });
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();

    const restored = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    useAuthStore.setState({ isAuthenticated: false, user: null });
    useAuthStore.setState({
      isAuthenticated: true,
      user: { userId: 'user-a', sessionId: 'session-a', role: 'user' },
    });
    expect(restored?.isCurrent()).toBe(false);
    restored?.dispose();
  });

  it.each(['removed', 'replaced', 'hidden'] as const)('invalida target quando a raiz é %s', (kind) => {
    const f = fixture();
    const lease = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    if (kind === 'removed') f.root.remove();
    if (kind === 'replaced') f.root.replaceWith(f.root.cloneNode(true));
    if (kind === 'hidden') f.root.hidden = true;
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();
  });

  it('invalida reorder mesmo quando a ordem volta ao snapshot original', () => {
    const f = fixture();
    const lease = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'c', 'b'], 'a') });
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'a') });
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();
  });

  it.each([false, true])('restaura foco de navegação iniciada no documento; outro controle recebe foco=%s', (focusChanged) => {
    const f = fixture();
    expect(document.activeElement).toBe(document.body);
    const destination = f.button('b')!;
    const immediate = vi.fn(() => { destination.focus(); return true; });
    cleanups.push(registerWorkspacePanelFocus('b', immediate, immediate, () => true));
    const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    if (focusChanged) f.sourceFocus.focus();
    lease?.apply();
    expect(document.activeElement).toBe(focusChanged ? f.sourceFocus : destination);
    expect(immediate).toHaveBeenCalledTimes(focusChanged ? 0 : 1);
    lease?.dispose();
  });

  it('foca destino pelo registry imediato após a transição esperada', () => {
    const f = fixture();
    f.sourceFocus.focus();
    const panel = document.createElement('section');
    panel.tabIndex = -1;
    f.root.append(panel);
    const immediate = vi.fn(() => { panel.focus(); return true; });
    cleanups.push(registerWorkspacePanelFocus('b', immediate, immediate, () => true));
    const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });

    lease?.apply();

    expect(immediate).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(panel);
    expect(announce).toHaveBeenCalledWith(expect.stringContaining('b'));
    lease?.dispose();
  });

  it('aguarda o handler do painel lazy e não usa botão aninhado antes do commit', async () => {
    const f = fixture();
    f.sourceFocus.focus();
    const nested = document.createElement('div');
    const nestedButton = document.createElement('button');
    nestedButton.setAttribute('role', 'tab');
    nestedButton.dataset.tabValue = 'b';
    nested.append(nestedButton);
    document.body.append(nested);
    const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    lease?.apply();
    lease?.apply();
    expect(document.activeElement).toBe(f.sourceFocus);
    expect(document.activeElement).not.toBe(nestedButton);
    cleanups.push(registerWorkspacePanelFocus('b', () => { f.button('b')!.focus(); return true; }));
    await flushRaf();
    expect(document.activeElement).toBe(f.button('b'));
    expect(announce).toHaveBeenCalledOnce();
    lease?.dispose();
  });

  it('não rouba foco em mudança do usuário, modal, rota, auth ou document sem foco', () => {
    const cases = ['focus', 'modal', 'route', 'auth', 'document'] as const;
    for (const reason of cases) {
      vi.mocked(document.hasFocus).mockReturnValue(true);
      const f = fixture();
      f.sourceFocus.focus();
      const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
      const outside = document.createElement('button');
      document.body.append(outside);
      if (reason === 'focus') outside.focus();
      if (reason === 'modal') {
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        document.body.append(overlay);
        registerOpenModal('navigation-test');
      }
      if (reason === 'route') f.setPathname('/settings');
      if (reason === 'auth') useAuthStore.setState({ isAuthenticated: false, user: null });
      if (reason === 'document') vi.mocked(document.hasFocus).mockReturnValue(false);
      useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
      lease?.apply();
      expect(document.activeElement).not.toBe(f.button('b'));
      lease?.dispose();
      document.body.replaceChildren();
      unregisterOpenModal('navigation-test');
      useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } });
    }
    vi.mocked(document.hasFocus).mockReturnValue(true);
  });

  it('rejeita ABA e não aceita destino arbitrário após troca rápida', () => {
    const f = fixture();
    f.sourceFocus.focus();
    const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'a') });
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    lease?.apply();
    expect(document.activeElement).toBe(f.sourceFocus);
    lease?.dispose();
  });

  it('não rouba foco que foi para outro controle, foi removido ou voltou ao body', () => {
    const f = fixture();
    f.sourceFocus.focus();
    const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
    const outside = document.createElement('button');
    document.body.append(outside);
    outside.focus();
    outside.remove();
    document.body.focus();
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    lease?.apply();
    expect(document.activeElement).toBe(document.body);
    expect(document.activeElement).not.toBe(f.button('b'));
    expect(announce).not.toHaveBeenCalled();
    lease?.dispose();
  });

  it('bloqueia modal, rota e blur ocorridos depois do commit da transição', () => {
    const reasons = ['modal', 'route', 'blur'] as const;
    for (const reason of reasons) {
      const f = fixture();
      f.sourceFocus.focus();
      const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
      useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
      if (reason === 'modal') {
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        document.body.append(overlay);
        registerOpenModal('navigation-test');
      }
      if (reason === 'route') f.setPathname('/settings');
      if (reason === 'blur') window.dispatchEvent(new Event('blur'));
      lease?.apply();
      expect(document.activeElement).not.toBe(f.button('b'));
      expect(announce).not.toHaveBeenCalled();
      lease?.dispose();
      document.body.replaceChildren();
      unregisterOpenModal('navigation-test');
    }
  });

  it('não aplica se a raiz capturada foi desconectada, mesmo com destino ativo', () => {
    const f = fixture();
    f.sourceFocus.focus();
    const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
    f.root.remove();
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    lease?.apply();
    expect(document.activeElement).toBe(document.body);
    expect(document.activeElement).not.toBe(f.button('b'));
    lease?.dispose();
  });

  it('aguarda o handler quando a fonte desmonta e o foco volta ao body', async () => {
    const f = fixture();
    const sourcePanel = document.createElement('section');
    sourcePanel.className = 'ws-content__panel';
    sourcePanel.dataset.tabId = 'a';
    sourcePanel.append(f.sourceFocus);
    f.root.append(sourcePanel);
    f.sourceFocus.focus();
    const lease = captureWorkspaceTabNavigationFocus(f.readPathname, 'workspace.tab.next');
    sourcePanel.remove();
    expect(document.activeElement).toBe(document.body);
    useWorkspaceStore.setState({ workspace: makeWorkspace(['a', 'b', 'c'], 'b') });
    lease?.apply();
    cleanups.push(registerWorkspacePanelFocus('b', () => { f.button('b')!.focus(); return true; }));
    await flushRaf();
    expect(document.activeElement).toBe(f.button('b'));
    expect(announce).toHaveBeenCalledOnce();
    lease?.dispose();
  });

  it('expõe snapshot de navegação congelado no target lease', () => {
    const f = fixture();
    const lease = captureWorkspaceTabNavigationTarget(f.readPathname, 'workspace.tab.next');
    expect(lease?.navigation).toEqual({ workspaceID: 'workspace-a', sourceTabID: 'a', targetTabID: 'b' });
    expect(Object.isFrozen(lease?.navigation)).toBe(true);
    lease?.dispose();
  });
});
